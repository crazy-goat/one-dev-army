package mvp

import (
	"context"
	"fmt"
	"log"
	"strings"
	"sync"
	"time"

	"github.com/crazy-goat/one-dev-army/internal/config"
	"github.com/crazy-goat/one-dev-army/internal/git"
	"github.com/crazy-goat/one-dev-army/internal/github"
	"github.com/crazy-goat/one-dev-army/internal/opencode"
)

type Orchestrator struct {
	cfg         *config.Config
	worker      *Worker
	gh          *github.Client
	oc          *opencode.Client
	wtMgr       *git.WorktreeManager
	running     bool
	paused      bool
	processing  bool
	currentTask *Task
	mu          sync.Mutex
}

func NewOrchestrator(cfg *config.Config, gh *github.Client, oc *opencode.Client, wtMgr *git.WorktreeManager) *Orchestrator {
	o := &Orchestrator{
		cfg:    cfg,
		gh:     gh,
		oc:     oc,
		wtMgr:  wtMgr,
		paused: true,
	}
	o.worker = NewWorker(1, cfg, oc, gh, wtMgr)
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
		var nextIssue *github.Issue
		for i := range issues {
			if !strings.EqualFold(issues[i].State, "open") {
				continue
			}
			openCount++
			if hasLabel(issues[i], "in-progress") || hasLabel(issues[i], "failed") {
				skippedCount++
				log.Printf("[Orchestrator]   skip #%d %q (labels: %v)", issues[i].Number, issues[i].Title, issues[i].GetLabelNames())
				continue
			}
			if nextIssue == nil {
				nextIssue = &issues[i]
			}
		}
		log.Printf("[Orchestrator] Found %d issues (%d open, %d skipped)", len(issues), openCount, skippedCount)

		if nextIssue == nil {
			log.Println("[Orchestrator] No available issues in sprint, waiting 30s...")
			o.sleep(ctx, 30*time.Second)
			continue
		}

		log.Printf("[Orchestrator] ▶ Picking up #%d: %s", nextIssue.Number, nextIssue.Title)

		if err := o.gh.AddLabel(nextIssue.Number, "in-progress"); err != nil {
			log.Printf("[Orchestrator] Error adding in-progress label: %v", err)
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

		if processErr != nil {
			log.Printf("[Orchestrator] ✗ Failed #%d: %v", nextIssue.Number, processErr)
			if err := o.gh.AddLabel(nextIssue.Number, "failed"); err != nil {
				log.Printf("[Orchestrator] Error adding failed label: %v", err)
			}
		} else {
			prURL := ""
			if task.Result != nil {
				prURL = task.Result.PRURL
			}
			log.Printf("[Orchestrator] ✓ Completed #%d: %s", nextIssue.Number, prURL)
			if prURL != "" {
				comment := fmt.Sprintf("Implemented in %s", prURL)
				if err := o.gh.AddComment(nextIssue.Number, comment); err != nil {
					log.Printf("[Orchestrator] Error adding comment: %v", err)
				}
			}
		}

		o.sleep(ctx, 5*time.Second)
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
