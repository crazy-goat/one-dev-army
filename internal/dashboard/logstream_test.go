package dashboard

import (
	"os"
	"path/filepath"
	"testing"
	"time"
)

func TestNewLogStreamManager(t *testing.T) {
	hub := NewHub(false)
	rootDir := "/tmp/test"

	lsm := NewLogStreamManager(hub, rootDir, 0) // 0 uses default

	if lsm == nil {
		t.Fatal("Expected LogStreamManager to be created")
	}

	if lsm.hub != hub {
		t.Error("Expected hub to be set")
	}

	if lsm.rootDir != rootDir {
		t.Errorf("Expected rootDir to be %s, got %s", rootDir, lsm.rootDir)
	}

	if lsm.pollInterval != 500*time.Millisecond {
		t.Errorf("Expected pollInterval to be 500ms, got %v", lsm.pollInterval)
	}

	if lsm.fileStates == nil {
		t.Error("Expected fileStates to be initialized")
	}

	if lsm.stopCh == nil {
		t.Error("Expected stopCh to be initialized")
	}
}

func TestNewLogStreamManager_CustomInterval(t *testing.T) {
	hub := NewHub(false)
	rootDir := "/tmp/test"
	customInterval := 1 * time.Second

	lsm := NewLogStreamManager(hub, rootDir, customInterval)

	if lsm.pollInterval != customInterval {
		t.Errorf("Expected pollInterval to be %v, got %v", customInterval, lsm.pollInterval)
	}
}

func TestLogStreamManager_StartMonitoring(t *testing.T) {
	tempDir := t.TempDir()
	hub := NewHub(false)
	lsm := NewLogStreamManager(hub, tempDir, 0)

	err := lsm.StartMonitoring(123)
	if err != nil {
		t.Fatalf("Failed to start monitoring: %v", err)
	}

	if !lsm.IsMonitoring() {
		t.Error("Expected monitoring to be active")
	}

	if lsm.GetCurrentIssue() != 123 {
		t.Errorf("Expected current issue to be 123, got %d", lsm.GetCurrentIssue())
	}

	// Clean up
	lsm.StopMonitoring()
}

func TestLogStreamManager_StopMonitoring(t *testing.T) {
	tempDir := t.TempDir()
	hub := NewHub(false)
	lsm := NewLogStreamManager(hub, tempDir, 0)

	// Start monitoring
	lsm.StartMonitoring(123)

	// Stop monitoring
	lsm.StopMonitoring()

	if lsm.IsMonitoring() {
		t.Error("Expected monitoring to be stopped")
	}

	if lsm.GetCurrentIssue() != 0 {
		t.Errorf("Expected current issue to be 0, got %d", lsm.GetCurrentIssue())
	}
}

func TestLogStreamManager_StartMonitoring_SwitchIssue(t *testing.T) {
	tempDir := t.TempDir()
	hub := NewHub(false)
	lsm := NewLogStreamManager(hub, tempDir, 0)

	// Start monitoring first issue
	err := lsm.StartMonitoring(100)
	if err != nil {
		t.Fatalf("Failed to start monitoring: %v", err)
	}

	if lsm.GetCurrentIssue() != 100 {
		t.Errorf("Expected current issue to be 100, got %d", lsm.GetCurrentIssue())
	}

	// Switch to second issue
	err = lsm.StartMonitoring(200)
	if err != nil {
		t.Fatalf("Failed to switch monitoring: %v", err)
	}

	if lsm.GetCurrentIssue() != 200 {
		t.Errorf("Expected current issue to be 200, got %d", lsm.GetCurrentIssue())
	}

	// Clean up
	lsm.StopMonitoring()
}

func TestLogStreamManager_extractStepName(t *testing.T) {
	tests := []struct {
		filename string
		expected string
	}{
		{"20260326_120000_test-step.log", "test-step"},
		{"20260326_120000_analysis.log", "analysis"},
		{"20260326_120000_coding.log", "coding"},
		{"step.log", "step"},
		{"myfile.log", "myfile"},
	}

	lsm := NewLogStreamManager(nil, "/tmp", 0)

	for _, test := range tests {
		result := lsm.extractStepName(test.filename)
		if result != test.expected {
			t.Errorf("extractStepName(%q) = %q, expected %q", test.filename, result, test.expected)
		}
	}
}

func TestLogStreamManager_parseLogLine(t *testing.T) {
	tests := []struct {
		line     string
		stepName string
		filename string
		expected LogEntry
	}{
		{
			line:     "[2026-03-26 12:00:00] STEP START: test-step",
			stepName: "test-step",
			filename: "test.log",
			expected: LogEntry{
				Timestamp: "2026-03-26 12:00:00",
				Step:      "test-step",
				Message:   "STEP START: test-step",
				Level:     LogLevelInfo,
				File:      "test.log",
			},
		},
		{
			line:     "[2026-03-26 12:00:01] ERROR: Something went wrong",
			stepName: "test-step",
			filename: "test.log",
			expected: LogEntry{
				Timestamp: "2026-03-26 12:00:01",
				Step:      "test-step",
				Message:   "ERROR: Something went wrong",
				Level:     LogLevelError,
				File:      "test.log",
			},
		},
		{
			line:     "[2026-03-26 12:00:02] WARN: Warning message",
			stepName: "test-step",
			filename: "test.log",
			expected: LogEntry{
				Timestamp: "2026-03-26 12:00:02",
				Step:      "test-step",
				Message:   "WARN: Warning message",
				Level:     LogLevelWarn,
				File:      "test.log",
			},
		},
		{
			line:     "[2026-03-26 12:00:03] DEBUG: Debug info",
			stepName: "test-step",
			filename: "test.log",
			expected: LogEntry{
				Timestamp: "2026-03-26 12:00:03",
				Step:      "test-step",
				Message:   "DEBUG: Debug info",
				Level:     LogLevelDebug,
				File:      "test.log",
			},
		},
		{
			line:     "Plain message without timestamp",
			stepName: "test-step",
			filename: "test.log",
			expected: LogEntry{
				Timestamp: "",
				Step:      "test-step",
				Message:   "Plain message without timestamp",
				Level:     LogLevelInfo,
				File:      "test.log",
			},
		},
	}

	lsm := NewLogStreamManager(nil, "/tmp", 0)

	for _, test := range tests {
		result := lsm.parseLogLine(test.line, test.stepName, test.filename)

		if result.Timestamp != test.expected.Timestamp {
			t.Errorf("parseLogLine(%q) Timestamp = %q, expected %q", test.line, result.Timestamp, test.expected.Timestamp)
		}

		if result.Step != test.expected.Step {
			t.Errorf("parseLogLine(%q) Step = %q, expected %q", test.line, result.Step, test.expected.Step)
		}

		if result.Message != test.expected.Message {
			t.Errorf("parseLogLine(%q) Message = %q, expected %q", test.line, result.Message, test.expected.Message)
		}

		if result.Level != test.expected.Level {
			t.Errorf("parseLogLine(%q) Level = %v, expected %v", test.line, result.Level, test.expected.Level)
		}

		if result.File != test.expected.File {
			t.Errorf("parseLogLine(%q) File = %q, expected %q", test.line, result.File, test.expected.File)
		}
	}
}

func TestLogStreamManager_poll(t *testing.T) {
	tempDir := t.TempDir()
	logDir := filepath.Join(tempDir, ".oda", "artifacts", "123", "logs")

	// Create log directory
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

	hub := NewHub(false)
	go hub.Run()
	defer hub.Stop()

	lsm := NewLogStreamManager(hub, tempDir, 50*time.Millisecond)

	// Start monitoring
	if err := lsm.StartMonitoring(123); err != nil {
		t.Fatalf("Failed to start monitoring: %v", err)
	}
	defer lsm.StopMonitoring()

	// Wait for polling to process the file (need at least one poll cycle)
	time.Sleep(100 * time.Millisecond)

	// The file should have been processed
	lsm.mu.RLock()
	state, exists := lsm.fileStates["20260326_120000_test-step.log"]
	lsm.mu.RUnlock()

	if !exists {
		t.Error("Expected file state to exist")
	} else {
		if state.offset == 0 {
			t.Error("Expected offset to be greater than 0")
		}
		if !state.complete {
			t.Error("Expected file to be marked complete (STEP END found)")
		}
	}
}

func TestLogStreamManager_ConcurrentAccess(t *testing.T) {
	tempDir := t.TempDir()
	hub := NewHub(false)
	lsm := NewLogStreamManager(hub, tempDir, 10*time.Millisecond)

	// Test concurrent Start/Stop operations
	done := make(chan bool)

	go func() {
		for i := range 10 {
			lsm.StartMonitoring(i)
			time.Sleep(5 * time.Millisecond)
			lsm.StopMonitoring()
		}
		done <- true
	}()

	go func() {
		for range 10 {
			_ = lsm.IsMonitoring()
			_ = lsm.GetCurrentIssue()
			time.Sleep(5 * time.Millisecond)
		}
		done <- true
	}()

	// Wait for both goroutines
	<-done
	<-done

	// If we get here without deadlock or panic, the test passes
}

func TestLogStreamManagerInterface(_ *testing.T) {
	// Test that LogStreamManager implements LogStreamManagerInterface
	var _ LogStreamManagerInterface = (*LogStreamManager)(nil)
}
