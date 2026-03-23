package mvp

import (
	"context"
	"encoding/json"
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
	"github.com/crazy-goat/one-dev-army/internal/plan"
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
Read the relevant source files and verify. If and ONLY if the existing code already fully satisfies
all issue requirements with no changes needed, respond with a single line starting with the
exact prefix ALREADY_DONE: followed by your concrete evidence (e.g. "method Foo already exists in bar.go:42").
Do NOT use this if the feature is only partially implemented or needs any modifications.

If changes ARE needed (which is the expected case), create a concrete, actionable plan covering:
1. Which files to create or modify (exact paths)
2. What code changes to make in each file
3. What tests to add or update
4. Order of operations

Be specific and actionable. Do NOT ask questions. Output the plan directly.`

const codeReviewPrompt = `You are reviewing code changes for GitHub issue #%d: %s

The changes are in PR %s in repository %s.
Fetch the PR diff yourself using available tools, then review.

Check for:
1. Correctness — does the code do what the issue requires?
2. Code quality — clean, readable, well-structured?
3. Error handling — are errors handled properly?
4. Tests — adequate test coverage?
5. Security — any vulnerabilities introduced?
6. Performance — any obvious performance issues?

Set "approved" to true if the code is acceptable, false if changes are required.
Set "already_done" to true ONLY if the issue was already fully implemented before this PR (extremely rare).`

const maxCRRetries = 10

const fixFromReviewPrompt = `Fix the issues found during code review for GitHub issue #%d: %s

Working directory: %s
Test command: %s

Code review feedback:
%s

Instructions:
- Read the files mentioned in the review
- Fix ALL issues raised by the reviewer
- Run the test command and ensure all tests pass
- Commit your fixes with a descriptive message
- You are in a fully automated pipeline. NEVER ask questions or wait for input.
- Make your best judgment and proceed immediately.
- CRITICAL: Do NOT use git worktrees. Work directly in the provided working directory. Do NOT run "git worktree" commands.`

const implementationPrompt = `Implement the following plan for GitHub issue #%d: %s

Implementation plan:
%s

Working directory: %s

Test command: %s

Instructions:
- Read existing files before modifying them
- Make all necessary code changes
- Create new files as needed
- After implementing, run the test command and fix any failures until all tests pass
- Do NOT proceed until tests pass — iterate on the code until they do
- Commit your changes with a descriptive message
- You are in a fully automated pipeline. NEVER ask questions or wait for input.
- Make your best judgment and proceed immediately.
- CRITICAL: Do NOT use git worktrees. Work directly in the provided working directory. Do NOT run "git worktree" commands.`

type Worker struct {
	id      int
	cfg     *config.Config
	oc      *opencode.Client
	gh      *github.Client
	brMgr   *git.BranchManager
	store   *db.Store
	repoDir string
}

func NewWorker(id int, cfg *config.Config, oc *opencode.Client, gh *github.Client, brMgr *git.BranchManager, store *db.Store) *Worker {
	return &Worker{
		id:      id,
		cfg:     cfg,
		oc:      oc,
		gh:      gh,
		brMgr:   brMgr,
		store:   store,
		repoDir: brMgr.RepoDir(),
	}
}

var stepOrder = []string{"analyze", "plan", "implement", "code-review", "create-pr"}

func stepIndex(name string) int {
	for i, s := range stepOrder {
		if s == name {
			return i
		}
	}
	return -1
}

func (w *Worker) Process(ctx context.Context, task *Task) error {
	log.Printf("[Worker %d] ▶ START processing #%d: %s", w.id, task.Issue.Number, task.Issue.Title)
	start := time.Now()

	resumeFrom := 0
	if w.store != nil {
		lastDone, _ := w.store.GetLastCompletedStep(task.Issue.Number)
		if lastDone != "" {
			idx := stepIndex(lastDone)
			if idx >= 0 {
				resumeFrom = idx + 1
				log.Printf("[Worker %d] Resuming from step %d (%s completed previously)", w.id, resumeFrom, lastDone)
			}
		}
	}

	task.Status = StatusAnalyzing

	branch := fmt.Sprintf("oda-%d-%s", task.Issue.Number, slug(task.Issue.Title))
	log.Printf("[Worker %d] Creating branch %q", w.id, branch)
	if err := w.brMgr.CreateBranch(branch); err != nil {
		task.Status = StatusFailed
		task.Result = &TaskResult{Error: fmt.Errorf("creating branch: %w", err)}
		log.Printf("[Worker %d] ✗ FAILED creating branch: %v", w.id, err)
		return task.Result.Error
	}
	task.Branch = branch
	task.Worktree = w.repoDir
	log.Printf("[Worker %d] Branch %s ready, working in %s", w.id, branch, w.repoDir)

	var analysis, plan, prURL string
	var err error

	if resumeFrom <= 0 {
		log.Printf("[Worker %d] [1/5] Analyzing #%d...", w.id, task.Issue.Number)
		stepStart := time.Now()
		analysis, err = w.analyze(ctx, task)
		if err != nil {
			task.Status = StatusFailed
			task.Result = &TaskResult{Error: fmt.Errorf("analyzing: %w", err)}
			log.Printf("[Worker %d] ✗ FAILED analyzing: %v", w.id, err)
			return task.Result.Error
		}
		log.Printf("[Worker %d] [1/5] Analysis done (%s, %d chars)", w.id, time.Since(stepStart).Round(time.Second), len(analysis))
	} else {
		log.Printf("[Worker %d] [1/5] Skipping analyze (completed previously)", w.id)
		if w.store != nil {
			analysis, _ = w.store.GetStepResponse(task.Issue.Number, "analyze")
		}
	}

	if resumeFrom <= 1 {
		task.Status = StatusPlanning
		log.Printf("[Worker %d] [2/5] Planning #%d...", w.id, task.Issue.Number)
		stepStart := time.Now()
		plan, err = w.plan(ctx, task, analysis)
		if err != nil {
			task.Status = StatusFailed
			task.Result = &TaskResult{Error: fmt.Errorf("planning: %w", err)}
			log.Printf("[Worker %d] ✗ FAILED planning: %v", w.id, err)
			return task.Result.Error
		}
		log.Printf("[Worker %d] [2/5] Planning done (%s, %d chars)", w.id, time.Since(stepStart).Round(time.Second), len(plan))
	} else {
		log.Printf("[Worker %d] [2/5] Skipping plan (completed previously)", w.id)
		if w.store != nil {
			plan, _ = w.store.GetStepResponse(task.Issue.Number, "plan")
		}
	}

	if resumeFrom <= 2 {
		task.Status = StatusCoding
		log.Printf("[Worker %d] [3/5] Implementing #%d (includes tests)...", w.id, task.Issue.Number)
		stepStart := time.Now()
		if err := w.implement(ctx, task, plan); err != nil {
			task.Status = StatusFailed
			task.Result = &TaskResult{Error: fmt.Errorf("implementing: %w", err)}
			log.Printf("[Worker %d] ✗ FAILED implementing: %v", w.id, err)
			return task.Result.Error
		}
		log.Printf("[Worker %d] [3/5] Implementation done (%s)", w.id, time.Since(stepStart).Round(time.Second))
	} else {
		log.Printf("[Worker %d] [3/5] Skipping implement (completed previously)", w.id)
	}

	if resumeFrom <= 3 {
		task.Status = StatusReviewing
		log.Printf("[Worker %d] [4/5] Code review #%d...", w.id, task.Issue.Number)
		stepStart := time.Now()
		approved, review, crErr := w.codeReview(ctx, task, "")
		if crErr != nil {
			task.Status = StatusFailed
			task.Result = &TaskResult{Error: fmt.Errorf("code review: %w", crErr)}
			log.Printf("[Worker %d] ✗ FAILED code review: %v", w.id, crErr)
			return task.Result.Error
		}
		log.Printf("[Worker %d] [4/5] Code review done (%s, approved=%v)", w.id, time.Since(stepStart).Round(time.Second), approved)

		if !approved {
			// Retry with fixes
			log.Printf("[Worker %d] Code review not approved, fixing...", w.id)
			task.Status = StatusCoding
			stepStart = time.Now()
			if fixErr := w.fixFromReview(ctx, task, review); fixErr != nil {
				task.Status = StatusFailed
				task.Result = &TaskResult{Error: fmt.Errorf("fixing from review: %w", fixErr)}
				log.Printf("[Worker %d] ✗ FAILED fixing from review: %v", w.id, fixErr)
				return task.Result.Error
			}
			log.Printf("[Worker %d] Fix from review done (%s), pushing...", w.id, time.Since(stepStart).Round(time.Second))
			if pushErr := w.brMgr.PushBranch(task.Branch); pushErr != nil {
				task.Status = StatusFailed
				task.Result = &TaskResult{Error: fmt.Errorf("pushing fixes: %w", pushErr)}
				log.Printf("[Worker %d] ✗ FAILED pushing fixes: %v", w.id, pushErr)
				return task.Result.Error
			}
			// Re-run code review after fixes
			log.Printf("[Worker %d] Re-running code review after fixes...", w.id)
			stepStart = time.Now()
			approved, _, crErr = w.codeReview(ctx, task, "")
			if crErr != nil {
				task.Status = StatusFailed
				task.Result = &TaskResult{Error: fmt.Errorf("code review after fixes: %w", crErr)}
				log.Printf("[Worker %d] ✗ FAILED code review after fixes: %v", w.id, crErr)
				return task.Result.Error
			}
			log.Printf("[Worker %d] Code review after fixes done (%s, approved=%v)", w.id, time.Since(stepStart).Round(time.Second), approved)
			if !approved {
				task.Status = StatusFailed
				task.Result = &TaskResult{Error: fmt.Errorf("code review not approved after fixes")}
				log.Printf("[Worker %d] ✗ FAILED: code review not approved after fixes", w.id)
				return task.Result.Error
			}
		}
	} else {
		log.Printf("[Worker %d] [4/5] Skipping code-review (completed previously)", w.id)
	}

	if resumeFrom <= 4 {
		task.Status = StatusCreatingPR
		log.Printf("[Worker %d] [5/5] Creating PR for #%d...", w.id, task.Issue.Number)
		stepStart := time.Now()
		prURL, err = w.createPR(ctx, task)
		if err != nil {
			task.Status = StatusFailed
			task.Result = &TaskResult{Error: fmt.Errorf("creating PR: %w", err)}
			log.Printf("[Worker %d] ✗ FAILED creating PR: %v", w.id, err)
			return task.Result.Error
		}
		log.Printf("[Worker %d] [5/5] PR created: %s (%s)", w.id, prURL, time.Since(stepStart).Round(time.Second))
	} else {
		log.Printf("[Worker %d] [5/5] Skipping create-pr (completed previously)", w.id)
		if w.store != nil {
			prURL, _ = w.store.GetStepResponse(task.Issue.Number, "create-pr")
		}
	}

	task.Status = StatusDone
	task.Result = &TaskResult{
		PRURL:   prURL,
		Summary: fmt.Sprintf("Implemented #%d: %s — awaiting manual approval", task.Issue.Number, task.Issue.Title),
	}
	log.Printf("[Worker %d] ✓ DONE #%d in %s → %s (awaiting approval)", w.id, task.Issue.Number, time.Since(start).Round(time.Second), prURL)
	return nil
}

func (w *Worker) analyze(ctx context.Context, task *Task) (string, error) {
	prompt := fmt.Sprintf(analysisPrompt, task.Issue.Number, task.Issue.Title, task.Issue.Body)
	analysis, err := w.llmStep(ctx, task, "analyze", prompt, w.cfg.Planning.LLM)
	if err != nil {
		return "", err
	}

	// Create initial plan.md with analysis
	wt := &git.Worktree{
		Name:   fmt.Sprintf("worker-%d", w.id),
		Path:   w.repoDir,
		Branch: task.Branch,
	}
	planMgr := plan.NewAttachmentManager(w.gh, wt)

	planURL, err := planMgr.CreateInitialPlan(task.Issue.Number, task.Branch, analysis)
	if err != nil {
		log.Printf("[Worker %d] Warning: failed to create initial plan.md: %v", w.id, err)
	} else {
		log.Printf("[Worker %d] Created plan.md: %s", w.id, planURL)
		// Store URL in database
		if w.store != nil {
			if err := w.store.UpdateStepPlanURL(task.Issue.Number, "analyze", planURL); err != nil {
				log.Printf("[Worker %d] Warning: failed to store plan URL: %v", w.id, err)
			}
		}
	}

	return analysis, nil
}

func (w *Worker) plan(ctx context.Context, task *Task, analysis string) (string, error) {
	prompt := fmt.Sprintf(planningPrompt, task.Issue.Number, task.Issue.Title, analysis)
	planContent, err := w.llmStep(ctx, task, "plan", prompt, w.cfg.Planning.LLM)
	if err != nil {
		return "", err
	}
	if reason := checkAlreadyDone(planContent); reason != "" {
		log.Printf("[Worker %d] Planning detected ticket already done: %s", w.id, reason)
		return "", fmt.Errorf("%w: %s", ErrAlreadyDone, reason)
	}

	// Update plan.md with implementation steps
	wt := &git.Worktree{
		Name:   fmt.Sprintf("worker-%d", w.id),
		Path:   w.repoDir,
		Branch: task.Branch,
	}
	planMgr := plan.NewAttachmentManager(w.gh, wt)

	planURL, err := planMgr.UpdatePlanWithImplementation(task.Issue.Number, task.Branch, analysis, planContent)
	if err != nil {
		log.Printf("[Worker %d] Warning: failed to update plan.md with implementation: %v", w.id, err)
	} else {
		log.Printf("[Worker %d] Updated plan.md with implementation: %s", w.id, planURL)
		// Store URL in database
		if w.store != nil {
			if err := w.store.UpdateStepPlanURL(task.Issue.Number, "plan", planURL); err != nil {
				log.Printf("[Worker %d] Warning: failed to store plan URL: %v", w.id, err)
			}
		}
	}

	return planContent, nil
}

func (w *Worker) implement(ctx context.Context, task *Task, planStr string) error {
	w.oc.SetDirectory(task.Worktree)
	defer w.oc.SetDirectory("")

	// Try to retrieve plan from GitHub if available
	wt := &git.Worktree{
		Name:   fmt.Sprintf("worker-%d", w.id),
		Path:   w.repoDir,
		Branch: task.Branch,
	}
	planMgr := plan.NewAttachmentManager(w.gh, wt)

	githubPlan, err := planMgr.GetFromIssue(ctx, task.Issue.Number)
	if err == nil && githubPlan != nil {
		// Use the plan from GitHub
		planStr = githubPlan.ToMarkdown()
		log.Printf("[Worker %d] Using plan.md from GitHub", w.id)
	} else {
		log.Printf("[Worker %d] Falling back to context-based plan", w.id)
	}

	testCmd := w.cfg.Tools.TestCmd
	if testCmd == "" {
		testCmd = "go test ./..."
	}
	prompt := fmt.Sprintf(implementationPrompt, task.Issue.Number, task.Issue.Title, planStr, task.Worktree, testCmd)
	_, err = w.llmStep(ctx, task, "implement", prompt, w.cfg.EpicAnalysis.LLM)
	if err != nil {
		return err
	}
	w.ensureCommit(task)
	return nil
}

func (w *Worker) ensureCommit(task *Task) {
	git.RunInWorktree(task.Worktree, "git", "add", "-A")
	out, err := git.RunInWorktree(task.Worktree, "git", "diff", "--cached", "--quiet")
	if err != nil {
		msg := fmt.Sprintf("feat: implement #%d %s", task.Issue.Number, task.Issue.Title)
		git.RunInWorktree(task.Worktree, "git", "commit", "-m", msg)
		log.Printf("[Worker %d] Auto-committed uncommitted changes", w.id)
	}
	_ = out
}

func (w *Worker) fixFromReview(ctx context.Context, task *Task, review string) error {
	w.oc.SetDirectory(task.Worktree)
	defer w.oc.SetDirectory("")

	testCmd := w.cfg.Tools.TestCmd
	if testCmd == "" {
		testCmd = "go test ./..."
	}
	prompt := fmt.Sprintf(fixFromReviewPrompt, task.Issue.Number, task.Issue.Title, task.Worktree, testCmd, review)
	_, err := w.llmStep(ctx, task, "fix-from-review", prompt, w.cfg.EpicAnalysis.LLM)
	if err != nil {
		return err
	}
	w.ensureCommit(task)
	return nil
}

var crSchema = json.RawMessage(`{
	"type": "object",
	"properties": {
		"approved":     {"type": "boolean"},
		"already_done": {"type": "boolean"},
		"issues":       {"type": "array", "items": {"type": "string"}},
		"suggestions":  {"type": "array", "items": {"type": "string"}},
		"verdict":      {"type": "string"}
	},
	"required": ["approved", "issues", "suggestions", "verdict"]
}`)

type crResult struct {
	Approved    bool     `json:"approved"`
	AlreadyDone bool     `json:"already_done"`
	Issues      []string `json:"issues"`
	Suggestions []string `json:"suggestions"`
	Verdict     string   `json:"verdict"`
}

func (w *Worker) codeReview(ctx context.Context, task *Task, prURL string) (approved bool, review string, err error) {
	prompt := fmt.Sprintf(codeReviewPrompt, task.Issue.Number, task.Issue.Title, prURL, w.gh.Repo)

	sessionTitle := fmt.Sprintf("code-review-%d", task.Issue.Number)
	session, err := w.oc.CreateSession(sessionTitle)
	if err != nil {
		return false, "", fmt.Errorf("creating session: %w", err)
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
		id, sErr := w.store.InsertStep(task.Issue.Number, "code-review", prompt, session.ID)
		if sErr != nil {
			log.Printf("[Worker %d] failed to insert step: %v", w.id, sErr)
		} else {
			stepID = id
		}
	}

	model := opencode.ParseModelRef(w.cfg.Planning.LLM)
	var result crResult
	if err := w.oc.SendMessageStructured(ctx, session.ID, prompt, model, crSchema, &result); err != nil {
		if w.store != nil && stepID > 0 {
			_ = w.store.FailStep(stepID, err.Error())
		}
		return false, "", fmt.Errorf("code review: %w", err)
	}

	reviewJSON, _ := json.Marshal(result)
	review = string(reviewJSON)

	if w.store != nil && stepID > 0 {
		_ = w.store.FinishStep(stepID, review)
	}

	log.Printf("[Worker %d] Code review result: %s", w.id, review)

	if result.AlreadyDone {
		log.Printf("[Worker %d] Code review detected ticket already done: %s", w.id, result.Verdict)
		return false, "", fmt.Errorf("%w: %s", ErrAlreadyDone, result.Verdict)
	}

	return result.Approved, review, nil
}

func (w *Worker) createPR(ctx context.Context, task *Task) (string, error) {
	// Check if PR already exists
	if existingBranch, err := w.gh.FindPRBranch(task.Issue.Number); err == nil && existingBranch != "" {
		log.Printf("[Worker %d] PR already exists for issue #%d (branch: %s)", w.id, task.Issue.Number, existingBranch)
		// Return empty - CreatePR below will get the existing PR URL
	}

	var stepID int64
	if w.store != nil {
		id, err := w.store.InsertStep(task.Issue.Number, "create-pr", fmt.Sprintf("push %s + create PR", task.Branch), "")
		if err != nil {
			log.Printf("[Worker %d] failed to insert create-pr step: %v", w.id, err)
		} else {
			stepID = id
		}
	}

	if err := w.brMgr.PushBranch(task.Branch); err != nil {
		if w.store != nil && stepID > 0 {
			_ = w.store.FailStep(stepID, err.Error())
		}
		return "", fmt.Errorf("pushing branch: %w", err)
	}

	body := fmt.Sprintf("Closes #%d\n\n%s", task.Issue.Number, task.Issue.Body)
	prURL, err := w.gh.CreatePR(task.Branch, task.Issue.Title, body)
	if err != nil {
		if existingURL := extractPRURL(err.Error()); existingURL != "" {
			log.Printf("[Worker %d] PR already exists: %s", w.id, existingURL)
			if w.store != nil && stepID > 0 {
				_ = w.store.FinishStep(stepID, existingURL)
			}
			return existingURL, nil
		}
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

var prURLRe = regexp.MustCompile(`https://github\.com/[^\s]+/pull/\d+`)

func extractPRURL(errMsg string) string {
	match := prURLRe.FindString(errMsg)
	return match
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
