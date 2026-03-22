package mvp

import (
	"sync"

	"github.com/crazy-goat/one-dev-army/internal/github"
)

type TaskStatus string

const (
	StatusPending    TaskStatus = "pending"
	StatusAnalyzing  TaskStatus = "analyzing"
	StatusPlanning   TaskStatus = "planning"
	StatusCoding     TaskStatus = "coding"
	StatusTesting    TaskStatus = "testing"
	StatusReviewing  TaskStatus = "reviewing"
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

	mu        sync.Mutex
	sessionID string
}

func (t *Task) SetSessionID(id string) {
	t.mu.Lock()
	defer t.mu.Unlock()
	t.sessionID = id
}

func (t *Task) SessionID() string {
	t.mu.Lock()
	defer t.mu.Unlock()
	return t.sessionID
}

type TaskResult struct {
	PRURL   string
	Error   error
	Summary string
}
