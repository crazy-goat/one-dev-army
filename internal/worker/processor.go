package worker

import (
	"context"
	"fmt"
	"regexp"
	"strings"
	"time"

	"github.com/one-dev-army/oda/internal/config"
	"github.com/one-dev-army/oda/internal/db"
	"github.com/one-dev-army/oda/internal/git"
	"github.com/one-dev-army/oda/internal/github"
	"github.com/one-dev-army/oda/internal/opencode"
	"github.com/one-dev-army/oda/internal/pipeline"
)

const maxSlugLen = 50

const promptAnalysis = `You are analyzing a GitHub issue for implementation.

## Issue #%d: %s

%s

## Instructions

Analyze this issue and produce a structured analysis. Consider:
1. What are the core requirements?
2. What files/packages might need changes?
3. What are the edge cases and potential risks?
4. What dependencies exist?

Respond with a JSON object:
{
  "summary": "brief summary of what needs to be done",
  "requirements": ["list of concrete requirements"],
  "affected_files": ["list of likely affected files/packages"],
  "risks": ["potential risks or edge cases"],
  "complexity": "low|medium|high"
}
`

const promptPlanning = `You are creating an implementation plan for a GitHub issue.

## Issue #%d: %s

%s

## Analysis from previous stage

%s

## Instructions

Create a detailed implementation plan. Include:
1. Step-by-step implementation order
2. Specific code changes needed
3. Test cases to write
4. Any refactoring needed

Respond with a JSON object:
{
  "steps": [
    {"order": 1, "description": "what to do", "files": ["affected files"], "details": "specific changes"}
  ],
  "test_plan": ["list of test cases to write"],
  "estimated_complexity": "low|medium|high"
}
`

const promptPlanReview = `You are reviewing an implementation plan for correctness and completeness.

## Issue #%d: %s

%s

## Implementation Plan

%s

## Instructions

Review this plan critically. Check for:
1. Missing steps or edge cases
2. Incorrect assumptions about the codebase
3. Missing test coverage
4. Potential breaking changes
5. Security concerns

Respond with a JSON object:
{
  "approved": true/false,
  "issues": ["list of issues found, if any"],
  "suggestions": ["list of improvements, if any"],
  "verdict": "brief summary of review"
}
`

const promptCoding = `You are implementing code changes for a GitHub issue.

## Issue #%d: %s

%s

## Implementation Plan

%s

## Tools

- Lint command: %s
- Test command: %s

## Instructions

Implement all changes according to the plan. Make sure to:
1. Follow existing code style and patterns
2. Write comprehensive tests
3. Run the lint and test commands to verify your changes
4. Handle errors properly
5. Keep changes minimal and focused

Implement the complete solution now.
`

const promptCodeReview = `You are reviewing code changes for a GitHub issue.

## Issue #%d: %s

%s

## Diff

%s

## Instructions

Review these code changes. Check for:
1. Correctness - does the code do what the issue requires?
2. Code quality - clean, readable, well-structured?
3. Error handling - are errors handled properly?
4. Tests - adequate test coverage?
5. Security - any vulnerabilities introduced?
6. Performance - any obvious performance issues?

Respond with a JSON object:
{
  "approved": true/false,
  "issues": ["list of issues found, if any"],
  "suggestions": ["list of improvements, if any"],
  "verdict": "brief summary of review"
}
`

var nonAlphanumeric = regexp.MustCompile(`[^a-z0-9-]+`)
var multiDash = regexp.MustCompile(`-{2,}`)

func Slugify(title string) string {
	s := strings.ToLower(strings.TrimSpace(title))
	s = strings.ReplaceAll(s, " ", "-")
	s = nonAlphanumeric.ReplaceAllString(s, "")
	s = multiDash.ReplaceAllString(s, "-")
	s = strings.Trim(s, "-")
	if len(s) > maxSlugLen {
		s = s[:maxSlugLen]
		s = strings.TrimRight(s, "-")
	}
	return s
}

func BranchName(issueNumber int, title string) string {
	return fmt.Sprintf("task/%d-%s", issueNumber, Slugify(title))
}

type Processor struct {
	cfg   *config.Config
	oc    *opencode.Client
	gh    *github.Client
	store *db.Store
	wtMgr *git.WorktreeManager
}

func NewProcessor(cfg *config.Config, oc *opencode.Client, gh *github.Client, store *db.Store, wtMgr *git.WorktreeManager) *Processor {
	return &Processor{
		cfg:   cfg,
		oc:    oc,
		gh:    gh,
		store: store,
		wtMgr: wtMgr,
	}
}

func (p *Processor) Process(ctx context.Context, w *Worker, task *Task) error {
	branch := BranchName(task.IssueNumber, task.Title)

	wt, err := p.wtMgr.Create(w.id, branch)
	if err != nil {
		return fmt.Errorf("creating worktree for task #%d: %w", task.IssueNumber, err)
	}

	executor := NewStageExecutor(p.cfg, p.oc, p.store, task, wt)

	startStage := pipeline.Stage(task.Stage)
	if startStage == "" || startStage == pipeline.StageQueued {
		startStage = pipeline.StageAnalysis
	}

	ppl := pipeline.New(p.cfg.Pipeline.MaxRetries, executor, func(_ int, s pipeline.Stage) {
		w.SetStage(string(s))
	})

	issueCtx := fmt.Sprintf("Issue #%d: %s\n\n%s", task.IssueNumber, task.Title, task.Body)
	result, err := ppl.Run(task.ID, startStage, issueCtx)
	if err != nil {
		return fmt.Errorf("pipeline failed for task #%d: %w", task.IssueNumber, err)
	}

	switch result.Stage {
	case pipeline.StageDone:
		prTitle := fmt.Sprintf("fix #%d: %s", task.IssueNumber, task.Title)
		prBody := fmt.Sprintf("Closes #%d\n\nAutomated implementation by ODA.", task.IssueNumber)

		_, prErr := p.gh.CreatePR(branch, prTitle, prBody)
		if prErr != nil {
			return fmt.Errorf("creating PR for task #%d: %w", task.IssueNumber, prErr)
		}

		mergeStage := p.findStageConfig("merge")
		if mergeStage != nil && mergeStage.ManualApproval {
			_ = p.gh.AddLabel(task.IssueNumber, "stage:needs-user")
		} else {
			if mergeErr := p.gh.MergePR(branch); mergeErr != nil {
				return fmt.Errorf("merging PR for task #%d: %w", task.IssueNumber, mergeErr)
			}
		}

		_ = p.wtMgr.Remove(w.id)

	case pipeline.StageBlocked:
		_ = p.gh.AddLabel(task.IssueNumber, "stage:needs-user")
		_ = p.gh.AddComment(task.IssueNumber, fmt.Sprintf("ODA pipeline blocked at stage. Last output:\n\n%s", result.Output))
	}

	return nil
}

func (p *Processor) findStageConfig(name string) *config.Stage {
	for i := range p.cfg.Pipeline.Stages {
		if p.cfg.Pipeline.Stages[i].Name == name {
			return &p.cfg.Pipeline.Stages[i]
		}
	}
	return nil
}

type StageExecutor struct {
	cfg      *config.Config
	oc       *opencode.Client
	store    *db.Store
	task     *Task
	worktree *git.Worktree
}

func NewStageExecutor(cfg *config.Config, oc *opencode.Client, store *db.Store, task *Task, wt *git.Worktree) *StageExecutor {
	return &StageExecutor{
		cfg:      cfg,
		oc:       oc,
		store:    store,
		task:     task,
		worktree: wt,
	}
}

func (e *StageExecutor) Execute(taskID int, stage pipeline.Stage, context string) (*pipeline.StageResult, error) {
	switch stage {
	case pipeline.StageAnalysis:
		return e.executeSession(taskID, stage, context, promptAnalysis)
	case pipeline.StagePlanning:
		return e.executeSession(taskID, stage, context, promptPlanning)
	case pipeline.StagePlanReview:
		return e.executeReview(taskID, stage, context, promptPlanReview)
	case pipeline.StageCoding:
		return e.executeCoding(taskID, stage, context)
	case pipeline.StageTesting:
		return e.executeTesting(taskID, stage)
	case pipeline.StageCodeReview:
		return e.executeReview(taskID, stage, context, promptCodeReview)
	case pipeline.StageMerging:
		return &pipeline.StageResult{Stage: stage, Success: true, Output: "ready to merge"}, nil
	default:
		return &pipeline.StageResult{Stage: stage, Success: true, Output: ""}, nil
	}
}

func (e *StageExecutor) executeSession(taskID int, stage pipeline.Stage, stageContext, promptTpl string) (*pipeline.StageResult, error) {
	llm := e.llmForStage(stage)
	title := fmt.Sprintf("ODA: %s for #%d", stage, e.task.IssueNumber)

	start := time.Now()

	session, err := e.oc.CreateSession(title)
	if err != nil {
		return nil, fmt.Errorf("creating session for %s: %w", stage, err)
	}

	prompt := fmt.Sprintf(promptTpl, e.task.IssueNumber, e.task.Title, e.task.Body, stageContext)
	msg, err := e.oc.SendMessage(session.ID, prompt, llm)
	if err != nil {
		return nil, fmt.Errorf("sending message for %s: %w", stage, err)
	}

	output := extractTextContent(msg)
	duration := time.Since(start)

	e.recordMetric(taskID, stage, llm, duration)

	return &pipeline.StageResult{
		Stage:   stage,
		Success: true,
		Output:  output,
	}, nil
}

func (e *StageExecutor) executeReview(taskID int, stage pipeline.Stage, stageContext, promptTpl string) (*pipeline.StageResult, error) {
	llm := e.llmForStage(stage)
	title := fmt.Sprintf("ODA: %s for #%d", stage, e.task.IssueNumber)

	start := time.Now()

	session, err := e.oc.CreateSession(title)
	if err != nil {
		return nil, fmt.Errorf("creating session for %s: %w", stage, err)
	}

	prompt := fmt.Sprintf(promptTpl, e.task.IssueNumber, e.task.Title, e.task.Body, stageContext)
	msg, err := e.oc.SendMessage(session.ID, prompt, llm)
	if err != nil {
		return nil, fmt.Errorf("sending message for %s: %w", stage, err)
	}

	output := extractTextContent(msg)
	duration := time.Since(start)

	e.recordMetric(taskID, stage, llm, duration)

	approved := strings.Contains(output, `"approved": true`) || strings.Contains(output, `"approved":true`)

	return &pipeline.StageResult{
		Stage:   stage,
		Success: approved,
		Output:  output,
	}, nil
}

func (e *StageExecutor) executeCoding(taskID int, stage pipeline.Stage, stageContext string) (*pipeline.StageResult, error) {
	llm := e.llmForStage(stage)
	title := fmt.Sprintf("ODA: %s for #%d", stage, e.task.IssueNumber)

	start := time.Now()

	session, err := e.oc.CreateSession(title)
	if err != nil {
		return nil, fmt.Errorf("creating session for %s: %w", stage, err)
	}

	prompt := fmt.Sprintf(promptCoding,
		e.task.IssueNumber, e.task.Title, e.task.Body,
		stageContext,
		e.cfg.Tools.LintCmd, e.cfg.Tools.TestCmd,
	)
	msg, err := e.oc.SendMessage(session.ID, prompt, llm)
	if err != nil {
		return nil, fmt.Errorf("sending message for %s: %w", stage, err)
	}

	output := extractTextContent(msg)
	duration := time.Since(start)

	e.recordMetric(taskID, stage, llm, duration)

	return &pipeline.StageResult{
		Stage:   stage,
		Success: true,
		Output:  output,
	}, nil
}

func (e *StageExecutor) executeTesting(taskID int, stage pipeline.Stage) (*pipeline.StageResult, error) {
	start := time.Now()
	var failures []string

	if e.cfg.Tools.LintCmd != "" {
		parts := strings.Fields(e.cfg.Tools.LintCmd)
		if _, err := git.RunInWorktree(e.worktree.Path, parts[0], parts[1:]...); err != nil {
			failures = append(failures, fmt.Sprintf("lint: %v", err))
		}
	}

	if e.cfg.Tools.TestCmd != "" {
		parts := strings.Fields(e.cfg.Tools.TestCmd)
		if _, err := git.RunInWorktree(e.worktree.Path, parts[0], parts[1:]...); err != nil {
			failures = append(failures, fmt.Sprintf("test: %v", err))
		}
	}

	if e.cfg.Tools.E2ECmd != "" {
		parts := strings.Fields(e.cfg.Tools.E2ECmd)
		if _, err := git.RunInWorktree(e.worktree.Path, parts[0], parts[1:]...); err != nil {
			failures = append(failures, fmt.Sprintf("e2e: %v", err))
		}
	}

	duration := time.Since(start)
	llm := e.llmForStage(stage)
	e.recordMetric(taskID, stage, llm, duration)

	if len(failures) > 0 {
		return &pipeline.StageResult{
			Stage:   stage,
			Success: false,
			Output:  strings.Join(failures, "\n"),
		}, nil
	}

	return &pipeline.StageResult{
		Stage:   stage,
		Success: true,
		Output:  "all checks passed",
	}, nil
}

func (e *StageExecutor) llmForStage(stage pipeline.Stage) string {
	for _, s := range e.cfg.Pipeline.Stages {
		if s.Name == string(stage) {
			return s.LLM
		}
	}
	return ""
}

func (e *StageExecutor) recordMetric(taskID int, stage pipeline.Stage, llm string, duration time.Duration) {
	if e.store == nil {
		return
	}
	_ = e.store.SaveStageMetric(db.StageMetric{
		TaskID:    taskID,
		Stage:     string(stage),
		LLM:       llm,
		DurationS: int(duration.Seconds()),
	})
}

func extractTextContent(msg *opencode.Message) string {
	if msg == nil {
		return ""
	}
	var parts []string
	for _, p := range msg.Parts {
		if p.Type == "text" && p.Content != "" {
			parts = append(parts, p.Content)
		}
	}
	return strings.Join(parts, "\n")
}
