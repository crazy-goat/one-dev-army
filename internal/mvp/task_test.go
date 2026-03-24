package mvp

import (
	"testing"
	"time"

	"github.com/crazy-goat/one-dev-army/internal/github"
)

func TestTaskStatusConstants(t *testing.T) {
	statuses := []TaskStatus{
		StatusPending,
		StatusAnalyzing,
		StatusPlanning,
		StatusCoding,
		StatusReviewing,
		StatusCreatingPR,
		StatusAwaitingApproval,
		StatusMerging,
		StatusDone,
		StatusFailed,
	}

	expected := []string{
		"pending",
		"analyzing",
		"planning",
		"coding",
		"reviewing",
		"creating_pr",
		"awaiting_approval",
		"merging",
		"done",
		"failed",
	}

	if len(statuses) != len(expected) {
		t.Fatalf("expected %d statuses, got %d", len(expected), len(statuses))
	}

	for i, s := range statuses {
		if string(s) != expected[i] {
			t.Errorf("status[%d] = %q, want %q", i, s, expected[i])
		}
	}
}

func TestTaskZeroValue(t *testing.T) {
	var task Task
	if task.Status != "" {
		t.Errorf("zero-value Status = %q, want empty", task.Status)
	}
	if task.Result != nil {
		t.Error("zero-value Result should be nil")
	}
	if task.Branch != "" {
		t.Errorf("zero-value Branch = %q, want empty", task.Branch)
	}
	if task.Worktree != "" {
		t.Errorf("zero-value Worktree = %q, want empty", task.Worktree)
	}
}

func TestTaskWithIssue(t *testing.T) {
	issue := github.Issue{
		Number: 42,
		Title:  "Add feature X",
		Body:   "Description of feature X",
		State:  "open",
	}

	task := Task{
		Issue:     issue,
		Milestone: "Sprint 1",
		Status:    StatusPending,
	}

	if task.Issue.Number != 42 {
		t.Errorf("Issue.Number = %d, want 42", task.Issue.Number)
	}
	if task.Milestone != "Sprint 1" {
		t.Errorf("Milestone = %q, want %q", task.Milestone, "Sprint 1")
	}
	if task.Status != StatusPending {
		t.Errorf("Status = %q, want %q", task.Status, StatusPending)
	}
}

func TestTaskResult(t *testing.T) {
	result := &TaskResult{
		PRURL:   "https://github.com/owner/repo/pull/1",
		Summary: "Implemented feature X",
	}

	if result.PRURL != "https://github.com/owner/repo/pull/1" {
		t.Errorf("PRURL = %q, want PR URL", result.PRURL)
	}
	if result.Error != nil {
		t.Error("Error should be nil for successful result")
	}
	if result.Summary != "Implemented feature X" {
		t.Errorf("Summary = %q, want %q", result.Summary, "Implemented feature X")
	}
}

// ChatHistory tests

func TestNewChatHistory(t *testing.T) {
	ch := NewChatHistory(100)
	if ch == nil {
		t.Fatal("NewChatHistory returned nil")
	}
	if ch.maxSize != 100 {
		t.Errorf("expected maxSize 100, got %d", ch.maxSize)
	}
	if !ch.IsEmpty() {
		t.Error("new ChatHistory should be empty")
	}
}

func TestNewChatHistory_DefaultSize(t *testing.T) {
	ch := NewChatHistory(0)
	if ch.maxSize != 1000 {
		t.Errorf("expected default maxSize 1000, got %d", ch.maxSize)
	}
	ch = NewChatHistory(-1)
	if ch.maxSize != 1000 {
		t.Errorf("expected default maxSize 1000 for negative input, got %d", ch.maxSize)
	}
}

func TestChatHistory_AddMessage(t *testing.T) {
	ch := NewChatHistory(10)

	ch.AddMessage("user", "Hello")
	ch.AddMessage("assistant", "Hi there!")

	if ch.Len() != 2 {
		t.Errorf("expected 2 messages, got %d", ch.Len())
	}

	msgs := ch.GetMessages()
	if len(msgs) != 2 {
		t.Errorf("expected 2 messages from GetMessages, got %d", len(msgs))
	}

	if msgs[0].Role != "user" || msgs[0].Content != "Hello" {
		t.Errorf("first message mismatch: %+v", msgs[0])
	}
	if msgs[1].Role != "assistant" || msgs[1].Content != "Hi there!" {
		t.Errorf("second message mismatch: %+v", msgs[1])
	}
}

func TestChatHistory_MaxSize(t *testing.T) {
	ch := NewChatHistory(3)

	ch.AddMessage("user", "msg1")
	ch.AddMessage("assistant", "msg2")
	ch.AddMessage("user", "msg3")
	ch.AddMessage("assistant", "msg4") // Should evict msg1

	if ch.Len() != 3 {
		t.Errorf("expected 3 messages (at max), got %d", ch.Len())
	}

	msgs := ch.GetMessages()
	if msgs[0].Content != "msg2" {
		t.Errorf("expected oldest message to be 'msg2', got '%s'", msgs[0].Content)
	}
	if msgs[2].Content != "msg4" {
		t.Errorf("expected newest message to be 'msg4', got '%s'", msgs[2].Content)
	}
}

func TestChatHistory_GetMessagesSince(t *testing.T) {
	ch := NewChatHistory(10)

	before := time.Now()
	time.Sleep(10 * time.Millisecond)

	ch.AddMessage("user", "msg1")
	time.Sleep(10 * time.Millisecond)

	middle := time.Now()
	time.Sleep(10 * time.Millisecond)

	ch.AddMessage("assistant", "msg2")
	time.Sleep(10 * time.Millisecond)

	after := time.Now()

	// Get messages since before - should return both
	msgs := ch.GetMessagesSince(before)
	if len(msgs) != 2 {
		t.Errorf("expected 2 messages since before, got %d", len(msgs))
	}

	// Get messages since middle - should return only msg2
	msgs = ch.GetMessagesSince(middle)
	if len(msgs) != 1 {
		t.Errorf("expected 1 message since middle, got %d", len(msgs))
	}
	if msgs[0].Content != "msg2" {
		t.Errorf("expected 'msg2', got '%s'", msgs[0].Content)
	}

	// Get messages since after - should return none
	msgs = ch.GetMessagesSince(after)
	if len(msgs) != 0 {
		t.Errorf("expected 0 messages since after, got %d", len(msgs))
	}
}

func TestChatHistory_Clear(t *testing.T) {
	ch := NewChatHistory(10)

	ch.AddMessage("user", "Hello")
	ch.AddMessage("assistant", "Hi!")

	if ch.Len() != 2 {
		t.Errorf("expected 2 messages before clear, got %d", ch.Len())
	}

	ch.Clear()

	if ch.Len() != 0 {
		t.Errorf("expected 0 messages after clear, got %d", ch.Len())
	}

	if !ch.IsEmpty() {
		t.Error("ChatHistory should be empty after Clear()")
	}
}

func TestChatHistory_ConcurrentAccess(t *testing.T) {
	ch := NewChatHistory(100)

	// Concurrent writes
	done := make(chan bool, 10)
	for i := 0; i < 10; i++ {
		go func(id int) {
			for j := 0; j < 10; j++ {
				ch.AddMessage("user", "msg")
			}
			done <- true
		}(i)
	}

	// Concurrent reads
	for i := 0; i < 5; i++ {
		go func() {
			for j := 0; j < 20; j++ {
				_ = ch.GetMessages()
				_ = ch.Len()
				_ = ch.IsEmpty()
			}
			done <- true
		}()
	}

	// Wait for all goroutines
	for i := 0; i < 15; i++ {
		<-done
	}

	// Verify final state
	if ch.Len() != 100 { // Max size reached
		t.Errorf("expected 100 messages (max), got %d", ch.Len())
	}
}

func TestChatMessage_Timestamp(t *testing.T) {
	ch := NewChatHistory(10)

	before := time.Now()
	ch.AddMessage("user", "test")
	after := time.Now()

	msgs := ch.GetMessages()
	if len(msgs) != 1 {
		t.Fatal("expected 1 message")
	}

	if msgs[0].Timestamp.Before(before) || msgs[0].Timestamp.After(after) {
		t.Error("message timestamp is outside expected range")
	}
}

func TestTask_SetSessionID_ClearsChatHistory(t *testing.T) {
	task := &Task{
		ChatHistory: NewChatHistory(100),
	}

	// Add some messages
	task.ChatHistory.AddMessage("user", "Hello")
	task.ChatHistory.AddMessage("assistant", "Hi!")

	if task.ChatHistory.Len() != 2 {
		t.Errorf("expected 2 messages, got %d", task.ChatHistory.Len())
	}

	// Set session ID to empty string should clear chat history
	task.SetSessionID("")

	if !task.ChatHistory.IsEmpty() {
		t.Error("chat history should be empty after SetSessionID(\"\")")
	}
}
