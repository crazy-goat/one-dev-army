package mvp

import (
	"testing"
	"time"
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
	for i := 0; i < 10; i++ {
		go func(id int) {
			for j := 0; j < 10; j++ {
				ch.AddMessage("user", "Message")
			}
			done <- true
		}(i)
	}

	// Wait for all goroutines
	for i := 0; i < 10; i++ {
		<-done
	}

	// Should have 100 messages (or less if ring buffer evicted some)
	if ch.Len() != 100 {
		t.Errorf("expected 100 messages after concurrent writes, got %d", ch.Len())
	}

	// Concurrent reads and writes
	done = make(chan bool, 20)
	for i := 0; i < 10; i++ {
		go func(id int) {
			for j := 0; j < 10; j++ {
				ch.AddMessage("assistant", "Response")
			}
			done <- true
		}(i)
		go func(id int) {
			for j := 0; j < 10; j++ {
				_ = ch.GetMessages()
			}
			done <- true
		}(i)
	}

	// Wait for all goroutines
	for i := 0; i < 20; i++ {
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
