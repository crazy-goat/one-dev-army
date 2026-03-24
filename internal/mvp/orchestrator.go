package mvp

import (
	"context"
	"errors"
	"fmt"
	"log"
	"regexp"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/crazy-goat/one-dev-army/internal/config"
	"github.com/crazy-goat/one-dev-army/internal/db"
	"github.com/crazy-goat/one-dev-army/internal/git"
	"github.com/crazy-goat/one-dev-army/internal/github"
	"github.com/crazy-goat/one-dev-army/internal/llm"
	"github.com/crazy-goat/one-dev-army/internal/opencode"
)

const pickTicketPrompt = `You are a sprint planner for repository %s.

Here are the open tickets in the current sprint milestone. Each ticket has a number and title.
Use the gh CLI tool to read ticket details: gh issue view <number> -R %s

Tickets:
%s

Your task:
1. Read each ticket using gh issue view to understand what it does
2. Analyze dependencies between tickets — which tickets must be done before others
3. Pick the ONE ticket that should be done NEXT based on these criteria:
   - First priority: the ticket that has the MOST other tickets depending on it (i.e. it unblocks the most work)
   - Second priority: highest priority label (priority:high > priority:medium > priority:low > no label)
   - Do NOT pick tickets labeled "epic" — those are tracking issues, not implementation tasks

Respond with ONLY this format on the last line:
NEXT: #<number>`

type StageBroadcaster interface {
	BroadcastIssueUpdate(issue github.Issue)
	BroadcastWorkerUpdate(workerID, status string, taskID int, taskTitle, stage string, elapsedSeconds int)
}

type Orchestrator struct {
	cfg           *config.Config
	worker        *Worker
	gh            *github.Client
	oc            *opencode.Client
	brMgr         *git.BranchManager
	store         *db.Store
	hub           StageBroadcaster
	router        *llm.Router
	running       bool
	paused        bool
	processing    bool
	currentTask   *Task
	mu            sync.Mutex
	workerEventCh chan WorkerEvent // Channel for worker events
}

func NewOrchestrator(cfg *config.Config, gh *github.Client, oc *opencode.Client, brMgr *git.BranchManager, store *db.Store, hub StageBroadcaster, router *llm.Router) *Orchestrator {
	o := &Orchestrator{
		cfg:           cfg,
		gh:            gh,
		oc:            oc,
		brMgr:         brMgr,
		store:         store,
		hub:           hub,
		router:        router,
		paused:        true,
		workerEventCh: make(chan WorkerEvent, 100), // Buffered channel for worker events
	}
	o.worker = NewWorker(1, cfg, oc, gh, brMgr, store, o, router)
	return o
}

func (o *Orchestrator) Start() {
	o.mu.Lock()
	defer o.mu.Unlock()
	o.paused = false
	log.Println("[Orchestrator] ▶ Started — polling for issues")
}

func (o *Orchestrator) Pause() {
	o.mu.Lock()
	defer o.mu.Unlock()
	o.paused = true
	log.Println("[Orchestrator] ⏸ Paused (will finish current ticket)")
}

func (o *Orchestrator) Stop() {
	o.mu.Lock()
	defer o.mu.Unlock()
	o.running = false
}

func (o *Orchestrator) OpenCodeURL() string {
	return o.oc.BaseURL()
}

func (o *Orchestrator) IsPaused() bool {
	o.mu.Lock()
	defer o.mu.Unlock()
	return o.paused
}

func (o *Orchestrator) IsProcessing() bool {
	o.mu.Lock()
	defer o.mu.Unlock()
	return o.processing
}

func (o *Orchestrator) CurrentTask() *Task {
	o.mu.Lock()
	defer o.mu.Unlock()
	return o.currentTask
}

func (o *Orchestrator) Run(ctx context.Context) error {
	o.mu.Lock()
	o.running = true
	o.mu.Unlock()

	for {
		o.mu.Lock()
		running := o.running
		paused := o.paused
		o.mu.Unlock()

		if !running {
			return nil
		}

		select {
		case <-ctx.Done():
			return ctx.Err()
		case event := <-o.workerEventCh:
			// Handle worker event
			o.HandleWorkerEvent(event)
			continue
		default:
		}

		if paused {
			select {
			case <-time.After(5 * time.Second):
				continue
			case <-ctx.Done():
				return ctx.Err()
			}
		}

		milestone, err := o.gh.GetOldestOpenMilestone()
		if err != nil {
			log.Printf("[Orchestrator] Error getting milestone: %v", err)
			o.sleep(ctx, 30*time.Second)
			continue
		}
		if milestone == nil {
			log.Println("[Orchestrator] No active milestone, waiting...")
			o.sleep(ctx, 30*time.Second)
			continue
		}

		log.Printf("[Orchestrator] Fetching issues for milestone %q...", milestone.Title)
		issues, err := o.store.GetOpenIssuesCacheByMilestone(milestone.Title)
		if err != nil {
			log.Printf("[Orchestrator] Error listing issues from cache: %v", err)
			o.sleep(ctx, 30*time.Second)
			continue
		}

		// Fetch project board columns to detect manually-blocked issues
		// TODO: Replace with label-based detection when implementing ticket #182
		blockedOnBoard := make(map[int]bool)

		var openCount, skippedCount int
		var inProgressIssue *github.Issue
		var candidates []github.Issue
		var blocking []github.Issue // issues with active branches that block new work
		for i := range issues {
			if !strings.EqualFold(issues[i].State, "open") {
				continue
			}
			openCount++
			if hasLabel(issues[i], "in-progress") || hasLabel(issues[i], "stage:coding") || hasLabel(issues[i], "stage:analysis") || hasLabel(issues[i], "stage:code-review") || hasLabel(issues[i], "stage:create-pr") {
				if inProgressIssue == nil {
					inProgressIssue = &issues[i]
				}
				continue
			}
			// Issues with active branches — block the entire queue
			if hasLabel(issues[i], "stage:awaiting-approval") || hasLabel(issues[i], "awaiting-approval") || hasLabel(issues[i], "stage:failed") || hasLabel(issues[i], "failed") {
				blocking = append(blocking, issues[i])
				log.Printf("[Orchestrator]   blocking #%d %q (%s)", issues[i].Number, issues[i].Title, labelNames(issues[i]))
				continue
			}
			// Issues waiting on external input — skip but don't block others
			if blockedOnBoard[issues[i].Number] {
				skippedCount++
				log.Printf("[Orchestrator]   skip #%d %q (blocked on board)", issues[i].Number, issues[i].Title)
				continue
			}
			candidates = append(candidates, issues[i])
		}
		log.Printf("[Orchestrator] Found %d issues (%d open, %d blocking, %d skipped, %d candidates)", len(issues), openCount, len(blocking), skippedCount, len(candidates))

		var nextIssue *github.Issue
		if inProgressIssue != nil {
			nextIssue = inProgressIssue
			log.Printf("[Orchestrator] Resuming in-progress #%d: %s", nextIssue.Number, nextIssue.Title)
		} else if len(blocking) > 0 {
			// Single-branch mode: don't start new work while any issue is unresolved.
			// Unmerged PRs, failed tasks, or blocked tasks leave branches that would
			// conflict with new work based on master.
			log.Printf("[Orchestrator] ⏳ Waiting — %d issue(s) need attention before starting new work", len(blocking))
		} else if len(candidates) > 0 {
			picked, err := o.pickNextTicket(ctx, candidates, nil)
			if err != nil {
				log.Printf("[Orchestrator] Error picking next ticket: %v — falling back to first candidate", err)
				picked = &candidates[0]
			}
			nextIssue = picked
		}

		if nextIssue == nil {
			log.Println("[Orchestrator] No available issues in sprint, waiting 30s...")
			o.sleep(ctx, 30*time.Second)
			continue
		}

		log.Printf("[Orchestrator] ▶ Picking up #%d: %s", nextIssue.Number, nextIssue.Title)

		if err := o.gh.RemoveLabel(nextIssue.Number, "merge-failed"); err != nil {
			log.Printf("[Orchestrator] Error removing merge-failed label: %v", err)
		}

		task := &Task{
			Issue:     *nextIssue,
			Milestone: milestone.Title,
			Status:    StatusPending,
		}

		o.mu.Lock()
		o.processing = true
		o.currentTask = task
		o.mu.Unlock()

		processErr := o.worker.Process(ctx, task)

		// Drain pending worker events before setting final stage.
		// Worker events carry intermediate stage transitions (analysis→coding→…)
		// that must be recorded in the ledger before the final post-process stage.
		o.drainWorkerEvents()

		// Explicit cleanup: ensure branch is deleted after worker finishes (success or failure)
		if task.Branch != "" {
			log.Printf("[Orchestrator] Cleaning up branch %q for issue #%d", task.Branch, task.Issue.Number)
			if err := o.brMgr.RemoveBranch(task.Branch); err != nil {
				log.Printf("[Orchestrator] Warning: failed to remove branch %q: %v", task.Branch, err)
			}
		}

		o.mu.Lock()
		o.processing = false
		o.currentTask = nil
		o.mu.Unlock()

		if processErr != nil && errors.Is(processErr, ErrAlreadyDone) {
			log.Printf("[Orchestrator] ✓ Already done #%d: %v", nextIssue.Number, processErr)
			o.recordStep(nextIssue.Number, "already-done", processErr.Error())
			comment := fmt.Sprintf("Ticket already done — closing automatically.\n\n%s", processErr.Error())
			if err := o.gh.AddComment(nextIssue.Number, comment); err != nil {
				log.Printf("[Orchestrator] Error adding comment: %v", err)
			}
			log.Printf("[Orchestrator] Setting stage:done for #%d (reason: %s)", nextIssue.Number, github.ReasonWorkerAlreadyDone)
			if err := o.ChangeStage(nextIssue.Number, github.StageDone, github.ReasonWorkerAlreadyDone); err != nil {
				log.Printf("[Orchestrator] Error setting stage:done for #%d: %v", nextIssue.Number, err)
			}
			o.recordStep(nextIssue.Number, "done", "Closed as already done")
		} else if processErr != nil {
			log.Printf("[Orchestrator] ✗ Failed #%d: %v", nextIssue.Number, processErr)
			o.recordStep(nextIssue.Number, "failed", processErr.Error())
			log.Printf("[Orchestrator] Setting stage:failed for #%d (reason: %s)", nextIssue.Number, github.ReasonWorkerFailed)
			if err := o.ChangeStage(nextIssue.Number, github.StageFailed, github.ReasonWorkerFailed); err != nil {
				log.Printf("[Orchestrator] Error setting stage:failed for #%d: %v", nextIssue.Number, err)
			}
		} else {
			prURL := ""
			if task.Result != nil {
				prURL = task.Result.PRURL
			}
			log.Printf("[Orchestrator] ✓ Completed #%d → awaiting approval: %s", nextIssue.Number, prURL)
			o.recordStep(nextIssue.Number, "waiting-for-approval", prURL)
			log.Printf("[Orchestrator] Setting stage:awaiting-approval for #%d (reason: %s)", nextIssue.Number, github.ReasonWorkerApprove)
			if err := o.ChangeStage(nextIssue.Number, github.StageApprove, github.ReasonWorkerApprove); err != nil {
				log.Printf("[Orchestrator] Error setting stage:awaiting-approval for #%d: %v", nextIssue.Number, err)
			}
			if prURL != "" {
				comment := fmt.Sprintf("AI review passed ✓ — awaiting manual approval.\n\nPR: %s", prURL)
				if err := o.gh.AddComment(nextIssue.Number, comment); err != nil {
					log.Printf("[Orchestrator] Error adding comment: %v", err)
				}
			}
		}

		o.sleep(ctx, 5*time.Second)
	}
}

var nextTicketRe = regexp.MustCompile(`NEXT:\s*#(\d+)`)

func (o *Orchestrator) pickNextTicket(ctx context.Context, candidates []github.Issue, awaitingApproval []github.Issue) (*github.Issue, error) {
	if len(candidates) == 1 && len(awaitingApproval) == 0 {
		return &candidates[0], nil
	}

	var lines []string
	for _, issue := range candidates {
		lines = append(lines, fmt.Sprintf("- #%d %s", issue.Number, issue.Title))
	}
	ticketList := strings.Join(lines, "\n")

	var pendingInfo string
	if len(awaitingApproval) > 0 {
		var pendingLines []string
		for _, issue := range awaitingApproval {
			pendingLines = append(pendingLines, fmt.Sprintf("- #%d %s", issue.Number, issue.Title))
		}
		pendingInfo = fmt.Sprintf("\n\nTickets awaiting approval (PR created, AI review passed, but NOT yet merged — treat as NOT DONE, do NOT pick tickets that depend on these):\n%s", strings.Join(pendingLines, "\n"))
	}

	prompt := fmt.Sprintf(pickTicketPrompt, o.gh.Repo, o.gh.Repo, ticketList) + pendingInfo

	log.Printf("[Orchestrator] Asking LLM to pick next ticket from %d candidates...", len(candidates))

	session, err := o.oc.CreateSession("pick-ticket")
	if err != nil {
		return nil, fmt.Errorf("creating session: %w", err)
	}
	defer func() {
		if delErr := o.oc.DeleteSession(session.ID); delErr != nil {
			log.Printf("[Orchestrator] failed to delete pick-ticket session: %v", delErr)
		}
	}()

	// Use router to select model for orchestration category
	llmModel := o.cfg.Planning.LLM
	if o.router != nil {
		llmModel = o.router.SelectModel(config.CategoryOrchestration, config.ComplexityMedium, nil)
	}

	model := opencode.ParseModelRef(llmModel)
	msg, err := o.oc.SendMessage(session.ID, prompt, model, nil)
	if err != nil {
		return nil, fmt.Errorf("sending pick-ticket message: %w", err)
	}

	response := extractText(msg)
	log.Printf("[Orchestrator] LLM pick-ticket response: %s", response)

	match := nextTicketRe.FindStringSubmatch(response)
	if match == nil {
		return nil, fmt.Errorf("LLM did not return NEXT: #N format")
	}

	num, err := strconv.Atoi(match[1])
	if err != nil {
		return nil, fmt.Errorf("parsing ticket number %q: %w", match[1], err)
	}

	for i := range candidates {
		if candidates[i].Number == num {
			log.Printf("[Orchestrator] LLM picked #%d: %s", candidates[i].Number, candidates[i].Title)
			return &candidates[i], nil
		}
	}

	return nil, fmt.Errorf("LLM picked #%d which is not in candidate list", num)
}

func (o *Orchestrator) recordStep(issueNumber int, stepName, response string) {
	if o.store == nil {
		return
	}
	id, err := o.store.InsertStep(issueNumber, stepName, stepName, "")
	if err != nil {
		log.Printf("[Orchestrator] failed to insert %s step for #%d: %v", stepName, issueNumber, err)
		return
	}
	_ = o.store.FinishStep(id, response)
}

func (o *Orchestrator) BroadcastStageUpdate(issueNumber int, stage github.Stage) {
	// Run stage change asynchronously to avoid blocking worker.
	// Delegates to ChangeStage which handles GitHub, cache, ledger, and WebSocket.
	go func() {
		if err := o.ChangeStage(issueNumber, stage, github.ReasonWorkerStageUpdate); err != nil {
			log.Printf("[Orchestrator] Error in async stage update for #%d: %v", issueNumber, err)
		}
	}()
}

func (o *Orchestrator) BroadcastWorkerStatus(workerID, status string, taskID int, taskTitle, stage string, elapsedSeconds int) {
	if o.hub != nil {
		o.hub.BroadcastWorkerUpdate(workerID, status, taskID, taskTitle, stage, elapsedSeconds)
	}
}

// HandleSyncEvent processes external changes from GitHub sync.
// This is called by SyncService when it detects changes from GitHub.
// All stage changes go through ChangeStage to ensure labels, cache, ledger,
// and WebSocket are updated consistently.
func (o *Orchestrator) HandleSyncEvent(issue github.Issue) {
	log.Printf("[Orchestrator] Processing sync event for issue #%d", issue.Number)

	// Skip if this issue is currently being processed by worker
	if o.currentTask != nil && o.currentTask.Issue.Number == issue.Number {
		log.Printf("[Orchestrator] Issue #%d is being processed, skipping sync event", issue.Number)
		return
	}

	// Closed issues always get stage:done (final state, takes priority over merging)
	if strings.EqualFold(issue.State, "CLOSED") {
		if !hasLabel(issue, "stage:done") {
			log.Printf("[Orchestrator] Sync: closed issue #%d missing stage:done, fixing", issue.Number)
			if err := o.ChangeStage(issue.Number, github.StageDone, github.ReasonSyncClosedIssue); err != nil {
				log.Printf("[Orchestrator] Error setting stage:done for #%d: %v", issue.Number, err)
			}
		}
		return // closed = done, no further checks needed
	}

	// Merged but still open PRs get stage:merging (waiting for close)
	if issue.PRMerged && !issue.MergedAt.IsZero() {
		if !hasLabel(issue, "stage:merging") {
			log.Printf("[Orchestrator] Sync: merged issue #%d missing stage:merging, fixing", issue.Number)
			if err := o.ChangeStage(issue.Number, github.StageMerge, github.ReasonSyncMergedPR); err != nil {
				log.Printf("[Orchestrator] Error setting stage:merging for #%d: %v", issue.Number, err)
			}
		}
	}
}

// drainWorkerEvents processes all pending worker events from the channel.
// Must be called after worker.Process() returns and before setting the final
// post-process stage, so the ledger records transitions in correct order.
func (o *Orchestrator) drainWorkerEvents() {
	for {
		select {
		case event := <-o.workerEventCh:
			o.HandleWorkerEvent(event)
		default:
			return
		}
	}
}

func (o *Orchestrator) sleep(ctx context.Context, d time.Duration) {
	select {
	case <-time.After(d):
	case <-ctx.Done():
	}
}

func hasLabel(issue github.Issue, name string) bool {
	for _, l := range issue.Labels {
		if l.Name == name {
			return true
		}
	}
	return false
}

func labelNames(issue github.Issue) string {
	var names []string
	for _, l := range issue.Labels {
		names = append(names, l.Name)
	}
	return strings.Join(names, ", ")
}

// getStageFromIssue extracts the current stage label from issue labels
func (o *Orchestrator) getStageFromIssue(issue github.Issue) (string, error) {
	for _, label := range issue.Labels {
		if strings.HasPrefix(label.Name, "stage:") {
			return label.Name, nil
		}
	}
	return "", fmt.Errorf("issue #%d has no stage label", issue.Number)
}

// HandleWorkerEvent processes events from worker
func (o *Orchestrator) HandleWorkerEvent(event WorkerEvent) {
	log.Printf("[Orchestrator] Received event from worker: issue #%d completed stage %s with status %s",
		event.IssueNumber, event.Stage, event.Status)

	// Decide next stage based on state machine
	nextStage, reason, ok := o.decideNextStage(event)
	if !ok {
		return
	}

	// Update stage
	if err := o.ChangeStage(event.IssueNumber, nextStage, reason); err != nil {
		log.Printf("[Orchestrator] Error changing stage for #%d: %v", event.IssueNumber, err)
	}
}

// decideNextStage determines the next stage based on current state and event.
// Returns the next Stage, the reason for the transition, and true if a transition should happen.
func (o *Orchestrator) decideNextStage(event WorkerEvent) (github.Stage, github.StageChangeReason, bool) {
	// State machine logic
	switch event.Stage {
	case "analysis":
		if event.Status == EventSuccess {
			return github.StageCode, github.ReasonWorkerCompletedAnalysis, true
		}
	case "coding":
		if event.Status == EventSuccess {
			return github.StageReview, github.ReasonWorkerCompletedCoding, true
		}
	case "code-review":
		if event.Status == EventSuccess {
			return github.StageCreatePR, github.ReasonWorkerCompletedCodeReview, true
		}
	case "create-pr":
		if event.Status == EventSuccess {
			return github.StageApprove, github.ReasonWorkerCompletedCreatePR, true
		}
	}

	if event.Status == EventFailed {
		return github.StageFailed, github.ReasonWorkerFailed, true
	}

	if event.Status == EventBlocked {
		return github.StageNeedsUser, github.ReasonWorkerBlocked, true
	}

	return "", "", false
}

// ChangeStage is the ONLY way to change stages in the entire system.
// Dashboard, worker, and any other component must call this method.
// It updates: GitHub labels, local cache, WebSocket broadcast, and ledger.
func (o *Orchestrator) ChangeStage(issueNumber int, toStage github.Stage, reason github.StageChangeReason) error {
	toLabel := toStage.Label()
	log.Printf("[Orchestrator] Changing stage of #%d to %s (reason: %s)", issueNumber, toLabel, reason.String())

	// Get current stage from cache
	fromStage := "unknown"
	if o.store != nil {
		if existing, err := o.store.GetIssueCache(issueNumber); err == nil {
			if s, err := o.getStageFromIssue(existing); err == nil {
				fromStage = s
			} else {
				log.Printf("[Orchestrator] Warning: %v", err)
			}
		}
	}

	// Update GitHub
	updatedIssue, err := o.gh.SetStageLabel(issueNumber, toStage)
	if err != nil {
		return fmt.Errorf("setting stage %s on #%d: %w", toLabel, issueNumber, err)
	}

	// Update cache
	if o.store != nil {
		milestone := o.activeMilestone()
		now := time.Now().UTC()
		updatedIssue.UpdatedAt = &now

		if err := o.store.SaveIssueCache(updatedIssue, milestone, true); err != nil {
			log.Printf("[Orchestrator] Error saving issue cache for #%d: %v", issueNumber, err)
		}
	}

	// Broadcast via WebSocket (after cache, before ledger)
	if o.hub != nil {
		o.hub.BroadcastIssueUpdate(updatedIssue)
	}

	// Save to ledger (last)
	if o.store != nil {
		if err := o.store.SaveStageChange(issueNumber, fromStage, toLabel, reason.Label(), "orchestrator"); err != nil {
			log.Printf("[Orchestrator] Error saving stage change to ledger for #%d: %v", issueNumber, err)
		}
	}

	log.Printf("[Orchestrator] Successfully changed stage of #%d from %s to %s", issueNumber, fromStage, toLabel)
	return nil
}

// activeMilestone returns the current active milestone title
func (o *Orchestrator) activeMilestone() string {
	if o.gh != nil && o.gh.GetActiveMilestone() != nil {
		return o.gh.GetActiveMilestone().Title
	}
	return ""
}
