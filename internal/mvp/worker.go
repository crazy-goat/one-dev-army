package mvp

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"log"
	"regexp"
	"strings"
	"sync/atomic"
	"time"

	"github.com/crazy-goat/one-dev-army/internal/config"
	"github.com/crazy-goat/one-dev-army/internal/db"
	"github.com/crazy-goat/one-dev-army/internal/git"
	"github.com/crazy-goat/one-dev-army/internal/github"
	"github.com/crazy-goat/one-dev-army/internal/llm"
	"github.com/crazy-goat/one-dev-army/internal/opencode"
	"github.com/crazy-goat/one-dev-army/internal/plan"
	"github.com/crazy-goat/one-dev-army/internal/prompts"
)

var ErrAlreadyDone = errors.New("ticket already done")

type Worker struct {
	id           int
	cfg          atomic.Pointer[config.Config]
	oc           *opencode.Client
	gh           *github.Client
	brMgr        *git.BranchManager
	store        *db.Store
	repoDir      string
	orchestrator *Orchestrator
	router       *llm.Router
	decisionCh   chan UserDecision // receives approve/decline from dashboard
}

func NewWorker(id int, cfg *config.Config, oc *opencode.Client, gh *github.Client, brMgr *git.BranchManager, store *db.Store, orchestrator *Orchestrator, router *llm.Router) *Worker {
	w := &Worker{
		id:           id,
		oc:           oc,
		gh:           gh,
		brMgr:        brMgr,
		store:        store,
		repoDir:      brMgr.RepoDir(),
		orchestrator: orchestrator,
		router:       router,
		decisionCh:   make(chan UserDecision, 1),
	}
	w.cfg.Store(cfg)
	return w
}

// Processes the event synchronously so that GitHub labels, cache, ledger,
// and WebSocket are updated immediately — not deferred until after Process().
func (w *Worker) reportStageComplete(stage string, status EventStatus, output string) {
	if w.orchestrator == nil || w.orchestrator.currentTask == nil {
		return
	}

	event := WorkerEvent{
		IssueNumber: w.orchestrator.currentTask.Issue.Number,
		Stage:       stage,
		Status:      status,
		Output:      output,
	}

	log.Printf("[Worker] Reporting completion of stage %s for issue #%d", stage, event.IssueNumber)
	w.orchestrator.HandleWorkerEvent(event)
}

var stepOrder = []string{"technical-planning", "implement", "code-review", "create-pr", "awaiting-approval", "merge"}

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
	task.StartTime = start

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

	// Deferred cleanup: delete branch if task did not complete successfully.
	// On successful merge, gh pr merge --delete-branch handles branch deletion.
	defer func() {
		if task.Branch != "" && task.Status != StatusDone {
			log.Printf("[Worker %d] Cleaning up branch %q (task not done)", w.id, task.Branch)
			if err := w.brMgr.RemoveBranch(task.Branch); err != nil {
				log.Printf("[Worker %d] Warning: failed to remove branch %q: %v", w.id, task.Branch, err)
			}
		}
	}()

	var analysis, implPlan, prURL string
	var err error

	if resumeFrom <= 0 {
		log.Printf("[Worker %d] [1/4] Technical planning for #%d...", w.id, task.Issue.Number)
		stepStart := time.Now()
		analysis, implPlan, err = w.technicalPlanning(ctx, task)
		if err != nil {
			task.Status = StatusFailed
			task.Result = &TaskResult{Error: fmt.Errorf("technical planning: %w", err)}
			log.Printf("[Worker %d] ✗ FAILED technical planning: %v", w.id, err)
			return task.Result.Error
		}
		log.Printf("[Worker %d] [1/4] Technical planning done (%s, analysis=%d chars, plan=%d chars)", w.id, time.Since(stepStart).Round(time.Second), len(analysis), len(implPlan))
		w.reportStageComplete("analysis", EventSuccess, "technical planning completed")
	} else {
		log.Printf("[Worker %d] [1/4] Skipping technical-planning (completed previously)", w.id)
		if w.store != nil {
			// Try to get combined response from new step name
			response, _ := w.store.GetStepResponse(task.Issue.Number, "technical-planning")
			if response != "" {
				_, implPlan = parseTechnicalPlanningResponse(response)
			} else {
				// Fallback: try to get from old step names for backward compatibility
				_, _ = w.store.GetStepResponse(task.Issue.Number, "analyze")
				implPlan, _ = w.store.GetStepResponse(task.Issue.Number, "plan")
			}
		}
	}

	if resumeFrom <= 1 {
		task.Status = StatusCoding
		log.Printf("[Worker %d] [2/4] Implementing #%d (includes tests)...", w.id, task.Issue.Number)
		stepStart := time.Now()
		if err := w.implement(ctx, task, implPlan); err != nil {
			task.Status = StatusFailed
			task.Result = &TaskResult{Error: fmt.Errorf("implementing: %w", err)}
			log.Printf("[Worker %d] ✗ FAILED implementing: %v", w.id, err)
			return task.Result.Error
		}
		log.Printf("[Worker %d] [2/4] Implementation done (%s)", w.id, time.Since(stepStart).Round(time.Second))
		w.reportStageComplete("coding", EventSuccess, "implementation completed")
	} else {
		log.Printf("[Worker %d] [2/4] Skipping implement (completed previously)", w.id)
	}

	if resumeFrom <= 2 {
		task.Status = StatusReviewing
		log.Printf("[Worker %d] [3/4] Code review #%d...", w.id, task.Issue.Number)
		stepStart := time.Now()
		approved, review, crErr := w.codeReview(ctx, task, "")
		if crErr != nil {
			task.Status = StatusFailed
			task.Result = &TaskResult{Error: fmt.Errorf("code review: %w", crErr)}
			log.Printf("[Worker %d] ✗ FAILED code review: %v", w.id, crErr)
			return task.Result.Error
		}
		log.Printf("[Worker %d] [3/4] Code review done (%s, approved=%v)", w.id, time.Since(stepStart).Round(time.Second), approved)

		if !approved {
			// Retry with fixes
			log.Printf("[Worker %d] Code review not approved, fixing...", w.id)
			task.Status = StatusCoding
			w.reportStageComplete("coding", EventInProgress, "fixing from AI review")
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
				task.Result = &TaskResult{Error: errors.New("code review not approved after fixes")}
				log.Printf("[Worker %d] ✗ FAILED: code review not approved after fixes", w.id)
				return task.Result.Error
			}
		}
		w.reportStageComplete("code-review", EventSuccess, "code review approved")
	} else {
		log.Printf("[Worker %d] [3/4] Skipping code-review (completed previously)", w.id)
	}

	if resumeFrom <= 3 {
		task.Status = StatusCreatingPR
		log.Printf("[Worker %d] [4/4] Creating PR for #%d...", w.id, task.Issue.Number)
		stepStart := time.Now()
		prURL, err = w.createPR(ctx, task)
		if err != nil {
			task.Status = StatusFailed
			task.Result = &TaskResult{Error: fmt.Errorf("creating PR: %w", err)}
			log.Printf("[Worker %d] ✗ FAILED creating PR: %v", w.id, err)
			return task.Result.Error
		}
		log.Printf("[Worker %d] [4/4] PR created: %s (%s)", w.id, prURL, time.Since(stepStart).Round(time.Second))
		w.reportStageComplete("create-pr", EventSuccess, "PR created: "+prURL)
	} else {
		log.Printf("[Worker %d] [4/4] Skipping create-pr (completed previously)", w.id)
		if w.store != nil {
			prURL, _ = w.store.GetStepResponse(task.Issue.Number, "create-pr")
		}
	}

	// If resuming from awaiting-approval or later, recover prURL from store
	if resumeFrom >= 4 && prURL == "" && w.store != nil {
		prURL, _ = w.store.GetStepResponse(task.Issue.Number, "create-pr")
	}

	// === APPROVAL + MERGE LOOP ===
	// Worker stays alive until ticket reaches terminal state.
	// User approve → merge → done. User decline → fix → re-review → wait again.
	for {
		task.Status = StatusAwaitingApproval
		log.Printf("[Worker %d] [5/6] Awaiting user approval for #%d (PR: %s)", w.id, task.Issue.Number, prURL)
		w.reportStageComplete("create-pr", EventSuccess, "PR created, awaiting approval: "+prURL)

		var decision UserDecision
		if w.cfg.Load().YoloMode {
			// YOLO mode: auto-approve without waiting for user
			log.Printf("[Worker %d] YOLO mode enabled - auto-approving #%d", w.id, task.Issue.Number)
			decision = UserDecision{Action: "approve"}
		} else {
			// Block until user sends decision or context canceled
			select {
			case decision = <-w.decisionCh:
				log.Printf("[Worker %d] Received decision for #%d: %s", w.id, task.Issue.Number, decision.Action)
			case <-ctx.Done():
				return ctx.Err()
			}
		}

		if decision.Action == "approve" {
			// Proceed to merge
			task.Status = StatusMerging
			log.Printf("[Worker %d] [6/6] Merging PR for #%d (branch: %s)", w.id, task.Issue.Number, task.Branch)
			w.reportStageComplete("awaiting-approval", EventSuccess, "user approved")

			if err := w.gh.MergePR(task.Branch); err != nil {
				log.Printf("[Worker %d] ✗ Merge failed for #%d: %v", w.id, task.Issue.Number, err)

				// Close PR on merge failure
				if closeErr := w.gh.ClosePR(task.Branch); closeErr != nil {
					log.Printf("[Worker %d] Error closing PR for #%d: %v", w.id, task.Issue.Number, closeErr)
				}

				task.Status = StatusFailed
				task.Result = &TaskResult{Error: fmt.Errorf("merge failed: %w", err)}

				comment := "Merge failed (likely conflict). PR closed, task moved to Failed.\n\nError: " + err.Error()
				if cmtErr := w.gh.AddComment(task.Issue.Number, comment); cmtErr != nil {
					log.Printf("[Worker %d] Error adding comment to #%d: %v", w.id, task.Issue.Number, cmtErr)
				}

				return task.Result.Error
			}

			w.reportStageComplete("merge", EventSuccess, "PR merged successfully")

			// Checkout default branch to prepare for next ticket
			if err := w.brMgr.CheckoutDefault(); err != nil {
				log.Printf("[Worker %d] Warning: failed to checkout default branch after merge: %v", w.id, err)
			}

			task.Status = StatusDone
			task.Result = &TaskResult{
				PRURL:   prURL,
				Summary: fmt.Sprintf("Implemented and merged #%d: %s", task.Issue.Number, task.Issue.Title),
			}
			log.Printf("[Worker %d] ✓ DONE #%d in %s → merged", w.id, task.Issue.Number, time.Since(start).Round(time.Second))
			return nil
		}

		// Decline — fix and retry
		log.Printf("[Worker %d] PR declined for #%d, fixing: %s", w.id, task.Issue.Number, decision.Reason)
		task.Status = StatusCoding
		w.reportStageComplete("coding", EventInProgress, "fixing from user decline")

		if decision.Reason != "" {
			comment := "**Declined** — sent back for fixes.\n\n" + decision.Reason
			if cmtErr := w.gh.AddComment(task.Issue.Number, comment); cmtErr != nil {
				log.Printf("[Worker %d] Error adding decline comment to #%d: %v", w.id, task.Issue.Number, cmtErr)
			}
		}

		w.reportStageComplete("awaiting-approval", EventFailed, "user declined: "+decision.Reason)

		// Fix from decline feedback
		if fixErr := w.fixFromReview(ctx, task, decision.Reason); fixErr != nil {
			task.Status = StatusFailed
			task.Result = &TaskResult{Error: fmt.Errorf("fixing from decline: %w", fixErr)}
			log.Printf("[Worker %d] ✗ FAILED fixing from decline: %v", w.id, fixErr)
			return task.Result.Error
		}

		// Push fixes
		if pushErr := w.brMgr.PushBranch(task.Branch); pushErr != nil {
			task.Status = StatusFailed
			task.Result = &TaskResult{Error: fmt.Errorf("pushing fixes after decline: %w", pushErr)}
			return task.Result.Error
		}

		// Re-run code review
		task.Status = StatusReviewing
		w.reportStageComplete("coding", EventSuccess, "fixes applied after decline")

		approved, review, crErr := w.codeReview(ctx, task, prURL)
		if crErr != nil {
			task.Status = StatusFailed
			task.Result = &TaskResult{Error: fmt.Errorf("code review after decline: %w", crErr)}
			return task.Result.Error
		}

		if !approved {
			// One more fix attempt
			task.Status = StatusCoding
			w.reportStageComplete("coding", EventInProgress, "fixing from re-review after decline")
			if fixErr := w.fixFromReview(ctx, task, review); fixErr != nil {
				task.Status = StatusFailed
				task.Result = &TaskResult{Error: fmt.Errorf("fixing from re-review: %w", fixErr)}
				return task.Result.Error
			}
			if pushErr := w.brMgr.PushBranch(task.Branch); pushErr != nil {
				task.Status = StatusFailed
				task.Result = &TaskResult{Error: fmt.Errorf("pushing re-review fixes: %w", pushErr)}
				return task.Result.Error
			}
		}

		w.reportStageComplete("code-review", EventSuccess, "code review passed after decline")
		// Loop back to await approval again
	}
}

func (w *Worker) technicalPlanning(ctx context.Context, task *Task) (analysis, implPlan string, err error) {
	prompt := fmt.Sprintf(prompts.MustGet(prompts.MVPTechnicalPlanning), task.Issue.Number, task.Issue.Title, task.Issue.Body)

	// Use router to select model for planning category
	llmModel := w.cfg.Load().LLM.Planning.Model
	if w.router != nil {
		llmModel = w.router.SelectModel(config.CategoryPlanning, config.ComplexityMedium, nil)
	}

	response, err := w.llmStep(ctx, task, "technical-planning", prompt, llmModel)
	if err != nil {
		return "", "", err
	}

	if reason := checkAlreadyDone(response); reason != "" {
		log.Printf("[Worker %d] Technical planning detected ticket already done: %s", w.id, reason)
		return "", "", fmt.Errorf("%w: %s", ErrAlreadyDone, reason)
	}

	// Parse response to extract analysis and plan sections
	analysis, implPlan = parseTechnicalPlanningResponse(response)

	// Create/update plan.md with both analysis and plan
	wt := &git.Worktree{
		Name:   fmt.Sprintf("worker-%d", w.id),
		Path:   w.repoDir,
		Branch: task.Branch,
	}
	planMgr := plan.NewAttachmentManager(w.gh, wt)

	_, err = planMgr.CreateFullPlan(task.Issue.Number, task.Branch, analysis, implPlan)
	if err != nil {
		log.Printf("[Worker %d] Warning: failed to add technical planning comment: %v", w.id, err)
	} else {
		log.Printf("[Worker %d] Added technical planning comment to issue #%d", w.id, task.Issue.Number)
	}

	return analysis, implPlan, nil
}

func parseTechnicalPlanningResponse(response string) (analysis, plan string) {
	// Look for ## Analysis and ## Implementation Plan headers
	analysisIdx := strings.Index(response, "## Analysis")
	planIdx := strings.Index(response, "## Implementation Plan")

	switch {
	case analysisIdx >= 0 && planIdx > analysisIdx:
		// Extract analysis section (between ## Analysis and ## Implementation Plan)
		analysisStart := analysisIdx + len("## Analysis")
		analysis = strings.TrimSpace(response[analysisStart:planIdx])

		// Extract plan section (after ## Implementation Plan)
		planStart := planIdx + len("## Implementation Plan")
		plan = strings.TrimSpace(response[planStart:])
	case analysisIdx >= 0:
		// Only analysis header found, treat rest as plan
		analysisStart := analysisIdx + len("## Analysis")
		analysis = strings.TrimSpace(response[analysisStart:])
		plan = analysis
	case planIdx >= 0:
		// Only plan header found, treat everything before as analysis
		analysis = strings.TrimSpace(response[:planIdx])
		planStart := planIdx + len("## Implementation Plan")
		plan = strings.TrimSpace(response[planStart:])
	default:
		// No headers found, use heuristics to split
		// Try to find a natural break point (e.g., "Implementation Plan" without ##)
		lowerResponse := strings.ToLower(response)
		implIdx := strings.Index(lowerResponse, "implementation plan")
		if implIdx > 0 {
			analysis = strings.TrimSpace(response[:implIdx])
			plan = strings.TrimSpace(response[implIdx:])
		} else {
			// Can't split, use full response for both
			analysis = response
			plan = response
		}
	}

	return analysis, plan
}

func (w *Worker) implement(ctx context.Context, task *Task, planStr string) error {
	w.oc.SetDirectory(task.Worktree)

	// Fetch all comments from GitHub issue to find technical planning
	comments, err := w.gh.ListComments(task.Issue.Number)
	if err == nil && len(comments) > 0 {
		// Look for technical planning comment
		for _, comment := range comments {
			if strings.Contains(comment.Body, "## 📋 Technical Planning") {
				planStr = comment.Body
				log.Printf("[Worker %d] Using technical planning from GitHub comment", w.id)
				break
			}
		}
	}

	if planStr == "" {
		log.Printf("[Worker %d] No technical planning comment found, using context-based plan", w.id)
	}

	testCmd := w.cfg.Load().Tools.TestCmd
	if testCmd == "" {
		testCmd = "go test ./..."
	}
	prompt := fmt.Sprintf(prompts.MustGet(prompts.MVPImplementation), task.Issue.Number, task.Issue.Title, planStr, task.Worktree, testCmd)

	// Use router to select model for code category with complexity detection
	llmModel := w.cfg.Load().LLM.Code.Model
	if w.router != nil {
		complexity := llm.DetectComplexity(planStr) //nolint:staticcheck // deprecated but still used
		llmModel = w.router.SelectModel(config.CategoryCode, complexity, nil)
	}

	_, err = w.llmStep(ctx, task, "implement", prompt, llmModel)
	if err != nil {
		return err
	}
	w.ensureCommit(task)
	return nil
}

func (w *Worker) ensureCommit(task *Task) {
	_, _ = git.RunInWorktree(task.Worktree, "git", "add", "-A")
	out, err := git.RunInWorktree(task.Worktree, "git", "diff", "--cached", "--quiet")
	if err != nil {
		msg := fmt.Sprintf("feat: implement #%d %s", task.Issue.Number, task.Issue.Title)
		_, _ = git.RunInWorktree(task.Worktree, "git", "commit", "-m", msg)
		log.Printf("[Worker %d] Auto-committed uncommitted changes", w.id)
	}
	_ = out
}

func (w *Worker) fixFromReview(ctx context.Context, task *Task, review string) error {
	w.oc.SetDirectory(task.Worktree)

	testCmd := w.cfg.Load().Tools.TestCmd
	if testCmd == "" {
		testCmd = "go test ./..."
	}
	prompt := fmt.Sprintf(prompts.MustGet(prompts.MVPFixFromReview), task.Issue.Number, task.Issue.Title, task.Worktree, testCmd, review)

	// Use router to select model for code category
	llmModel := w.cfg.Load().LLM.Code.Model
	if w.router != nil {
		llmModel = w.router.SelectModel(config.CategoryCode, config.ComplexityMedium, nil)
	}

	_, err := w.llmStep(ctx, task, "fix-from-review", prompt, llmModel)
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
	prompt := fmt.Sprintf(prompts.MustGet(prompts.MVPCodeReview), task.Issue.Number, task.Issue.Title, prURL, w.gh.Repo)

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

	// Capture the prompt in chat history
	task.AddChatMessage("user", prompt)

	// Use router to select model for code category with high complexity (code review is important)
	llmModel := w.cfg.Load().LLM.Code.Model
	if w.router != nil {
		hints := map[string]any{"stage": "code-review"}
		llmModel = w.router.SelectModel(config.CategoryCode, config.ComplexityHigh, hints)
	}

	model := opencode.ParseModelRef(llmModel)
	var result crResult
	if err := w.oc.SendMessageStructured(ctx, session.ID, prompt, model, crSchema, &result); err != nil {
		if w.store != nil && stepID > 0 {
			_ = w.store.FailStep(stepID, err.Error())
		}
		return false, "", fmt.Errorf("code review: %w", err)
	}

	reviewJSON, _ := json.Marshal(result)
	review = string(reviewJSON)

	// Capture the response in chat history
	task.AddChatMessage("assistant", review)

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

func (w *Worker) createPR(_ context.Context, task *Task) (string, error) {
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

func (w *Worker) llmStep(_ context.Context, task *Task, stepName, prompt, llm string) (string, error) {
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

	// Capture the prompt in chat history
	task.AddChatMessage("user", prompt)

	model := opencode.ParseModelRef(llm)
	msg, err := w.oc.SendMessage(session.ID, prompt, model, nil)
	if err != nil {
		if w.store != nil && stepID > 0 {
			_ = w.store.FailStep(stepID, err.Error())
		}
		return "", fmt.Errorf("sending message: %w", err)
	}

	response := extractText(msg)

	// Capture the response in chat history
	task.AddChatMessage("assistant", response)

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
	for line := range strings.SplitSeq(response, "\n") {
		line = strings.TrimSpace(line)
		if after, ok := strings.CutPrefix(line, "ALREADY_DONE:"); ok {
			return strings.TrimSpace(after)
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

// UpdateConfig updates the worker's configuration atomically.
// This method is called by the ConfigPropagator when config changes.
func (w *Worker) UpdateConfig(cfg *config.Config) {
	w.cfg.Store(cfg)
	log.Printf("[Worker %d] Configuration updated (YoloMode=%v)", w.id, cfg.YoloMode)
}

// Compile-time interface check: ensure Worker implements ConfigAwareWorker.
var _ config.ConfigAwareWorker = (*Worker)(nil)
