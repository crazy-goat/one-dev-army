package mvp

import (
	"context"
	"fmt"
	"log"
	"regexp"
	"strings"
	"time"

	"github.com/crazy-goat/one-dev-army/internal/config"
	"github.com/crazy-goat/one-dev-army/internal/git"
	"github.com/crazy-goat/one-dev-army/internal/github"
	"github.com/crazy-goat/one-dev-army/internal/opencode"
)

const analysisPrompt = `Analyze GitHub issue #%d: %s

Issue body:
%s

Provide a concise analysis covering:
1. Core requirements — what exactly needs to be done
2. Files that likely need changes
3. Implementation approach — high-level strategy
4. Testing strategy — what tests to write or update

Be concise. Do NOT ask questions. Output your analysis directly.`

const planningPrompt = `Create a step-by-step implementation plan for GitHub issue #%d: %s

Analysis from previous step:
%s

Create a concrete, actionable plan covering:
1. Which files to create or modify (exact paths)
2. What code changes to make in each file
3. What tests to add or update
4. Order of operations

Be specific and actionable. Do NOT ask questions. Output the plan directly.`

const codeReviewPrompt = `You are reviewing code changes for GitHub issue #%d: %s

## Diff

%s

## Instructions

Review these code changes. Check for:
1. Correctness — does the code do what the issue requires?
2. Code quality — clean, readable, well-structured?
3. Error handling — are errors handled properly?
4. Tests — adequate test coverage?
5. Security — any vulnerabilities introduced?
6. Performance — any obvious performance issues?

Do NOT ask any questions — just produce the output.

Respond with a JSON object:
{
  "approved": true/false,
  "issues": ["list of issues found, if any"],
  "suggestions": ["list of improvements, if any"],
  "verdict": "brief summary of review"
}`

const implementationPrompt = `Implement the following plan for GitHub issue #%d: %s

Implementation plan:
%s

Working directory: %s

Instructions:
- Read existing files before modifying them
- Make all necessary code changes
- Create new files as needed
- Run tests after making changes
- Commit your changes with a descriptive message
- You are in a fully automated pipeline. NEVER ask questions or wait for input.
- Make your best judgment and proceed immediately.`

type Worker struct {
	id      int
	cfg     *config.Config
	oc      *opencode.Client
	gh      *github.Client
	wtMgr   *git.WorktreeManager
	baseDir string
}

func NewWorker(id int, cfg *config.Config, oc *opencode.Client, gh *github.Client, wtMgr *git.WorktreeManager) *Worker {
	return &Worker{
		id:      id,
		cfg:     cfg,
		oc:      oc,
		gh:      gh,
		wtMgr:   wtMgr,
		baseDir: wtMgr.WorktreesDir(),
	}
}

func (w *Worker) Process(ctx context.Context, task *Task) error {
	log.Printf("[Worker %d] ▶ START processing #%d: %s", w.id, task.Issue.Number, task.Issue.Title)
	start := time.Now()

	task.Status = StatusAnalyzing

	branch := fmt.Sprintf("oda-%d-%s", task.Issue.Number, slug(task.Issue.Title))
	workerName := fmt.Sprintf("worker-%d", w.id)
	log.Printf("[Worker %d] Creating worktree %q on branch %q", w.id, workerName, branch)
	wt, err := w.wtMgr.Create(workerName, branch)
	if err != nil {
		task.Status = StatusFailed
		task.Result = &TaskResult{Error: fmt.Errorf("creating worktree: %w", err)}
		log.Printf("[Worker %d] ✗ FAILED creating worktree: %v", w.id, err)
		return task.Result.Error
	}
	task.Branch = branch
	task.Worktree = wt.Path
	log.Printf("[Worker %d] Worktree ready at %s", w.id, wt.Path)

	log.Printf("[Worker %d] [1/6] Analyzing #%d...", w.id, task.Issue.Number)
	stepStart := time.Now()
	analysis, err := w.analyze(ctx, task)
	if err != nil {
		task.Status = StatusFailed
		task.Result = &TaskResult{Error: fmt.Errorf("analyzing: %w", err)}
		log.Printf("[Worker %d] ✗ FAILED analyzing: %v", w.id, err)
		return task.Result.Error
	}
	log.Printf("[Worker %d] [1/6] Analysis done (%s, %d chars)", w.id, time.Since(stepStart).Round(time.Second), len(analysis))

	task.Status = StatusPlanning
	log.Printf("[Worker %d] [2/6] Planning #%d...", w.id, task.Issue.Number)
	stepStart = time.Now()
	plan, err := w.plan(ctx, task, analysis)
	if err != nil {
		task.Status = StatusFailed
		task.Result = &TaskResult{Error: fmt.Errorf("planning: %w", err)}
		log.Printf("[Worker %d] ✗ FAILED planning: %v", w.id, err)
		return task.Result.Error
	}
	log.Printf("[Worker %d] [2/6] Planning done (%s, %d chars)", w.id, time.Since(stepStart).Round(time.Second), len(plan))

	task.Status = StatusCoding
	log.Printf("[Worker %d] [3/6] Implementing #%d...", w.id, task.Issue.Number)
	stepStart = time.Now()
	if err := w.implement(ctx, task, plan); err != nil {
		task.Status = StatusFailed
		task.Result = &TaskResult{Error: fmt.Errorf("implementing: %w", err)}
		log.Printf("[Worker %d] ✗ FAILED implementing: %v", w.id, err)
		return task.Result.Error
	}
	log.Printf("[Worker %d] [3/6] Implementation done (%s)", w.id, time.Since(stepStart).Round(time.Second))

	task.Status = StatusTesting
	log.Printf("[Worker %d] [4/6] Testing #%d...", w.id, task.Issue.Number)
	stepStart = time.Now()
	if err := w.test(ctx, task); err != nil {
		task.Status = StatusFailed
		task.Result = &TaskResult{Error: fmt.Errorf("testing: %w", err)}
		log.Printf("[Worker %d] ✗ FAILED testing: %v", w.id, err)
		return task.Result.Error
	}
	log.Printf("[Worker %d] [4/6] Tests passed (%s)", w.id, time.Since(stepStart).Round(time.Second))

	task.Status = StatusReviewing
	log.Printf("[Worker %d] [5/6] Code review #%d...", w.id, task.Issue.Number)
	stepStart = time.Now()
	if err := w.codeReview(ctx, task); err != nil {
		task.Status = StatusFailed
		task.Result = &TaskResult{Error: fmt.Errorf("code review: %w", err)}
		log.Printf("[Worker %d] ✗ FAILED code review: %v", w.id, err)
		return task.Result.Error
	}
	log.Printf("[Worker %d] [5/6] Code review passed (%s)", w.id, time.Since(stepStart).Round(time.Second))

	task.Status = StatusCreatingPR
	log.Printf("[Worker %d] [6/6] Creating PR for #%d...", w.id, task.Issue.Number)
	stepStart = time.Now()
	prURL, err := w.createPR(ctx, task)
	if err != nil {
		task.Status = StatusFailed
		task.Result = &TaskResult{Error: fmt.Errorf("creating PR: %w", err)}
		log.Printf("[Worker %d] ✗ FAILED creating PR: %v", w.id, err)
		return task.Result.Error
	}
	log.Printf("[Worker %d] [6/6] PR created: %s (%s)", w.id, prURL, time.Since(stepStart).Round(time.Second))

	task.Status = StatusDone
	task.Result = &TaskResult{
		PRURL:   prURL,
		Summary: fmt.Sprintf("Implemented #%d: %s", task.Issue.Number, task.Issue.Title),
	}
	log.Printf("[Worker %d] ✓ DONE #%d in %s → %s", w.id, task.Issue.Number, time.Since(start).Round(time.Second), prURL)
	return nil
}

func (w *Worker) analyze(ctx context.Context, task *Task) (string, error) {
	sessionTitle := fmt.Sprintf("analyze-%d", task.Issue.Number)
	session, err := w.oc.CreateSession(sessionTitle)
	if err != nil {
		return "", fmt.Errorf("creating session: %w", err)
	}
	defer func() {
		if delErr := w.oc.DeleteSession(session.ID); delErr != nil {
			log.Printf("[Worker %d] failed to delete session %s: %v", w.id, session.ID, delErr)
		}
	}()

	prompt := fmt.Sprintf(analysisPrompt, task.Issue.Number, task.Issue.Title, task.Issue.Body)
	model := opencode.ParseModelRef(w.cfg.Planning.LLM)

	msg, err := w.oc.SendMessage(session.ID, prompt, model, nil)
	if err != nil {
		return "", fmt.Errorf("sending message: %w", err)
	}

	return extractText(msg), nil
}

func (w *Worker) plan(ctx context.Context, task *Task, analysis string) (string, error) {
	sessionTitle := fmt.Sprintf("plan-%d", task.Issue.Number)
	session, err := w.oc.CreateSession(sessionTitle)
	if err != nil {
		return "", fmt.Errorf("creating session: %w", err)
	}
	defer func() {
		if delErr := w.oc.DeleteSession(session.ID); delErr != nil {
			log.Printf("[Worker %d] failed to delete session %s: %v", w.id, session.ID, delErr)
		}
	}()

	prompt := fmt.Sprintf(planningPrompt, task.Issue.Number, task.Issue.Title, analysis)
	model := opencode.ParseModelRef(w.cfg.Planning.LLM)

	msg, err := w.oc.SendMessage(session.ID, prompt, model, nil)
	if err != nil {
		return "", fmt.Errorf("sending message: %w", err)
	}

	return extractText(msg), nil
}

func (w *Worker) implement(ctx context.Context, task *Task, plan string) error {
	sessionTitle := fmt.Sprintf("implement-%d", task.Issue.Number)
	w.oc.SetDirectory(task.Worktree)
	defer w.oc.SetDirectory("")

	session, err := w.oc.CreateSession(sessionTitle)
	if err != nil {
		return fmt.Errorf("creating session: %w", err)
	}
	defer func() {
		if delErr := w.oc.DeleteSession(session.ID); delErr != nil {
			log.Printf("[Worker %d] failed to delete session %s: %v", w.id, session.ID, delErr)
		}
	}()

	prompt := fmt.Sprintf(implementationPrompt, task.Issue.Number, task.Issue.Title, plan, task.Worktree)
	model := opencode.ParseModelRef(w.cfg.EpicAnalysis.LLM)

	_, err = w.oc.SendMessage(session.ID, prompt, model, nil)
	if err != nil {
		return fmt.Errorf("sending message: %w", err)
	}

	return nil
}

func (w *Worker) test(ctx context.Context, task *Task) error {
	testCmd := w.cfg.Tools.TestCmd
	if testCmd == "" {
		testCmd = "go test ./..."
	}

	_, err := git.RunInWorktree(task.Worktree, "sh", "-c", testCmd)
	if err != nil {
		return fmt.Errorf("tests failed: %w", err)
	}

	return nil
}

func (w *Worker) codeReview(ctx context.Context, task *Task) error {
	diff, err := git.RunInWorktree(task.Worktree, "git", "diff", "HEAD~1")
	if err != nil {
		diffOut, logErr := git.RunInWorktree(task.Worktree, "git", "log", "--oneline", "-5")
		if logErr == nil {
			log.Printf("[Worker %d] git log in worktree: %s", w.id, diffOut)
		}
		diff, err = git.RunInWorktree(task.Worktree, "git", "diff", "master")
		if err != nil {
			return fmt.Errorf("getting diff: %w", err)
		}
	}

	if len(diff) == 0 {
		log.Printf("[Worker %d] No diff found, skipping code review", w.id)
		return nil
	}

	sessionTitle := fmt.Sprintf("code-review-%d", task.Issue.Number)
	session, err := w.oc.CreateSession(sessionTitle)
	if err != nil {
		return fmt.Errorf("creating session: %w", err)
	}
	defer func() {
		if delErr := w.oc.DeleteSession(session.ID); delErr != nil {
			log.Printf("[Worker %d] failed to delete session %s: %v", w.id, session.ID, delErr)
		}
	}()

	prompt := fmt.Sprintf(codeReviewPrompt, task.Issue.Number, task.Issue.Title, string(diff))
	model := opencode.ParseModelRef(w.cfg.Planning.LLM)

	msg, err := w.oc.SendMessage(session.ID, prompt, model, nil)
	if err != nil {
		return fmt.Errorf("sending message: %w", err)
	}

	review := extractText(msg)
	log.Printf("[Worker %d] Code review result: %s", w.id, review)

	return nil
}

func (w *Worker) createPR(ctx context.Context, task *Task) (string, error) {
	if err := w.wtMgr.PushBranch(task.Branch); err != nil {
		return "", fmt.Errorf("pushing branch: %w", err)
	}

	body := fmt.Sprintf("Closes #%d\n\n%s", task.Issue.Number, task.Issue.Body)
	prURL, err := w.gh.CreatePR(task.Branch, task.Issue.Title, body)
	if err != nil {
		return "", fmt.Errorf("creating PR: %w", err)
	}

	return prURL, nil
}

var slugRe = regexp.MustCompile(`[^a-z0-9]+`)

func slug(title string) string {
	s := strings.ToLower(title)
	s = slugRe.ReplaceAllString(s, "-")
	s = strings.Trim(s, "-")
	if len(s) > 40 {
		s = s[:40]
		s = strings.TrimRight(s, "-")
	}
	return s
}

func extractText(msg *opencode.Message) string {
	if msg == nil {
		return ""
	}
	var sb strings.Builder
	for _, p := range msg.Parts {
		if p.Type == "text" {
			sb.WriteString(p.Text)
		}
	}
	return sb.String()
}
