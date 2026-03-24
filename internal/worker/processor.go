package worker

import (
	"context"
	"encoding/json"
	"fmt"
	"log"
	"regexp"
	"strings"
	"time"

	"os"

	"github.com/crazy-goat/one-dev-army/internal/config"
	"github.com/crazy-goat/one-dev-army/internal/db"
	"github.com/crazy-goat/one-dev-army/internal/git"
	"github.com/crazy-goat/one-dev-army/internal/github"
	"github.com/crazy-goat/one-dev-army/internal/llm"
	"github.com/crazy-goat/one-dev-army/internal/opencode"
	"github.com/crazy-goat/one-dev-army/internal/pipeline"
)

const maxSlugLen = 50

const automatedPipelineNotice = "CRITICAL: You are running in a fully automated pipeline with NO human operator. " +
	"NEVER ask questions, request clarification, or wait for input - nobody will answer and the pipeline will hang forever. " +
	"Make your best judgment and produce output immediately.\n\n"

const promptAnalysis = `You are analyzing a GitHub issue for implementation.

## Issue #%d: %s

%s

## Instructions

Analyze this issue and produce a structured analysis. Consider:
1. What are the core requirements?
2. What files/packages might need changes?
3. What are the edge cases and potential risks?
4. What dependencies exist?

Do NOT ask any questions - just produce the output.

Respond with a JSON object:
{
  "summary": "brief summary of what needs to be done",
  "requirements": ["list of concrete requirements"],
  "affected_files": ["list of likely affected files/packages"],
  "risks": ["potential risks or edge cases"],
  "complexity": "low|medium|high"
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

Do NOT ask any questions - just implement the solution.

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

Do NOT ask any questions - just produce the output.

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

// StageChanger is the single entry point for all stage transitions.
// Orchestrator implements this interface. Worker and other components
// must use it instead of calling GitHub API or ledger directly.
type StageChanger interface {
	ChangeStage(issueNumber int, stage github.Stage, reason github.StageChangeReason) error
}

type Processor struct {
	cfg          *config.Config
	oc           *opencode.Client
	gh           *github.Client
	store        *db.Store
	brMgr        *git.BranchManager
	router       *llm.Router
	stageChanger StageChanger
}

func NewProcessor(cfg *config.Config, oc *opencode.Client, gh *github.Client, store *db.Store, brMgr *git.BranchManager, router *llm.Router, stageChanger StageChanger) *Processor {
	return &Processor{
		cfg:          cfg,
		oc:           oc,
		gh:           gh,
		store:        store,
		brMgr:        brMgr,
		router:       router,
		stageChanger: stageChanger,
	}
}

func (p *Processor) Process(ctx context.Context, w *Worker, task *Task) error {
	branch := BranchName(task.IssueNumber, task.Title)

	if err := p.brMgr.CreateBranch(branch); err != nil {
		return fmt.Errorf("creating branch for task #%d: %w", task.IssueNumber, err)
	}

	wt := &git.Worktree{
		Name:   w.id,
		Path:   p.brMgr.RepoDir(),
		Branch: branch,
	}
	executor := NewStageExecutor(p.cfg, p.oc, p.store, task, wt, p.router)

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

		// Always require manual approval for merge - use NeedsUser stage
		if p.stageChanger != nil {
			if err := p.stageChanger.ChangeStage(task.IssueNumber, github.StageNeedsUser, github.ReasonWorkerNeedsUser); err != nil {
				log.Printf("[Worker] Error setting NeedsUser stage for #%d: %v", task.IssueNumber, err)
			}
		}

		_ = p.brMgr.RemoveBranch(branch)

	case pipeline.StageBlocked:
		// Set NeedsUser stage when blocked
		if p.stageChanger != nil {
			if err := p.stageChanger.ChangeStage(task.IssueNumber, github.StageNeedsUser, github.ReasonWorkerBlocked); err != nil {
				log.Printf("[Worker] Error setting NeedsUser stage for #%d: %v", task.IssueNumber, err)
			}
		}
		_ = p.gh.AddComment(task.IssueNumber, fmt.Sprintf("ODA pipeline blocked at stage. Last output:\n\n%s", result.Output))
	}

	return nil
}

type StageExecutor struct {
	cfg      *config.Config
	oc       *opencode.Client
	store    *db.Store
	task     *Task
	worktree *git.Worktree
	router   *llm.Router
}

func NewStageExecutor(cfg *config.Config, oc *opencode.Client, store *db.Store, task *Task, wt *git.Worktree, router *llm.Router) *StageExecutor {
	return &StageExecutor{
		cfg:      cfg,
		oc:       oc,
		store:    store,
		task:     task,
		worktree: wt,
		router:   router,
	}
}

func (e *StageExecutor) Execute(taskID int, stage pipeline.Stage, context string) (*pipeline.StageResult, error) {
	switch stage {
	case pipeline.StageAnalysis:
		return e.executeSession(taskID, stage, context, promptAnalysis)
	case pipeline.StageCoding:
		return e.executeCoding(taskID, stage, context)
	case pipeline.StageCodeReview:
		return e.executeReview(taskID, stage, context, promptCodeReview)
	case pipeline.StageCreatePR:
		return &pipeline.StageResult{Stage: stage, Success: true, Output: "ready to create PR"}, nil
	case pipeline.StageApprove:
		return &pipeline.StageResult{Stage: stage, Success: true, Output: "awaiting approval"}, nil
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

	prompt := automatedPipelineNotice + fmt.Sprintf(promptTpl, e.task.IssueNumber, e.task.Title, e.task.Body, stageContext)
	msg, err := e.oc.SendMessage(session.ID, prompt, opencode.ParseModelRef(llm), os.Stdout)
	if err != nil {
		return nil, fmt.Errorf("sending message for %s: %w", stage, err)
	}

	output := ExtractFullContent(msg)
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

	prompt := automatedPipelineNotice + fmt.Sprintf(promptTpl, e.task.IssueNumber, e.task.Title, e.task.Body, stageContext)
	msg, err := e.oc.SendMessage(session.ID, prompt, opencode.ParseModelRef(llm), os.Stdout)
	if err != nil {
		return nil, fmt.Errorf("sending message for %s: %w", stage, err)
	}

	output := ExtractFullContent(msg)
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

	prompt := automatedPipelineNotice + fmt.Sprintf(promptCoding,
		e.task.IssueNumber, e.task.Title, e.task.Body,
		stageContext,
		e.cfg.Tools.LintCmd, e.cfg.Tools.TestCmd,
	)
	msg, err := e.oc.SendMessage(session.ID, prompt, opencode.ParseModelRef(llm), os.Stdout)
	if err != nil {
		return nil, fmt.Errorf("sending message for %s: %w", stage, err)
	}

	output := ExtractFullContent(msg)
	duration := time.Since(start)

	e.recordMetric(taskID, stage, llm, duration)

	return &pipeline.StageResult{
		Stage:   stage,
		Success: true,
		Output:  output,
	}, nil
}

func (e *StageExecutor) llmForStage(stage pipeline.Stage) string {
	// Use the router to select the appropriate model
	if e.router != nil {
		model := e.router.SelectModelForStage(string(stage), "")
		if model != "" {
			return model
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
		if p.Type == "text" && p.Text != "" {
			parts = append(parts, p.Text)
		}
	}
	return strings.Join(parts, "\n")
}

func ExtractFullContent(msg *opencode.Message) string {
	if msg == nil {
		return ""
	}
	var parts []string
	for _, p := range msg.Parts {
		switch p.Type {
		case "text":
			if p.Text != "" {
				parts = append(parts, p.Text)
			}
		case "tool_call":
			if p.ToolCall != nil {
				parts = append(parts, formatToolCall(p.ToolCall))
			}
		case "tool_result":
			if p.ToolResult != nil {
				parts = append(parts, formatToolResult(p.ToolResult))
			}
		}
	}
	return strings.Join(parts, "\n\n")
}

func formatToolCall(tc *opencode.ToolCall) string {
	args, _ := json.MarshalIndent(tc.Arguments, "", "  ")
	return fmt.Sprintf("[Tool Call: %s]\nArguments: %s", tc.Name, string(args))
}

func formatToolResult(tr *opencode.ToolResult) string {
	if tr.Error != "" {
		return fmt.Sprintf("[Tool Result: %s]\nError: %s", tr.ID, tr.Error)
	}
	return fmt.Sprintf("[Tool Result: %s]\n%s", tr.ID, tr.Output)
}
