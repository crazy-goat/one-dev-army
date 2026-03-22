package mvp

import (
	"context"
	"fmt"
	"log"
	"regexp"
	"strings"

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
	task.Status = StatusAnalyzing

	branch := fmt.Sprintf("oda-%d-%s", task.Issue.Number, slug(task.Issue.Title))
	workerName := fmt.Sprintf("worker-%d", w.id)
	wt, err := w.wtMgr.Create(workerName, branch)
	if err != nil {
		task.Status = StatusFailed
		task.Result = &TaskResult{Error: fmt.Errorf("creating worktree: %w", err)}
		return task.Result.Error
	}
	task.Branch = branch
	task.Worktree = wt.Path

	analysis, err := w.analyze(ctx, task)
	if err != nil {
		task.Status = StatusFailed
		task.Result = &TaskResult{Error: fmt.Errorf("analyzing: %w", err)}
		return task.Result.Error
	}

	task.Status = StatusPlanning
	plan, err := w.plan(ctx, task, analysis)
	if err != nil {
		task.Status = StatusFailed
		task.Result = &TaskResult{Error: fmt.Errorf("planning: %w", err)}
		return task.Result.Error
	}

	task.Status = StatusCoding
	if err := w.implement(ctx, task, plan); err != nil {
		task.Status = StatusFailed
		task.Result = &TaskResult{Error: fmt.Errorf("implementing: %w", err)}
		return task.Result.Error
	}

	task.Status = StatusTesting
	if err := w.test(ctx, task); err != nil {
		task.Status = StatusFailed
		task.Result = &TaskResult{Error: fmt.Errorf("testing: %w", err)}
		return task.Result.Error
	}

	task.Status = StatusCreatingPR
	prURL, err := w.createPR(ctx, task)
	if err != nil {
		task.Status = StatusFailed
		task.Result = &TaskResult{Error: fmt.Errorf("creating PR: %w", err)}
		return task.Result.Error
	}

	task.Status = StatusDone
	task.Result = &TaskResult{
		PRURL:   prURL,
		Summary: fmt.Sprintf("Implemented #%d: %s", task.Issue.Number, task.Issue.Title),
	}
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
