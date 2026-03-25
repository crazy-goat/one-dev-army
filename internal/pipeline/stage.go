package pipeline

// Stage represents a step in the ticket processing pipeline.
// Stage names align with the state machine defined in docs/state-machine.md.
type Stage string

const (
	StageQueued        Stage = "queued"
	StageAnalysis      Stage = "analysis"
	StageCoding        Stage = "coding"
	StageCodeReview    Stage = "code-review"
	StageCreatePR      Stage = "create-pr"
	StageCheckPipeline Stage = "check-pipeline"
	StageApprove       Stage = "awaiting-approval"
	StageMerging       Stage = "merging"
	StageDone          Stage = "done"
	StageFailed        Stage = "failed"
	StageBlocked       Stage = "blocked"
)

// Column represents a dashboard column.
type Column string

const (
	ColumnBacklog       Column = "Backlog"
	ColumnPlan          Column = "Plan"
	ColumnCode          Column = "Code"
	ColumnAIReview      Column = "AI Review"
	ColumnCheckPipeline Column = "Pipeline"
	ColumnApprove       Column = "Approve"
	ColumnMerge         Column = "Merge"
	ColumnDone          Column = "Done"
	ColumnFailed        Column = "Failed"
	ColumnBlocked       Column = "Blocked"
)

var stageOrder = []Stage{
	StageQueued,
	StageAnalysis,
	StageCoding,
	StageCodeReview,
	StageCreatePR,
	StageCheckPipeline,
	StageApprove,
	StageMerging,
	StageDone,
}

// Column returns the dashboard column this stage maps to.
func (s Stage) Column() Column {
	switch s {
	case StageQueued:
		return ColumnBacklog
	case StageAnalysis:
		return ColumnPlan
	case StageCoding:
		return ColumnCode
	case StageCodeReview, StageCreatePR:
		return ColumnAIReview
	case StageCheckPipeline:
		return ColumnCheckPipeline
	case StageApprove:
		return ColumnApprove
	case StageMerging:
		return ColumnMerge
	case StageDone:
		return ColumnDone
	case StageFailed:
		return ColumnFailed
	case StageBlocked:
		return ColumnBlocked
	default:
		return ColumnBacklog
	}
}

// Label returns the GitHub label for this stage.
// All stage labels use the "stage:" prefix.
func (s Stage) Label() string {
	switch s {
	case StageAnalysis:
		return "stage:analysis"
	case StageCoding:
		return "stage:coding"
	case StageCodeReview:
		return "stage:code-review"
	case StageCreatePR:
		return "stage:create-pr"
	case StageCheckPipeline:
		return "stage:check-pipeline"
	case StageApprove:
		return "stage:awaiting-approval"
	case StageMerging:
		return "stage:merging"
	case StageFailed:
		return "stage:failed"
	case StageBlocked:
		return "stage:blocked"
	default:
		return ""
	}
}

// Next returns the next stage in the pipeline order.
func (s Stage) Next() Stage {
	for i, st := range stageOrder {
		if st == s && i+1 < len(stageOrder) {
			return stageOrder[i+1]
		}
	}
	return s
}

// RetryTarget returns the stage to retry from when this stage fails.
// Per state machine: all retries go back to Code.
func (s Stage) RetryTarget() Stage {
	switch s {
	case StageCodeReview, StageCreatePR, StageCheckPipeline, StageMerging:
		return StageCoding
	default:
		return s
	}
}
