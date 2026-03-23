package dashboard

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/crazy-goat/one-dev-army/internal/github"
	"github.com/gorilla/websocket"
)

func TestNewHub(t *testing.T) {
	hub := NewHub(false)
	if hub == nil {
		t.Fatal("NewHub() returned nil")
	}
	if hub.maxClients != defaultConnectionLimit {
		t.Errorf("Expected maxClients to be %d, got %d", defaultConnectionLimit, hub.maxClients)
	}
	if hub.closed {
		t.Error("New hub should not be closed")
	}
	if hub.debug {
		t.Error("New hub should have debug disabled by default")
	}
}

func TestNewHubWithLimit(t *testing.T) {
	tests := []struct {
		name     string
		limit    int
		expected int
	}{
		{"positive limit", 50, 50},
		{"zero limit", 0, defaultConnectionLimit},
		{"negative limit", -1, defaultConnectionLimit},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			hub := NewHubWithLimit(tt.limit, false)
			if hub.maxClients != tt.expected {
				t.Errorf("Expected maxClients to be %d, got %d", tt.expected, hub.maxClients)
			}
		})
	}
}

func TestHubClientCount(t *testing.T) {
	hub := NewHub(false)
	go hub.Run()
	defer hub.Stop()

	// Initially should be 0
	if count := hub.ClientCount(); count != 0 {
		t.Errorf("Expected 0 clients, got %d", count)
	}
}

func TestHubRegisterUnregister(t *testing.T) {
	hub := NewHub(false)
	go hub.Run()
	defer hub.Stop()

	// Create a test server
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		ServeWs(hub, w, r)
	}))
	defer server.Close()

	// Connect a client
	wsURL := "ws" + strings.TrimPrefix(server.URL, "http")
	conn, _, err := websocket.DefaultDialer.Dial(wsURL, nil)
	if err != nil {
		t.Fatalf("Failed to connect: %v", err)
	}
	defer conn.Close()

	// Wait for registration using polling
	waitForClientCount(t, hub, 1, time.Second)

	if count := hub.ClientCount(); count != 1 {
		t.Errorf("Expected 1 client, got %d", count)
	}

	// Close connection to trigger unregistration
	conn.Close()

	// Wait for unregistration using polling
	waitForClientCount(t, hub, 0, time.Second)

	if count := hub.ClientCount(); count != 0 {
		t.Errorf("Expected 0 clients after disconnect, got %d", count)
	}
}

func TestHubBroadcast(t *testing.T) {
	hub := NewHub(false)
	go hub.Run()
	defer hub.Stop()

	// Create a test server
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		ServeWs(hub, w, r)
	}))
	defer server.Close()

	// Connect multiple clients
	wsURL := "ws" + strings.TrimPrefix(server.URL, "http")
	var conns []*websocket.Conn
	for i := 0; i < 3; i++ {
		conn, _, err := websocket.DefaultDialer.Dial(wsURL, nil)
		if err != nil {
			t.Fatalf("Failed to connect client %d: %v", i, err)
		}
		conns = append(conns, conn)
		defer conn.Close()
	}

	// Wait for registration using polling
	waitForClientCount(t, hub, 3, time.Second)

	// Give clients time to start their read pumps
	time.Sleep(50 * time.Millisecond)

	// Broadcast a message
	testMsg := []byte(`{"type":"test","payload":"hello"}`)
	hub.Broadcast(testMsg)

	// Verify all clients received the message
	for i, conn := range conns {
		conn.SetReadDeadline(time.Now().Add(time.Second))
		_, msg, err := conn.ReadMessage()
		if err != nil {
			t.Errorf("Client %d failed to receive message: %v", i, err)
			continue
		}
		if string(msg) != string(testMsg) {
			t.Errorf("Client %d received wrong message: got %s, want %s", i, string(msg), string(testMsg))
		}
	}
}

func TestHubBroadcastIssueUpdate(t *testing.T) {
	hub := NewHub(false)
	go hub.Run()
	defer hub.Stop()

	// Create a test server
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		ServeWs(hub, w, r)
	}))
	defer server.Close()

	// Connect a client
	wsURL := "ws" + strings.TrimPrefix(server.URL, "http")
	conn, _, err := websocket.DefaultDialer.Dial(wsURL, nil)
	if err != nil {
		t.Fatalf("Failed to connect: %v", err)
	}
	defer conn.Close()

	// Wait for registration using polling
	waitForClientCount(t, hub, 1, time.Second)

	// Broadcast issue update
	issue := github.Issue{
		Number: 42,
		Title:  "Test Issue",
		State:  "open",
	}
	hub.BroadcastIssueUpdate(issue)

	// Verify client received the message
	conn.SetReadDeadline(time.Now().Add(time.Second))
	_, msg, err := conn.ReadMessage()
	if err != nil {
		t.Fatalf("Failed to receive message: %v", err)
	}

	var received Message
	if err := json.Unmarshal(msg, &received); err != nil {
		t.Fatalf("Failed to unmarshal message: %v", err)
	}

	if received.Type != MessageTypeIssueUpdate {
		t.Errorf("Expected type %s, got %s", MessageTypeIssueUpdate, received.Type)
	}

	var payload IssueUpdatePayload
	if err := json.Unmarshal(received.Payload, &payload); err != nil {
		t.Fatalf("Failed to unmarshal payload: %v", err)
	}

	if payload.Number != 42 {
		t.Errorf("Expected issue number 42, got %d", payload.Number)
	}
	if payload.Title != "Test Issue" {
		t.Errorf("Expected title 'Test Issue', got %s", payload.Title)
	}
	if payload.State != "open" {
		t.Errorf("Expected state 'open', got %s", payload.State)
	}
}

func TestHubBroadcastSyncComplete(t *testing.T) {
	hub := NewHub(false)
	go hub.Run()
	defer hub.Stop()

	// Create a test server
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		ServeWs(hub, w, r)
	}))
	defer server.Close()

	// Connect a client
	wsURL := "ws" + strings.TrimPrefix(server.URL, "http")
	conn, _, err := websocket.DefaultDialer.Dial(wsURL, nil)
	if err != nil {
		t.Fatalf("Failed to connect: %v", err)
	}
	defer conn.Close()

	// Wait for registration using polling
	waitForClientCount(t, hub, 1, time.Second)

	// Broadcast sync complete
	hub.BroadcastSyncComplete(10)

	// Verify client received the message
	conn.SetReadDeadline(time.Now().Add(time.Second))
	_, msg, err := conn.ReadMessage()
	if err != nil {
		t.Fatalf("Failed to receive message: %v", err)
	}

	var received Message
	if err := json.Unmarshal(msg, &received); err != nil {
		t.Fatalf("Failed to unmarshal message: %v", err)
	}

	if received.Type != MessageTypeSyncComplete {
		t.Errorf("Expected type %s, got %s", MessageTypeSyncComplete, received.Type)
	}

	var payload SyncCompletePayload
	if err := json.Unmarshal(received.Payload, &payload); err != nil {
		t.Fatalf("Failed to unmarshal payload: %v", err)
	}

	if payload.Count != 10 {
		t.Errorf("Expected count 10, got %d", payload.Count)
	}
}

func TestHubBroadcastWorkerUpdate(t *testing.T) {
	hub := NewHub(false)
	go hub.Run()
	defer hub.Stop()

	// Create a test server
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		ServeWs(hub, w, r)
	}))
	defer server.Close()

	// Connect a client
	wsURL := "ws" + strings.TrimPrefix(server.URL, "http")
	conn, _, err := websocket.DefaultDialer.Dial(wsURL, nil)
	if err != nil {
		t.Fatalf("Failed to connect: %v", err)
	}
	defer conn.Close()

	// Wait for registration using polling
	waitForClientCount(t, hub, 1, time.Second)

	// Broadcast worker update
	hub.BroadcastWorkerUpdate("worker-1", "processing", 42, "Test Issue", "Code", 120)

	// Verify client received the message
	conn.SetReadDeadline(time.Now().Add(time.Second))
	_, msg, err := conn.ReadMessage()
	if err != nil {
		t.Fatalf("Failed to receive message: %v", err)
	}

	var received Message
	if err := json.Unmarshal(msg, &received); err != nil {
		t.Fatalf("Failed to unmarshal message: %v", err)
	}

	if received.Type != MessageTypeWorkerUpdate {
		t.Errorf("Expected type %s, got %s", MessageTypeWorkerUpdate, received.Type)
	}

	var payload WorkerUpdatePayload
	if err := json.Unmarshal(received.Payload, &payload); err != nil {
		t.Fatalf("Failed to unmarshal payload: %v", err)
	}

	if payload.WorkerID != "worker-1" {
		t.Errorf("Expected worker_id 'worker-1', got %s", payload.WorkerID)
	}
	if payload.Status != "processing" {
		t.Errorf("Expected status 'processing', got %s", payload.Status)
	}
	if payload.TaskID != 42 {
		t.Errorf("Expected task_id 42, got %d", payload.TaskID)
	}
	if payload.TaskTitle != "Test Issue" {
		t.Errorf("Expected task_title 'Test Issue', got %s", payload.TaskTitle)
	}
	if payload.Stage != "Code" {
		t.Errorf("Expected stage 'Code', got %s", payload.Stage)
	}
	if payload.ElapsedSeconds != 120 {
		t.Errorf("Expected elapsed_seconds 120, got %d", payload.ElapsedSeconds)
	}
}

func TestWorkerUpdatePayloadMarshal(t *testing.T) {
	payload := WorkerUpdatePayload{
		WorkerID:       "worker-1",
		Status:         "processing",
		TaskID:         42,
		TaskTitle:      "Test Issue",
		Stage:          "Code",
		ElapsedSeconds: 120,
	}

	data, err := json.Marshal(payload)
	if err != nil {
		t.Fatalf("Failed to marshal payload: %v", err)
	}

	var decoded WorkerUpdatePayload
	if err := json.Unmarshal(data, &decoded); err != nil {
		t.Fatalf("Failed to unmarshal payload: %v", err)
	}

	if decoded.WorkerID != payload.WorkerID {
		t.Errorf("WorkerID mismatch: got %s, want %s", decoded.WorkerID, payload.WorkerID)
	}
	if decoded.Status != payload.Status {
		t.Errorf("Status mismatch: got %s, want %s", decoded.Status, payload.Status)
	}
	if decoded.TaskID != payload.TaskID {
		t.Errorf("TaskID mismatch: got %d, want %d", decoded.TaskID, payload.TaskID)
	}
	if decoded.TaskTitle != payload.TaskTitle {
		t.Errorf("TaskTitle mismatch: got %s, want %s", decoded.TaskTitle, payload.TaskTitle)
	}
	if decoded.Stage != payload.Stage {
		t.Errorf("Stage mismatch: got %s, want %s", decoded.Stage, payload.Stage)
	}
	if decoded.ElapsedSeconds != payload.ElapsedSeconds {
		t.Errorf("ElapsedSeconds mismatch: got %d, want %d", decoded.ElapsedSeconds, payload.ElapsedSeconds)
	}
}

func TestHubConnectionLimit(t *testing.T) {
	hub := NewHubWithLimit(2, false)
	go hub.Run()
	defer hub.Stop()

	// Create a test server
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		ServeWs(hub, w, r)
	}))
	defer server.Close()

	wsURL := "ws" + strings.TrimPrefix(server.URL, "http")

	// Connect clients up to the limit
	var conns []*websocket.Conn
	for i := 0; i < 2; i++ {
		conn, _, err := websocket.DefaultDialer.Dial(wsURL, nil)
		if err != nil {
			t.Fatalf("Failed to connect client %d: %v", i, err)
		}
		conns = append(conns, conn)
		defer conn.Close()
	}

	// Wait for registration using polling
	waitForClientCount(t, hub, 2, time.Second)

	if count := hub.ClientCount(); count != 2 {
		t.Errorf("Expected 2 clients, got %d", count)
	}

	// Try to connect a third client (should fail or be rejected)
	conn, _, err := websocket.DefaultDialer.Dial(wsURL, nil)
	if err == nil {
		// If connection succeeded, it should be closed immediately
		conn.Close()
	}

	// Wait a bit for any rejection to process using polling
	start := time.Now()
	for time.Since(start) < 200*time.Millisecond {
		if hub.ClientCount() == 2 {
			break
		}
		time.Sleep(10 * time.Millisecond)
	}

	// Should still have only 2 clients
	if count := hub.ClientCount(); count != 2 {
		t.Errorf("Expected still 2 clients after rejected connection, got %d", count)
	}
}

func TestHubConcurrentClients(t *testing.T) {
	hub := NewHub(false)
	go hub.Run()
	defer hub.Stop()

	// Create a test server
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		ServeWs(hub, w, r)
	}))
	defer server.Close()

	wsURL := "ws" + strings.TrimPrefix(server.URL, "http")

	// Concurrently connect multiple clients
	var wg sync.WaitGroup
	numClients := 10
	conns := make([]*websocket.Conn, numClients)

	for i := 0; i < numClients; i++ {
		wg.Add(1)
		go func(idx int) {
			defer wg.Done()
			conn, _, err := websocket.DefaultDialer.Dial(wsURL, nil)
			if err != nil {
				t.Errorf("Failed to connect client %d: %v", idx, err)
				return
			}
			conns[idx] = conn
		}(i)
	}

	wg.Wait()

	// Wait for all registrations using polling
	waitForClientCount(t, hub, numClients, time.Second)

	if count := hub.ClientCount(); count != numClients {
		t.Errorf("Expected %d clients, got %d", numClients, count)
	}

	// Close all connections
	for _, conn := range conns {
		if conn != nil {
			conn.Close()
		}
	}

	// Wait for unregistrations using polling
	waitForClientCount(t, hub, 0, time.Second)

	if count := hub.ClientCount(); count != 0 {
		t.Errorf("Expected 0 clients after disconnect, got %d", count)
	}
}

func TestHubStop(t *testing.T) {
	hub := NewHub(false)
	go hub.Run()

	// Create a test server
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		ServeWs(hub, w, r)
	}))
	defer server.Close()

	wsURL := "ws" + strings.TrimPrefix(server.URL, "http")

	// Connect some clients
	var conns []*websocket.Conn
	for i := 0; i < 3; i++ {
		conn, _, err := websocket.DefaultDialer.Dial(wsURL, nil)
		if err != nil {
			t.Fatalf("Failed to connect client %d: %v", i, err)
		}
		conns = append(conns, conn)
	}

	// Wait for registration using polling
	waitForClientCount(t, hub, 3, time.Second)

	if count := hub.ClientCount(); count != 3 {
		t.Errorf("Expected 3 clients, got %d", count)
	}

	// Stop the hub
	hub.Stop()

	// Wait for stop to process using polling
	waitForClientCount(t, hub, 0, time.Second)

	if count := hub.ClientCount(); count != 0 {
		t.Errorf("Expected 0 clients after stop, got %d", count)
	}

	// Verify connections are closed
	for i, conn := range conns {
		conn.SetReadDeadline(time.Now().Add(100 * time.Millisecond))
		_, _, err := conn.ReadMessage()
		if err == nil {
			t.Errorf("Client %d connection should be closed", i)
		}
	}
}

func TestMessageTypes(t *testing.T) {
	tests := []struct {
		msgType  MessageType
		expected string
	}{
		{MessageTypeIssueUpdate, "issue_update"},
		{MessageTypeSyncComplete, "sync_complete"},
		{MessageTypeWorkerUpdate, "worker_update"},
		{MessageTypePing, "ping"},
		{MessageTypePong, "pong"},
	}

	for _, tt := range tests {
		t.Run(string(tt.msgType), func(t *testing.T) {
			if string(tt.msgType) != tt.expected {
				t.Errorf("Expected %s, got %s", tt.expected, tt.msgType)
			}
		})
	}
}

func TestIssueUpdatePayloadMarshal(t *testing.T) {
	payload := IssueUpdatePayload{
		Number: 123,
		Title:  "Test Title",
		State:  "open",
		Column: "In Progress",
	}

	data, err := json.Marshal(payload)
	if err != nil {
		t.Fatalf("Failed to marshal payload: %v", err)
	}

	var decoded IssueUpdatePayload
	if err := json.Unmarshal(data, &decoded); err != nil {
		t.Fatalf("Failed to unmarshal payload: %v", err)
	}

	if decoded.Number != payload.Number {
		t.Errorf("Number mismatch: got %d, want %d", decoded.Number, payload.Number)
	}
	if decoded.Title != payload.Title {
		t.Errorf("Title mismatch: got %s, want %s", decoded.Title, payload.Title)
	}
	if decoded.State != payload.State {
		t.Errorf("State mismatch: got %s, want %s", decoded.State, payload.State)
	}
	if decoded.Column != payload.Column {
		t.Errorf("Column mismatch: got %s, want %s", decoded.Column, payload.Column)
	}
}

func TestSyncCompletePayloadMarshal(t *testing.T) {
	payload := SyncCompletePayload{
		Count: 42,
	}

	data, err := json.Marshal(payload)
	if err != nil {
		t.Fatalf("Failed to marshal payload: %v", err)
	}

	var decoded SyncCompletePayload
	if err := json.Unmarshal(data, &decoded); err != nil {
		t.Fatalf("Failed to unmarshal payload: %v", err)
	}

	if decoded.Count != payload.Count {
		t.Errorf("Count mismatch: got %d, want %d", decoded.Count, payload.Count)
	}
}

func TestClientPingPong(t *testing.T) {
	hub := NewHub(false)
	go hub.Run()
	defer hub.Stop()

	// Create a test server
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		ServeWs(hub, w, r)
	}))
	defer server.Close()

	wsURL := "ws" + strings.TrimPrefix(server.URL, "http")
	conn, _, err := websocket.DefaultDialer.Dial(wsURL, nil)
	if err != nil {
		t.Fatalf("Failed to connect: %v", err)
	}
	defer conn.Close()

	// Wait for registration using polling
	waitForClientCount(t, hub, 1, time.Second)

	// Send a ping message
	ping := Message{Type: MessageTypePing}
	pingData, _ := json.Marshal(ping)
	if err := conn.WriteMessage(websocket.TextMessage, pingData); err != nil {
		t.Fatalf("Failed to send ping: %v", err)
	}

	// Wait for pong response
	conn.SetReadDeadline(time.Now().Add(time.Second))
	_, msg, err := conn.ReadMessage()
	if err != nil {
		t.Fatalf("Failed to receive pong: %v", err)
	}

	var received Message
	if err := json.Unmarshal(msg, &received); err != nil {
		t.Fatalf("Failed to unmarshal pong: %v", err)
	}

	if received.Type != MessageTypePong {
		t.Errorf("Expected pong, got %s", received.Type)
	}
}

// waitForClientCount polls the hub until it reaches the expected client count or timeout
func waitForClientCount(t *testing.T, hub *Hub, expected int, timeout time.Duration) {
	t.Helper()
	start := time.Now()
	for {
		if hub.ClientCount() == expected {
			return
		}
		if time.Since(start) > timeout {
			t.Fatalf("Timeout waiting for client count %d, got %d", expected, hub.ClientCount())
		}
		time.Sleep(10 * time.Millisecond)
	}
}
