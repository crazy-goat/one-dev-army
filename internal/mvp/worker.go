package mvp

import (
	"context"
	"errors"
	"fmt"
	"log"
	"regexp"
	"strings"
	"time"

	"github.com/crazy-goat/one-dev-army/internal/config"
	"github.com/crazy-goat/one-dev-army/internal/db"
	"github.com/crazy-goat/one-dev-army/internal/git"
	"github.com/crazy-goat/one-dev-army/internal/github"
	"github.com/crazy-goat/one-dev-army/internal/opencode"
)

var ErrAlreadyDone = errors.New("ticket already done")

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

IMPORTANT: First check if this feature/fix is ALREADY IMPLEMENTED in the codebase.
If the code already satisfies the issue requirements, respond with ONLY:
ALREADY_DONE: <brief explanation of why the ticket is already done>

Otherwise, create a concrete, actionable plan covering:
1. Which files to create or modify (exact paths)
2. What code changes to make in each file
3. What tests to add or update
4. Order of operations

Be specific and actionable. Do NOT ask questions. Output the plan directly.`

const codeReviewPrompt = `You are reviewing code changes for GitHub issue #%d: %s

The changes are in PR %s in repository %s.
Fetch the PR diff yourself using available tools, then review.

IMPORTANT: If you discover that the issue requirements were ALREADY satisfied by existing code
(before this PR's changes), and the PR is unnecessary, respond with ONLY:
ALREADY_DONE: <brief explanation of why the ticket was already done>

Otherwise check for:
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
	store   *db.Store
	baseDir string
}

func NewWorker(id int, cfg *config.Config, oc *opencode.Client, gh *github.Client, wtMgr *git.WorktreeManager, store *db.Store) *Worker {
	return &Worker{
		id:      id,
		cfg:     cfg,
		oc:      oc,
		gh:      gh,
		wtMgr:   wtMgr,
		store:   store,
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

	task.Status = StatusCreatingPR
	log.Printf("[Worker %d] [5/6] Creating PR for #%d...", w.id, task.Issue.Number)
	stepStart = time.Now()
	prURL, err := w.createPR(ctx, task)
	if err != nil {
		task.Status = StatusFailed
		task.Result = &TaskResult{Error: fmt.Errorf("creating PR: %w", err)}
		log.Printf("[Worker %d] ✗ FAILED creating PR: %v", w.id, err)
		return task.Result.Error
	}
	log.Printf("[Worker %d] [5/6] PR created: %s (%s)", w.id, prURL, time.Since(stepStart).Round(time.Second))

	task.Status = StatusReviewing
	log.Printf("[Worker %d] [6/6] Code review #%d (PR: %s)...", w.id, task.Issue.Number, prURL)
	stepStart = time.Now()
	if err := w.codeReview(ctx, task, prURL); err != nil {
		task.Status = StatusFailed
		task.Result = &TaskResult{Error: fmt.Errorf("code review: %w", err)}
		log.Printf("[Worker %d] ✗ FAILED code review: %v", w.id, err)
		return task.Result.Error
	}
	log.Printf("[Worker %d] [6/6] Code review done (%s)", w.id, time.Since(stepStart).Round(time.Second))

	task.Status = StatusDone
	task.Result = &TaskResult{
		PRURL:   prURL,
		Summary: fmt.Sprintf("Implemented #%d: %s", task.Issue.Number, task.Issue.Title),
	}
	log.Printf("[Worker %d] ✓ DONE #%d in %s → %s", w.id, task.Issue.Number, time.Since(start).Round(time.Second), prURL)
	return nil
}

func (w *Worker) analyze(ctx context.Context, task *Task) (string, error) {
	prompt := fmt.Sprintf(analysisPrompt, task.Issue.Number, task.Issue.Title, task.Issue.Body)
	return w.llmStep(ctx, task, "analyze", prompt, w.cfg.Planning.LLM)
}

func (w *Worker) plan(ctx context.Context, task *Task, analysis string) (string, error) {
	prompt := fmt.Sprintf(planningPrompt, task.Issue.Number, task.Issue.Title, analysis)
	result, err := w.llmStep(ctx, task, "plan", prompt, w.cfg.Planning.LLM)
	if err != nil {
		return "", err
	}
	if reason := checkAlreadyDone(result); reason != "" {
		log.Printf("[Worker %d] Planning detected ticket already done: %s", w.id, reason)
		return "", fmt.Errorf("%w: %s", ErrAlreadyDone, reason)
	}
	return result, nil
}

func (w *Worker) implement(ctx context.Context, task *Task, plan string) error {
	w.oc.SetDirectory(task.Worktree)
	defer w.oc.SetDirectory("")

	prompt := fmt.Sprintf(implementationPrompt, task.Issue.Number, task.Issue.Title, plan, task.Worktree)
	_, err := w.llmStep(ctx, task, "implement", prompt, w.cfg.EpicAnalysis.LLM)
	return err
}

func (w *Worker) test(ctx context.Context, task *Task) error {
	testCmd := w.cfg.Tools.TestCmd
	if testCmd == "" {
		testCmd = "go test ./..."
	}

	var stepID int64
	if w.store != nil {
		id, err := w.store.InsertStep(task.Issue.Number, "test", testCmd, "")
		if err != nil {
			log.Printf("[Worker %d] failed to insert test step: %v", w.id, err)
		} else {
			stepID = id
		}
	}

	out, err := git.RunInWorktree(task.Worktree, "sh", "-c", testCmd)
	if err != nil {
		if w.store != nil && stepID > 0 {
			_ = w.store.FailStep(stepID, fmt.Sprintf("%v\n%s", err, out))
		}
		return fmt.Errorf("tests failed: %w", err)
	}

	if w.store != nil && stepID > 0 {
		_ = w.store.FinishStep(stepID, string(out))
	}
	return nil
}

func (w *Worker) codeReview(ctx context.Context, task *Task, prURL string) error {
	prompt := fmt.Sprintf(codeReviewPrompt, task.Issue.Number, task.Issue.Title, prURL, w.gh.Repo)
	review, err := w.llmStep(ctx, task, "code-review", prompt, w.cfg.Planning.LLM)
	if err != nil {
		return err
	}
	if reason := checkAlreadyDone(review); reason != "" {
		log.Printf("[Worker %d] Code review detected ticket already done: %s", w.id, reason)
		return fmt.Errorf("%w: %s", ErrAlreadyDone, reason)
	}
	log.Printf("[Worker %d] Code review result: %s", w.id, review)
	return nil
}

func (w *Worker) createPR(ctx context.Context, task *Task) (string, error) {
	var stepID int64
	if w.store != nil {
		id, err := w.store.InsertStep(task.Issue.Number, "create-pr", fmt.Sprintf("push %s + create PR", task.Branch), "")
		if err != nil {
			log.Printf("[Worker %d] failed to insert create-pr step: %v", w.id, err)
		} else {
			stepID = id
		}
	}

	if err := w.wtMgr.PushBranch(task.Branch); err != nil {
		if w.store != nil && stepID > 0 {
			_ = w.store.FailStep(stepID, err.Error())
		}
		return "", fmt.Errorf("pushing branch: %w", err)
	}

	body := fmt.Sprintf("Closes #%d\n\n%s", task.Issue.Number, task.Issue.Body)
	prURL, err := w.gh.CreatePR(task.Branch, task.Issue.Title, body)
	if err != nil {
		if w.store != nil && stepID > 0 {
			_ = w.store.FailStep(stepID, err.Error())
		}
		return "", fmt.Errorf("creating PR: %w", err)
	}

	if w.store != nil && stepID > 0 {
		_ = w.store.FinishStep(stepID, prURL)
	}
	return prURL, nil
}

func (w *Worker) llmStep(ctx context.Context, task *Task, stepName, prompt, llm string) (string, error) {
	sessionTitle := fmt.Sprintf("%s-%d", stepName, task.Issue.Number)
	session, err := w.oc.CreateSession(sessionTitle)
	if err != nil {
		return "", fmt.Errorf("creating session: %w", err)
	}

	task.SetSessionID(session.ID)
	defer task.SetSessionID("")
	defer func() {
		if delErr := w.oc.DeleteSession(session.ID); delErr != nil {
			log.Printf("[Worker %d] failed to delete session %s: %v", w.id, session.ID, delErr)
		}
	}()

	var stepID int64
	if w.store != nil {
		id, sErr := w.store.InsertStep(task.Issue.Number, stepName, prompt, session.ID)
		if sErr != nil {
			log.Printf("[Worker %d] failed to insert step: %v", w.id, sErr)
		} else {
			stepID = id
		}
	}

	model := opencode.ParseModelRef(llm)
	msg, err := w.oc.SendMessage(session.ID, prompt, model, nil)
	if err != nil {
		if w.store != nil && stepID > 0 {
			_ = w.store.FailStep(stepID, err.Error())
		}
		return "", fmt.Errorf("sending message: %w", err)
	}

	response := extractText(msg)
	if w.store != nil && stepID > 0 {
		_ = w.store.FinishStep(stepID, response)
	}

	return response, nil
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

func checkAlreadyDone(response string) string {
	for _, line := range strings.Split(response, "\n") {
		line = strings.TrimSpace(line)
		if strings.HasPrefix(line, "ALREADY_DONE:") {
			return strings.TrimSpace(strings.TrimPrefix(line, "ALREADY_DONE:"))
		}
	}
	return ""
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
