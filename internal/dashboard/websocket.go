package dashboard

import (
	"encoding/json"
	"fmt"
	"log"
	"net/http"
	"os"
	"strings"
	"sync"
	"time"

	"github.com/crazy-goat/one-dev-army/internal/github"
	"github.com/gorilla/websocket"
)

const (
	// Time allowed to write a message to the peer
	writeWait = 10 * time.Second

	// Time allowed to read the next pong message from the peer
	pongWait = 60 * time.Second

	// Send pings to peer with this period (must be less than pongWait)
	pingPeriod = 30 * time.Second

	// Maximum message size allowed from peer
	maxMessageSize = 512

	// Default connection limit
	defaultConnectionLimit = 100
)

var upgrader = websocket.Upgrader{
	ReadBufferSize:  1024,
	WriteBufferSize: 1024,
	CheckOrigin: func(r *http.Request) bool {
		origin := r.Header.Get("Origin")
		if origin == "" {
			return true
		}

		// Allow localhost origins for development
		if strings.Contains(origin, "localhost") || strings.Contains(origin, "127.0.0.1") {
			return true
		}

		// Check allowed origins from environment variable
		allowedOrigins := os.Getenv("WEBSOCKET_ALLOWED_ORIGINS")
		if allowedOrigins == "" {
			// Default: only allow same-origin requests
			return origin == "" || r.Host == origin
		}

		allowedList := strings.Split(allowedOrigins, ",")
		for _, allowed := range allowedList {
			if strings.TrimSpace(allowed) == origin {
				return true
			}
		}

		log.Printf("[WebSocket] Rejected connection from origin: %s", origin)
		return false
	},
}

// MessageType represents the type of WebSocket message
type MessageType string

const (
	MessageTypeIssueUpdate  MessageType = "issue_update"
	MessageTypeSyncComplete MessageType = "sync_complete"
	MessageTypePing         MessageType = "ping"
	MessageTypePong         MessageType = "pong"
)

// Message represents a WebSocket message
type Message struct {
	Type    MessageType     `json:"type"`
	Payload json.RawMessage `json:"payload,omitempty"`
}

// IssueUpdatePayload represents the payload for issue_update messages
type IssueUpdatePayload struct {
	Number int    `json:"number"`
	Title  string `json:"title"`
	State  string `json:"state"`
	Column string `json:"column"`
}

// SyncCompletePayload represents the payload for sync_complete messages
type SyncCompletePayload struct {
	Count int `json:"count"`
}

// Client represents a single WebSocket connection
type Client struct {
	hub         *Hub
	conn        *websocket.Conn
	send        chan []byte
	id          string
	connectedAt time.Time
}

// Hub maintains the set of active clients and broadcasts messages
type Hub struct {
	clients    map[*Client]bool
	broadcast  chan []byte
	register   chan *Client
	unregister chan *Client
	mu         sync.RWMutex
	maxClients int
	closed     bool
}

// NewHub creates a new Hub instance
func NewHub() *Hub {
	return NewHubWithLimit(defaultConnectionLimit)
}

// NewHubWithLimit creates a new Hub with a specific connection limit
func NewHubWithLimit(limit int) *Hub {
	if limit <= 0 {
		limit = defaultConnectionLimit
	}
	return &Hub{
		clients:    make(map[*Client]bool),
		broadcast:  make(chan []byte),
		register:   make(chan *Client),
		unregister: make(chan *Client),
		maxClients: limit,
		closed:     false,
	}
}

// Run starts the Hub's event loop
func (h *Hub) Run() {
	for {
		select {
		case client := <-h.register:
			h.mu.Lock()
			if h.closed {
				h.mu.Unlock()
				close(client.send)
				continue
			}
			if len(h.clients) >= h.maxClients {
				h.mu.Unlock()
				log.Printf("[WebSocket] Connection limit reached (%d), rejecting client", h.maxClients)
				close(client.send)
				client.conn.Close()
				continue
			}
			h.clients[client] = true
			clientCount := len(h.clients)
			h.mu.Unlock()
			log.Printf("[WebSocket] Client registered. Total clients: %d", clientCount)

		case client := <-h.unregister:
			h.mu.Lock()
			if _, ok := h.clients[client]; ok {
				delete(h.clients, client)
				close(client.send)
				clientCount := len(h.clients)
				h.mu.Unlock()
				log.Printf("[WebSocket] Client unregistered. Total clients: %d", clientCount)
			} else {
				h.mu.Unlock()
			}

		case message := <-h.broadcast:
			h.mu.RLock()
			clients := make([]*Client, 0, len(h.clients))
			for client := range h.clients {
				clients = append(clients, client)
			}
			h.mu.RUnlock()

			for _, client := range clients {
				select {
				case client.send <- message:
				default:
					// Client's send buffer is full, close it
					h.unregister <- client
				}
			}
		}
	}
}

// Stop gracefully shuts down the Hub
func (h *Hub) Stop() {
	h.mu.Lock()
	defer h.mu.Unlock()

	if h.closed {
		return
	}

	h.closed = true

	// Close all client connections
	for client := range h.clients {
		close(client.send)
		client.conn.Close()
		delete(h.clients, client)
	}

	log.Printf("[WebSocket] Hub stopped, all clients disconnected")
}

// ClientCount returns the current number of connected clients
func (h *Hub) ClientCount() int {
	h.mu.RLock()
	defer h.mu.RUnlock()
	return len(h.clients)
}

// Broadcast sends a message to all connected clients
func (h *Hub) Broadcast(message []byte) {
	select {
	case h.broadcast <- message:
	default:
		log.Printf("[WebSocket] Broadcast channel full, message dropped")
	}
}

// BroadcastIssueUpdate sends an issue update to all clients
func (h *Hub) BroadcastIssueUpdate(issue github.Issue) {
	column := inferColumnFromIssue(issue)
	payload := IssueUpdatePayload{
		Number: issue.Number,
		Title:  issue.Title,
		State:  issue.State,
		Column: column,
	}

	payloadBytes, err := json.Marshal(payload)
	if err != nil {
		log.Printf("[WebSocket] Error marshaling issue update payload: %v", err)
		return
	}

	msg := Message{
		Type:    MessageTypeIssueUpdate,
		Payload: payloadBytes,
	}

	msgBytes, err := json.Marshal(msg)
	if err != nil {
		log.Printf("[WebSocket] Error marshaling issue update message: %v", err)
		return
	}

	h.Broadcast(msgBytes)
	log.Printf("[WebSocket] Broadcast issue update for #%d to %d clients", issue.Number, h.ClientCount())
}

// BroadcastSyncComplete sends a sync completion message to all clients
func (h *Hub) BroadcastSyncComplete(count int) {
	payload := SyncCompletePayload{
		Count: count,
	}

	payloadBytes, err := json.Marshal(payload)
	if err != nil {
		log.Printf("[WebSocket] Error marshaling sync complete payload: %v", err)
		return
	}

	msg := Message{
		Type:    MessageTypeSyncComplete,
		Payload: payloadBytes,
	}

	msgBytes, err := json.Marshal(msg)
	if err != nil {
		log.Printf("[WebSocket] Error marshaling sync complete message: %v", err)
		return
	}

	h.Broadcast(msgBytes)
	log.Printf("[WebSocket] Broadcast sync complete (count=%d) to %d clients", count, h.ClientCount())
}

// readPump pumps messages from the WebSocket connection to the hub
func (c *Client) readPump() {
	defer func() {
		c.hub.unregister <- c
		c.conn.Close()
	}()

	c.conn.SetReadLimit(maxMessageSize)
	c.conn.SetReadDeadline(time.Now().Add(pongWait))
	c.conn.SetPongHandler(func(string) error {
		c.conn.SetReadDeadline(time.Now().Add(pongWait))
		return nil
	})

	for {
		_, message, err := c.conn.ReadMessage()
		if err != nil {
			if websocket.IsUnexpectedCloseError(err, websocket.CloseGoingAway, websocket.CloseAbnormalClosure) {
				log.Printf("[WebSocket] Read error: %v", err)
			}
			break
		}

		// Handle incoming messages (mainly ping/pong)
		var msg Message
		if err := json.Unmarshal(message, &msg); err == nil {
			if msg.Type == MessageTypePing {
				// Respond with pong
				pong := Message{Type: MessageTypePong}
				if pongBytes, err := json.Marshal(pong); err == nil {
					select {
					case c.send <- pongBytes:
					default:
					}
				}
			}
		}
	}
}

// writePump pumps messages from the hub to the WebSocket connection
func (c *Client) writePump() {
	ticker := time.NewTicker(pingPeriod)
	defer func() {
		ticker.Stop()
		c.hub.unregister <- c
		c.conn.Close()
	}()

	for {
		select {
		case message, ok := <-c.send:
			c.conn.SetWriteDeadline(time.Now().Add(writeWait))
			if !ok {
				// Hub closed the channel
				c.conn.WriteMessage(websocket.CloseMessage, []byte{})
				return
			}

			c.conn.WriteMessage(websocket.TextMessage, message)

		case <-ticker.C:
			c.conn.SetWriteDeadline(time.Now().Add(writeWait))
			if err := c.conn.WriteMessage(websocket.PingMessage, nil); err != nil {
				return
			}
		}
	}
}

// ServeWs handles WebSocket requests from clients
func ServeWs(hub *Hub, w http.ResponseWriter, r *http.Request) {
	// Check authentication token if WEBSOCKET_AUTH_TOKEN is set
	authToken := os.Getenv("WEBSOCKET_AUTH_TOKEN")
	if authToken != "" {
		queryToken := r.URL.Query().Get("token")
		if queryToken != authToken {
			log.Printf("[WebSocket] Authentication failed: invalid or missing token from %s", r.RemoteAddr)
			http.Error(w, "Unauthorized", http.StatusUnauthorized)
			return
		}
	}

	conn, err := upgrader.Upgrade(w, r, nil)
	if err != nil {
		log.Printf("[WebSocket] Upgrade error: %v", err)
		return
	}

	client := &Client{
		hub:         hub,
		conn:        conn,
		send:        make(chan []byte, 256),
		id:          generateClientID(),
		connectedAt: time.Now(),
	}

	client.hub.register <- client

	// Start goroutines for reading and writing
	go client.writePump()
	go client.readPump()
}

// generateClientID generates a unique client ID
var clientIDCounter int64
var clientIDMu sync.Mutex

func generateClientID() string {
	clientIDMu.Lock()
	defer clientIDMu.Unlock()
	clientIDCounter++
	return fmt.Sprintf("%s-%d", time.Now().Format("20060102150405"), clientIDCounter)
}
