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

type Orchestrator struct {
	cfg           *config.Config
	worker        *Worker
	gh            *github.Client
	oc            *opencode.Client
	brMgr         *git.BranchManager
	store         *db.Store
	projectNumber int
	running       bool
	paused        bool
	processing    bool
	currentTask   *Task
	mu            sync.Mutex
}

func NewOrchestrator(cfg *config.Config, gh *github.Client, oc *opencode.Client, brMgr *git.BranchManager, store *db.Store, projectNumber int) *Orchestrator {
	o := &Orchestrator{
		cfg:           cfg,
		gh:            gh,
		oc:            oc,
		brMgr:         brMgr,
		store:         store,
		projectNumber: projectNumber,
		paused:        true,
	}
	o.worker = NewWorker(1, cfg, oc, gh, brMgr, store)
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
		issues, err := o.gh.ListIssuesForMilestone(milestone.Title)
		if err != nil {
			log.Printf("[Orchestrator] Error listing issues: %v", err)
			o.sleep(ctx, 30*time.Second)
			continue
		}

		var openCount, skippedCount int
		var inProgressIssue *github.Issue
		var candidates []github.Issue
		var awaitingApproval []github.Issue
		for i := range issues {
			if !strings.EqualFold(issues[i].State, "open") {
				continue
			}
			openCount++
			if hasLabel(issues[i], "failed") {
				skippedCount++
				log.Printf("[Orchestrator]   skip #%d %q (failed)", issues[i].Number, issues[i].Title)
				continue
			}
			if hasLabel(issues[i], "in-progress") {
				if inProgressIssue == nil {
					inProgressIssue = &issues[i]
				}
				continue
			}
			if hasLabel(issues[i], "awaiting-approval") {
				awaitingApproval = append(awaitingApproval, issues[i])
				log.Printf("[Orchestrator]   skip #%d %q (awaiting-approval)", issues[i].Number, issues[i].Title)
				continue
			}
			candidates = append(candidates, issues[i])
		}
		log.Printf("[Orchestrator] Found %d issues (%d open, %d failed-skipped, %d awaiting-approval, %d candidates)", len(issues), openCount, skippedCount, len(awaitingApproval), len(candidates))

		var nextIssue *github.Issue
		if inProgressIssue != nil {
			nextIssue = inProgressIssue
			log.Printf("[Orchestrator] Resuming in-progress #%d: %s", nextIssue.Number, nextIssue.Title)
		} else if len(awaitingApproval) > 0 {
			// Single-branch mode: don't start new work while PRs await approval.
			// New branches would be based on master which lacks unmerged PR changes,
			// causing conflicts when those PRs get merged.
			log.Printf("[Orchestrator] ⏳ Waiting — %d issue(s) awaiting approval, not starting new work", len(awaitingApproval))
		} else if len(candidates) > 0 {
			picked, err := o.pickNextTicket(ctx, candidates, awaitingApproval)
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

		if err := o.gh.AddLabel(nextIssue.Number, "in-progress"); err != nil {
			log.Printf("[Orchestrator] Error adding in-progress label: %v", err)
		}
		if err := o.gh.RemoveLabel(nextIssue.Number, "merge-failed"); err != nil {
			log.Printf("[Orchestrator] Error removing merge-failed label: %v", err)
		}
		o.moveToColumn(nextIssue.Number, "In Progress")

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

		if processErr != nil && errors.Is(processErr, ErrAlreadyDone) {
			log.Printf("[Orchestrator] ✓ Already done #%d: %v", nextIssue.Number, processErr)
			o.recordStep(nextIssue.Number, "already-done", processErr.Error())
			comment := fmt.Sprintf("Ticket already done — closing automatically.\n\n%s", processErr.Error())
			if err := o.gh.AddComment(nextIssue.Number, comment); err != nil {
				log.Printf("[Orchestrator] Error adding comment: %v", err)
			}
			if err := o.gh.RemoveLabel(nextIssue.Number, "in-progress"); err != nil {
				log.Printf("[Orchestrator] Error removing in-progress label: %v", err)
			}
			if err := o.gh.CloseIssue(nextIssue.Number); err != nil {
				log.Printf("[Orchestrator] Error closing issue: %v", err)
			}
			o.moveToColumn(nextIssue.Number, "Done")
			o.recordStep(nextIssue.Number, "done", "Closed as already done")
		} else if processErr != nil {
			log.Printf("[Orchestrator] ✗ Failed #%d: %v", nextIssue.Number, processErr)
			o.recordStep(nextIssue.Number, "failed", processErr.Error())
			if err := o.gh.RemoveLabel(nextIssue.Number, "in-progress"); err != nil {
				log.Printf("[Orchestrator] Error removing in-progress label: %v", err)
			}
			if err := o.gh.AddLabel(nextIssue.Number, "failed"); err != nil {
				log.Printf("[Orchestrator] Error adding failed label: %v", err)
			}
			o.moveToColumn(nextIssue.Number, "Blocked")
		} else {
			prURL := ""
			if task.Result != nil {
				prURL = task.Result.PRURL
			}
			log.Printf("[Orchestrator] ✓ Completed #%d → awaiting approval: %s", nextIssue.Number, prURL)
			o.recordStep(nextIssue.Number, "waiting-for-approval", prURL)
			if err := o.gh.RemoveLabel(nextIssue.Number, "in-progress"); err != nil {
				log.Printf("[Orchestrator] Error removing in-progress label: %v", err)
			}
			if err := o.gh.AddLabel(nextIssue.Number, "awaiting-approval"); err != nil {
				log.Printf("[Orchestrator] Error adding awaiting-approval label: %v", err)
			}
			o.moveToColumn(nextIssue.Number, "Approve")
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

	model := opencode.ParseModelRef(o.cfg.Planning.LLM)
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

func (o *Orchestrator) moveToColumn(issueNumber int, column string) {
	if o.projectNumber == 0 {
		return
	}
	if err := o.gh.MoveItemToColumn(o.projectNumber, issueNumber, column); err != nil {
		log.Printf("[Orchestrator] Error moving #%d to %q: %v", issueNumber, column, err)
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
