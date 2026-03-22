package pipeline

type Stage string

const (
	StageQueued     Stage = "queued"
	StageAnalysis   Stage = "analysis"
	StagePlanning   Stage = "planning"
	StagePlanReview Stage = "plan-review"
	StageCoding     Stage = "coding"
	StageTesting    Stage = "testing"
	StageCodeReview Stage = "code-review"
	StageMerging    Stage = "merging"
	StageDone       Stage = "done"
	StageBlocked    Stage = "blocked"
)

type Column string

const (
	ColumnBacklog    Column = "Backlog"
	ColumnInProgress Column = "In Progress"
	ColumnAIReview   Column = "AI Review"
	ColumnApprove    Column = "Approve"
	ColumnDone       Column = "Done"
	ColumnBlocked    Column = "Blocked"
)

var stageOrder = []Stage{
	StageQueued,
	StageAnalysis,
	StagePlanning,
	StagePlanReview,
	StageCoding,
	StageTesting,
	StageCodeReview,
	StageMerging,
	StageDone,
}

func (s Stage) Column() Column {
	switch s {
	case StageQueued:
		return ColumnBacklog
	case StageAnalysis, StagePlanning, StageCoding, StageTesting:
		return ColumnInProgress
	case StagePlanReview, StageCodeReview:
		return ColumnAIReview
	case StageMerging:
		return ColumnApprove
	case StageDone:
		return ColumnDone
	case StageBlocked:
		return ColumnBlocked
	default:
		return ColumnBacklog
	}
}

func (s Stage) Label() string {
	switch s {
	case StageAnalysis, StagePlanning, StagePlanReview, StageCoding, StageTesting, StageCodeReview:
		return "stage:" + string(s)
	default:
		return ""
	}
}

func (s Stage) Next() Stage {
	for i, st := range stageOrder {
		if st == s && i+1 < len(stageOrder) {
			return stageOrder[i+1]
		}
	}
	return s
}

func (s Stage) RetryTarget() Stage {
	switch s {
	case StagePlanReview:
		return StagePlanning
	case StageTesting, StageCodeReview:
		return StageCoding
	default:
		return s
	}
}
