package mvp

import (
	"sync"
	"time"
)

// ChatMessage represents a single message in the chat history
type ChatMessage struct {
	Role      string    `json:"role"`      // "user" or "assistant"
	Content   string    `json:"content"`   // Message content
	Timestamp time.Time `json:"timestamp"` // When the message was sent
}

// ChatHistory stores chat messages for a task with thread-safe access
// It maintains a ring buffer with a maximum size to prevent unbounded memory growth
type ChatHistory struct {
	mu       sync.RWMutex
	messages []ChatMessage
	maxSize  int
}

// NewChatHistory creates a new ChatHistory with the specified maximum size
func NewChatHistory(maxSize int) *ChatHistory {
	if maxSize <= 0 {
		maxSize = 1000 // Default max size
	}
	return &ChatHistory{
		messages: make([]ChatMessage, 0, maxSize),
		maxSize:  maxSize,
	}
}

// AddMessage adds a new message to the chat history
// If the buffer is full, the oldest message is removed (FIFO)
func (ch *ChatHistory) AddMessage(role, content string) {
	ch.mu.Lock()
	defer ch.mu.Unlock()

	msg := ChatMessage{
		Role:      role,
		Content:   content,
		Timestamp: time.Now(),
	}

	// If at capacity, remove oldest message (index 0)
	if len(ch.messages) >= ch.maxSize {
		ch.messages = ch.messages[1:]
	}

	ch.messages = append(ch.messages, msg)
}

// GetMessages returns a copy of all messages in the chat history
func (ch *ChatHistory) GetMessages() []ChatMessage {
	ch.mu.RLock()
	defer ch.mu.RUnlock()

	// Return a copy to prevent external modification
	result := make([]ChatMessage, len(ch.messages))
	copy(result, ch.messages)
	return result
}

// GetMessagesSince returns messages added after the given timestamp
func (ch *ChatHistory) GetMessagesSince(since time.Time) []ChatMessage {
	ch.mu.RLock()
	defer ch.mu.RUnlock()

	var result []ChatMessage
	for _, msg := range ch.messages {
		if msg.Timestamp.After(since) {
			result = append(result, msg)
		}
	}
	return result
}

// Clear removes all messages from the chat history
func (ch *ChatHistory) Clear() {
	ch.mu.Lock()
	defer ch.mu.Unlock()

	ch.messages = ch.messages[:0]
}

// Len returns the number of messages in the chat history
func (ch *ChatHistory) Len() int {
	ch.mu.RLock()
	defer ch.mu.RUnlock()

	return len(ch.messages)
}

// IsEmpty returns true if the chat history has no messages
func (ch *ChatHistory) IsEmpty() bool {
	ch.mu.RLock()
	defer ch.mu.RUnlock()

	return len(ch.messages) == 0
}
