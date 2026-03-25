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
	"sync/atomic"
	"time"

	"github.com/crazy-goat/one-dev-army/internal/config"
	"github.com/crazy-goat/one-dev-army/internal/db"
	"github.com/crazy-goat/one-dev-army/internal/git"
	"github.com/crazy-goat/one-dev-army/internal/github"
	"github.com/crazy-goat/one-dev-army/internal/llm"
	"github.com/crazy-goat/one-dev-army/internal/opencode"
	"github.com/crazy-goat/one-dev-army/internal/prompts"
)

type StageBroadcaster interface {
	BroadcastIssueUpdate(issue github.Issue)
	BroadcastWorkerUpdate(workerID, status string, taskID int, taskTitle, stage string, elapsedSeconds int)
}

type Orchestrator struct {
	cfg         atomic.Pointer[config.Config]
	worker      *Worker
	gh          *github.Client
	oc          *opencode.Client
	brMgr       *git.BranchManager
	store       *db.Store
	hub         StageBroadcaster
	router      *llm.Router
	running     bool
	paused      bool
	processing  bool
	currentTask *Task
	mu          sync.Mutex
}

func NewOrchestrator(cfg *config.Config, gh *github.Client, oc *opencode.Client, brMgr *git.BranchManager, store *db.Store, hub StageBroadcaster, router *llm.Router) *Orchestrator {
	o := &Orchestrator{
		gh:     gh,
		oc:     oc,
		brMgr:  brMgr,
		store:  store,
		hub:    hub,
		router: router,
		paused: true,
	}
	o.cfg.Store(cfg)
	o.worker = NewWorker(1, cfg, oc.Clone(), gh, brMgr, store, o, router)
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

// GetWorker returns the orchestrator's worker for config propagation.
func (o *Orchestrator) GetWorker() *Worker {
	return o.worker
}

// UpdateConfig updates the orchestrator's configuration atomically and propagates to the worker and router.
// This method is called by the ConfigPropagator when config changes.
func (o *Orchestrator) UpdateConfig(cfg *config.Config) {
	o.cfg.Store(cfg)

	// Propagate to router (LLM config only)
	if o.router != nil {
		o.router.UpdateConfig(&cfg.LLM)
	}

	// Propagate to worker
	if o.worker != nil {
		o.worker.UpdateConfig(cfg)
	}

	log.Printf("[Orchestrator] Configuration updated (YoloMode=%v)", cfg.YoloMode)
}

// Compile-time interface check: ensure Orchestrator implements ConfigAwareWorker.
var _ config.ConfigAwareWorker = (*Orchestrator)(nil)

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

		// Classify issues: candidates (backlog) vs resumable (worker stages after restart)
		var candidates []github.Issue
		var resumeIssue *github.Issue
		for i := range issues {
			if !strings.EqualFold(issues[i].State, "open") {
				continue
			}

			stage := getStageLabel(issues[i])
			if stage == "" || stage == string(github.StageBacklog) {
				// No stage label or stage:backlog = backlog candidate
				candidates = append(candidates, issues[i])
			} else if isWorkerStage(stage) && resumeIssue == nil {
				// Worker stage (analysis/coding/review/create-pr/awaiting-approval) = resume after restart
				resumeIssue = &issues[i]
			}
			// Everything else (merging, failed, blocked, done, needs-user)
			// is ignored — orchestrator doesn't pick these up.
		}
		log.Printf("[Orchestrator] Found %d candidates, resume=%v", len(candidates), resumeIssue != nil)

		var nextIssue *github.Issue
		isResume := false
		if resumeIssue != nil {
			nextIssue = resumeIssue
			isResume = true
			log.Printf("[Orchestrator] Resuming in-progress #%d: %s", nextIssue.Number, nextIssue.Title)
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

		// Only set stage:analysis for NEW issues (backlog candidates).
		// Resume issues already have a valid stage label — resetting them
		// to analysis would cause an infinite loop: the worker resumes from
		// the stored step, but the label says analysis, so on next restart
		// the orchestrator picks it up again and resets the label again.
		if !isResume {
			if err := o.ChangeStage(nextIssue.Number, github.StagePlan, github.ReasonWorkerPickedUp); err != nil {
				log.Printf("[Orchestrator] Error setting initial stage for #%d: %v", nextIssue.Number, err)
			}
		} else {
			log.Printf("[Orchestrator] Keeping existing stage for resumed issue #%d", nextIssue.Number)
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

		o.mu.Lock()
		o.processing = false
		o.currentTask = nil
		o.mu.Unlock()

		// Worker.Process() returns nil only after successful merge (done).
		// Any error means failed or already-done.
		switch {
		case processErr != nil && errors.Is(processErr, ErrAlreadyDone):
			log.Printf("[Orchestrator] ✓ Already done #%d: %v", nextIssue.Number, processErr)
			o.recordStep(nextIssue.Number, "already-done", processErr.Error())
			comment := "Ticket already done — closing automatically.\n\n" + processErr.Error()
			if err := o.gh.AddComment(nextIssue.Number, comment); err != nil {
				log.Printf("[Orchestrator] Error adding comment: %v", err)
			}
			if err := o.ChangeStage(nextIssue.Number, github.StageDone, github.ReasonWorkerAlreadyDone); err != nil {
				log.Printf("[Orchestrator] Error setting stage:done for #%d: %v", nextIssue.Number, err)
			}
		case processErr != nil:
			log.Printf("[Orchestrator] ✗ Failed #%d: %v", nextIssue.Number, processErr)
			o.recordStep(nextIssue.Number, "failed", processErr.Error())
			if err := o.ChangeStage(nextIssue.Number, github.StageFailed, github.ReasonWorkerFailed); err != nil {
				log.Printf("[Orchestrator] Error setting stage:failed for #%d: %v", nextIssue.Number, err)
			}
		default:
			log.Printf("[Orchestrator] ✓ Completed #%d (merged)", nextIssue.Number)
			o.recordStep(nextIssue.Number, "done", "Ticket completed and merged")
		}

		o.sleep(ctx, 5*time.Second)
	}
}

var nextTicketRe = regexp.MustCompile(`NEXT:\s*#(\d+)`)

func (o *Orchestrator) pickNextTicket(_ context.Context, candidates []github.Issue, awaitingApproval []github.Issue) (*github.Issue, error) {
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
		pendingInfo = "\n\nTickets awaiting approval (PR created, AI review passed, but NOT yet merged — treat as NOT DONE, do NOT pick tickets that depend on these):\n" + strings.Join(pendingLines, "\n")
	}

	prompt := fmt.Sprintf(prompts.MustGet(prompts.MVPPickTicket), o.gh.Repo, o.gh.Repo, ticketList) + pendingInfo

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
	llmModel := o.cfg.Load().LLM.Orchestration.Model
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
		return nil, errors.New("LLM did not return NEXT: #N format")
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

func (*Orchestrator) sleep(ctx context.Context, d time.Duration) {
	select {
	case <-time.After(d):
	case <-ctx.Done():
	}
}

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
	case "stage:analysis", "stage:coding", "stage:code-review", "stage:create-pr", "stage:awaiting-approval":
		return true
	default:
		return false
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

// getStageFromIssue extracts the current stage label from issue labels
func (*Orchestrator) getStageFromIssue(issue github.Issue) (string, error) {
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
func (*Orchestrator) decideNextStage(event WorkerEvent) (github.Stage, github.StageChangeReason, bool) {
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
		if event.Status == EventInProgress {
			return github.StageCode, github.ReasonWorkerFixingFromReview, true
		}
	case "code-review":
		if event.Status == EventSuccess {
			return github.StageCreatePR, github.ReasonWorkerCompletedCodeReview, true
		}
	case "create-pr":
		if event.Status == EventSuccess {
			return github.StageApprove, github.ReasonWorkerCompletedCreatePR, true
		}
	case "awaiting-approval":
		if event.Status == EventSuccess {
			return github.StageMerge, github.ReasonManualMerge, true
		}
	case "merge":
		if event.Status == EventSuccess {
			return github.StageDone, github.ReasonWorkerCompletedMerge, true
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

// activeMilestone returns the current active milestone title
func (o *Orchestrator) activeMilestone() string {
	if o.gh != nil && o.gh.GetActiveMilestone() != nil {
		return o.gh.GetActiveMilestone().Title
	}
	return ""
}

// CheckoutDefault switches to the default branch (main or master) after merge.
// This is a non-critical operation - errors are logged but not returned.
func (o *Orchestrator) CheckoutDefault() {
	if o.brMgr == nil {
		return
	}
	if err := o.brMgr.CheckoutDefault(); err != nil {
		log.Printf("[Orchestrator] Warning: failed to checkout default branch: %v", err)
	}
}
