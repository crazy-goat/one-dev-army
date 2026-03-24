package dashboard

import (
	"fmt"
	"log"
	"strings"
	"time"

	"github.com/crazy-goat/one-dev-army/internal/db"
	"github.com/crazy-goat/one-dev-army/internal/github"
)

// StageChangeReason describes why a stage was changed
type StageChangeReason string

const (
	// Manual changes via dashboard
	ReasonManualApprove       StageChangeReason = "manual_approve"
	ReasonManualReject        StageChangeReason = "manual_reject"
	ReasonManualRetry         StageChangeReason = "manual_retry"
	ReasonManualBlock         StageChangeReason = "manual_block"
	ReasonManualUnblock       StageChangeReason = "manual_unblock"
	ReasonManualDecline       StageChangeReason = "manual_decline"
	ReasonManualMergeApproved StageChangeReason = "manual_merge_approved"

	// Worker/Orchestrator changes
	ReasonWorkerDone        StageChangeReason = "worker_done"
	ReasonWorkerFailed      StageChangeReason = "worker_failed"
	ReasonWorkerApprove     StageChangeReason = "worker_approve"
	ReasonWorkerStageUpdate StageChangeReason = "worker_stage_update"
	ReasonWorkerNeedsUser   StageChangeReason = "worker_needs_user"
	ReasonWorkerBlocked     StageChangeReason = "worker_blocked"

	// Sync changes
	ReasonSyncInitial  StageChangeReason = "sync_initial"
	ReasonSyncPeriodic StageChangeReason = "sync_periodic"
	ReasonSyncManual   StageChangeReason = "sync_manual"
)

// String returns the human-readable description of the reason
func (r StageChangeReason) String() string {
	switch r {
	case ReasonManualApprove:
		return "Manual approve via dashboard"
	case ReasonManualReject:
		return "Manual reject via dashboard"
	case ReasonManualRetry:
		return "Manual retry via dashboard"
	case ReasonManualBlock:
		return "Manual block via dashboard"
	case ReasonManualUnblock:
		return "Manual unblock via dashboard"
	case ReasonManualDecline:
		return "Manual decline via dashboard"
	case ReasonManualMergeApproved:
		return "Manual merge approved via dashboard"
	case ReasonWorkerDone:
		return "Worker completed task"
	case ReasonWorkerFailed:
		return "Worker failed task"
	case ReasonWorkerApprove:
		return "Worker awaiting approval"
	case ReasonWorkerStageUpdate:
		return "Worker stage update"
	case ReasonWorkerNeedsUser:
		return "Worker needs user intervention"
	case ReasonWorkerBlocked:
		return "Worker blocked"
	case ReasonSyncInitial:
		return "Initial sync from GitHub"
	case ReasonSyncPeriodic:
		return "Periodic sync from GitHub"
	case ReasonSyncManual:
		return "Manual sync from GitHub"
	default:
		return string(r)
	}
}

// StageManager handles stage changes with proper caching and broadcasting
type StageManager struct {
	gh           *github.Client
	store        *db.Store
	hub          *Hub
	getMilestone func() string
}

// NewStageManager creates a new stage manager
func NewStageManager(gh *github.Client, store *db.Store, hub *Hub, getMilestone func() string) *StageManager {
	return &StageManager{
		gh:           gh,
		store:        store,
		hub:          hub,
		getMilestone: getMilestone,
	}
}

// ChangeStage changes the stage of an issue with a given reason
// It is the ONLY way to change stages - all code must use this function
// It handles: GitHub update, cache save, WebSocket broadcast, and ledger logging
func (sm *StageManager) ChangeStage(issueNumber int, toStage string, reason StageChangeReason, changedBy string) (*github.Issue, error) {
	// Convert stage name to label for logging
	toLabel := getStageLabel(toStage)
	log.Printf("[StageManager] Changing stage of #%d to %s (reason: %s, by: %s)", issueNumber, toLabel, reason.String(), changedBy)

	// Get current issue to determine from_stage
	fromStage := "Unknown"
	if sm.store != nil {
		existing, err := sm.store.GetIssueCache(issueNumber)
		if err == nil {
			fromStage = sm.getStageFromIssue(existing)
		}
	}

	// Update stage on GitHub
	updatedIssue, err := sm.gh.SetStageLabel(issueNumber, toStage)
	if err != nil {
		return nil, fmt.Errorf("setting stage %s on #%d: %w", toStage, issueNumber, err)
	}

	// Update local cache with current timestamp to prevent sync from overwriting
	if sm.store != nil {
		milestone := sm.getMilestone()
		now := time.Now().UTC()
		updatedIssue.UpdatedAt = &now

		if err := sm.store.SaveIssueCache(updatedIssue, milestone, true); err != nil {
			log.Printf("[StageManager] Error saving issue cache for #%d: %v", issueNumber, err)
			// Don't fail the operation if cache update fails
		}
	}

	// Broadcast update via WebSocket (after cache, before ledger)
	if sm.hub != nil {
		sm.hub.BroadcastIssueUpdate(updatedIssue)
	}

	// Save to ledger with full label names (last)
	if sm.store != nil {
		toLabel := getStageLabel(toStage)
		if err := sm.store.SaveStageChange(issueNumber, fromStage, toLabel, string(reason), changedBy); err != nil {
			log.Printf("[StageManager] Error saving stage change to ledger for #%d: %v", issueNumber, err)
			// Don't fail the operation if ledger save fails
		}
	}

	log.Printf("[StageManager] Successfully changed stage of #%d from %s to %s", issueNumber, fromStage, getStageLabel(toStage))
	return &updatedIssue, nil
}

// getStageFromIssue extracts the current stage from issue labels
func (sm *StageManager) getStageFromIssue(issue github.Issue) string {
	for _, label := range issue.Labels {
		if strings.HasPrefix(label.Name, "stage:") {
			return label.Name
		}
	}
	return "Backlog"
}

// getStageLabel returns the full label name for a stage
func getStageLabel(stage string) string {
	if labels, ok := github.StageToLabels[stage]; ok && len(labels) > 0 {
		return labels[0]
	}
	// For stages without labels (Backlog, Done), return stage name
	return stage
}
