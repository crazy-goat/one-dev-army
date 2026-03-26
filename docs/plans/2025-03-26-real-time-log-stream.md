# Real-Time Log Stream Display Implementation Plan

> **For Claude:** REQUIRED SUB-SKILL: Use superpowers:executing-plans to implement this plan task-by-task.

**Goal:** Add a real-time log stream display below the stage title in the processing div, showing live logs from the currently running pipeline stage with timestamps, auto-scroll, and reconnection logic.

**Architecture:** 
- Extend existing WebSocket hub to broadcast log messages for active tasks
- Add SSE endpoint `/api/logs/{issue}/stream` (already exists) as fallback
- Implement JavaScript log stream client with reconnection and 100-line buffer
- Style with monospace font, dark theme, and scrollable container

**Tech Stack:** Go, WebSocket (gorilla/websocket), SSE, HTMX, vanilla JavaScript, CSS

---

## Task 1: Add Log Message Type to WebSocket Protocol

**Files:**
- Modify: `internal/dashboard/websocket.go:74-83`

**Step 1: Add new message type constant**

Add `MessageTypeLogStream` to the MessageType constants:

```go
const (
	MessageTypeIssueUpdate    MessageType = "issue_update"
	MessageTypeSyncComplete   MessageType = "sync_complete"
	MessageTypeWorkerUpdate   MessageType = "worker_update"
	MessageTypeSprintClosable MessageType = "can_close_sprint"
	MessageTypeLogStream      MessageType = "log_stream"  // NEW
	MessageTypePing           MessageType = "ping"
	MessageTypePong           MessageType = "pong"
)
```

**Step 2: Add log stream payload struct**

Add after line 118 (after SprintClosablePayload):

```go
// LogStreamPayload represents a single log line for real-time streaming
type LogStreamPayload struct {
	IssueNumber int    `json:"issue_number"`
	Step        string `json:"step"`
	Timestamp   string `json:"timestamp"`
	Message     string `json:"message"`
	Level       string `json:"level,omitempty"` // info, error, warn
}
```

**Step 3: Add broadcast method for log streams**

Add after line 379 (after BroadcastSprintClosable):

```go
// BroadcastLogStream sends a log line to all clients for a specific issue
func (h *Hub) BroadcastLogStream(issueNum int, step, timestamp, message, level string) {
	payload := LogStreamPayload{
		IssueNumber: issueNum,
		Step:        step,
		Timestamp:   timestamp,
		Message:     message,
		Level:       level,
	}

	payloadBytes, err := json.Marshal(payload)
	if err != nil {
		h.logf("Error marshaling log stream payload: %v", err)
		return
	}

	msg := Message{
		Type:    MessageTypeLogStream,
		Payload: payloadBytes,
	}

	msgBytes, err := json.Marshal(msg)
	if err != nil {
		h.logf("Error marshaling log stream message: %v", err)
		return
	}

	h.Broadcast(msgBytes)
	h.logf("Broadcast log stream for #%d (step=%s) to %d clients", issueNum, step, h.ClientCount())
}
```

**Step 4: Run tests to verify compilation**

Run: `go build ./internal/dashboard/...`
Expected: SUCCESS (no errors)

**Step 5: Commit**

```bash
git add internal/dashboard/websocket.go
git commit -m "feat: add log stream message type and broadcast method to WebSocket hub"
```

---

## Task 2: Create Log Stream Manager

**Files:**
- Create: `internal/dashboard/logstream.go`
- Create: `internal/dashboard/logstream_test.go`

**Step 1: Write the failing test**

Create `internal/dashboard/logstream_test.go`:

```go
package dashboard

import (
	"testing"
	"time"
)

func TestLogStreamManager_StartStop(t *testing.T) {
	hub := NewHub(false)
	mgr := NewLogStreamManager(hub, "/tmp/test")
	
	if mgr == nil {
		t.Fatal("expected non-nil manager")
	}
	
	if mgr.hub != hub {
		t.Error("expected hub to be set")
	}
}

func TestLogStreamManager_StartMonitoring(t *testing.T) {
	hub := NewHub(false)
	mgr := NewLogStreamManager(hub, "/tmp/test")
	
	// Should not panic when starting monitoring
	mgr.StartMonitoring(123)
	
	// Verify active issue is set
	if mgr.activeIssue != 123 {
		t.Errorf("expected active issue 123, got %d", mgr.activeIssue)
	}
}

func TestLogStreamManager_StopMonitoring(t *testing.T) {
	hub := NewHub(false)
	mgr := NewLogStreamManager(hub, "/tmp/test")
	
	mgr.StartMonitoring(123)
	mgr.StopMonitoring()
	
	if mgr.activeIssue != 0 {
		t.Errorf("expected active issue 0 after stop, got %d", mgr.activeIssue)
	}
}
```

**Step 2: Run test to verify it fails**

Run: `go test ./internal/dashboard -run TestLogStream -v`
Expected: FAIL with "undefined: NewLogStreamManager"

**Step 3: Write minimal implementation**

Create `internal/dashboard/logstream.go`:

```go
package dashboard

import (
	"bufio"
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"sync"
	"time"
)

// LogStreamManager manages real-time log streaming for the active issue
type LogStreamManager struct {
	hub         *Hub
	rootDir     string
	activeIssue int
	mu          sync.RWMutex
	stopCh      chan struct{}
	wg          sync.WaitGroup
}

// NewLogStreamManager creates a new log stream manager
func NewLogStreamManager(hub *Hub, rootDir string) *LogStreamManager {
	return &LogStreamManager{
		hub:     hub,
		rootDir: rootDir,
		stopCh:  make(chan struct{}),
	}
}

// StartMonitoring starts monitoring logs for a specific issue
func (m *LogStreamManager) StartMonitoring(issueNum int) {
	m.mu.Lock()
	defer m.mu.Unlock()
	
	// Stop any existing monitoring
	if m.activeIssue != 0 {
		close(m.stopCh)
		m.wg.Wait()
		m.stopCh = make(chan struct{})
	}
	
	m.activeIssue = issueNum
	
	// Start monitoring in background
	m.wg.Add(1)
	go m.monitorLogs(issueNum)
}

// StopMonitoring stops the current log monitoring
func (m *LogStreamManager) StopMonitoring() {
	m.mu.Lock()
	defer m.mu.Unlock()
	
	if m.activeIssue != 0 {
		close(m.stopCh)
		m.wg.Wait()
		m.stopCh = make(chan struct{})
		m.activeIssue = 0
	}
}

// GetActiveIssue returns the currently monitored issue number
func (m *LogStreamManager) GetActiveIssue() int {
	m.mu.RLock()
	defer m.mu.RUnlock()
	return m.activeIssue
}

// monitorLogs monitors log files for the given issue and broadcasts updates
func (m *LogStreamManager) monitorLogs(issueNum int) {
	defer m.wg.Done()
	
	logDir := filepath.Join(m.rootDir, ".oda", "artifacts", string(issueNum), "logs")
	
	// Track file offsets for tailing
	fileOffsets := make(map[string]int64)
	
	ticker := time.NewTicker(500 * time.Millisecond)
	defer ticker.Stop()
	
	for {
		select {
		case <-m.stopCh:
			return
		case <-ticker.C:
			m.scanLogFiles(logDir, issueNum, fileOffsets)
		}
	}
}

// scanLogFiles scans log files and broadcasts new lines
func (m *LogStreamManager) scanLogFiles(logDir string, issueNum int, offsets map[string]int64) {
	entries, err := os.ReadDir(logDir)
	if err != nil {
		return // Directory might not exist yet
	}
	
	for _, entry := range entries {
		if entry.IsDir() {
			continue
		}
		
		name := entry.Name()
		if !strings.HasSuffix(name, ".log") {
			continue
		}
		
		m.tailLogFile(filepath.Join(logDir, name), name, issueNum, offsets)
	}
}

// tailLogFile tails a single log file and broadcasts new lines
func (m *LogStreamManager) tailLogFile(filepath string, filename string, issueNum int, offsets map[string]int64) {
	file, err := os.Open(filepath)
	if err != nil {
		return
	}
	defer file.Close()
	
	// Get current file size
	stat, err := file.Stat()
	if err != nil {
		return
	}
	
	currentSize := stat.Size()
	lastOffset, exists := offsets[filename]
	
	if !exists {
		// First time reading this file - start from beginning
		offsets[filename] = 0
		lastOffset = 0
	}
	
	if currentSize <= lastOffset {
		// No new content
		return
	}
	
	// Seek to last position
	_, err = file.Seek(lastOffset, 0)
	if err != nil {
		return
	}
	
	// Extract step name from filename (format: YYYYmmddHHMMSS_<step>.log)
	stepName := extractStepName(filename)
	
	scanner := bufio.NewScanner(file)
	for scanner.Scan() {
		line := scanner.Text()
		timestamp, message := parseLogLine(line)
		
		// Broadcast via WebSocket
		if m.hub != nil {
			m.hub.BroadcastLogStream(issueNum, stepName, timestamp, message, "info")
		}
	}
	
	// Update offset
	newOffset, _ := file.Seek(0, 1)
	offsets[filename] = newOffset
}

// extractStepName extracts the step name from log filename
func extractStepName(filename string) string {
	// Remove .log extension
	name := strings.TrimSuffix(filename, ".log")
	
	// Find last underscore to get step name
	if idx := strings.LastIndex(name, "_"); idx != -1 {
		return name[idx+1:]
	}
	
	return name
}

// parseLogLine parses a log line and extracts timestamp and message
// Expected format: [YYYY-MM-DD HH:MM:SS] message
func parseLogLine(line string) (timestamp, message string) {
	// Try to match timestamp pattern
	re := regexp.MustCompile(`^\[(\d{4}-\d{2}-\d{2} \d{2}:\d{2}:\d{2})\] (.*)$`)
	matches := re.FindStringSubmatch(line)
	
	if len(matches) == 3 {
		return matches[1], matches[2]
	}
	
	// No timestamp found, return empty timestamp and full line
	return "", line
}
```

**Step 4: Run test to verify it passes**

Run: `go test ./internal/dashboard -run TestLogStream -v`
Expected: PASS

**Step 5: Commit**

```bash
git add internal/dashboard/logstream.go internal/dashboard/logstream_test.go
git commit -m "feat: add log stream manager for real-time log monitoring"
```

---

## Task 3: Integrate Log Stream Manager with Orchestrator

**Files:**
- Modify: `internal/dashboard/server.go:26-47` (add logStreamManager field)
- Modify: `internal/dashboard/server.go:49-97` (initialize in constructor)
- Modify: `internal/dashboard/handlers.go:710-735` (start monitoring on current task)

**Step 1: Add log stream manager to Server struct**

Modify `internal/dashboard/server.go` line 46, add field:

```go
	yoloOverride     *bool // Runtime YOLO mode override (nil = use config file)
	logStreamMgr     *LogStreamManager  // NEW: Log stream manager
}
```

**Step 2: Initialize log stream manager in constructor**

Modify `internal/dashboard/server.go` around line 78, add initialization:

```go
		yoloOverride:     nil,
		logStreamMgr:     NewLogStreamManager(hub, rootDir),  // NEW
	}
```

**Step 3: Add method to expose log stream manager**

Add at the end of `server.go` (after line 371):

```go
// LogStreamManager returns the log stream manager instance
func (s *Server) LogStreamManager() *LogStreamManager {
	return s.logStreamMgr
}
```

**Step 4: Start monitoring when task becomes active**

Modify `internal/dashboard/handlers.go` in `handleCurrentTask` (line 710-735), add monitoring start:

```go
func (s *Server) handleCurrentTask(w http.ResponseWriter, _ *http.Request) {
	w.Header().Set("Content-Type", "application/json")
	if s.orchestrator == nil {
		if err := json.NewEncoder(w).Encode(map[string]any{"active": false}); err != nil {
			log.Printf("[Dashboard] Error encoding JSON: %v", err)
		}
		return
	}
	task := s.orchestrator.CurrentTask()
	if task == nil {
		if err := json.NewEncoder(w).Encode(map[string]any{"active": false}); err != nil {
			log.Printf("[Dashboard] Error encoding JSON: %v", err)
		}
		// Stop monitoring when no active task
		if s.logStreamMgr != nil {
			s.logStreamMgr.StopMonitoring()
		}
		return
	}
	
	// Start monitoring logs for this task
	if s.logStreamMgr != nil {
		s.logStreamMgr.StartMonitoring(task.Issue.Number)
	}
	
	if err := json.NewEncoder(w).Encode(map[string]any{
		"active":       true,
		"issue_number": task.Issue.Number,
		"issue_title":  task.Issue.Title,
		"status":       string(task.Status),
		"milestone":    task.Milestone,
		"branch":       task.Branch,
	}); err != nil {
		log.Printf("[Dashboard] Error encoding JSON: %v", err)
	}
}
```

**Step 5: Run build to verify compilation**

Run: `go build ./internal/dashboard/...`
Expected: SUCCESS

**Step 6: Commit**

```bash
git add internal/dashboard/server.go internal/dashboard/handlers.go
git commit -m "feat: integrate log stream manager with dashboard server"
```

---

## Task 4: Add Log Container to Processing Panel HTML

**Files:**
- Modify: `internal/dashboard/templates/board.html:330-368`

**Step 1: Add log container HTML structure**

Replace the processing panel section (lines 330-368) with:

```html
    <!-- Processing Panel at the bottom of center column -->
    <div class="processing-panel{{if not .CurrentTicket}} processing-panel-idle{{end}}" id="processing-panel" data-total-tickets="{{.TotalTickets}}">
      <div class="processing-panel-content">
        <div class="processing-panel-title">Processing</div>
        {{if .CurrentTicket}}
        <div class="processing-labels">
          {{if .CurrentTicket.Priority}}
          <span class="processing-badge processing-priority-{{.CurrentTicket.Priority}}">
            {{if eq .CurrentTicket.Priority "high"}}🔴{{else if eq .CurrentTicket.Priority "medium"}}🟡{{else}}🟢{{end}} {{.CurrentTicket.Priority}}
          </span>
          {{end}}
          {{if .CurrentTicket.Type}}
          <span class="processing-badge">
            {{if eq .CurrentTicket.Type "bug"}}🐛 Bug{{else}}✨ Feature{{end}}
          </span>
          {{end}}
          {{if .CurrentTicket.Size}}
          <span class="processing-badge">📏 {{.CurrentTicket.Size}}</span>
          {{end}}
        </div>
        <a href="/task/{{.CurrentTicket.Number}}" class="processing-ticket">
          <span class="processing-id">#{{.CurrentTicket.Number}}</span>
          <span class="processing-title">{{.CurrentTicket.Title}}</span>
        </a>
        <!-- Log Stream Container -->
        <div class="log-stream-container" id="log-stream-container" data-issue="{{.CurrentTicket.Number}}">
          <div class="log-stream-header">
            <span class="log-stream-title">📋 Live Logs</span>
            <span class="log-stream-status" id="log-stream-status">Connecting...</span>
          </div>
          <div class="log-stream-content" id="log-stream-content">
            <div class="log-stream-empty">Waiting for logs...</div>
          </div>
        </div>
        {{else}}
        {{if eq .TotalTickets 0}}
        <div class="processing-empty-labels">
          <span class="processing-badge">🎯 Sprint</span>
        </div>
        <div class="processing-empty-message">
          <span class="processing-empty-title">No tickets in sprint</span>
          <span class="processing-empty-subtitle">Create your first ticket to get started</span>
          <a href="/wizard" class="processing-cta">+ New Ticket</a>
        </div>
        {{else}}
        <span class="processing-idle-text">No active ticket &mdash; Worker ready</span>
        {{end}}
        {{end}}
      </div>
    </div>
```

**Step 2: Add CSS styles for log stream**

Add to the `<style>` section in `board.html` (after line 73, before `</style>`):

```css
/* Log Stream Styles */
.log-stream-container {
  margin-top: 0.75rem;
  background: rgba(0, 0, 0, 0.3);
  border: 1px solid var(--border);
  border-radius: 6px;
  overflow: hidden;
  font-family: 'SF Mono', Monaco, Inconsolata, 'Fira Code', monospace;
  font-size: 0.75rem;
  max-height: 200px;
  display: flex;
  flex-direction: column;
}

.log-stream-header {
  display: flex;
  justify-content: space-between;
  align-items: center;
  padding: 0.4rem 0.6rem;
  background: rgba(255, 255, 255, 0.05);
  border-bottom: 1px solid var(--border);
  flex-shrink: 0;
}

.log-stream-title {
  font-weight: 600;
  color: var(--accent);
}

.log-stream-status {
  font-size: 0.7rem;
  color: var(--muted);
  font-style: italic;
}

.log-stream-status.connected {
  color: var(--green);
}

.log-stream-status.error {
  color: var(--red);
}

.log-stream-content {
  flex: 1;
  overflow-y: auto;
  padding: 0.4rem 0.6rem;
  scroll-behavior: smooth;
}

.log-stream-empty {
  color: var(--muted);
  font-style: italic;
  text-align: center;
  padding: 1rem;
}

.log-line {
  display: flex;
  gap: 0.5rem;
  padding: 0.15rem 0;
  line-height: 1.4;
  border-bottom: 1px solid rgba(255, 255, 255, 0.03);
}

.log-line:last-child {
  border-bottom: none;
}

.log-timestamp {
  color: var(--muted);
  flex-shrink: 0;
  min-width: 130px;
}

.log-step {
  color: var(--accent);
  flex-shrink: 0;
  min-width: 80px;
  text-transform: uppercase;
  font-size: 0.7rem;
}

.log-message {
  color: var(--text);
  flex: 1;
  word-break: break-word;
}

.log-line.error .log-message {
  color: var(--red);
}

.log-line.warn .log-message {
  color: var(--orange);
}

/* Scrollbar styling for log stream */
.log-stream-content::-webkit-scrollbar {
  width: 6px;
}

.log-stream-content::-webkit-scrollbar-track {
  background: transparent;
}

.log-stream-content::-webkit-scrollbar-thumb {
  background: var(--border);
  border-radius: 3px;
}

.log-stream-content::-webkit-scrollbar-thumb:hover {
  background: var(--muted);
}
```

**Step 3: Verify template syntax**

Run: `go build ./internal/dashboard/...`
Expected: SUCCESS

**Step 4: Commit**

```bash
git add internal/dashboard/templates/board.html
git commit -m "feat: add log stream container to processing panel with styling"
```

---

## Task 5: Implement JavaScript Log Stream Client

**Files:**
- Modify: `internal/dashboard/templates/layout.html:346-699` (add to existing script)

**Step 1: Add LogStreamClient class to layout.html**

Add before the closing `</script>` tag in layout.html (around line 698):

```javascript
// Log Stream Client
(function() {
    const MAX_LOG_LINES = 100;
    
    class LogStreamClient {
        constructor() {
            this.container = null;
            this.content = null;
            this.status = null;
            this.issueNumber = null;
            this.logLines = [];
            this.reconnectDelay = 1000;
            this.reconnectTimer = null;
            this.isConnected = false;
        }
        
        init() {
            this.container = document.getElementById('log-stream-container');
            if (!this.container) return;
            
            this.content = document.getElementById('log-stream-content');
            this.status = document.getElementById('log-stream-status');
            this.issueNumber = this.container.getAttribute('data-issue');
            
            if (this.issueNumber) {
                this.connect();
            }
        }
        
        connect() {
            if (!this.issueNumber) return;
            
            // Use existing WebSocket connection from WebSocketClient
            this.isConnected = true;
            this.updateStatus('connected', 'Live');
            this.clearLogs();
            
            // Listen for log stream messages
            this.setupMessageListener();
        }
        
        setupMessageListener() {
            // Hook into existing WebSocket message handler
            const originalHandler = window.WebSocketClient.handleWorkerUpdate;
            window.WebSocketClient.handleWorkerUpdate = (data) => {
                originalHandler(data);
                
                // Check if issue changed
                if (data.issue_id && data.issue_id !== parseInt(this.issueNumber)) {
                    this.handleIssueChange(data.issue_id);
                }
            };
        }
        
        handleLogMessage(payload) {
            if (payload.issue_number !== parseInt(this.issueNumber)) {
                return; // Ignore logs for other issues
            }
            
            this.addLogLine({
                timestamp: payload.timestamp || new Date().toISOString(),
                step: payload.step,
                message: payload.message,
                level: payload.level || 'info'
            });
        }
        
        handleIssueChange(newIssueId) {
            // Clear logs and update to new issue
            this.issueNumber = newIssueId;
            this.container.setAttribute('data-issue', newIssueId);
            this.clearLogs();
            this.updateStatus('connected', 'Live');
        }
        
        addLogLine(log) {
            this.logLines.push(log);
            
            // Keep only last 100 lines
            if (this.logLines.length > MAX_LOG_LINES) {
                this.logLines.shift();
            }
            
            this.renderLogLine(log);
            this.scrollToBottom();
        }
        
        renderLogLine(log) {
            if (!this.content) return;
            
            // Remove empty state if present
            const emptyState = this.content.querySelector('.log-stream-empty');
            if (emptyState) {
                emptyState.remove();
            }
            
            const line = document.createElement('div');
            line.className = `log-line ${log.level}`;
            
            const timestamp = log.timestamp ? this.formatTimestamp(log.timestamp) : '';
            
            line.innerHTML = `
                <span class="log-timestamp">${timestamp}</span>
                <span class="log-step">${this.escapeHtml(log.step)}</span>
                <span class="log-message">${this.escapeHtml(log.message)}</span>
            `;
            
            this.content.appendChild(line);
            
            // Remove old lines if exceeding max
            while (this.content.children.length > MAX_LOG_LINES) {
                this.content.removeChild(this.content.firstChild);
            }
        }
        
        formatTimestamp(timestamp) {
            try {
                const date = new Date(timestamp);
                return date.toLocaleTimeString('en-US', { 
                    hour12: false,
                    hour: '2-digit',
                    minute: '2-digit',
                    second: '2-digit'
                });
            } catch (e) {
                return timestamp;
            }
        }
        
        escapeHtml(text) {
            const div = document.createElement('div');
            div.textContent = text;
            return div.innerHTML;
        }
        
        scrollToBottom() {
            if (this.content) {
                this.content.scrollTop = this.content.scrollHeight;
            }
        }
        
        clearLogs() {
            this.logLines = [];
            if (this.content) {
                this.content.innerHTML = '<div class="log-stream-empty">Waiting for logs...</div>';
            }
        }
        
        updateStatus(state, text) {
            if (!this.status) return;
            
            this.status.textContent = text;
            this.status.className = 'log-stream-status ' + state;
        }
        
        disconnect() {
            this.isConnected = false;
            this.updateStatus('error', 'Disconnected');
        }
    }
    
    // Create global instance
    window.LogStreamClient = new LogStreamClient();
    
    // Initialize on page load
    document.addEventListener('DOMContentLoaded', function() {
        window.LogStreamClient.init();
    });
    
    // Hook into WebSocket message handling for log_stream messages
    const originalOnMessage = window.WebSocketClient.onmessage;
    window.WebSocketClient.onmessage = function(event) {
        const msg = JSON.parse(event.data);
        
        if (msg.type === 'log_stream' && msg.payload) {
            const payload = JSON.parse(msg.payload);
            window.LogStreamClient.handleLogMessage(payload);
        }
        
        // Call original handler
        if (originalOnMessage) {
            originalOnMessage(event);
        }
    };
})();
```

**Step 2: Modify WebSocket onmessage handler to support hooks**

Modify the WebSocket onmessage handler in layout.html (around line 375) to support external handlers:

```javascript
            ws.onmessage = function(event) {
                const msg = JSON.parse(event.data);
                console.log('[WebSocket] Received:', msg.type);
                
                // Handle log stream messages
                if (msg.type === 'log_stream' && window.LogStreamClient) {
                    const payload = JSON.parse(msg.payload);
                    window.LogStreamClient.handleLogMessage(payload);
                }
                
                if (msg.type === 'issue_update' || msg.type === 'sync_complete' || msg.type === 'worker_update') {
                    refreshBoard();
                }
                
                // Handle sprint closable updates
                if (msg.type === 'can_close_sprint' && msg.payload) {
                    const payload = JSON.parse(msg.payload);
                    const closeForm = document.getElementById('close-sprint-form');
                    if (closeForm) {
                        closeForm.style.display = payload.can_close ? 'inline' : 'none';
                    }
                }
                
                // Handle worker status updates
                if (msg.type === 'worker_update' && msg.payload) {
                    const payload = JSON.parse(msg.payload);
                    handleWorkerUpdate({
                        active: payload.status === 'active',
                        paused: false,
                        step: payload.stage,
                        issue_id: payload.task_id,
                        issue_title: payload.task_title,
                        elapsed: payload.elapsed_seconds
                    });
                    
                    // Notify log stream client of issue changes
                    if (window.LogStreamClient && payload.task_id) {
                        window.LogStreamClient.handleIssueChange(payload.task_id);
                    }
                }
            };
```

**Step 3: Run build to verify**

Run: `go build ./...`
Expected: SUCCESS

**Step 4: Commit**

```bash
git add internal/dashboard/templates/layout.html
git commit -m "feat: implement JavaScript log stream client with WebSocket integration"
```

---

## Task 6: Add Reconnection Logic and Error Handling

**Files:**
- Modify: `internal/dashboard/templates/layout.html` (enhance LogStreamClient)

**Step 1: Enhance LogStreamClient with reconnection logic**

Replace the LogStreamClient implementation with enhanced version:

```javascript
// Log Stream Client with Reconnection
(function() {
    const MAX_LOG_LINES = 100;
    const RECONNECT_DELAY_BASE = 1000;
    const RECONNECT_DELAY_MAX = 30000;
    
    class LogStreamClient {
        constructor() {
            this.container = null;
            this.content = null;
            this.status = null;
            this.issueNumber = null;
            this.logLines = [];
            this.reconnectDelay = RECONNECT_DELAY_BASE;
            this.reconnectTimer = null;
            this.isConnected = false;
            this.sseSource = null;
            this.useSSE = false;
            this.lastLogTime = null;
            this.healthCheckInterval = null;
        }
        
        init() {
            this.container = document.getElementById('log-stream-container');
            if (!this.container) return;
            
            this.content = document.getElementById('log-stream-content');
            this.status = document.getElementById('log-stream-status');
            this.issueNumber = this.container.getAttribute('data-issue');
            
            if (this.issueNumber) {
                this.connect();
            }
            
            // Setup health check
            this.startHealthCheck();
        }
        
        connect() {
            if (!this.issueNumber) {
                this.updateStatus('error', 'No issue');
                return;
            }
            
            // Try WebSocket first, fallback to SSE
            if (window.WebSocketClient && this.isWebSocketConnected()) {
                this.useSSE = false;
                this.isConnected = true;
                this.updateStatus('connected', 'Live (WebSocket)');
                this.clearLogs();
                this.reconnectDelay = RECONNECT_DELAY_BASE;
            } else {
                this.useSSE = true;
                this.connectSSE();
            }
        }
        
        isWebSocketConnected() {
            // Check if WebSocket is connected via the status indicator
            const wsStatus = document.getElementById('ws-status');
            return wsStatus && wsStatus.classList.contains('ws-connected');
        }
        
        connectSSE() {
            if (this.sseSource) {
                this.sseSource.close();
            }
            
            const url = `/api/logs/${this.issueNumber}/stream?follow=true`;
            
            try {
                this.sseSource = new EventSource(url);
                
                this.sseSource.onopen = () => {
                    this.isConnected = true;
                    this.updateStatus('connected', 'Live (SSE)');
                    this.reconnectDelay = RECONNECT_DELAY_BASE;
                    this.lastLogTime = Date.now();
                };
                
                this.sseSource.onmessage = (event) => {
                    try {
                        const data = JSON.parse(event.data);
                        
                        if (data.event === 'log:new') {
                            this.handleLogMessage({
                                issue_number: parseInt(this.issueNumber),
                                step: data.step,
                                timestamp: data.timestamp,
                                message: data.message,
                                level: 'info'
                            });
                            this.lastLogTime = Date.now();
                        } else if (data.event === 'log:error') {
                            this.updateStatus('error', data.error);
                        } else if (data.event === 'log:complete') {
                            this.updateStatus('connected', 'Complete');
                        }
                    } catch (e) {
                        console.error('[LogStream] Error parsing SSE message:', e);
                    }
                };
                
                this.sseSource.onerror = () => {
                    this.handleDisconnect();
                };
                
            } catch (e) {
                console.error('[LogStream] Error creating SSE connection:', e);
                this.handleDisconnect();
            }
        }
        
        handleDisconnect() {
            this.isConnected = false;
            this.updateStatus('error', 'Reconnecting...');
            
            if (this.sseSource) {
                this.sseSource.close();
                this.sseSource = null;
            }
            
            // Schedule reconnection
            if (!this.reconnectTimer) {
                console.log(`[LogStream] Reconnecting in ${this.reconnectDelay}ms...`);
                this.reconnectTimer = setTimeout(() => {
                    this.reconnectTimer = null;
                    this.connect();
                }, this.reconnectDelay);
                
                // Exponential backoff
                this.reconnectDelay = Math.min(this.reconnectDelay * 2, RECONNECT_DELAY_MAX);
            }
        }
        
        startHealthCheck() {
            // Check connection health every 10 seconds
            this.healthCheckInterval = setInterval(() => {
                if (!this.isConnected || !this.issueNumber) return;
                
                // If no logs for 30 seconds, try to reconnect
                if (this.lastLogTime && (Date.now() - this.lastLogTime > 30000)) {
                    console.log('[LogStream] Health check: no logs for 30s, reconnecting...');
                    this.reconnectDelay = RECONNECT_DELAY_BASE;
                    this.handleDisconnect();
                }
            }, 10000);
        }
        
        handleLogMessage(payload) {
            if (payload.issue_number !== parseInt(this.issueNumber)) {
                return;
            }
            
            this.addLogLine({
                timestamp: payload.timestamp || new Date().toISOString(),
                step: payload.step,
                message: payload.message,
                level: payload.level || 'info'
            });
            
            this.lastLogTime = Date.now();
        }
        
        handleIssueChange(newIssueId) {
            if (newIssueId === parseInt(this.issueNumber)) {
                return; // Same issue, no change needed
            }
            
            // Clear logs and update to new issue
            this.issueNumber = newIssueId;
            this.container.setAttribute('data-issue', newIssueId);
            this.clearLogs();
            
            // Reset connection
            this.reconnectDelay = RECONNECT_DELAY_BASE;
            if (this.sseSource) {
                this.sseSource.close();
                this.sseSource = null;
            }
            
            this.connect();
        }
        
        addLogLine(log) {
            this.logLines.push(log);
            
            if (this.logLines.length > MAX_LOG_LINES) {
                this.logLines.shift();
            }
            
            this.renderLogLine(log);
            this.scrollToBottom();
        }
        
        renderLogLine(log) {
            if (!this.content) return;
            
            const emptyState = this.content.querySelector('.log-stream-empty');
            if (emptyState) {
                emptyState.remove();
            }
            
            const line = document.createElement('div');
            line.className = `log-line ${log.level}`;
            
            const timestamp = log.timestamp ? this.formatTimestamp(log.timestamp) : '';
            
            line.innerHTML = `
                <span class="log-timestamp">${timestamp}</span>
                <span class="log-step">${this.escapeHtml(log.step)}</span>
                <span class="log-message">${this.escapeHtml(log.message)}</span>
            `;
            
            this.content.appendChild(line);
            
            while (this.content.children.length > MAX_LOG_LINES) {
                this.content.removeChild(this.content.firstChild);
            }
        }
        
        formatTimestamp(timestamp) {
            try {
                const date = new Date(timestamp);
                return date.toLocaleTimeString('en-US', { 
                    hour12: false,
                    hour: '2-digit',
                    minute: '2-digit',
                    second: '2-digit'
                });
            } catch (e) {
                return timestamp;
            }
        }
        
        escapeHtml(text) {
            const div = document.createElement('div');
            div.textContent = text;
            return div.innerHTML;
        }
        
        scrollToBottom() {
            if (this.content) {
                this.content.scrollTop = this.content.scrollHeight;
            }
        }
        
        clearLogs() {
            this.logLines = [];
            this.lastLogTime = null;
            if (this.content) {
                this.content.innerHTML = '<div class="log-stream-empty">Waiting for logs...</div>';
            }
        }
        
        updateStatus(state, text) {
            if (!this.status) return;
            
            this.status.textContent = text;
            this.status.className = 'log-stream-status ' + state;
        }
        
        disconnect() {
            this.isConnected = false;
            
            if (this.sseSource) {
                this.sseSource.close();
                this.sseSource = null;
            }
            
            if (this.reconnectTimer) {
                clearTimeout(this.reconnectTimer);
                this.reconnectTimer = null;
            }
            
            if (this.healthCheckInterval) {
                clearInterval(this.healthCheckInterval);
                this.healthCheckInterval = null;
            }
            
            this.updateStatus('error', 'Disconnected');
        }
    }
    
    window.LogStreamClient = new LogStreamClient();
    
    document.addEventListener('DOMContentLoaded', function() {
        window.LogStreamClient.init();
    });
    
    // Cleanup on page unload
    window.addEventListener('beforeunload', function() {
        if (window.LogStreamClient) {
            window.LogStreamClient.disconnect();
        }
    });
})();
```

**Step 2: Run build to verify**

Run: `go build ./...`
Expected: SUCCESS

**Step 3: Commit**

```bash
git add internal/dashboard/templates/layout.html
git commit -m "feat: add reconnection logic and SSE fallback to log stream client"
```

---

## Task 7: Write Integration Tests

**Files:**
- Create: `internal/dashboard/logstream_integration_test.go`

**Step 1: Write integration test**

```go
package dashboard

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/crazy-goat/one-dev-army/internal/github"
)

func TestLogStreamManager_Integration(t *testing.T) {
	// Create temp directory for logs
	tmpDir := t.TempDir()
	
	// Create log directory structure
	logDir := filepath.Join(tmpDir, ".oda", "artifacts", "123", "logs")
	if err := os.MkdirAll(logDir, 0755); err != nil {
		t.Fatalf("failed to create log dir: %v", err)
	}
	
	// Create a test log file
	logFile := filepath.Join(logDir, "20250325120000_analysis.log")
	content := "[2025-03-25 12:00:00] Starting analysis step\n[2025-03-25 12:00:05] Analysis complete\n"
	if err := os.WriteFile(logFile, []byte(content), 0644); err != nil {
		t.Fatalf("failed to write log file: %v", err)
	}
	
	// Create hub and manager
	hub := NewHub(false)
	mgr := NewLogStreamManager(hub, tmpDir)
	
	// Start monitoring
	mgr.StartMonitoring(123)
	defer mgr.StopMonitoring()
	
	// Give it time to scan
	time.Sleep(100 * time.Millisecond)
	
	// Verify active issue
	if mgr.GetActiveIssue() != 123 {
		t.Errorf("expected active issue 123, got %d", mgr.GetActiveIssue())
	}
}

func TestLogStreamWebSocket_Broadcast(t *testing.T) {
	hub := NewHub(false)
	
	// Test broadcast method
	hub.BroadcastLogStream(123, "analysis", "2025-03-25 12:00:00", "Test message", "info")
	
	// The broadcast should complete without panic
	// In a real test with connected clients, we'd verify message delivery
}

func TestLogStreamPayload_Marshal(t *testing.T) {
	payload := LogStreamPayload{
		IssueNumber: 123,
		Step:        "analysis",
		Timestamp:   "2025-03-25 12:00:00",
		Message:     "Test message",
		Level:       "info",
	}
	
	data, err := json.Marshal(payload)
	if err != nil {
		t.Fatalf("failed to marshal payload: %v", err)
	}
	
	var decoded LogStreamPayload
	if err := json.Unmarshal(data, &decoded); err != nil {
		t.Fatalf("failed to unmarshal payload: %v", err)
	}
	
	if decoded.IssueNumber != payload.IssueNumber {
		t.Errorf("issue number mismatch: got %d, want %d", decoded.IssueNumber, payload.IssueNumber)
	}
}

func TestHandleLogStreamEndpoint(t *testing.T) {
	// Create temp directory with test logs
	tmpDir := t.TempDir()
	logDir := filepath.Join(tmpDir, ".oda", "artifacts", "456", "logs")
	if err := os.MkdirAll(logDir, 0755); err != nil {
		t.Fatalf("failed to create log dir: %v", err)
	}
	
	// Create test log file
	logFile := filepath.Join(logDir, "20250325120000_coding.log")
	content := "[2025-03-25 12:00:00] Starting coding step\n[2025-03-25 12:00:10] Writing code\n"
	if err := os.WriteFile(logFile, []byte(content), 0644); err != nil {
		t.Fatalf("failed to write log file: %v", err)
	}
	
	// Create minimal server
	server := &Server{
		rootDir: tmpDir,
	}
	
	// Create request
	req := httptest.NewRequest("GET", "/api/logs/456/stream", nil)
	rec := httptest.NewRecorder()
	
	// Call handler
	server.handleLogStream(rec, req)
	
	// Verify response
	if rec.Code != http.StatusOK {
		t.Errorf("expected status 200, got %d", rec.Code)
	}
	
	contentType := rec.Header().Get("Content-Type")
	if contentType != "text/event-stream" {
		t.Errorf("expected content-type text/event-stream, got %s", contentType)
	}
}
```

**Step 2: Run tests**

Run: `go test ./internal/dashboard -run TestLogStream -v`
Expected: PASS

**Step 3: Run all dashboard tests**

Run: `go test ./internal/dashboard/... -v`
Expected: All tests PASS

**Step 4: Commit**

```bash
git add internal/dashboard/logstream_integration_test.go
git commit -m "test: add integration tests for log stream functionality"
```

---

## Task 8: Run Full Test Suite and Lint

**Step 1: Run linting**

Run: `golangci-lint run ./internal/dashboard/...`
Expected: No errors

**Step 2: Run all tests**

Run: `go test -race ./internal/dashboard/...`
Expected: All tests PASS

**Step 3: Build entire project**

Run: `go build ./...`
Expected: SUCCESS

**Step 4: Commit**

```bash
git commit -m "chore: verify all tests pass and code compiles successfully"
```

---

## Task 9: Create Feature Documentation

**Files:**
- Create: `docs/features/log-stream-display.md`

**Step 1: Write documentation**

```markdown
# Real-Time Log Stream Display

## Overview

The dashboard now includes a real-time log stream display that shows live logs from the currently running pipeline stage. This gives users visibility into what's happening during ticket processing without needing to check log files manually.

## Features

- **Real-time updates**: Log lines appear instantly as they are generated
- **WebSocket + SSE**: Uses WebSocket when available, falls back to Server-Sent Events
- **Auto-scroll**: Automatically scrolls to show the latest logs
- **100-line buffer**: Keeps only the last 100 lines to prevent memory issues
- **Reconnection**: Automatically reconnects if the connection drops
- **Issue tracking**: Automatically switches to new issues when the worker changes tasks
- **Timestamp formatting**: Shows human-readable timestamps for each log line
- **Step identification**: Shows which pipeline step generated each log line

## Architecture

### Components

1. **LogStreamManager** (`internal/dashboard/logstream.go`)
   - Monitors log files in `.oda/artifacts/<issue>/logs/`
   - Broadcasts new log lines via WebSocket hub
   - Tails log files in real-time

2. **WebSocket Message Type** (`internal/dashboard/websocket.go`)
   - New `log_stream` message type
   - `LogStreamPayload` struct for log data
   - `BroadcastLogStream()` method for broadcasting

3. **JavaScript Client** (`internal/dashboard/templates/layout.html`)
   - `LogStreamClient` class manages the UI
   - Handles WebSocket and SSE connections
   - Implements reconnection logic with exponential backoff
   - Maintains 100-line circular buffer

4. **HTML/CSS** (`internal/dashboard/templates/board.html`)
   - Log stream container in processing panel
   - Monospace font styling
   - Scrollable area with custom scrollbar
   - Status indicator showing connection state

### Data Flow

```
Pipeline Step → Log File → LogStreamManager → WebSocket Hub → Browser
                                    ↓
                              SSE (fallback)
```

## Usage

The log stream appears automatically in the processing panel when a ticket is being processed. No user action is required.

### Connection States

- **Connecting...**: Initial connection attempt
- **Live (WebSocket)**: Connected via WebSocket
- **Live (SSE)**: Connected via Server-Sent Events
- **Reconnecting...**: Connection lost, attempting to reconnect
- **Complete**: Step completed, no more logs expected

## Configuration

No configuration required. The feature works automatically when:

1. Logs are written to `.oda/artifacts/<issue_number>/logs/`
2. The dashboard is running
3. A ticket is being processed

## File Locations

- Log files: `.oda/artifacts/<issue_number>/logs/YYYYmmddHHMMSS_<step>.log`
- Dashboard template: `internal/dashboard/templates/board.html`
- Layout template: `internal/dashboard/templates/layout.html`
- Log stream manager: `internal/dashboard/logstream.go`
- WebSocket handlers: `internal/dashboard/websocket.go`

## Testing

Run the log stream tests:

```bash
go test ./internal/dashboard -run TestLogStream -v
```

Run all dashboard tests:

```bash
go test ./internal/dashboard/... -v
```
```

**Step 2: Commit documentation**

```bash
git add docs/features/log-stream-display.md
git commit -m "docs: add documentation for real-time log stream feature"
```

---

## Summary

This implementation plan adds a complete real-time log stream display to the ODA dashboard:

1. **Backend**: LogStreamManager monitors log files and broadcasts via WebSocket
2. **Protocol**: New `log_stream` message type with structured payload
3. **Frontend**: JavaScript client with WebSocket + SSE support
4. **UI**: Styled log container with timestamps, step names, and auto-scroll
5. **Reliability**: Reconnection logic, health checks, and 100-line buffer
6. **Integration**: Automatically starts/stops with task changes
7. **Testing**: Unit and integration tests included

**Plan complete and saved to `docs/plans/2025-03-26-real-time-log-stream.md`.**

**Two execution options:**

**1. Subagent-Driven (this session)** - I dispatch fresh subagent per task, review between tasks, fast iteration

**2. Parallel Session (separate)** - Open new session with executing-plans, batch execution with checkpoints

**Which approach?**
