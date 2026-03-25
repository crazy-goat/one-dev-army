package mvp

import (
	"fmt"
	"sync"
	"testing"
	"time"

	"github.com/crazy-goat/one-dev-army/internal/github"
)

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

	messages := ch.GetMessages()
	if len(messages) != 2 {
		t.Fatalf("expected 2 messages from GetMessages, got %d", len(messages))
	}

	if messages[0].Role != "user" || messages[0].Content != "Hello" {
		t.Errorf("first message mismatch: got role=%s, content=%s", messages[0].Role, messages[0].Content)
	}

	if messages[1].Role != "assistant" || messages[1].Content != "Hi there!" {
		t.Errorf("second message mismatch: got role=%s, content=%s", messages[1].Role, messages[1].Content)
	}
}

func TestChatHistory_RingBuffer(t *testing.T) {
	ch := NewChatHistory(3)

	// Add more messages than capacity
	ch.AddMessage("user", "Message 1")
	ch.AddMessage("assistant", "Message 2")
	ch.AddMessage("user", "Message 3")
	ch.AddMessage("assistant", "Message 4") // Should evict Message 1

	if ch.Len() != 3 {
		t.Errorf("expected 3 messages (max capacity), got %d", ch.Len())
	}

	messages := ch.GetMessages()
	if len(messages) != 3 {
		t.Fatalf("expected 3 messages from GetMessages, got %d", len(messages))
	}

	// First message should be Message 2 (Message 1 was evicted)
	if messages[0].Content != "Message 2" {
		t.Errorf("expected first message to be 'Message 2', got '%s'", messages[0].Content)
	}

	// Last message should be Message 4
	if messages[2].Content != "Message 4" {
		t.Errorf("expected last message to be 'Message 4', got '%s'", messages[2].Content)
	}
}

func TestChatHistory_GetMessages(t *testing.T) {
	ch := NewChatHistory(10)

	// Add some messages
	ch.AddMessage("user", "Test message")
	ch.AddMessage("assistant", "Response")

	// Get messages
	messages := ch.GetMessages()

	// Verify we got a copy (modifying returned slice shouldn't affect original)
	if len(messages) != 2 {
		t.Fatalf("expected 2 messages, got %d", len(messages))
	}

	// Modify the returned slice
	messages[0].Content = "Modified"

	// Get messages again
	messages2 := ch.GetMessages()
	if messages2[0].Content != "Test message" {
		t.Error("GetMessages should return a copy, but original was modified")
	}
}

func TestChatHistory_GetMessagesSince(t *testing.T) {
	ch := NewChatHistory(10)

	// Add messages with delays
	ch.AddMessage("user", "Old message")
	time.Sleep(10 * time.Millisecond)
	cutoff := time.Now()
	time.Sleep(10 * time.Millisecond)
	ch.AddMessage("assistant", "New message")

	// Get messages since cutoff
	messages := ch.GetMessagesSince(cutoff)

	if len(messages) != 1 {
		t.Errorf("expected 1 message since cutoff, got %d", len(messages))
	}

	if messages[0].Content != "New message" {
		t.Errorf("expected 'New message', got '%s'", messages[0].Content)
	}
}

func TestChatHistory_Clear(t *testing.T) {
	ch := NewChatHistory(10)

	ch.AddMessage("user", "Test")
	ch.AddMessage("assistant", "Response")

	if ch.IsEmpty() {
		t.Error("ChatHistory should not be empty after adding messages")
	}

	ch.Clear()

	if !ch.IsEmpty() {
		t.Error("ChatHistory should be empty after Clear()")
	}

	if ch.Len() != 0 {
		t.Errorf("expected Len() to be 0 after Clear(), got %d", ch.Len())
	}
}

func TestChatHistory_ConcurrentAccess(t *testing.T) {
	ch := NewChatHistory(100)

	// Concurrent writes
	done := make(chan bool, 10)
	for i := range 10 {
		go func(_ int) {
			for range 10 {
				ch.AddMessage("user", "Message")
			}
			done <- true
		}(i)
	}

	// Wait for all goroutines
	for range 10 {
		<-done
	}

	// Should have 100 messages (or less if ring buffer evicted some)
	if ch.Len() != 100 {
		t.Errorf("expected 100 messages after concurrent writes, got %d", ch.Len())
	}

	// Concurrent reads and writes
	done = make(chan bool, 20)
	for i := range 10 {
		go func(_ int) {
			for range 10 {
				ch.AddMessage("assistant", "Response")
			}
			done <- true
		}(i)
		go func(_ int) {
			for range 10 {
				_ = ch.GetMessages()
			}
			done <- true
		}(i)
	}

	// Wait for all goroutines
	for range 20 {
		<-done
	}

	// Should still have 100 messages (max capacity)
	if ch.Len() != 100 {
		t.Errorf("expected 100 messages (max capacity), got %d", ch.Len())
	}
}

func TestChatMessage_Timestamp(t *testing.T) {
	ch := NewChatHistory(10)

	before := time.Now()
	ch.AddMessage("user", "Test")
	after := time.Now()

	messages := ch.GetMessages()
	if len(messages) != 1 {
		t.Fatal("expected 1 message")
	}

	msg := messages[0]
	if msg.Timestamp.Before(before) || msg.Timestamp.After(after) {
		t.Error("message timestamp should be between before and after")
	}
}

// TestTaskChatHistory_Integration tests the complete chat history flow on Task struct
func TestTaskChatHistory_Integration(t *testing.T) {
	task := &Task{
		Issue: github.Issue{
			Number: 123,
			Title:  "Test Issue",
		},
		Status: StatusCoding,
	}

	// Add some chat messages to the task
	task.AddChatMessage("user", "Implement feature X")
	task.AddChatMessage("assistant", "I'll help you implement feature X")
	task.AddChatMessage("user", "Make sure to add tests")
	task.SetSessionID("test-session-123")

	// Test that GetChatMessages returns the stored messages
	messages := task.GetChatMessages()
	if len(messages) != 3 {
		t.Errorf("expected 3 chat messages, got %d", len(messages))
	}

	// Verify message content
	if messages[0].Role != "user" || messages[0].Content != "Implement feature X" {
		t.Errorf("first message mismatch: got role=%s, content=%s", messages[0].Role, messages[0].Content)
	}

	if messages[1].Role != "assistant" || messages[1].Content != "I'll help you implement feature X" {
		t.Errorf("second message mismatch: got role=%s, content=%s", messages[1].Role, messages[1].Content)
	}

	// Test that chat history is cleared when session ends
	task.SetSessionID("")
	messagesAfterClear := task.GetChatMessages()
	if len(messagesAfterClear) != 0 {
		t.Errorf("expected 0 messages after session clear, got %d", len(messagesAfterClear))
	}
}

// TestTaskChatHistory_ConcurrentAccess tests thread-safe concurrent access to chat history
func TestTaskChatHistory_ConcurrentAccess(t *testing.T) {
	task := &Task{
		Issue: github.Issue{
			Number: 456,
			Title:  "Concurrent Test Issue",
		},
		Status: StatusCoding,
	}

	// Add messages concurrently
	var wg sync.WaitGroup
	for i := range 10 {
		wg.Add(1)
		go func() {
			defer wg.Done()
			for j := range 10 {
				role := "user"
				if j%2 == 0 {
					role = "assistant"
				}
				task.AddChatMessage(role, fmt.Sprintf("Message %d-%d", i, j))
			}
		}()
	}

	// Read messages concurrently
	for range 5 {
		wg.Go(func() {
			for range 20 {
				_ = task.GetChatMessages()
			}
		})
	}

	wg.Wait()

	// Verify we have all messages (100 total, but capped at 1000 max)
	messages := task.GetChatMessages()
	if len(messages) != 100 {
		t.Errorf("expected 100 messages after concurrent writes, got %d", len(messages))
	}
}

// TestTaskChatHistory_MaxSize tests that chat history respects max size limit
func TestTaskChatHistory_MaxSize(t *testing.T) {
	task := &Task{
		Issue: github.Issue{
			Number: 789,
			Title:  "Max Size Test Issue",
		},
		Status: StatusCoding,
	}

	// Add more than 1000 messages (the default max)
	for i := range 1100 {
		task.AddChatMessage("user", fmt.Sprintf("Message %d", i))
	}

	messages := task.GetChatMessages()
	if len(messages) != 1000 {
		t.Errorf("expected 1000 messages (max capacity), got %d", len(messages))
	}

	// Verify oldest messages were evicted (ring buffer behavior)
	// First message should be Message 100 (Message 0-99 were evicted)
	if messages[0].Content != "Message 100" {
		t.Errorf("expected first message to be 'Message 100' (oldest evicted), got '%s'", messages[0].Content)
	}

	// Last message should be Message 1099
	if messages[999].Content != "Message 1099" {
		t.Errorf("expected last message to be 'Message 1099', got '%s'", messages[999].Content)
	}
}
