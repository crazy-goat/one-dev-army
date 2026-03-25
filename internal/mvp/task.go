package mvp

import (
	"sync"
	"time"

	"github.com/crazy-goat/one-dev-army/internal/github"
)

type TaskStatus string

const (
	StatusPending          TaskStatus = "pending"
	StatusAnalyzing        TaskStatus = "analyzing"
	StatusPlanning         TaskStatus = "planning"
	StatusCoding           TaskStatus = "coding"
	StatusReviewing        TaskStatus = "reviewing"
	StatusCreatingPR       TaskStatus = "creating_pr"
	StatusCheckingPipeline TaskStatus = "checking_pipeline"
	StatusAwaitingApproval TaskStatus = "awaiting_approval"
	StatusMerging          TaskStatus = "merging"
	StatusDone             TaskStatus = "done"
	StatusFailed           TaskStatus = "failed"
)

type Task struct {
	Issue       github.Issue
	Milestone   string
	Branch      string
	Worktree    string
	Status      TaskStatus
	Result      *TaskResult
	StartTime   time.Time
	ChatHistory *ChatHistory

	mu        sync.Mutex
	sessionID string
}

func (t *Task) SetSessionID(id string) {
	t.mu.Lock()
	defer t.mu.Unlock()
	t.sessionID = id
	// Clear chat history when session ends
	if id == "" && t.ChatHistory != nil {
		t.ChatHistory.Clear()
	}
}

func (t *Task) SessionID() string {
	t.mu.Lock()
	defer t.mu.Unlock()
	return t.sessionID
}

// AddChatMessage adds a message to the task's chat history
// Thread-safe and initializes ChatHistory if nil
func (t *Task) AddChatMessage(role, content string) {
	t.mu.Lock()
	defer t.mu.Unlock()

	if t.ChatHistory == nil {
		t.ChatHistory = NewChatHistory(1000)
	}
	t.ChatHistory.AddMessage(role, content)
}

// GetChatMessages returns all chat messages for this task
// Thread-safe and returns empty slice if no history
func (t *Task) GetChatMessages() []ChatMessage {
	t.mu.Lock()
	defer t.mu.Unlock()

	if t.ChatHistory == nil {
		return []ChatMessage{}
	}
	return t.ChatHistory.GetMessages()
}

type TaskResult struct {
	PRURL   string
	Error   error
	Summary string
}
