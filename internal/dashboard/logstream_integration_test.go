package dashboard

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/gorilla/websocket"
)

func TestLogStreamManager_Integration(t *testing.T) {
	// Create temporary log directory
	tempDir := t.TempDir()
	logDir := filepath.Join(tempDir, ".oda", "artifacts", "123", "logs")
	if err := os.MkdirAll(logDir, 0755); err != nil {
		t.Fatalf("Failed to create log directory: %v", err)
	}

	// Create a test log file
	logFile := filepath.Join(logDir, "20260326_120000_test-step.log")
	content := `[2026-03-26 12:00:00] STEP START: test-step
[2026-03-26 12:00:01] Processing item 1
[2026-03-26 12:00:02] Processing item 2
[2026-03-26 12:00:03] STEP END: test-step
`
	if err := os.WriteFile(logFile, []byte(content), 0644); err != nil {
		t.Fatalf("Failed to write log file: %v", err)
	}

	// Create hub and log stream manager
	hub := NewHub(false)
	go hub.Run()
	defer hub.Stop()

	lsm := NewLogStreamManager(hub, tempDir, 50*time.Millisecond)

	// Create test server
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		ServeWs(hub, w, r)
	}))
	defer server.Close()

	// Connect WebSocket client
	wsURL := "ws" + strings.TrimPrefix(server.URL, "http")
	//nolint:bodyclose // WebSocket dial doesn't use standard HTTP response body
	conn, _, err := websocket.DefaultDialer.Dial(wsURL, nil)
	if err != nil {
		t.Fatalf("Failed to connect WebSocket: %v", err)
	}
	defer conn.Close()

	// Wait for registration
	waitForClientCount(t, hub, 1)

	// Start monitoring
	if err := lsm.StartMonitoring(123); err != nil {
		t.Fatalf("Failed to start monitoring: %v", err)
	}

	// Give the log stream manager time to poll and broadcast (at least 2 poll cycles)
	time.Sleep(150 * time.Millisecond)

	// Stop monitoring before reading from websocket
	lsm.StopMonitoring()

	// Read messages from websocket with timeout
	var receivedLogs []LogStreamPayload
	readDone := make(chan bool)

	go func() {
		defer close(readDone)
		for {
			conn.SetReadDeadline(time.Now().Add(500 * time.Millisecond))
			_, msg, err := conn.ReadMessage()
			if err != nil {
				return
			}

			var message Message
			if err := json.Unmarshal(msg, &message); err != nil {
				continue
			}

			if message.Type == MessageTypeLogStream {
				var payload LogStreamPayload
				if err := json.Unmarshal(message.Payload, &payload); err != nil {
					continue
				}
				receivedLogs = append(receivedLogs, payload)
				if len(receivedLogs) >= 3 {
					return
				}
			}
		}
	}()

	// Wait for reading to complete or timeout
	select {
	case <-readDone:
		// Successfully read messages
	case <-time.After(2 * time.Second):
		// Timeout - that's okay, we might have received some messages
	}

	// Verify received logs (we expect at least 1, but may not get all 3 due to timing)
	if len(receivedLogs) == 0 {
		t.Error("Expected at least 1 log entry, got 0")
	}

	// Verify log content
	for i, log := range receivedLogs {
		if log.IssueNumber != 123 {
			t.Errorf("Log %d: Expected issue number 123, got %d", i, log.IssueNumber)
		}
		if log.Step != "test-step" {
			t.Errorf("Log %d: Expected step 'test-step', got %s", i, log.Step)
		}
		if log.Timestamp == "" {
			t.Errorf("Log %d: Expected non-empty timestamp", i)
		}
	}
}

func TestLogStreamManager_TaskSwitching(t *testing.T) {
	tempDir := t.TempDir()

	// Create log directories for two issues
	logDir1 := filepath.Join(tempDir, ".oda", "artifacts", "100", "logs")
	logDir2 := filepath.Join(tempDir, ".oda", "artifacts", "200", "logs")
	os.MkdirAll(logDir1, 0755)
	os.MkdirAll(logDir2, 0755)

	// Create log files
	os.WriteFile(filepath.Join(logDir1, "step1.log"), []byte("[2026-03-26 12:00:00] Issue 100 log\n"), 0644)
	os.WriteFile(filepath.Join(logDir2, "step2.log"), []byte("[2026-03-26 12:00:00] Issue 200 log\n"), 0644)

	hub := NewHub(false)
	lsm := NewLogStreamManager(hub, tempDir, 0)

	// Start monitoring first issue
	if err := lsm.StartMonitoring(100); err != nil {
		t.Fatalf("Failed to start monitoring: %v", err)
	}

	if !lsm.IsMonitoring() {
		t.Error("Expected monitoring to be active")
	}

	if lsm.GetCurrentIssue() != 100 {
		t.Errorf("Expected current issue 100, got %d", lsm.GetCurrentIssue())
	}

	// Switch to second issue
	if err := lsm.StartMonitoring(200); err != nil {
		t.Fatalf("Failed to switch monitoring: %v", err)
	}

	if lsm.GetCurrentIssue() != 200 {
		t.Errorf("Expected current issue 200, got %d", lsm.GetCurrentIssue())
	}

	// Stop monitoring
	lsm.StopMonitoring()

	if lsm.IsMonitoring() {
		t.Error("Expected monitoring to be stopped")
	}
}

func TestBroadcastLogStream(t *testing.T) {
	hub := NewHub(false)
	go hub.Run()
	defer hub.Stop()

	// Create test server
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		ServeWs(hub, w, r)
	}))
	defer server.Close()

	// Connect WebSocket client
	wsURL := "ws" + strings.TrimPrefix(server.URL, "http")
	//nolint:bodyclose // WebSocket dial doesn't use standard HTTP response body
	conn, _, err := websocket.DefaultDialer.Dial(wsURL, nil)
	if err != nil {
		t.Fatalf("Failed to connect WebSocket: %v", err)
	}
	defer conn.Close()

	// Wait for registration
	waitForClientCount(t, hub, 1)

	// Broadcast a log stream message
	hub.BroadcastLogStream(123, "test-step", "2026-03-26 12:00:00", "Test message", "info", "test.log")

	// Read the message
	conn.SetReadDeadline(time.Now().Add(1 * time.Second))
	_, msg, err := conn.ReadMessage()
	if err != nil {
		t.Fatalf("Failed to read message: %v", err)
	}

	var message Message
	if err := json.Unmarshal(msg, &message); err != nil {
		t.Fatalf("Failed to unmarshal message: %v", err)
	}

	if message.Type != MessageTypeLogStream {
		t.Errorf("Expected message type %s, got %s", MessageTypeLogStream, message.Type)
	}

	var payload LogStreamPayload
	if err := json.Unmarshal(message.Payload, &payload); err != nil {
		t.Fatalf("Failed to unmarshal payload: %v", err)
	}

	if payload.IssueNumber != 123 {
		t.Errorf("Expected issue number 123, got %d", payload.IssueNumber)
	}

	if payload.Step != "test-step" {
		t.Errorf("Expected step 'test-step', got %s", payload.Step)
	}

	if payload.Message != "Test message" {
		t.Errorf("Expected message 'Test message', got %s", payload.Message)
	}

	if payload.Level != "info" {
		t.Errorf("Expected level 'info', got %s", payload.Level)
	}

	if payload.File != "test.log" {
		t.Errorf("Expected file 'test.log', got %s", payload.File)
	}
}

func TestLogStreamPayload_Marshaling(t *testing.T) {
	payload := LogStreamPayload{
		IssueNumber: 123,
		Step:        "test-step",
		Timestamp:   "2026-03-26 12:00:00",
		Message:     "Test message",
		Level:       "info",
		File:        "test.log",
	}

	data, err := json.Marshal(payload)
	if err != nil {
		t.Fatalf("Failed to marshal payload: %v", err)
	}

	var decoded LogStreamPayload
	if err := json.Unmarshal(data, &decoded); err != nil {
		t.Fatalf("Failed to unmarshal payload: %v", err)
	}

	if decoded.IssueNumber != payload.IssueNumber {
		t.Errorf("IssueNumber mismatch: expected %d, got %d", payload.IssueNumber, decoded.IssueNumber)
	}

	if decoded.Step != payload.Step {
		t.Errorf("Step mismatch: expected %s, got %s", payload.Step, decoded.Step)
	}

	if decoded.Timestamp != payload.Timestamp {
		t.Errorf("Timestamp mismatch: expected %s, got %s", payload.Timestamp, decoded.Timestamp)
	}

	if decoded.Message != payload.Message {
		t.Errorf("Message mismatch: expected %s, got %s", payload.Message, decoded.Message)
	}

	if decoded.Level != payload.Level {
		t.Errorf("Level mismatch: expected %s, got %s", payload.Level, decoded.Level)
	}

	if decoded.File != payload.File {
		t.Errorf("File mismatch: expected %s, got %s", payload.File, decoded.File)
	}
}

func TestMessageTypeLogStream_Constant(t *testing.T) {
	if MessageTypeLogStream != "log_stream" {
		t.Errorf("Expected MessageTypeLogStream to be 'log_stream', got %s", MessageTypeLogStream)
	}
}
