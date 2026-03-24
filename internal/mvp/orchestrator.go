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
			log.Printf("[Orchestrator] Setting Done stage for #%d (reason: worker_done)", nextIssue.Number)
			if _, err := o.gh.SetStageLabel(nextIssue.Number, "Done"); err != nil {
				log.Printf("[Orchestrator] Error setting Done stage for #%d: %v", nextIssue.Number, err)
			} else if o.store != nil {
				// Get actual current stage from cache
				fromStage := "stage:coding"
				if existing, err := o.store.GetIssueCache(nextIssue.Number); err == nil {
					fromStage = o.getStageFromIssue(existing)
				}
				if err := o.store.SaveStageChange(nextIssue.Number, fromStage, "stage:done", "worker_done", "orchestrator"); err != nil {
					log.Printf("[Orchestrator] Error saving stage change to ledger for #%d: %v", nextIssue.Number, err)
				}
			}
			o.recordStep(nextIssue.Number, "done", "Closed as already done")
		} else if processErr != nil {
			log.Printf("[Orchestrator] ✗ Failed #%d: %v", nextIssue.Number, processErr)
			o.recordStep(nextIssue.Number, "failed", processErr.Error())
			log.Printf("[Orchestrator] Setting Failed stage for #%d (reason: worker_failed)", nextIssue.Number)
			if _, err := o.gh.SetStageLabel(nextIssue.Number, "Failed"); err != nil {
				log.Printf("[Orchestrator] Error setting Failed stage for #%d: %v", nextIssue.Number, err)
			} else if o.store != nil {
				// Get actual current stage from cache
				fromStage := "stage:coding"
				if existing, err := o.store.GetIssueCache(nextIssue.Number); err == nil {
					fromStage = o.getStageFromIssue(existing)
				}
				if err := o.store.SaveStageChange(nextIssue.Number, fromStage, "stage:failed", "worker_failed", "orchestrator"); err != nil {
					log.Printf("[Orchestrator] Error saving stage change to ledger for #%d: %v", nextIssue.Number, err)
				}
			}
		} else {
			prURL := ""
			if task.Result != nil {
				prURL = task.Result.PRURL
			}
			log.Printf("[Orchestrator] ✓ Completed #%d → awaiting approval: %s", nextIssue.Number, prURL)
			o.recordStep(nextIssue.Number, "waiting-for-approval", prURL)
			log.Printf("[Orchestrator] Setting Approve stage for #%d (reason: worker_approve)", nextIssue.Number)
			if _, err := o.gh.SetStageLabel(nextIssue.Number, "Approve"); err != nil {
				log.Printf("[Orchestrator] Error setting Approve stage for #%d: %v", nextIssue.Number, err)
			} else if o.store != nil {
				// Get actual current stage from cache
				fromStage := "stage:coding"
				if existing, err := o.store.GetIssueCache(nextIssue.Number); err == nil {
					fromStage = o.getStageFromIssue(existing)
				}
				if err := o.store.SaveStageChange(nextIssue.Number, fromStage, "stage:awaiting-approval", "worker_approve", "orchestrator"); err != nil {
					log.Printf("[Orchestrator] Error saving stage change to ledger for #%d: %v", nextIssue.Number, err)
				}
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

func (o *Orchestrator) BroadcastStageUpdate(issueNumber int, stage string) {
	// Run GitHub API call asynchronously to avoid blocking worker
	go func() {
		log.Printf("[Orchestrator] Updating stage label %s for #%d (async, reason: worker_stage_update)", stage, issueNumber)

		// Get current stage from cache for ledger
		fromStage := "Unknown"
		if o.store != nil {
			if existing, err := o.store.GetIssueCache(issueNumber); err == nil {
				fromStage = o.getStageFromIssue(existing)
			}
		}

		// Set the stage label on GitHub
		updatedIssue, err := o.gh.SetStageLabel(issueNumber, stage)
		if err != nil {
			log.Printf("[Orchestrator] Error setting stage label %s for #%d: %v", stage, issueNumber, err)
			return
		}

		// Save to ledger
		if o.store != nil {
			// Convert stage name to label for consistency
			toLabel := stage
			if labels, ok := github.StageToLabels[stage]; ok && len(labels) > 0 {
				toLabel = labels[0]
			}
			if err := o.store.SaveStageChange(issueNumber, fromStage, toLabel, "worker_stage_update", "orchestrator"); err != nil {
				log.Printf("[Orchestrator] Error saving stage change to ledger for #%d: %v", issueNumber, err)
			}
		}

		// Broadcast the update via hub if available
		if o.hub != nil {
			log.Printf("[Orchestrator] Broadcasting stage update %s for #%d to WebSocket clients", stage, issueNumber)
			o.hub.BroadcastIssueUpdate(updatedIssue)
		}
	}()
}

func (o *Orchestrator) BroadcastWorkerStatus(workerID, status string, taskID int, taskTitle, stage string, elapsedSeconds int) {
	if o.hub != nil {
		o.hub.BroadcastWorkerUpdate(workerID, status, taskID, taskTitle, stage, elapsedSeconds)
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

// getStageFromIssue extracts the current stage from issue labels
func (o *Orchestrator) getStageFromIssue(issue github.Issue) string {
	for _, label := range issue.Labels {
		if strings.HasPrefix(label.Name, "stage:") {
			return label.Name
		}
	}
	return "Backlog"
}

// HandleWorkerEvent processes events from worker
func (o *Orchestrator) HandleWorkerEvent(event WorkerEvent) {
	log.Printf("[Orchestrator] Received event from worker: issue #%d completed stage %s with status %s",
		event.IssueNumber, event.Stage, event.Status)

	// Decide next stage based on state machine
	nextStage := o.decideNextStage(event)

	// Update stage
	if nextStage != "" {
		if err := o.changeStage(event.IssueNumber, nextStage, "worker_completed_"+event.Stage); err != nil {
			log.Printf("[Orchestrator] Error changing stage for #%d: %v", event.IssueNumber, err)
		}
	}
}

// decideNextStage determines the next stage based on current state and event
func (o *Orchestrator) decideNextStage(event WorkerEvent) string {
	// State machine logic
	switch event.Stage {
	case "analysis":
		if event.Status == EventSuccess {
			return "Code"
		}
	case "coding":
		if event.Status == EventSuccess {
			return "AI Review"
		}
	case "code-review":
		if event.Status == EventSuccess {
			return "Create PR"
		}
	case "create-pr":
		if event.Status == EventSuccess {
			return "Approve"
		}
	}

	if event.Status == EventFailed {
		return "Failed"
	}

	if event.Status == EventBlocked {
		return "NeedsUser"
	}

	return ""
}

// changeStage is the ONLY way to change stages
// It updates: GitHub, cache, ledger, WebSocket
func (o *Orchestrator) changeStage(issueNumber int, toStage, reason string) error {
	log.Printf("[Orchestrator] Changing stage of #%d to %s (reason: %s)", issueNumber, toStage, reason)

	// Get current stage from cache
	fromStage := "Unknown"
	if o.store != nil {
		if existing, err := o.store.GetIssueCache(issueNumber); err == nil {
			fromStage = o.getStageFromIssue(existing)
		}
	}

	// Update GitHub
	updatedIssue, err := o.gh.SetStageLabel(issueNumber, toStage)
	if err != nil {
		return fmt.Errorf("setting stage %s on #%d: %w", toStage, issueNumber, err)
	}

	// Update cache
	if o.store != nil {
		milestone := o.activeMilestone()
		now := time.Now().UTC()
		updatedIssue.UpdatedAt = &now

		if err := o.store.SaveIssueCache(updatedIssue, milestone, true); err != nil {
			log.Printf("[Orchestrator] Error saving issue cache for #%d: %v", issueNumber, err)
		}

		// Save to ledger
		toLabel := toStage
		if labels, ok := github.StageToLabels[toStage]; ok && len(labels) > 0 {
			toLabel = labels[0]
		}
		if err := o.store.SaveStageChange(issueNumber, fromStage, toLabel, reason, "orchestrator"); err != nil {
			log.Printf("[Orchestrator] Error saving stage change to ledger for #%d: %v", issueNumber, err)
		}
	}

	// Broadcast
	if o.hub != nil {
		o.hub.BroadcastIssueUpdate(updatedIssue)
	}

	log.Printf("[Orchestrator] Successfully changed stage of #%d from %s to %s", issueNumber, fromStage, toStage)
	return nil
}

// activeMilestone returns the current active milestone
func (o *Orchestrator) activeMilestone() string {
	// TODO: Get from config or current sprint
	return ""
}
