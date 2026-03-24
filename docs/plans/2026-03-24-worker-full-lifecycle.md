# Worker Full Lifecycle Implementation Plan

> **For Claude:** REQUIRED SUB-SKILL: Use superpowers:executing-plans to implement this plan task-by-task.

**Goal:** Make the worker own the full ticket lifecycle (analysis → coding → review → PR → await approval → merge/decline → done), so the orchestrator never picks a new ticket until the current one reaches a terminal state (done/failed/blocked).

**Architecture:** Worker.Process() becomes a blocking call that spans the entire ticket lifecycle including user approval and merge. A `decisionCh` channel lets the dashboard send approve/decline decisions to the worker via the orchestrator. The orchestrator loop simplifies to: pick ticket → Process() → handle result → repeat. No more label-based filtering for "blocking" issues — the orchestrator is simply busy while Process() runs.

**Tech Stack:** Go channels, existing orchestrator/worker/dashboard architecture.

---

### Task 1: Add UserDecision type and decisionCh to Worker

**Files:**
- Create: `internal/mvp/decision.go`
- Modify: `internal/mvp/worker.go:107-131`

**Step 1: Create decision.go with UserDecision type**

```go
// internal/mvp/decision.go
package mvp

// UserDecision represents a user's approve/decline decision from the dashboard.
type UserDecision struct {
	Action string // "approve" or "decline"
	Reason string // decline reason (empty for approve)
}
```

**Step 2: Add decisionCh field to Worker struct**

In `internal/mvp/worker.go`, modify the Worker struct (line 107-117):

```go
type Worker struct {
	id           int
	cfg          *config.Config
	oc           *opencode.Client
	gh           *github.Client
	brMgr        *git.BranchManager
	store        *db.Store
	repoDir      string
	orchestrator *Orchestrator
	router       *llm.Router
	decisionCh   chan UserDecision // receives approve/decline from dashboard
}
```

**Step 3: Initialize decisionCh in NewWorker**

In `internal/mvp/worker.go`, modify NewWorker (line 119-131):

```go
func NewWorker(id int, cfg *config.Config, oc *opencode.Client, gh *github.Client, brMgr *git.BranchManager, store *db.Store, orchestrator *Orchestrator, router *llm.Router) *Worker {
	return &Worker{
		id:           id,
		cfg:          cfg,
		oc:           oc,
		gh:           gh,
		brMgr:        brMgr,
		store:        store,
		repoDir:      brMgr.RepoDir(),
		orchestrator: orchestrator,
		router:       router,
		decisionCh:   make(chan UserDecision, 1),
	}
}
```

**Step 4: Run tests**

Run: `go build ./...`
Expected: PASS (no behavior change yet)

**Step 5: Commit**

```bash
git add internal/mvp/decision.go internal/mvp/worker.go
git commit -m "feat: add UserDecision type and decisionCh channel to Worker"
```

---

### Task 2: Add new worker reasons and merge stage event to state machine

**Files:**
- Modify: `internal/github/reasons.go`
- Modify: `internal/mvp/worker_events.go`
- Modify: `internal/mvp/orchestrator.go` (decideNextStage)

**Step 1: Add new reasons to reasons.go**

In `internal/github/reasons.go`, add after `ReasonWorkerCompletedCreatePR`:

```go
ReasonWorkerCompletedMerge      StageChangeReason = "worker_completed_merge"
ReasonWorkerDeclined            StageChangeReason = "worker_declined"
```

Add corresponding String() cases:

```go
case ReasonWorkerCompletedMerge:
	return "Worker completed merge, ticket done"
case ReasonWorkerDeclined:
	return "User declined PR via worker, sent back for fixes"
```

**Step 2: Add "merge" case to decideNextStage in orchestrator.go**

In `internal/mvp/orchestrator.go`, in `decideNextStage` (around line 520), add a new case inside the switch:

```go
case "merge":
	if event.Status == EventSuccess {
		return github.StageDone, github.ReasonWorkerCompletedMerge, true
	}
```

**Step 3: Add "awaiting-approval" event handling**

The worker will report "awaiting-approval" stage completion when it reaches the wait point. Add to decideNextStage:

```go
case "awaiting-approval":
	if event.Status == EventSuccess {
		return github.StageMerge, github.ReasonManualMerge, true
	}
```

**Step 4: Run tests**

Run: `go build ./... && go test ./internal/mvp/... ./internal/github/...`
Expected: PASS

**Step 5: Commit**

```bash
git add internal/github/reasons.go internal/mvp/orchestrator.go internal/mvp/worker_events.go
git commit -m "feat: add merge/decline reasons and state machine transitions"
```

---

### Task 3: Add StatusAwaitingApproval and StatusMerging to task.go

**Files:**
- Modify: `internal/mvp/task.go:12-21`

**Step 1: Add new statuses**

```go
const (
	StatusPending          TaskStatus = "pending"
	StatusAnalyzing        TaskStatus = "analyzing"
	StatusPlanning         TaskStatus = "planning"
	StatusCoding           TaskStatus = "coding"
	StatusReviewing        TaskStatus = "reviewing"
	StatusCreatingPR       TaskStatus = "creating_pr"
	StatusAwaitingApproval TaskStatus = "awaiting_approval"
	StatusMerging          TaskStatus = "merging"
	StatusDone             TaskStatus = "done"
	StatusFailed           TaskStatus = "failed"
)
```

**Step 2: Update task_test.go to include new statuses**

In `internal/mvp/task_test.go`, add the new statuses to the list (around line 11-17):

```go
StatusPending,
StatusAnalyzing,
StatusPlanning,
StatusCoding,
StatusReviewing,
StatusCreatingPR,
StatusAwaitingApproval,
StatusMerging,
StatusDone,
StatusFailed,
```

**Step 3: Run tests**

Run: `go test ./internal/mvp/...`
Expected: PASS

**Step 4: Commit**

```bash
git add internal/mvp/task.go internal/mvp/task_test.go
git commit -m "feat: add StatusAwaitingApproval and StatusMerging to task statuses"
```

---

### Task 4: Extend Worker.Process() with approval wait loop and merge step

This is the core change. Worker.Process() currently ends after create-pr. We extend it to:
1. After create-pr → report awaiting-approval → wait on decisionCh
2. On approve → merge PR → report done → return nil
3. On decline → fix from review → loop back to code-review
4. On merge failure → return error (orchestrator sets failed)

**Files:**
- Modify: `internal/mvp/worker.go:162-330`

**Step 1: Update stepOrder to include new steps**

```go
var stepOrder = []string{"technical-planning", "implement", "code-review", "create-pr", "awaiting-approval", "merge"}
```

**Step 2: Rewrite the end of Process() — after create-pr, add approval loop**

Replace lines 322-329 (the current ending of Process after create-pr) with the approval/merge loop. The full new Process() ending (after the create-pr block) should be:

```go
	// === APPROVAL + MERGE LOOP ===
	// Worker stays alive until ticket reaches terminal state.
	// User approve → merge → done. User decline → fix → re-review → re-PR → wait again.
	for {
		if resumeFrom <= 4 {
			task.Status = StatusAwaitingApproval
			log.Printf("[Worker %d] [5/6] Awaiting user approval for #%d (PR: %s)", w.id, task.Issue.Number, prURL)
			w.reportStageComplete("create-pr", EventSuccess, "PR created, awaiting approval: "+prURL)

			// Block until user sends decision or context cancelled
			var decision UserDecision
			select {
			case decision = <-w.decisionCh:
				log.Printf("[Worker %d] Received decision for #%d: %s", w.id, task.Issue.Number, decision.Action)
			case <-ctx.Done():
				return ctx.Err()
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

					comment := fmt.Sprintf("Merge failed (likely conflict). PR closed, task moved to Failed.\n\nError: %s", err.Error())
					if cmtErr := w.gh.AddComment(task.Issue.Number, comment); cmtErr != nil {
						log.Printf("[Worker %d] Error adding comment to #%d: %v", w.id, task.Issue.Number, cmtErr)
					}

					return task.Result.Error
				}

				w.reportStageComplete("merge", EventSuccess, "PR merged successfully")

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

			if decision.Reason != "" {
				comment := fmt.Sprintf("**Declined** — sent back for fixes.\n\n%s", decision.Reason)
				if cmtErr := w.gh.AddComment(task.Issue.Number, comment); cmtErr != nil {
					log.Printf("[Worker %d] Error adding decline comment to #%d: %v", w.id, task.Issue.Number, cmtErr)
				}
			}

			// Fix from decline feedback
			w.reportStageComplete("awaiting-approval", EventFailed, "user declined: "+decision.Reason)

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

			// PR already exists (we pushed to same branch), loop back to await approval
			resumeFrom = 0 // reset so we enter the awaiting block again
			continue
		}

		break
	}

	// Should not reach here in normal flow, but handle gracefully
	task.Status = StatusDone
	task.Result = &TaskResult{
		PRURL:   prURL,
		Summary: fmt.Sprintf("Completed #%d: %s", task.Issue.Number, task.Issue.Title),
	}
	return nil
```

**Step 3: Remove the old ending of Process()**

Delete lines 323-329 (the old `task.Status = StatusDone` block at the end of Process).

**Step 4: Remove the deferred branch cleanup from worker**

The branch cleanup in the worker defer (lines 193-201) should be removed. Branch cleanup after merge is handled by `gh pr merge --delete-branch`. For failed cases, the orchestrator already has branch cleanup (lines 263-269 in orchestrator.go).

Actually, keep the defer but only clean up if NOT merged (merge deletes the branch via `--delete-branch`):

```go
defer func() {
	if task.Branch != "" && task.Status != StatusDone {
		log.Printf("[Worker %d] Cleaning up branch %q (task not done)", w.id, task.Branch)
		if err := w.brMgr.RemoveBranch(task.Branch); err != nil {
			log.Printf("[Worker %d] Warning: failed to remove branch %q: %v", w.id, task.Branch, err)
		}
	}
}()
```

**Step 5: Run tests**

Run: `go build ./...`
Expected: PASS (compilation check)

**Step 6: Commit**

```bash
git add internal/mvp/worker.go
git commit -m "feat: extend Worker.Process() with approval wait loop and merge step"
```

---

### Task 5: Add SendDecision to Orchestrator

**Files:**
- Modify: `internal/mvp/orchestrator.go`

**Step 1: Add SendDecision method**

Add after the `ChangeStage` method:

```go
// SendDecision sends a user decision (approve/decline) to the worker processing the given issue.
// Returns an error if no worker is currently processing that issue.
func (o *Orchestrator) SendDecision(issueNumber int, decision UserDecision) error {
	o.mu.Lock()
	task := o.currentTask
	o.mu.Unlock()

	if task == nil || task.Issue.Number != issueNumber {
		return fmt.Errorf("worker is not processing issue #%d", issueNumber)
	}

	select {
	case o.worker.decisionCh <- decision:
		log.Printf("[Orchestrator] Sent %s decision to worker for #%d", decision.Action, issueNumber)
		return nil
	default:
		return fmt.Errorf("decision channel full for issue #%d (worker may not be waiting)", issueNumber)
	}
}
```

**Step 2: Run tests**

Run: `go build ./...`
Expected: PASS

**Step 3: Commit**

```bash
git add internal/mvp/orchestrator.go
git commit -m "feat: add SendDecision method to Orchestrator"
```

---

### Task 6: Simplify Orchestrator.Run() — remove label-based filtering

The orchestrator no longer needs to classify issues as "in-progress" vs "blocking" by labels. While Process() is running, the orchestrator is busy. The only filtering needed is: find issues without any `stage:*` label (= backlog candidates).

**Files:**
- Modify: `internal/mvp/orchestrator.go:121-316`

**Step 1: Simplify the issue filtering loop**

Replace lines 175-208 (the filtering loop) with:

```go
		var candidates []github.Issue
		var resumeIssue *github.Issue
		for i := range issues {
			if !strings.EqualFold(issues[i].State, "open") {
				continue
			}

			stage := getStageLabel(issues[i])
			if stage == "" {
				// No stage label = backlog candidate
				candidates = append(candidates, issues[i])
			} else if isWorkerStage(stage) && resumeIssue == nil {
				// Worker stage (analysis/coding/review/create-pr) = resume after restart
				resumeIssue = &issues[i]
			}
			// Everything else (awaiting-approval, merging, failed, blocked, done, needs-user)
			// is ignored — orchestrator doesn't pick these up.
		}
		log.Printf("[Orchestrator] Found %d candidates, resume=%v", len(candidates), resumeIssue != nil)
```

Add helper functions:

```go
// getStageLabel returns the stage:* label from an issue, or "" if none.
func getStageLabel(issue github.Issue) string {
	for _, l := range issue.Labels {
		if strings.HasPrefix(l.Name, "stage:") {
			return l.Name
		}
	}
	return ""
}

// isWorkerStage returns true if the stage is one the worker actively processes.
// These stages indicate the worker was interrupted (e.g. ODA restart) and should resume.
func isWorkerStage(stage string) bool {
	switch stage {
	case "stage:analysis", "stage:coding", "stage:code-review", "stage:create-pr":
		return true
	default:
		return false
	}
}
```

**Step 2: Simplify the nextIssue selection**

Replace lines 210-226 with:

```go
		var nextIssue *github.Issue
		if resumeIssue != nil {
			nextIssue = resumeIssue
			log.Printf("[Orchestrator] Resuming in-progress #%d: %s", nextIssue.Number, nextIssue.Title)
		} else if len(candidates) > 0 {
			picked, err := o.pickNextTicket(ctx, candidates, nil)
			if err != nil {
				log.Printf("[Orchestrator] Error picking next ticket: %v — falling back to first candidate", err)
				picked = &candidates[0]
			}
			nextIssue = picked
		}
```

**Step 3: Simplify post-Process() handling**

Replace lines 276-312 (the big if/else after Process returns) with:

```go
		if processErr != nil && errors.Is(processErr, ErrAlreadyDone) {
			log.Printf("[Orchestrator] ✓ Already done #%d: %v", nextIssue.Number, processErr)
			o.recordStep(nextIssue.Number, "already-done", processErr.Error())
			comment := fmt.Sprintf("Ticket already done — closing automatically.\n\n%s", processErr.Error())
			if err := o.gh.AddComment(nextIssue.Number, comment); err != nil {
				log.Printf("[Orchestrator] Error adding comment: %v", err)
			}
			if err := o.ChangeStage(nextIssue.Number, github.StageDone, github.ReasonWorkerAlreadyDone); err != nil {
				log.Printf("[Orchestrator] Error setting stage:done for #%d: %v", nextIssue.Number, err)
			}
		} else if processErr != nil {
			log.Printf("[Orchestrator] ✗ Failed #%d: %v", nextIssue.Number, processErr)
			o.recordStep(nextIssue.Number, "failed", processErr.Error())
			if err := o.ChangeStage(nextIssue.Number, github.StageFailed, github.ReasonWorkerFailed); err != nil {
				log.Printf("[Orchestrator] Error setting stage:failed for #%d: %v", nextIssue.Number, err)
			}
		} else {
			log.Printf("[Orchestrator] ✓ Completed #%d (merged)", nextIssue.Number)
			o.recordStep(nextIssue.Number, "done", "Ticket completed and merged")
		}
```

Note: The `else` branch no longer sets `stage:awaiting-approval` — because Process() now returns nil only after successful merge (which already set stage:done via reportStageComplete).

**Step 4: Remove drainWorkerEvents() call and the duplicate branch cleanup**

Remove lines 258-269 (drainWorkerEvents + branch cleanup). The worker now handles its own lifecycle including branch cleanup.

**Step 5: Remove the `blockedOnBoard` map and related code**

The `blockedOnBoard` map (line 177) and its usage are no longer needed.

**Step 6: Run tests**

Run: `go build ./... && go test ./internal/mvp/...`
Expected: PASS

**Step 7: Commit**

```bash
git add internal/mvp/orchestrator.go
git commit -m "refactor: simplify orchestrator loop — worker owns full lifecycle"
```

---

### Task 7: Rewire dashboard handlers to use SendDecision

**Files:**
- Modify: `internal/dashboard/handlers.go` (handleApproveMerge, handleDecline)

**Step 1: Rewrite handleApproveMerge**

Replace the entire `handleApproveMerge` function (lines 466-537) with:

```go
func (s *Server) handleApproveMerge(w http.ResponseWriter, r *http.Request) {
	id := r.PathValue("id")
	issueNum := 0
	if _, err := fmt.Sscanf(id, "%d", &issueNum); err != nil {
		log.Printf("[Dashboard] Error parsing issue ID %q: %v", id, err)
	}
	if issueNum == 0 {
		http.Redirect(w, r, "/", http.StatusSeeOther)
		return
	}

	s.recordStep(issueNum, "approved", "Manual approval granted")

	err := s.orchestrator.SendDecision(issueNum, mvp.UserDecision{Action: "approve"})
	if err != nil {
		log.Printf("[Dashboard] Error sending approve decision for #%d: %v", issueNum, err)
		// Fallback: if worker is not processing (e.g. after restart), do direct merge
		s.handleDirectMerge(w, r, issueNum)
		return
	}

	log.Printf("[Dashboard] ✓ Sent approve decision for #%d", issueNum)
	http.Redirect(w, r, "/", http.StatusSeeOther)
}
```

**Step 2: Add handleDirectMerge as fallback**

This handles the edge case where ODA was restarted while in awaiting-approval and the worker is not running:

```go
// handleDirectMerge is a fallback for when the worker is not processing the issue
// (e.g. after ODA restart while in awaiting-approval state).
func (s *Server) handleDirectMerge(w http.ResponseWriter, r *http.Request, issueNum int) {
	if s.gh == nil {
		http.Redirect(w, r, "/", http.StatusSeeOther)
		return
	}

	branch, err := s.gh.FindPRBranch(issueNum)
	if err != nil {
		log.Printf("[Dashboard] Error finding PR for #%d: %v", issueNum, err)
		http.Redirect(w, r, "/", http.StatusSeeOther)
		return
	}

	mergeStageErr := s.orchestrator.ChangeStage(issueNum, github.StageMerge, github.ReasonManualMerge)
	if mergeStageErr != nil {
		log.Printf("[Dashboard] Error setting Merge stage on #%d: %v", issueNum, mergeStageErr)
	}

	if err := s.gh.MergePR(branch); err != nil {
		log.Printf("[Dashboard] ✗ Direct merge failed for #%d: %v", issueNum, err)
		if closeErr := s.gh.ClosePR(branch); closeErr != nil {
			log.Printf("[Dashboard] Error closing PR for #%d: %v", issueNum, closeErr)
		}
		_ = s.orchestrator.ChangeStage(issueNum, github.StageFailed, github.ReasonManualMergeFailed)
		comment := fmt.Sprintf("Merge failed (likely conflict). PR closed, task moved to Failed.\n\nError: %s", err.Error())
		_ = s.gh.AddComment(issueNum, comment)
		http.Redirect(w, r, "/", http.StatusSeeOther)
		return
	}

	_ = s.orchestrator.ChangeStage(issueNum, github.StageDone, github.ReasonManualMerge)
	log.Printf("[Dashboard] ✓ Direct merged #%d (fallback)", issueNum)
	http.Redirect(w, r, "/", http.StatusSeeOther)
}
```

**Step 3: Rewrite handleDecline**

Replace the `handleDecline` function (lines 426-464) with:

```go
func (s *Server) handleDecline(w http.ResponseWriter, r *http.Request) {
	id := r.PathValue("id")
	issueNum := 0
	if _, err := fmt.Sscanf(id, "%d", &issueNum); err != nil {
		log.Printf("[Dashboard] Error parsing issue ID %q: %v", id, err)
	}
	if issueNum == 0 {
		http.Redirect(w, r, "/", http.StatusSeeOther)
		return
	}

	reason := r.FormValue("reason")
	s.recordStep(issueNum, "declined", reason)

	err := s.orchestrator.SendDecision(issueNum, mvp.UserDecision{Action: "decline", Reason: reason})
	if err != nil {
		log.Printf("[Dashboard] Error sending decline decision for #%d: %v — falling back to direct stage change", issueNum, err)
		// Fallback: direct stage change if worker not processing
		_ = s.orchestrator.ChangeStage(issueNum, github.StageCode, github.ReasonManualDecline)
		if reason != "" {
			comment := fmt.Sprintf("**Declined** — sent back for fixes.\n\n%s", reason)
			_ = s.gh.AddComment(issueNum, comment)
		}
		if s.store != nil {
			_ = s.store.DeleteSteps(issueNum)
		}
	} else {
		log.Printf("[Dashboard] ✓ Sent decline decision for #%d", issueNum)
	}

	http.Redirect(w, r, "/", http.StatusSeeOther)
}
```

**Step 4: Add mvp import to handlers.go**

Add `"github.com/crazy-goat/one-dev-army/internal/mvp"` to the imports in handlers.go.

**Step 5: Run tests**

Run: `go build ./... && go test ./internal/dashboard/...`
Expected: PASS

**Step 6: Commit**

```bash
git add internal/dashboard/handlers.go
git commit -m "refactor: dashboard sends decisions via orchestrator instead of direct merge"
```

---

### Task 8: Handle awaiting-approval resume after ODA restart

When ODA restarts while a ticket is in `stage:awaiting-approval`, the worker needs to resume from that point (skip analysis/coding/review/create-pr, go straight to waiting on decisionCh).

**Files:**
- Modify: `internal/mvp/worker.go` (Process function, stepOrder)
- Modify: `internal/mvp/orchestrator.go` (isWorkerStage)

**Step 1: Add awaiting-approval to isWorkerStage**

In `orchestrator.go`, update `isWorkerStage`:

```go
func isWorkerStage(stage string) bool {
	switch stage {
	case "stage:analysis", "stage:coding", "stage:code-review", "stage:create-pr", "stage:awaiting-approval":
		return true
	default:
		return false
	}
}
```

**Step 2: Handle resumeFrom for awaiting-approval in Process()**

The stepOrder already includes "awaiting-approval" (from Task 4). When `resumeFrom` is 4 (awaiting-approval), the worker skips all previous steps and goes straight to the approval wait loop. The existing `if resumeFrom <= N` guards handle this — steps 0-3 are skipped, and the approval loop at step 4 runs.

We need to ensure `prURL` is recovered from the store when resuming:

After the create-pr skip block (around the existing `else` at line 316-321), add:

```go
	// If resuming from awaiting-approval, recover prURL
	if resumeFrom >= 4 && prURL == "" && w.store != nil {
		prURL, _ = w.store.GetStepResponse(task.Issue.Number, "create-pr")
	}
```

**Step 3: Run tests**

Run: `go build ./... && go test ./internal/mvp/...`
Expected: PASS

**Step 4: Commit**

```bash
git add internal/mvp/worker.go internal/mvp/orchestrator.go
git commit -m "feat: handle awaiting-approval resume after ODA restart"
```

---

### Task 9: Remove obsolete code

**Files:**
- Modify: `internal/mvp/orchestrator.go` — remove `drainWorkerEvents`, `BroadcastStageUpdate` (if unused), `workerEventCh` handling in Run loop
- Modify: `internal/dashboard/handlers.go` — remove old merge-related imports if unused

**Step 1: Check if BroadcastStageUpdate is used anywhere**

Search for `BroadcastStageUpdate` usage. If only used by the old async pattern, remove it.

**Step 2: Remove drainWorkerEvents if no longer called**

The function `drainWorkerEvents` (lines 452-464) is no longer called after Task 6 changes. Remove it.

**Step 3: Simplify workerEventCh handling**

The `workerEventCh` in the Run loop (lines 136-144) may still be needed if `reportStageComplete` sends events through the channel. Check: `reportStageComplete` calls `HandleWorkerEvent` directly (synchronously), not via channel. So the channel handling in Run loop can be removed.

Actually — check if anything else sends to `workerEventCh`. If nothing does, remove the channel and the select case.

**Step 4: Run tests**

Run: `go build ./... && go test ./...`
Expected: PASS

**Step 5: Commit**

```bash
git add internal/mvp/orchestrator.go internal/dashboard/handlers.go
git commit -m "refactor: remove obsolete drainWorkerEvents and unused code"
```

---

### Task 10: Update state-machine.md documentation

**Files:**
- Modify: `docs/state-machine.md`

**Step 1: Update the Approve section**

The Approve state now has transitions handled by the worker, not the dashboard:

```markdown
### Approve

**Entry Conditions:**
- Label: `stage:awaiting-approval`
- Worker is blocked waiting on `decisionCh`

**Exit Transitions:**

| To | Trigger | Actions |
|----|---------|---------|
| **Merge** | User clicks "Approve & Merge" on dashboard | Dashboard sends approve decision → Worker merges PR |
| **Code** | User clicks "Decline" on dashboard | Dashboard sends decline decision → Worker fixes and retries |
```

**Step 2: Update the Merge section**

```markdown
### Merge

**Entry Conditions:**
- Label: `stage:merging`
- Worker is actively merging the PR

**Exit Transitions:**

| To | Trigger | Actions |
|----|---------|---------|
| **Done** | Merge successful | Worker reports merge complete, `SetStageLabel("Done")` → closes issue |
| **Failed** | Merge conflict | Worker reports failure, orchestrator sets `stage:failed` |
```

**Step 3: Add note about worker lifecycle**

Add to Implementation Notes:

```markdown
### Worker Lifecycle

The worker owns the full ticket lifecycle from pickup to terminal state:
- `Process()` is a blocking call that spans analysis → coding → review → PR → approval → merge
- The orchestrator cannot pick a new ticket while `Process()` is running
- User decisions (approve/decline) are sent to the worker via a channel
- `Process()` returns only when the ticket reaches done, failed, or context cancelled
```

**Step 4: Commit**

```bash
git add docs/state-machine.md
git commit -m "docs: update state machine to reflect worker full lifecycle"
```

---

### Task 11: Write tests for the new decision flow

**Files:**
- Modify: `internal/mvp/orchestrator_test.go`
- Modify: `internal/mvp/integration_test.go`

**Step 1: Test SendDecision when worker is processing**

```go
func TestSendDecision_WorkerProcessing(t *testing.T) {
	o := &Orchestrator{
		worker: &Worker{decisionCh: make(chan UserDecision, 1)},
	}
	o.currentTask = &Task{
		Issue: github.Issue{Number: 42},
	}

	err := o.SendDecision(42, UserDecision{Action: "approve"})
	if err != nil {
		t.Fatalf("SendDecision() error = %v", err)
	}

	select {
	case d := <-o.worker.decisionCh:
		if d.Action != "approve" {
			t.Errorf("decision.Action = %q, want %q", d.Action, "approve")
		}
	default:
		t.Fatal("expected decision on channel")
	}
}
```

**Step 2: Test SendDecision when worker is NOT processing**

```go
func TestSendDecision_NoWorker(t *testing.T) {
	o := &Orchestrator{
		worker: &Worker{decisionCh: make(chan UserDecision, 1)},
	}

	err := o.SendDecision(42, UserDecision{Action: "approve"})
	if err == nil {
		t.Fatal("expected error when no worker processing")
	}
}
```

**Step 3: Test SendDecision wrong issue**

```go
func TestSendDecision_WrongIssue(t *testing.T) {
	o := &Orchestrator{
		worker: &Worker{decisionCh: make(chan UserDecision, 1)},
	}
	o.currentTask = &Task{
		Issue: github.Issue{Number: 42},
	}

	err := o.SendDecision(99, UserDecision{Action: "approve"})
	if err == nil {
		t.Fatal("expected error for wrong issue number")
	}
}
```

**Step 4: Test getStageLabel and isWorkerStage helpers**

```go
func TestGetStageLabel(t *testing.T) {
	tests := []struct {
		labels []string
		want   string
	}{
		{nil, ""},
		{[]string{"bug", "priority:high"}, ""},
		{[]string{"stage:coding"}, "stage:coding"},
		{[]string{"bug", "stage:merging"}, "stage:merging"},
	}
	for _, tt := range tests {
		issue := github.Issue{}
		for _, l := range tt.labels {
			issue.Labels = append(issue.Labels, struct {
				Name string `json:"name"`
			}{Name: l})
		}
		got := getStageLabel(issue)
		if got != tt.want {
			t.Errorf("getStageLabel(%v) = %q, want %q", tt.labels, got, tt.want)
		}
	}
}

func TestIsWorkerStage(t *testing.T) {
	workerStages := []string{"stage:analysis", "stage:coding", "stage:code-review", "stage:create-pr", "stage:awaiting-approval"}
	for _, s := range workerStages {
		if !isWorkerStage(s) {
			t.Errorf("isWorkerStage(%q) = false, want true", s)
		}
	}

	nonWorkerStages := []string{"stage:merging", "stage:done", "stage:failed", "stage:blocked", "stage:needs-user", "stage:backlog", ""}
	for _, s := range nonWorkerStages {
		if isWorkerStage(s) {
			t.Errorf("isWorkerStage(%q) = true, want false", s)
		}
	}
}
```

**Step 5: Run all tests**

Run: `go test ./...`
Expected: PASS

**Step 6: Commit**

```bash
git add internal/mvp/orchestrator_test.go internal/mvp/integration_test.go
git commit -m "test: add tests for SendDecision, getStageLabel, isWorkerStage"
```

---

### Task 12: Final verification

**Step 1: Run full test suite**

Run: `go test ./... -count=1`
Expected: All PASS

**Step 2: Run linter**

Run: `go vet ./...`
Expected: No issues

**Step 3: Build**

Run: `go build -o /dev/null ./...`
Expected: PASS

**Step 4: Verify no compilation errors**

Run: `go build ./...`
Expected: PASS
