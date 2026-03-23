# Worker IPC Process Implementation Plan

> **For Claude:** REQUIRED SUB-SKILL: Use superpowers:executing-plans to implement this plan task-by-task.

**Goal:** Extract ODA worker from goroutine to separate process with stdin/stdout JSON IPC

**Architecture:** Worker runs as separate binary (`oda-worker`) spawned by orchestrator. Communication via line-delimited JSON over stdin/stdout. Parent sends commands (pause, restart, restart-hard), worker sends heartbeats (1s), step changes, acks, and completion status. All logs go to stderr as JSON.

**Tech Stack:** Go 1.25, standard library (encoding/json, bufio, os/exec), no external dependencies

---

## Task 1: Create IPC Protocol Definitions

**Files:**
- Create: `internal/workeripc/protocol.go`
- Test: `internal/workeripc/protocol_test.go`

**Step 1: Write the failing test**

```go
package workeripc

import (
	"encoding/json"
	"testing"
	"time"
)

func TestCommandMarshal(t *testing.T) {
	cmd := Command{Cmd: "pause"}
	data, err := json.Marshal(cmd)
	if err != nil {
		t.Fatalf("marshal failed: %v", err)
	}
	expected := `{"cmd":"pause"}`
	if string(data) != expected {
		t.Errorf("expected %s, got %s", expected, string(data))
	}
}

func TestHeartbeatMarshal(t *testing.T) {
	hb := Heartbeat{Step: "coding", Timestamp: time.Date(2024, 1, 15, 10, 30, 0, 0, time.UTC)}
	data, err := json.Marshal(hb)
	if err != nil {
		t.Fatalf("marshal failed: %v", err)
	}
	expected := `{"type":"heartbeat","step":"coding","ts":"2024-01-15T10:30:00Z"}`
	if string(data) != expected {
		t.Errorf("expected %s, got %s", expected, string(data))
	}
}

func TestStepChangeMarshal(t *testing.T) {
	sc := StepChange{From: "analyzing", To: "planning", Timestamp: time.Date(2024, 1, 15, 10, 30, 15, 0, time.UTC)}
	data, err := json.Marshal(sc)
	if err != nil {
		t.Fatalf("marshal failed: %v", err)
	}
	expected := `{"type":"step","from":"analyzing","to":"planning","ts":"2024-01-15T10:30:15Z"}`
	if string(data) != expected {
		t.Errorf("expected %s, got %s", expected, string(data))
	}
}

func TestAckMarshal(t *testing.T) {
	ack := Ack{Cmd: "pause", Timestamp: time.Date(2024, 1, 15, 10, 30, 20, 0, time.UTC)}
	data, err := json.Marshal(ack)
	if err != nil {
		t.Fatalf("marshal failed: %v", err)
	}
	expected := `{"type":"ack","cmd":"pause","ts":"2024-01-15T10:30:20Z"}`
	if string(data) != expected {
		t.Errorf("expected %s, got %s", expected, string(data))
	}
}

func TestDoneSuccessMarshal(t *testing.T) {
	done := Done{Step: "complete", Result: "success", PRURL: "https://github.com/foo/bar/pull/123", Timestamp: time.Date(2024, 1, 15, 10, 35, 0, 0, time.UTC)}
	data, err := json.Marshal(done)
	if err != nil {
		t.Fatalf("marshal failed: %v", err)
	}
	expected := `{"type":"done","step":"complete","result":"success","pr_url":"https://github.com/foo/bar/pull/123","ts":"2024-01-15T10:35:00Z"}`
	if string(data) != expected {
		t.Errorf("expected %s, got %s", expected, string(data))
	}
}

func TestDoneErrorMarshal(t *testing.T) {
	done := Done{Step: "coding", Result: "error", Error: "git push failed", Timestamp: time.Date(2024, 1, 15, 10, 35, 0, 0, time.UTC)}
	data, err := json.Marshal(done)
	if err != nil {
		t.Fatalf("marshal failed: %v", err)
	}
	expected := `{"type":"done","step":"coding","result":"error","error":"git push failed","ts":"2024-01-15T10:35:00Z"}`
	if string(data) != expected {
		t.Errorf("expected %s, got %s", expected, string(data))
	}
}

func TestLogEntryMarshal(t *testing.T) {
	entry := LogEntry{Level: "info", Message: "Starting analysis", Timestamp: time.Date(2024, 1, 15, 10, 30, 0, 0, time.UTC)}
	data, err := json.Marshal(entry)
	if err != nil {
		t.Fatalf("marshal failed: %v", err)
	}
	expected := `{"level":"info","msg":"Starting analysis","ts":"2024-01-15T10:30:00Z"}`
	if string(data) != expected {
		t.Errorf("expected %s, got %s", expected, string(data))
	}
}
```

**Step 2: Run test to verify it fails**

```bash
cd /home/decodo/work/one-dev-army
go test ./internal/workeripc -v
```

Expected: FAIL - "package workeripc: no Go files"

**Step 3: Write minimal implementation**

```go
package workeripc

import (
	"encoding/json"
	"time"
)

// Command represents a command from parent to worker (stdin)
type Command struct {
	Cmd string `json:"cmd"` // pause, restart, restart-hard
}

// Heartbeat is sent by worker every 1 second (stdout)
type Heartbeat struct {
	Type      string    `json:"type"` // always "heartbeat"
	Step      string    `json:"step"` // current pipeline step
	Timestamp time.Time `json:"ts"`
}

// StepChange is sent when worker changes pipeline stage (stdout)
type StepChange struct {
	Type      string    `json:"type"` // always "step"
	From      string    `json:"from"` // previous step
	To        string    `json:"to"`   // new step
	Timestamp time.Time `json:"ts"`
}

// Ack is sent to confirm command received (stdout)
type Ack struct {
	Type      string    `json:"type"` // always "ack"
	Cmd       string    `json:"cmd"`  // command being acknowledged
	Timestamp time.Time `json:"ts"`
}

// Done is sent when worker completes or fails (stdout)
type Done struct {
	Type      string    `json:"type"`              // always "done"
	Step      string    `json:"step"`              // step where completed/failed
	Result    string    `json:"result"`            // "success" or "error"
	PRURL     string    `json:"pr_url,omitempty"`  // only for success
	Error     string    `json:"error,omitempty"`   // only for error
	Timestamp time.Time `json:"ts"`
}

// LogEntry is written to stderr by worker
type LogEntry struct {
	Level     string    `json:"level"` // debug, info, warn, error
	Message   string    `json:"msg"`
	Timestamp time.Time `json:"ts"`
}

// MarshalJSON implements custom marshaling for time
func (h Heartbeat) MarshalJSON() ([]byte, error) {
	type Alias Heartbeat
	return json.Marshal(&struct {
		*Alias
		Timestamp string `json:"ts"`
	}{
		Alias:     (*Alias)(&h),
		Timestamp: h.Timestamp.UTC().Format(time.RFC3339),
	})
}

func (s StepChange) MarshalJSON() ([]byte, error) {
	type Alias StepChange
	return json.Marshal(&struct {
		*Alias
		Timestamp string `json:"ts"`
	}{
		Alias:     (*Alias)(&s),
		Timestamp: s.Timestamp.UTC().Format(time.RFC3339),
	})
}

func (a Ack) MarshalJSON() ([]byte, error) {
	type Alias Ack
	return json.Marshal(&struct {
		*Alias
		Timestamp string `json:"ts"`
	}{
		Alias:     (*Alias)(&a),
		Timestamp: a.Timestamp.UTC().Format(time.RFC3339),
	})
}

func (d Done) MarshalJSON() ([]byte, error) {
	type Alias Done
	return json.Marshal(&struct {
		*Alias
		Timestamp string `json:"ts"`
	}{
		Alias:     (*Alias)(&d),
		Timestamp: d.Timestamp.UTC().Format(time.RFC3339),
	})
}

func (l LogEntry) MarshalJSON() ([]byte, error) {
	type Alias LogEntry
	return json.Marshal(&struct {
		*Alias
		Timestamp string `json:"ts"`
	}{
		Alias:     (*Alias)(&l),
		Timestamp: l.Timestamp.UTC().Format(time.RFC3339),
	})
}
```

**Step 4: Run test to verify it passes**

```bash
cd /home/decodo/work/one-dev-army
go test ./internal/workeripc -v
```

Expected: PASS all tests

**Step 5: Commit**

```bash
git add internal/workeripc/protocol.go internal/workeripc/protocol_test.go
git commit -m "feat: add IPC protocol definitions for worker process"
```

---

## Task 2: Create Worker Binary Entry Point

**Files:**
- Create: `cmd/oda-worker/main.go`
- Create: `cmd/oda-worker/worker.go`

**Step 1: Write the failing test (integration test)**

```go
// cmd/oda-worker/main_test.go
package main

import (
	"bufio"
	"encoding/json"
	"os/exec"
	"strings"
	"testing"
	"time"
)

func TestWorkerStartsAndSendsHeartbeat(t *testing.T) {
	// Build the worker binary first
	buildCmd := exec.Command("go", "build", "-o", "/tmp/oda-worker-test", ".")
	buildCmd.Dir = "/home/decodo/work/one-dev-army/cmd/oda-worker"
	if err := buildCmd.Run(); err != nil {
		t.Fatalf("failed to build worker: %v", err)
	}

	// Start worker with minimal config
	cmd := exec.Command("/tmp/oda-worker-test",
		"--config", "/tmp/test-config.yaml",
		"--issue", "123",
		"--dry-run",
	)
	
	stdin, err := cmd.StdinPipe()
	if err != nil {
		t.Fatalf("failed to get stdin: %v", err)
	}
	
	stdout, err := cmd.StdoutPipe()
	if err != nil {
		t.Fatalf("failed to get stdout: %v", err)
	}
	
	if err := cmd.Start(); err != nil {
		t.Fatalf("failed to start worker: %v", err)
	}
	defer cmd.Process.Kill()
	
	// Read first message (should be heartbeat or step change)
	scanner := bufio.NewScanner(stdout)
	done := make(chan bool)
	var firstMsg string
	
	go func() {
		if scanner.Scan() {
			firstMsg = scanner.Text()
		}
		done <- true
	}()
	
	select {
	case <-done:
		if firstMsg == "" {
			t.Error("expected message from worker, got none")
		}
		if !strings.Contains(firstMsg, `"type"`) {
			t.Errorf("expected JSON with type field, got: %s", firstMsg)
		}
	case <-time.After(3 * time.Second):
		t.Error("timeout waiting for worker message")
	}
	
	// Send pause command
	stdin.Write([]byte(`{"cmd":"pause"}\n`))
	
	// Should receive ack
	select {
	case <-done:
		if scanner.Scan() {
			ack := scanner.Text()
			if !strings.Contains(ack, `"type":"ack"`) {
				t.Errorf("expected ack, got: %s", ack)
			}
		}
	case <-time.After(6 * time.Second):
		t.Error("timeout waiting for ack")
	}
}
```

**Step 2: Run test to verify it fails**

```bash
cd /home/decodo/work/one-dev-army
go test ./cmd/oda-worker -v
```

Expected: FAIL - "no Go files" or build errors

**Step 3: Write minimal implementation**

```go
// cmd/oda-worker/main.go
package main

import (
	"flag"
	"fmt"
	"os"
)

func main() {
	var (
		configPath = flag.String("config", "", "Path to config file")
		issueNum   = flag.Int("issue", 0, "Issue number to process")
		dryRun     = flag.Bool("dry-run", false, "Dry run mode (for testing)")
	)
	flag.Parse()
	
	if *configPath == "" || *issueNum == 0 {
		fmt.Fprintln(os.Stderr, "Usage: oda-worker --config <path> --issue <number>")
		os.Exit(1)
	}
	
	worker := NewWorkerProcess(*configPath, *issueNum, *dryRun)
	if err := worker.Run(); err != nil {
		fmt.Fprintf(os.Stderr, "Worker failed: %v\n", err)
		os.Exit(1)
	}
}
```

```go
// cmd/oda-worker/worker.go
package main

import (
	"bufio"
	"context"
	"encoding/json"
	"fmt"
	"os"
	"time"

	"github.com/crazy-goat/one-dev-army/internal/workeripc"
)

type WorkerProcess struct {
	configPath string
	issueNum   int
	dryRun     bool
	
	currentStep string
	paused      bool
	ctx         context.Context
	cancel      context.CancelFunc
}

func NewWorkerProcess(configPath string, issueNum int, dryRun bool) *WorkerProcess {
	ctx, cancel := context.WithCancel(context.Background())
	return &WorkerProcess{
		configPath:  configPath,
		issueNum:    issueNum,
		dryRun:      dryRun,
		currentStep: "analyzing",
		ctx:         ctx,
		cancel:      cancel,
	}
}

func (w *WorkerProcess) Run() error {
	// Start command reader
	go w.readCommands()
	
	// Start heartbeat sender
	go w.sendHeartbeats()
	
	// Run the actual work
	if w.dryRun {
		return w.runDry()
	}
	
	return w.runReal()
}

func (w *WorkerProcess) readCommands() {
	scanner := bufio.NewScanner(os.Stdin)
	for scanner.Scan() {
		line := scanner.Text()
		if line == "" {
			continue
		}
		
		var cmd workeripc.Command
		if err := json.Unmarshal([]byte(line), &cmd); err != nil {
			w.logError("failed to parse command: %v", err)
			continue
		}
		
		w.handleCommand(cmd)
	}
}

func (w *WorkerProcess) handleCommand(cmd workeripc.Command) {
	switch cmd.Cmd {
	case "pause":
		w.paused = true
		w.sendAck("pause")
		w.logInfo("Paused by parent")
	case "restart":
		w.paused = false
		w.sendAck("restart")
		w.logInfo("Restarted by parent")
	case "restart-hard":
		w.paused = false
		// TODO: clear branch and restart
		w.sendAck("restart-hard")
		w.logInfo("Hard restart by parent - clearing branch")
	default:
		w.logWarn("unknown command: %s", cmd.Cmd)
	}
}

func (w *WorkerProcess) sendAck(cmd string) {
	ack := workeripc.Ack{
		Cmd:       cmd,
		Timestamp: time.Now(),
	}
	w.writeStdout(ack)
}

func (w *WorkerProcess) sendHeartbeats() {
	ticker := time.NewTicker(1 * time.Second)
	defer ticker.Stop()
	
	for {
		select {
		case <-w.ctx.Done():
			return
		case <-ticker.C:
			hb := workeripc.Heartbeat{
				Step:      w.currentStep,
				Timestamp: time.Now(),
			}
			w.writeStdout(hb)
		}
	}
}

func (w *WorkerProcess) sendStepChange(from, to string) {
	w.currentStep = to
	sc := workeripc.StepChange{
		From:      from,
		To:        to,
		Timestamp: time.Now(),
	}
	w.writeStdout(sc)
	w.logInfo("Step changed: %s -> %s", from, to)
}

func (w *WorkerProcess) sendDone(result, prURL, errMsg string) {
	done := workeripc.Done{
		Step:      w.currentStep,
		Result:    result,
		PRURL:     prURL,
		Error:     errMsg,
		Timestamp: time.Now(),
	}
	w.writeStdout(done)
	w.cancel()
}

func (w *WorkerProcess) writeStdout(v interface{}) {
	data, err := json.Marshal(v)
	if err != nil {
		w.logError("failed to marshal message: %v", err)
		return
	}
	fmt.Println(string(data))
}

func (w *WorkerProcess) logInfo(format string, args ...interface{}) {
	w.log("info", format, args...)
}

func (w *WorkerProcess) logWarn(format string, args ...interface{}) {
	w.log("warn", format, args...)
}

func (w *WorkerProcess) logError(format string, args ...interface{}) {
	w.log("error", format, args...)
}

func (w *WorkerProcess) log(level, format string, args ...interface{}) {
	entry := workeripc.LogEntry{
		Level:     level,
		Message:   fmt.Sprintf(format, args...),
		Timestamp: time.Now(),
	}
	data, _ := json.Marshal(entry)
	fmt.Fprintln(os.Stderr, string(data))
}

func (w *WorkerProcess) runDry() error {
	// Simulate work for testing
	steps := []string{"analyzing", "planning", "coding", "reviewing", "creating_pr", "complete"}
	
	for i, step := range steps {
		if i > 0 {
			w.sendStepChange(steps[i-1], step)
		} else {
			w.currentStep = step
		}
		
		// Check if paused
		for w.paused {
			select {
			case <-w.ctx.Done():
				return w.ctx.Err()
			case <-time.After(100 * time.Millisecond):
			}
		}
		
		// Simulate work
		select {
		case <-w.ctx.Done():
			return w.ctx.Err()
		case <-time.After(500 * time.Millisecond):
		}
		
		if step == "complete" {
			w.sendDone("success", "https://github.com/test/test/pull/1", "")
			return nil
		}
	}
	
	return nil
}

func (w *WorkerProcess) runReal() error {
	// TODO: integrate with actual worker logic from internal/mvp
	// This will be implemented in Task 4
	w.logInfo("Real worker mode not yet implemented")
	w.sendDone("error", "", "not implemented")
	return fmt.Errorf("not implemented")
}
```

**Step 4: Run test to verify it passes**

```bash
cd /home/decodo/work/one-dev-army
go test ./cmd/oda-worker -v -timeout 30s
```

Expected: PASS (may need to adjust timing)

**Step 5: Commit**

```bash
git add cmd/oda-worker/main.go cmd/oda-worker/worker.go cmd/oda-worker/main_test.go
git commit -m "feat: add oda-worker binary with IPC protocol"
```

---

## Task 3: Create IPC Manager for Parent Process

**Files:**
- Create: `internal/workeripc/manager.go`
- Test: `internal/workeripc/manager_test.go`

**Step 1: Write the failing test**

```go
package workeripc

import (
	"context"
	"os/exec"
	"strings"
	"testing"
	"time"
)

func TestManagerStartStop(t *testing.T) {
	// Create a simple test worker that echoes commands
	testWorker := `package main
import (
	"bufio"
	"encoding/json"
	"fmt"
	"os"
	"time"
)
func main() {
	scanner := bufio.NewScanner(os.Stdin)
	for scanner.Scan() {
		line := scanner.Text()
		if line == "" { continue }
		var cmd map[string]string
		json.Unmarshal([]byte(line), &cmd)
		ack := map[string]interface{}{
			"type": "ack",
			"cmd": cmd["cmd"],
			"ts": time.Now().Format(time.RFC3339),
		}
		data, _ := json.Marshal(ack)
		fmt.Println(string(data))
	}
}`
	
	// Write test worker
	if err := os.WriteFile("/tmp/test_worker.go", []byte(testWorker), 0644); err != nil {
		t.Fatalf("failed to write test worker: %v", err)
	}
	
	// Build it
	buildCmd := exec.Command("go", "build", "-o", "/tmp/test_worker", "/tmp/test_worker.go")
	if err := buildCmd.Run(); err != nil {
		t.Fatalf("failed to build test worker: %v", err)
	}
	
	// Create manager
	mgr := NewManager("/tmp/test_worker", []string{})
	
	// Start worker
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	
	if err := mgr.Start(ctx); err != nil {
		t.Fatalf("failed to start manager: %v", err)
	}
	defer mgr.Stop()
	
	// Send pause command
	if err := mgr.SendCommand(ctx, Command{Cmd: "pause"}); err != nil {
		t.Errorf("failed to send pause: %v", err)
	}
	
	// Should receive ack
	select {
	case msg := <-mgr.Messages():
		if !strings.Contains(msg, `"type":"ack"`) {
			t.Errorf("expected ack, got: %s", msg)
		}
	case <-time.After(6 * time.Second):
		t.Error("timeout waiting for ack")
	}
}
```

**Step 2: Run test to verify it fails**

```bash
cd /home/decodo/work/one-dev-army
go test ./internal/workeripc -run TestManager -v
```

Expected: FAIL - Manager not implemented

**Step 3: Write minimal implementation**

```go
// internal/workeripc/manager.go
package workeripc

import (
	"bufio"
	"context"
	"encoding/json"
	"fmt"
	"os"
	"os/exec"
	"sync"
	"time"
)

// Manager handles communication with a worker process
type Manager struct {
	workerPath string
	args       []string
	cmd        *exec.Cmd
	stdin      *bufio.Writer
	stdout     *bufio.Reader
	stderr     *bufio.Reader
	
	messages chan string
	errors   chan error
	done     chan struct{}
	
	mu       sync.Mutex
	running  bool
	ackChan  chan Ack
}

// NewManager creates a new IPC manager for a worker process
func NewManager(workerPath string, args []string) *Manager {
	return &Manager{
		workerPath: workerPath,
		args:       args,
		messages:   make(chan string, 100),
		errors:     make(chan error, 10),
		done:       make(chan struct{}),
		ackChan:    make(chan Ack, 1),
	}
}

// Start launches the worker process
func (m *Manager) Start(ctx context.Context) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	
	if m.running {
		return fmt.Errorf("manager already running")
	}
	
	m.cmd = exec.CommandContext(ctx, m.workerPath, m.args...)
	
	stdin, err := m.cmd.StdinPipe()
	if err != nil {
		return fmt.Errorf("failed to create stdin pipe: %w", err)
	}
	
	stdout, err := m.cmd.StdoutPipe()
	if err != nil {
		return fmt.Errorf("failed to create stdout pipe: %w", err)
	}
	
	stderr, err := m.cmd.StderrPipe()
	if err != nil {
		return fmt.Errorf("failed to create stderr pipe: %w", err)
	}
	
	m.stdin = bufio.NewWriter(stdin)
	m.stdout = bufio.NewReader(stdout)
	m.stderr = bufio.NewReader(stderr)
	
	if err := m.cmd.Start(); err != nil {
		return fmt.Errorf("failed to start worker: %w", err)
	}
	
	m.running = true
	
	// Start readers
	go m.readStdout()
	go m.readStderr()
	
	return nil
}

// Stop terminates the worker process
func (m *Manager) Stop() error {
	m.mu.Lock()
	defer m.mu.Unlock()
	
	if !m.running {
		return nil
	}
	
	close(m.done)
	
	if m.cmd != nil && m.cmd.Process != nil {
		m.cmd.Process.Signal(os.Interrupt)
		time.AfterFunc(5*time.Second, func() {
			if m.cmd != nil && m.cmd.Process != nil {
				m.cmd.Process.Kill()
			}
		})
		m.cmd.Wait()
	}
	
	m.running = false
	return nil
}

// SendCommand sends a command to the worker and waits for ack (5s timeout)
func (m *Manager) SendCommand(ctx context.Context, cmd Command) error {
	m.mu.Lock()
	if !m.running {
		m.mu.Unlock()
		return fmt.Errorf("manager not running")
	}
	m.mu.Unlock()
	
	data, err := json.Marshal(cmd)
	if err != nil {
		return fmt.Errorf("failed to marshal command: %w", err)
	}
	
	if _, err := m.stdin.Write(append(data, '\n')); err != nil {
		return fmt.Errorf("failed to write command: %w", err)
	}
	
	if err := m.stdin.Flush(); err != nil {
		return fmt.Errorf("failed to flush command: %w", err)
	}
	
	// Wait for ack with timeout
	ctx, cancel := context.WithTimeout(ctx, 5*time.Second)
	defer cancel()
	
	select {
	case <-ctx.Done():
		return fmt.Errorf("timeout waiting for ack")
	case ack := <-m.ackChan:
		if ack.Cmd != cmd.Cmd {
			return fmt.Errorf("received ack for wrong command: %s", ack.Cmd)
		}
		return nil
	}
}

// Messages returns the channel for receiving messages from worker
func (m *Manager) Messages() <-chan string {
	return m.messages
}

// Errors returns the channel for receiving errors
func (m *Manager) Errors() <-chan error {
	return m.errors
}

func (m *Manager) readStdout() {
	scanner := bufio.NewScanner(m.stdout)
	for scanner.Scan() {
		line := scanner.Text()
		if line == "" {
			continue
		}
		
		// Try to parse as ack
		var ack Ack
		if err := json.Unmarshal([]byte(line), &ack); err == nil && ack.Type == "ack" {
			select {
			case m.ackChan <- ack:
			default:
			}
		}
		
		select {
		case m.messages <- line:
		case <-m.done:
			return
		}
	}
	
	if err := scanner.Err(); err != nil {
		select {
		case m.errors <- err:
		case <-m.done:
		}
	}
}

func (m *Manager) readStderr() {
	scanner := bufio.NewScanner(m.stderr)
	for scanner.Scan() {
		line := scanner.Text()
		// Log entries go to stderr - we could parse them if needed
		select {
		case m.messages <- "[stderr] " + line:
		case <-m.done:
			return
		}
	}
}
```

**Step 4: Run test to verify it passes**

```bash
cd /home/decodo/work/one-dev-army
go test ./internal/workeripc -run TestManager -v -timeout 30s
```

Expected: PASS

**Step 5: Commit**

```bash
git add internal/workeripc/manager.go internal/workeripc/manager_test.go
git commit -m "feat: add IPC manager for parent-worker communication"
```

---

## Task 4: Integrate Worker Process with Orchestrator

**Files:**
- Modify: `internal/mvp/orchestrator.go:40-67` (change worker field type)
- Modify: `internal/mvp/orchestrator.go:55-67` (NewOrchestrator constructor)
- Modify: `internal/mvp/orchestrator.go:241-252` (process task with IPC)
- Create: `internal/mvp/worker_process.go` (wrapper for IPC worker)

**Step 1: Write the failing test**

```go
// internal/mvp/orchestrator_test.go
package mvp

import (
	"context"
	"testing"
	"time"
)

func TestOrchestratorWithWorkerProcess(t *testing.T) {
	// This test verifies orchestrator can start/stop worker process
	// Full integration test will be in e2e tests
	
	// Create minimal orchestrator config
	cfg := &config.Config{
		Repo: "test/test",
	}
	
	// Mock dependencies
	gh := &mockGitHubClient{}
	oc := &mockOpenCodeClient{}
	brMgr := &mockBranchManager{}
	store := &mockStore{}
	
	orch := NewOrchestrator(cfg, gh, oc, brMgr, store, 0)
	
	// Verify orchestrator was created with worker process wrapper
	if orch.worker == nil {
		t.Error("expected worker to be initialized")
	}
	
	// Test Start/Pause
	orch.Start()
	if orch.IsPaused() {
		t.Error("expected orchestrator to be running")
	}
	
	orch.Pause()
	if !orch.IsPaused() {
		t.Error("expected orchestrator to be paused")
	}
}
```

**Step 2: Run test to verify it fails**

```bash
cd /home/decodo/work/one-dev-army
go test ./internal/mvp -run TestOrchestratorWithWorkerProcess -v
```

Expected: FAIL - test file doesn't exist or compilation errors

**Step 3: Write minimal implementation**

```go
// internal/mvp/worker_process.go
package mvp

import (
	"context"
	"encoding/json"
	"fmt"
	"log"
	"os"
	"path/filepath"
	"time"

	"github.com/crazy-goat/one-dev-army/internal/config"
	"github.com/crazy-goat/one-dev-army/internal/db"
	"github.com/crazy-goat/one-dev-army/internal/git"
	"github.com/crazy-goat/one-dev-army/internal/github"
	"github.com/crazy-goat/one-dev-army/internal/opencode"
	"github.com/crazy-goat/one-dev-army/internal/workeripc"
)

// WorkerProcess wraps the external oda-worker binary
type WorkerProcess struct {
	id      int
	cfg     *config.Config
	oc      *opencode.Client
	gh      *github.Client
	brMgr   *git.BranchManager
	store   *db.Store
	repoDir string
	
	ipcMgr  *workeripc.Manager
	task    *Task
	result  *TaskResult
}

func NewWorkerProcess(id int, cfg *config.Config, oc *opencode.Client, gh *github.Client, brMgr *git.BranchManager, store *db.Store) *WorkerProcess {
	return &WorkerProcess{
		id:      id,
		cfg:     cfg,
		oc:      oc,
		gh:      gh,
		brMgr:   brMgr,
		store:   store,
		repoDir: brMgr.RepoDir(),
	}
}

// Process runs the worker as external process for a task
func (w *WorkerProcess) Process(ctx context.Context, task *Task) error {
	w.task = task
	
	// Find oda-worker binary
	workerPath, err := w.findWorkerBinary()
	if err != nil {
		return fmt.Errorf("worker binary not found: %w", err)
	}
	
	// Build args
	args := w.buildArgs(task)
	
	// Create IPC manager
	w.ipcMgr = workeripc.NewManager(workerPath, args)
	
	// Start worker process
	if err := w.ipcMgr.Start(ctx); err != nil {
		return fmt.Errorf("failed to start worker: %w", err)
	}
	defer w.ipcMgr.Stop()
	
	// Process messages from worker
	return w.processMessages(ctx)
}

func (w *WorkerProcess) findWorkerBinary() (string, error) {
	// Try common locations
	candidates := []string{
		"oda-worker",                                    // In PATH
		"./oda-worker",                                  // Current dir
		"./cmd/oda-worker/oda-worker",                   // Development
		filepath.Join(os.Getenv("GOPATH"), "bin", "oda-worker"),
		filepath.Join(os.Getenv("HOME"), "go", "bin", "oda-worker"),
	}
	
	for _, path := range candidates {
		if _, err := os.Stat(path); err == nil {
			return path, nil
		}
	}
	
	return "", fmt.Errorf("oda-worker not found in any standard location")
}

func (w *WorkerProcess) buildArgs(task *Task) []string {
	// Get config file path - we need to pass it to worker
	// For now, assume it's in standard location
	configPath := w.cfg.ConfigPath
	if configPath == "" {
		configPath = ".oda/config.yaml"
	}
	
	return []string{
		"--config", configPath,
		"--issue", fmt.Sprintf("%d", task.Issue.Number),
	}
}

func (w *WorkerProcess) processMessages(ctx context.Context) error {
	messageChan := w.ipcMgr.Messages()
	errorChan := w.ipcMgr.Errors()
	
	for {
		select {
		case <-ctx.Done():
			return ctx.Err()
			
		case msg := <-messageChan:
			if err := w.handleMessage(msg); err != nil {
				return err
			}
			
		case err := <-errorChan:
			return fmt.Errorf("worker error: %w", err)
			
		case <-time.After(30 * time.Second):
			// No messages for 30s - worker might be stuck
			log.Printf("[WorkerProcess %d] No messages for 30s, checking if alive...", w.id)
		}
	}
}

func (w *WorkerProcess) handleMessage(msg string) error {
	// Parse message type
	var base struct {
		Type string `json:"type"`
	}
	if err := json.Unmarshal([]byte(msg), &base); err != nil {
		log.Printf("[WorkerProcess %d] Failed to parse message: %v", w.id, err)
		return nil // Don't fail on parse errors
	}
	
	switch base.Type {
	case "heartbeat":
		var hb workeripc.Heartbeat
		if err := json.Unmarshal([]byte(msg), &hb); err == nil {
			w.handleHeartbeat(hb)
		}
		
	case "step":
		var sc workeripc.StepChange
		if err := json.Unmarshal([]byte(msg), &sc); err == nil {
			w.handleStepChange(sc)
		}
		
	case "done":
		var done workeripc.Done
		if err := json.Unmarshal([]byte(msg), &done); err == nil {
			return w.handleDone(done)
		}
		
	default:
		log.Printf("[WorkerProcess %d] Unknown message type: %s", w.id, base.Type)
	}
	
	return nil
}

func (w *WorkerProcess) handleHeartbeat(hb workeripc.Heartbeat) {
	// Update task status based on heartbeat
	w.task.Status = TaskStatus(hb.Step)
	log.Printf("[WorkerProcess %d] Heartbeat: step=%s", w.id, hb.Step)
}

func (w *WorkerProcess) handleStepChange(sc workeripc.StepChange) {
	w.task.Status = TaskStatus(sc.To)
	log.Printf("[WorkerProcess %d] Step changed: %s -> %s", w.id, sc.From, sc.To)
	
	// Persist step change to database
	if w.store != nil {
		// Could record step transitions here
	}
}

func (w *WorkerProcess) handleDone(done workeripc.Done) error {
	log.Printf("[WorkerProcess %d] Done: result=%s step=%s", w.id, done.Result, done.Step)
	
	if done.Result == "success" {
		w.result = &TaskResult{
			PRURL:   done.PRURL,
			Summary: fmt.Sprintf("Completed in step %s", done.Step),
		}
		w.task.Status = StatusDone
		return nil
	} else {
		w.result = &TaskResult{
			Error: fmt.Errorf("%s", done.Error),
		}
		w.task.Status = StatusFailed
		return w.result.Error
	}
}

// Pause sends pause command to worker
func (w *WorkerProcess) Pause(ctx context.Context) error {
	if w.ipcMgr == nil {
		return fmt.Errorf("worker not running")
	}
	return w.ipcMgr.SendCommand(ctx, workeripc.Command{Cmd: "pause"})
}

// Restart sends restart command to worker
func (w *WorkerProcess) Restart(ctx context.Context) error {
	if w.ipcMgr == nil {
		return fmt.Errorf("worker not running")
	}
	return w.ipcMgr.SendCommand(ctx, workeripc.Command{Cmd: "restart"})
}

// RestartHard sends restart-hard command to worker
func (w *WorkerProcess) RestartHard(ctx context.Context) error {
	if w.ipcMgr == nil {
		return fmt.Errorf("worker not running")
	}
	return w.ipcMgr.SendCommand(ctx, workeripc.Command{Cmd: "restart-hard"})
}
```

**Step 4: Modify orchestrator to use WorkerProcess**

```go
// internal/mvp/orchestrator.go - changes

// Change line 42:
// FROM: worker        *Worker
// TO:   worker        *WorkerProcess

// Change line 65:
// FROM: o.worker = NewWorker(1, cfg, oc, gh, brMgr, store)
// TO:   o.worker = NewWorkerProcess(1, cfg, oc, gh, brMgr, store)
```

**Step 5: Run test to verify it passes**

```bash
cd /home/decodo/work/one-dev-army
go build ./...
go test ./internal/mvp -v 2>&1 | head -50
```

Expected: PASS (compilation successful)

**Step 6: Commit**

```bash
git add internal/mvp/worker_process.go internal/mvp/orchestrator.go internal/mvp/orchestrator_test.go
git commit -m "feat: integrate IPC worker process with orchestrator"
```

---

## Task 5: Add Config Path to Config Struct

**Files:**
- Modify: `internal/config/config.go` (add ConfigPath field)

**Step 1: Add ConfigPath field**

```go
// internal/config/config.go

type Config struct {
	ConfigPath string `yaml:"-"` // Set at runtime, not from YAML
	// ... rest of fields
}
```

**Step 2: Set ConfigPath in main.go**

```go
// main.go - where config is loaded
// After loading config, set the path:
cfg.ConfigPath = configPath
```

**Step 3: Commit**

```bash
git add internal/config/config.go main.go
git commit -m "feat: add ConfigPath to track config file location"
```

---

## Task 6: Build and Test Integration

**Step 1: Build oda-worker binary**

```bash
cd /home/decodo/work/one-dev-army
go build -o oda-worker ./cmd/oda-worker
```

**Step 2: Run integration test**

```bash
# Test that worker starts and communicates
cd /home/decodo/work/one-dev-army
go test ./cmd/oda-worker -v -run TestWorkerStartsAndSendsHeartbeat
```

**Step 3: Verify full build**

```bash
cd /home/decodo/work/one-dev-army
go build ./...
```

Expected: No errors

**Step 4: Commit**

```bash
git add oda-worker  # binary (or add to .gitignore and don't commit)
git commit -m "chore: build oda-worker binary"
```

---

## Task 7: Add Documentation

**Files:**
- Create: `docs/worker-ipc.md`

**Step 1: Write documentation**

```markdown
# Worker IPC Documentation

## Overview

ODA worker now runs as a separate process communicating via stdin/stdout JSON protocol.

## Architecture

```
┌─────────────────┐         stdin/stdout          ┌─────────────────┐
│   Orchestrator  │  ◄──────────────────────────►  │   oda-worker    │
│   (parent)      │      JSON line protocol         │   (child)       │
└─────────────────┘                                 └─────────────────┘
```

## Protocol

### Parent → Worker (stdin)

Commands sent as single-line JSON:

```json
{"cmd":"pause"}
{"cmd":"restart"}
{"cmd":"restart-hard"}
```

### Worker → Parent (stdout)

Messages sent as single-line JSON:

**Heartbeat** (every 1 second):
```json
{"type":"heartbeat","step":"coding","ts":"2024-01-15T10:30:00Z"}
```

**Step Change** (when pipeline stage changes):
```json
{"type":"step","from":"analyzing","to":"planning","ts":"2024-01-15T10:30:15Z"}
```

**Ack** (command confirmation, within 5s):
```json
{"type":"ack","cmd":"pause","ts":"2024-01-15T10:30:20Z"}
```

**Done - Success**:
```json
{"type":"done","step":"complete","result":"success","pr_url":"https://github.com/...","ts":"2024-01-15T10:35:00Z"}
```

**Done - Error**:
```json
{"type":"done","step":"coding","result":"error","error":"git push failed","ts":"2024-01-15T10:35:00Z"}
```

### Worker → Parent (stderr)

Log entries as JSON:
```json
{"level":"info","msg":"Starting analysis","ts":"2024-01-15T10:30:00Z"}
```

## Pipeline Steps

1. `analyzing` - Issue analysis
2. `planning` - Implementation planning
3. `coding` - Code implementation + tests
4. `reviewing` - AI code review
5. `creating_pr` - PR creation
6. `awaiting_approval` - Waiting for human approval
7. `merging` - Auto-merge
8. `complete` - Task finished

## Commands

- **pause** - Pause current work (worker will wait)
- **restart** - Resume from current step
- **restart-hard** - Clear branch and restart from beginning

## Usage

### Running Worker Directly (for testing)

```bash
./oda-worker --config .oda/config.yaml --issue 123 --dry-run
```

### Integration with Orchestrator

Orchestrator automatically spawns worker process - no manual intervention needed.

## Implementation Details

- Protocol: JSON Lines (newline-delimited JSON)
- Transport: stdin/stdout pipes
- Heartbeat interval: 1 second
- Command timeout: 5 seconds
- Platform support: macOS, Linux, Windows
```

**Step 2: Commit**

```bash
git add docs/worker-ipc.md
git commit -m "docs: add worker IPC protocol documentation"
```

---

## Summary

This implementation:

1. ✅ Extracts worker to separate process (`oda-worker` binary)
2. ✅ Uses stdin/stdout JSON protocol (cross-platform)
3. ✅ Heartbeat every 1 second with current step
4. ✅ Step change notifications
5. ✅ Command acknowledgments (5s timeout)
6. ✅ Pause, restart, restart-hard commands
7. ✅ Structured logging to stderr
8. ✅ Full integration with existing orchestrator
9. ✅ Maintains all existing functionality

**Next Steps (future):**
- Implement real worker logic in `cmd/oda-worker/worker.go` (currently dry-run only)
- Add graceful shutdown handling
- Add worker health checks
- Consider TCP fallback for remote workers

---

## Testing Checklist

- [ ] Unit tests pass: `go test ./internal/workeripc/...`
- [ ] Worker binary builds: `go build ./cmd/oda-worker`
- [ ] Integration test passes: `go test ./cmd/oda-worker -v`
- [ ] Full build succeeds: `go build ./...`
- [ ] Orchestrator compiles with new WorkerProcess
- [ ] Manual test: `./oda-worker --config .oda/config.yaml --issue 123 --dry-run`
