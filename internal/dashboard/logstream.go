package dashboard

import (
	"bufio"
	"context"
	"log"
	"os"
	"path/filepath"
	"regexp"
	"sort"
	"strconv"
	"strings"
	"sync"
	"time"
)

// LogLevel represents the severity of a log message.
type LogLevel string

const (
	LogLevelInfo  LogLevel = "info"
	LogLevelWarn  LogLevel = "warn"
	LogLevelError LogLevel = "error"
	LogLevelDebug LogLevel = "debug"
)

// LogEntry represents a single log line.
type LogEntry struct {
	Timestamp string   `json:"timestamp"`
	Step      string   `json:"step"`
	Message   string   `json:"message"`
	Level     LogLevel `json:"level"`
	File      string   `json:"file"`
}

// fileState tracks the current state of a log file.
type fileState struct {
	offset   int64
	complete bool
	modTime  time.Time
}

// LogStreamManager manages real-time log streaming for active tasks.
// It uses a WebSocket-first strategy with Server-Sent Events (SSE) as a fallback
// for clients that cannot establish WebSocket connections. This provides
// reliable real-time log delivery with automatic reconnection support.
type LogStreamManager struct {
	hub     *Hub
	rootDir string

	// Current monitoring state
	mu          sync.RWMutex
	issueNumber int
	logDir      string
	active      bool

	// File tracking
	fileStates map[string]*fileState

	// Control channels
	stopCh chan struct{}

	// Goroutine management - ensures clean shutdown before restart
	monitorWg sync.WaitGroup

	// Configuration
	pollInterval time.Duration

	// Regex for parsing log lines
	timestampRe *regexp.Regexp
	levelRe     *regexp.Regexp
}

// NewLogStreamManager creates a new log stream manager.
// The pollInterval parameter controls how frequently log files are checked for updates.
// If zero or negative, defaults to 500ms.
func NewLogStreamManager(hub *Hub, rootDir string, pollInterval time.Duration) *LogStreamManager {
	if pollInterval <= 0 {
		pollInterval = 500 * time.Millisecond
	}

	return &LogStreamManager{
		hub:          hub,
		rootDir:      rootDir,
		fileStates:   make(map[string]*fileState),
		stopCh:       make(chan struct{}),
		pollInterval: pollInterval,
		timestampRe:  regexp.MustCompile(`^\[(\d{4}-\d{2}-\d{2}\s+\d{2}:\d{2}:\d{2})\]\s*(.*)$`),
		levelRe:      regexp.MustCompile(`(?i)\b(ERROR|WARN|WARNING|INFO|DEBUG)\b`),
	}
}

// StartMonitoring begins monitoring logs for a specific issue.
func (m *LogStreamManager) StartMonitoring(issueNumber int) error {
	m.mu.Lock()

	// Stop any existing monitoring and wait for goroutine to exit
	if m.active {
		m.stopInternal()
		m.mu.Unlock()

		// Wait for the old monitor goroutine to fully stop before proceeding
		// This prevents race conditions when recreating the stop channel
		// This must be done outside the lock to avoid deadlock
		m.monitorWg.Wait()

		m.mu.Lock()
	}

	m.issueNumber = issueNumber
	m.logDir = filepath.Join(m.rootDir, ".oda", "artifacts", strconv.Itoa(issueNumber), "logs")
	m.active = true
	m.fileStates = make(map[string]*fileState)
	m.stopCh = make(chan struct{})

	log.Printf("[LogStreamManager] Started monitoring for issue #%d", issueNumber)

	// Start the monitoring goroutine
	m.monitorWg.Add(1)
	go m.monitor()

	m.mu.Unlock()
	return nil
}

// StopMonitoring stops the current log monitoring.
func (m *LogStreamManager) StopMonitoring() {
	m.mu.Lock()

	if !m.active {
		m.mu.Unlock()
		return
	}

	m.stopInternal()
	m.mu.Unlock()

	// Wait for the monitor goroutine to fully stop
	// This must be done outside the lock to avoid deadlock
	m.monitorWg.Wait()

	m.mu.Lock()
	issueNumber := m.issueNumber
	m.mu.Unlock()

	log.Printf("[LogStreamManager] Stopped monitoring for issue #%d", issueNumber)
}

// stopInternal stops monitoring without acquiring lock (must be called with lock held).
func (m *LogStreamManager) stopInternal() {
	close(m.stopCh)
	m.active = false
	m.issueNumber = 0
	m.logDir = ""
}

// IsMonitoring returns true if currently monitoring a log stream.
func (m *LogStreamManager) IsMonitoring() bool {
	m.mu.RLock()
	defer m.mu.RUnlock()
	return m.active
}

// GetCurrentIssue returns the currently monitored issue number.
func (m *LogStreamManager) GetCurrentIssue() int {
	m.mu.RLock()
	defer m.mu.RUnlock()
	return m.issueNumber
}

// monitor is the main monitoring loop.
func (m *LogStreamManager) monitor() {
	defer m.monitorWg.Done()

	// Capture the current stopCh value to avoid race conditions
	// when StartMonitoring replaces the channel
	m.mu.RLock()
	stopCh := m.stopCh
	m.mu.RUnlock()

	// Create a context that can be canceled via stopCh
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	// Listen for stop signal to cancel context
	go func() {
		select {
		case <-stopCh:
			cancel()
		case <-ctx.Done():
			// Context already canceled, nothing to do
		}
	}()

	ticker := time.NewTicker(m.pollInterval)
	defer ticker.Stop()

	for {
		select {
		case <-ctx.Done():
			return
		case <-stopCh:
			return
		case <-ticker.C:
			m.poll(ctx)
		}
	}
}

// poll checks for new log files and reads new content.
// The context parameter allows for cancellation and timeout control following Go best practices.
func (m *LogStreamManager) poll(ctx context.Context) {
	m.mu.RLock()
	logDir := m.logDir
	issueNumber := m.issueNumber
	m.mu.RUnlock()

	if logDir == "" {
		return
	}

	// Check if context is canceled before proceeding
	select {
	case <-ctx.Done():
		return
	default:
	}

	// Check if log directory exists
	info, err := os.Stat(logDir)
	if err != nil || !info.IsDir() {
		return
	}

	// Read directory entries
	entries, err := os.ReadDir(logDir)
	if err != nil {
		log.Printf("[LogStreamManager] Error reading log directory: %v", err)
		return
	}

	// Collect log files
	var logFiles []string
	for _, entry := range entries {
		if entry.IsDir() {
			continue
		}
		name := entry.Name()
		if !strings.HasSuffix(name, ".log") {
			continue
		}
		logFiles = append(logFiles, name)
	}

	// Sort files for consistent ordering
	sort.Strings(logFiles)

	// Process each log file
	for _, filename := range logFiles {
		// Check context before processing each file
		select {
		case <-ctx.Done():
			return
		default:
		}
		m.processFile(filename, logDir, issueNumber)
	}
}

// processFile reads new content from a single log file.
func (m *LogStreamManager) processFile(filename, logDir string, issueNumber int) {
	filepath := filepath.Join(logDir, filename)

	// Get current file state
	m.mu.Lock()
	state, exists := m.fileStates[filename]
	if !exists {
		state = &fileState{}
		m.fileStates[filename] = state
	}

	if state.complete {
		m.mu.Unlock()
		return
	}
	m.mu.Unlock()

	// Check file modification time first (optimization)
	info, err := os.Stat(filepath)
	if err != nil {
		return
	}

	if info.ModTime().Equal(state.modTime) && state.offset > 0 {
		// File hasn't changed since last read
		return
	}

	// Open and read file
	file, err := os.Open(filepath)
	if err != nil {
		log.Printf("[LogStreamManager] Error opening log file %s: %v", filename, err)
		return
	}
	defer file.Close()

	// Seek to last known position
	if state.offset > 0 {
		_, err = file.Seek(state.offset, 0)
		if err != nil {
			log.Printf("[LogStreamManager] Error seeking in log file %s: %v", filename, err)
			return
		}
	}

	// Read new lines
	scanner := bufio.NewScanner(file)
	stepName := m.extractStepName(filename)

	for scanner.Scan() {
		line := scanner.Text()
		entry := m.parseLogLine(line, stepName, filename)

		// Broadcast via WebSocket
		if m.hub != nil {
			m.hub.BroadcastLogStream(
				issueNumber,
				entry.Step,
				entry.Timestamp,
				entry.Message,
				string(entry.Level),
				entry.File,
			)
		}

		// Check for completion marker
		if strings.Contains(line, "STEP END:") {
			m.mu.Lock()
			state.complete = true
			m.mu.Unlock()
			break
		}
	}

	if err := scanner.Err(); err != nil {
		log.Printf("[LogStreamManager] Error reading log file %s: %v", filename, err)
		return
	}

	// Update file state
	currentOffset, _ := file.Seek(0, 1)
	m.mu.Lock()
	state.offset = currentOffset
	state.modTime = info.ModTime()
	m.mu.Unlock()
}

// extractStepName extracts the step name from a log filename.
// Format: YYYYmmddHHMMSS_<step>.log
func (*LogStreamManager) extractStepName(filename string) string {
	// Remove .log extension
	name := strings.TrimSuffix(filename, ".log")

	// Find the last underscore
	if idx := strings.LastIndex(name, "_"); idx != -1 {
		return name[idx+1:]
	}

	return name
}

// parseLogLine parses a single log line into a LogEntry.
func (m *LogStreamManager) parseLogLine(line, stepName, filename string) LogEntry {
	entry := LogEntry{
		Step:  stepName,
		File:  filename,
		Level: LogLevelInfo,
	}

	// Try to extract timestamp
	matches := m.timestampRe.FindStringSubmatch(line)
	if len(matches) >= 3 {
		entry.Timestamp = matches[1]
		entry.Message = matches[2]
	} else {
		entry.Message = line
	}

	// Try to extract log level
	levelMatches := m.levelRe.FindStringSubmatch(line)
	if len(levelMatches) > 0 {
		levelStr := strings.ToUpper(levelMatches[0])
		switch levelStr {
		case "ERROR":
			entry.Level = LogLevelError
		case "WARN", "WARNING":
			entry.Level = LogLevelWarn
		case "DEBUG":
			entry.Level = LogLevelDebug
		default:
			entry.Level = LogLevelInfo
		}
	}

	return entry
}

// Compile-time interface check: ensure LogStreamManager implements LogStreamManagerInterface.
var _ LogStreamManagerInterface = (*LogStreamManager)(nil)

// LogStreamManagerInterface defines the interface for log stream management.
type LogStreamManagerInterface interface {
	StartMonitoring(issueNumber int) error
	StopMonitoring()
	IsMonitoring() bool
}
