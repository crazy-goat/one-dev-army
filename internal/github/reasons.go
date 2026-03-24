package github

// StageChangeReason describes why a stage transition occurred.
// Every stage change in the system must use one of these typed reasons.
// Label() returns the short DB-safe value, String() returns a human-readable description.
type StageChangeReason string

const (
	// Manual changes via dashboard
	ReasonManualApprove     StageChangeReason = "manual_approve"
	ReasonManualReject      StageChangeReason = "manual_reject"
	ReasonManualRetry       StageChangeReason = "manual_retry"
	ReasonManualRetryFresh  StageChangeReason = "manual_retry_fresh"
	ReasonManualBlock       StageChangeReason = "manual_block"
	ReasonManualUnblock     StageChangeReason = "manual_unblock"
	ReasonManualDecline     StageChangeReason = "manual_decline"
	ReasonManualMerge       StageChangeReason = "manual_merge"
	ReasonManualMergeFailed StageChangeReason = "manual_merge_failed"

	// Worker/Orchestrator pipeline changes
	ReasonWorkerPickedUp            StageChangeReason = "worker_picked_up"
	ReasonWorkerAlreadyDone         StageChangeReason = "worker_already_done"
	ReasonWorkerFailed              StageChangeReason = "worker_failed"
	ReasonWorkerApprove             StageChangeReason = "worker_approve"
	ReasonWorkerCompletedAnalysis   StageChangeReason = "worker_completed_analysis"
	ReasonWorkerCompletedCoding     StageChangeReason = "worker_completed_coding"
	ReasonWorkerCompletedCodeReview StageChangeReason = "worker_completed_code_review"
	ReasonWorkerCompletedCreatePR   StageChangeReason = "worker_completed_create_pr"
	ReasonWorkerCompletedMerge      StageChangeReason = "worker_completed_merge"
	ReasonWorkerDeclined            StageChangeReason = "worker_declined"
	ReasonWorkerFixingFromReview    StageChangeReason = "worker_fixing_from_review"
	ReasonWorkerNeedsUser           StageChangeReason = "worker_needs_user"
	ReasonWorkerBlocked             StageChangeReason = "worker_blocked"
	ReasonWorkerStageUpdate         StageChangeReason = "worker_stage_update"

	// Sync changes
	ReasonSyncInitial     StageChangeReason = "sync_initial"
	ReasonSyncPeriodic    StageChangeReason = "sync_periodic"
	ReasonSyncManual      StageChangeReason = "sync_manual"
	ReasonSyncClosedIssue StageChangeReason = "sync_closed_issue"
	ReasonSyncMergedPR    StageChangeReason = "sync_merged_pr"
)

// Label returns the short, DB-safe identifier for this reason.
func (r StageChangeReason) Label() string {
	return string(r)
}

// String returns a human-readable description of the reason.
func (r StageChangeReason) String() string {
	switch r {
	case ReasonManualApprove:
		return "User approved issue via dashboard"
	case ReasonManualReject:
		return "User rejected issue via dashboard, moved to backlog"
	case ReasonManualRetry:
		return "User requested retry via dashboard"
	case ReasonManualRetryFresh:
		return "User requested fresh retry via dashboard, PR closed and steps cleared"
	case ReasonManualBlock:
		return "User blocked issue via dashboard"
	case ReasonManualUnblock:
		return "User unblocked issue via dashboard, moved to backlog"
	case ReasonManualDecline:
		return "User declined PR via dashboard, sent back for fixes"
	case ReasonManualMerge:
		return "User approved and merged PR via dashboard"
	case ReasonManualMergeFailed:
		return "Merge failed (likely conflict), PR closed"
	case ReasonWorkerPickedUp:
		return "Orchestrator picked up ticket for processing"
	case ReasonWorkerAlreadyDone:
		return "Worker detected ticket already completed, closing"
	case ReasonWorkerFailed:
		return "Worker pipeline failed"
	case ReasonWorkerApprove:
		return "Worker completed pipeline, awaiting manual approval"
	case ReasonWorkerCompletedAnalysis:
		return "Worker completed analysis, advancing to coding"
	case ReasonWorkerCompletedCoding:
		return "Worker completed coding, advancing to code-review"
	case ReasonWorkerCompletedCodeReview:
		return "Worker completed code-review, advancing to create-pr"
	case ReasonWorkerCompletedCreatePR:
		return "Worker completed PR creation, advancing to approval"
	case ReasonWorkerCompletedMerge:
		return "Worker completed merge, ticket done"
	case ReasonWorkerDeclined:
		return "User declined PR via worker, sent back for fixes"
	case ReasonWorkerFixingFromReview:
		return "Worker fixing code based on review feedback"
	case ReasonWorkerNeedsUser:
		return "Worker needs user intervention"
	case ReasonWorkerBlocked:
		return "Worker pipeline blocked"
	case ReasonWorkerStageUpdate:
		return "Worker stage update (async)"
	case ReasonSyncInitial:
		return "Initial sync from GitHub"
	case ReasonSyncPeriodic:
		return "Periodic sync from GitHub"
	case ReasonSyncManual:
		return "Manual sync from GitHub"
	case ReasonSyncClosedIssue:
		return "Sync detected closed issue without stage:done"
	case ReasonSyncMergedPR:
		return "Sync detected merged PR without stage:merging"
	default:
		return string(r)
	}
}
