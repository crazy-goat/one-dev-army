package dashboard

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/gorilla/websocket"
)

func TestNewHub(t *testing.T) {
	hub := NewHub()
	if hub == nil {
		t.Fatal("NewHub() returned nil")
	}
	if hub.maxClients != defaultConnectionLimit {
		t.Errorf("Expected maxClients to be %d, got %d", defaultConnectionLimit, hub.maxClients)
	}
	if hub.closed {
		t.Error("New hub should not be closed")
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
			hub := NewHubWithLimit(tt.limit)
			if hub.maxClients != tt.expected {
				t.Errorf("Expected maxClients to be %d, got %d", tt.expected, hub.maxClients)
			}
		})
	}
}

func TestHubClientCount(t *testing.T) {
	hub := NewHub()
	go hub.Run()
	defer hub.Stop()

	// Initially should be 0
	if count := hub.ClientCount(); count != 0 {
		t.Errorf("Expected 0 clients, got %d", count)
	}
}

func TestHubRegisterUnregister(t *testing.T) {
	hub := NewHub()
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

	// Wait for registration
	time.Sleep(100 * time.Millisecond)

	if count := hub.ClientCount(); count != 1 {
		t.Errorf("Expected 1 client, got %d", count)
	}

	// Close connection to trigger unregistration
	conn.Close()

	// Wait for unregistration
	time.Sleep(100 * time.Millisecond)

	if count := hub.ClientCount(); count != 0 {
		t.Errorf("Expected 0 clients after disconnect, got %d", count)
	}
}

func TestHubBroadcast(t *testing.T) {
	hub := NewHub()
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

	// Wait for registration
	time.Sleep(100 * time.Millisecond)

	// Broadcast a message
	testMsg := []byte(`{"type":"test","payload":"hello"}`)
	hub.Broadcast(testMsg)

	// Wait for message delivery
	time.Sleep(100 * time.Millisecond)

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
	hub := NewHub()
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

	// Wait for registration
	time.Sleep(100 * time.Millisecond)

	// Broadcast issue update
	hub.BroadcastIssueUpdate(42, "Test Issue", "open", "Backlog")

	// Wait for message delivery
	time.Sleep(100 * time.Millisecond)

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

	if payload.IssueNumber != 42 {
		t.Errorf("Expected issue number 42, got %d", payload.IssueNumber)
	}
	if payload.Title != "Test Issue" {
		t.Errorf("Expected title 'Test Issue', got %s", payload.Title)
	}
	if payload.Column != "Backlog" {
		t.Errorf("Expected column 'Backlog', got %s", payload.Column)
	}
}

func TestHubBroadcastSyncComplete(t *testing.T) {
	hub := NewHub()
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

	// Wait for registration
	time.Sleep(100 * time.Millisecond)

	// Broadcast sync complete
	hub.BroadcastSyncComplete(true, "Sprint 1", "")

	// Wait for message delivery
	time.Sleep(100 * time.Millisecond)

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

	if !payload.Success {
		t.Error("Expected success to be true")
	}
	if payload.Milestone != "Sprint 1" {
		t.Errorf("Expected milestone 'Sprint 1', got %s", payload.Milestone)
	}
}

func TestHubConnectionLimit(t *testing.T) {
	hub := NewHubWithLimit(2)
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

	// Wait for registration
	time.Sleep(100 * time.Millisecond)

	if count := hub.ClientCount(); count != 2 {
		t.Errorf("Expected 2 clients, got %d", count)
	}

	// Try to connect a third client (should fail or be rejected)
	conn, _, err := websocket.DefaultDialer.Dial(wsURL, nil)
	if err == nil {
		// If connection succeeded, it should be closed immediately
		conn.Close()
	}

	// Wait a bit for any rejection to process
	time.Sleep(100 * time.Millisecond)

	// Should still have only 2 clients
	if count := hub.ClientCount(); count != 2 {
		t.Errorf("Expected still 2 clients after rejected connection, got %d", count)
	}
}

func TestHubConcurrentClients(t *testing.T) {
	hub := NewHub()
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

	// Wait for all registrations
	time.Sleep(100 * time.Millisecond)

	if count := hub.ClientCount(); count != numClients {
		t.Errorf("Expected %d clients, got %d", numClients, count)
	}

	// Close all connections
	for _, conn := range conns {
		if conn != nil {
			conn.Close()
		}
	}

	// Wait for unregistrations
	time.Sleep(100 * time.Millisecond)

	if count := hub.ClientCount(); count != 0 {
		t.Errorf("Expected 0 clients after disconnect, got %d", count)
	}
}

func TestHubStop(t *testing.T) {
	hub := NewHub()
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

	// Wait for registration
	time.Sleep(100 * time.Millisecond)

	if count := hub.ClientCount(); count != 3 {
		t.Errorf("Expected 3 clients, got %d", count)
	}

	// Stop the hub
	hub.Stop()

	// Wait for stop to process
	time.Sleep(100 * time.Millisecond)

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
		IssueNumber: 123,
		Title:       "Test Title",
		Status:      "open",
		Column:      "In Progress",
	}

	data, err := json.Marshal(payload)
	if err != nil {
		t.Fatalf("Failed to marshal payload: %v", err)
	}

	var decoded IssueUpdatePayload
	if err := json.Unmarshal(data, &decoded); err != nil {
		t.Fatalf("Failed to unmarshal payload: %v", err)
	}

	if decoded.IssueNumber != payload.IssueNumber {
		t.Errorf("IssueNumber mismatch: got %d, want %d", decoded.IssueNumber, payload.IssueNumber)
	}
	if decoded.Title != payload.Title {
		t.Errorf("Title mismatch: got %s, want %s", decoded.Title, payload.Title)
	}
	if decoded.Status != payload.Status {
		t.Errorf("Status mismatch: got %s, want %s", decoded.Status, payload.Status)
	}
	if decoded.Column != payload.Column {
		t.Errorf("Column mismatch: got %s, want %s", decoded.Column, payload.Column)
	}
}

func TestSyncCompletePayloadMarshal(t *testing.T) {
	payload := SyncCompletePayload{
		Success:   false,
		Milestone: "",
		Error:     "connection failed",
	}

	data, err := json.Marshal(payload)
	if err != nil {
		t.Fatalf("Failed to marshal payload: %v", err)
	}

	var decoded SyncCompletePayload
	if err := json.Unmarshal(data, &decoded); err != nil {
		t.Fatalf("Failed to unmarshal payload: %v", err)
	}

	if decoded.Success != payload.Success {
		t.Errorf("Success mismatch: got %v, want %v", decoded.Success, payload.Success)
	}
	if decoded.Error != payload.Error {
		t.Errorf("Error mismatch: got %s, want %s", decoded.Error, payload.Error)
	}
}

func TestClientPingPong(t *testing.T) {
	hub := NewHub()
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

	// Wait for registration
	time.Sleep(100 * time.Millisecond)

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
