package mvp

import "github.com/crazy-goat/one-dev-army/internal/github"

type TaskStatus string

const (
	StatusPending    TaskStatus = "pending"
	StatusAnalyzing  TaskStatus = "analyzing"
	StatusPlanning   TaskStatus = "planning"
	StatusCoding     TaskStatus = "coding"
	StatusTesting    TaskStatus = "testing"
	StatusCreatingPR TaskStatus = "creating_pr"
	StatusDone       TaskStatus = "done"
	StatusFailed     TaskStatus = "failed"
)

type Task struct {
	Issue     github.Issue
	Milestone string
	Branch    string
	Worktree  string
	Status    TaskStatus
	Result    *TaskResult
}

type TaskResult struct {
	PRURL   string
	Error   error
	Summary string
}
