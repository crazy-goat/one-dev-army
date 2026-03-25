package worker

import (
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/crazy-goat/one-dev-army/internal/opencode"
)

func TestStepLogger_CreatesLogFile(t *testing.T) {
	tmpDir := t.TempDir()
	artifactDir := filepath.Join(tmpDir, ".oda", "artifacts")

	logger, err := NewStepLogger(artifactDir, 123, "test-step", "")
	if err != nil {
		t.Fatalf("NewStepLogger failed: %v", err)
	}

	if logger.issueNum != 123 {
		t.Errorf("expected issueNum=123, got %d", logger.issueNum)
	}

	if logger.stepName != "test-step" {
		t.Errorf("expected stepName='test-step', got %s", logger.stepName)
	}

	expectedLogDir := filepath.Join(artifactDir, "logs")
	if logger.logDir != expectedLogDir {
		t.Errorf("expected logDir=%s, got %s", expectedLogDir, logger.logDir)
	}

	if _, err := os.Stat(expectedLogDir); os.IsNotExist(err) {
		t.Errorf("log directory was not created: %s", expectedLogDir)
	}
}

func TestStepLogger_StartAndEnd(t *testing.T) {
	tmpDir := t.TempDir()
	artifactDir := filepath.Join(tmpDir, ".oda", "artifacts")

	logger, err := NewStepLogger(artifactDir, 456, "technical-planning", "")
	if err != nil {
		t.Fatalf("NewStepLogger failed: %v", err)
	}

	if err := logger.Start(); err != nil {
		t.Fatalf("Start failed: %v", err)
	}

	time.Sleep(10 * time.Millisecond)

	if err := logger.End(true, "test output"); err != nil {
		t.Fatalf("End failed: %v", err)
	}

	logger.Close()

	entries, err := os.ReadDir(logger.logDir)
	if err != nil {
		t.Fatalf("failed to read log directory: %v", err)
	}

	if len(entries) != 1 {
		t.Fatalf("expected 1 log file, got %d", len(entries))
	}

	content, err := os.ReadFile(filepath.Join(logger.logDir, entries[0].Name()))
	if err != nil {
		t.Fatalf("failed to read log file: %v", err)
	}

	contentStr := string(content)

	if !strings.Contains(contentStr, "STEP START: technical-planning") {
		t.Error("log should contain STEP START marker")
	}

	if !strings.Contains(contentStr, "Issue: #456") {
		t.Error("log should contain issue number")
	}

	if !strings.Contains(contentStr, "STEP END: technical-planning") {
		t.Error("log should contain STEP END marker")
	}

	if !strings.Contains(contentStr, "Status: SUCCESS") {
		t.Error("log should contain SUCCESS status")
	}

	if !strings.Contains(contentStr, "Duration:") {
		t.Error("log should contain duration")
	}

	if !strings.Contains(contentStr, "Output: test output") {
		t.Error("log should contain output")
	}
}

func TestStepLogger_EndFailure(t *testing.T) {
	tmpDir := t.TempDir()
	artifactDir := filepath.Join(tmpDir, ".oda", "artifacts")

	logger, err := NewStepLogger(artifactDir, 789, "implement", "")
	if err != nil {
		t.Fatalf("NewStepLogger failed: %v", err)
	}

	if err := logger.Start(); err != nil {
		t.Fatalf("Start failed: %v", err)
	}

	if err := logger.End(false, ""); err != nil {
		t.Fatalf("End failed: %v", err)
	}

	logger.Close()

	entries, err := os.ReadDir(logger.logDir)
	if err != nil {
		t.Fatalf("failed to read log directory: %v", err)
	}

	content, _ := os.ReadFile(filepath.Join(logger.logDir, entries[0].Name()))

	if !strings.Contains(string(content), "Status: FAILURE") {
		t.Error("log should contain FAILURE status")
	}
}

func TestStepLogger_LogLLMResponse_TextOnly(t *testing.T) {
	tmpDir := t.TempDir()
	artifactDir := filepath.Join(tmpDir, ".oda", "artifacts")

	logger, err := NewStepLogger(artifactDir, 100, "test-step", "")
	if err != nil {
		t.Fatalf("NewStepLogger failed: %v", err)
	}

	if err := logger.Start(); err != nil {
		t.Fatalf("Start failed: %v", err)
	}

	msg := &opencode.Message{
		Parts: []opencode.Part{
			{Type: "text", Text: "Hello, this is a test response"},
		},
	}

	logger.LogLLMResponse(msg)
	logger.End(true, "")
	logger.Close()

	entries, _ := os.ReadDir(logger.logDir)
	content, _ := os.ReadFile(filepath.Join(logger.logDir, entries[0].Name()))

	if !strings.Contains(string(content), "LLM Response:") {
		t.Error("log should contain LLM Response header")
	}

	if !strings.Contains(string(content), "Hello, this is a test response") {
		t.Error("log should contain text content")
	}
}

func TestStepLogger_LogLLMResponse_WithToolCall(t *testing.T) {
	tmpDir := t.TempDir()
	artifactDir := filepath.Join(tmpDir, ".oda", "artifacts")

	logger, err := NewStepLogger(artifactDir, 101, "test-step", "")
	if err != nil {
		t.Fatalf("NewStepLogger failed: %v", err)
	}

	if err := logger.Start(); err != nil {
		t.Fatalf("Start failed: %v", err)
	}

	msg := &opencode.Message{
		Parts: []opencode.Part{
			{Type: "text", Text: "I'll help you with that"},
			{
				Type: "tool_call",
				ToolCall: &opencode.ToolCall{
					ID:        "call-123",
					Name:      "read_file",
					Arguments: []byte(`{"path": "/tmp/test.txt"}`),
				},
			},
		},
	}

	logger.LogLLMResponse(msg)
	logger.End(true, "")
	logger.Close()

	entries, _ := os.ReadDir(logger.logDir)
	content, _ := os.ReadFile(filepath.Join(logger.logDir, entries[0].Name()))

	if !strings.Contains(string(content), "Tool Call: read_file (ID: call-123)") {
		t.Error("log should contain tool call info")
	}

	if !strings.Contains(string(content), `"path": "/tmp/test.txt"`) {
		t.Error("log should contain tool arguments")
	}
}

func TestStepLogger_LogLLMResponse_WithToolResult(t *testing.T) {
	tmpDir := t.TempDir()
	artifactDir := filepath.Join(tmpDir, ".oda", "artifacts")

	logger, err := NewStepLogger(artifactDir, 102, "test-step", "")
	if err != nil {
		t.Fatalf("NewStepLogger failed: %v", err)
	}

	if err := logger.Start(); err != nil {
		t.Fatalf("Start failed: %v", err)
	}

	msg := &opencode.Message{
		Parts: []opencode.Part{
			{
				Type: "tool_result",
				ToolResult: &opencode.ToolResult{
					ID:     "call-123",
					Output: "File contents here",
				},
			},
		},
	}

	logger.LogLLMResponse(msg)
	logger.End(true, "")
	logger.Close()

	entries, _ := os.ReadDir(logger.logDir)
	content, _ := os.ReadFile(filepath.Join(logger.logDir, entries[0].Name()))

	if !strings.Contains(string(content), "Tool Result: SUCCESS (ID: call-123)") {
		t.Error("log should contain successful tool result")
	}

	if !strings.Contains(string(content), "File contents here") {
		t.Error("log should contain tool output")
	}
}

func TestStepLogger_LogLLMResponse_WithToolError(t *testing.T) {
	tmpDir := t.TempDir()
	artifactDir := filepath.Join(tmpDir, ".oda", "artifacts")

	logger, err := NewStepLogger(artifactDir, 103, "test-step", "")
	if err != nil {
		t.Fatalf("NewStepLogger failed: %v", err)
	}

	if err := logger.Start(); err != nil {
		t.Fatalf("Start failed: %v", err)
	}

	msg := &opencode.Message{
		Parts: []opencode.Part{
			{
				Type: "tool_result",
				ToolResult: &opencode.ToolResult{
					ID:    "call-456",
					Error: "file not found",
				},
			},
		},
	}

	logger.LogLLMResponse(msg)
	logger.End(true, "")
	logger.Close()

	entries, _ := os.ReadDir(logger.logDir)
	content, _ := os.ReadFile(filepath.Join(logger.logDir, entries[0].Name()))

	if !strings.Contains(string(content), "Tool Result: ERROR (ID: call-456)") {
		t.Error("log should contain error tool result")
	}

	if !strings.Contains(string(content), "file not found") {
		t.Error("log should contain error message")
	}
}

func TestStepLogger_NilSafety(t *testing.T) {
	var logger *StepLogger

	logger.LogLLMResponse(nil)
	logger.Logf("test")
	logger.End(true, "")
	logger.Close()

	logger, _ = NewStepLogger(t.TempDir(), 1, "test", "")
	logger.LogLLMResponse(nil)
	logger.Logf("test")
	logger.End(true, "")
	logger.Close()
}

func TestStepLogger_Logf(t *testing.T) {
	tmpDir := t.TempDir()
	artifactDir := filepath.Join(tmpDir, ".oda", "artifacts")

	logger, err := NewStepLogger(artifactDir, 200, "test-step", "")
	if err != nil {
		t.Fatalf("NewStepLogger failed: %v", err)
	}

	if err := logger.Start(); err != nil {
		t.Fatalf("Start failed: %v", err)
	}

	logger.Logf("Custom message: %s", "hello")
	logger.Logf("Number: %d", 42)
	logger.End(true, "")
	logger.Close()

	entries, _ := os.ReadDir(logger.logDir)
	content, _ := os.ReadFile(filepath.Join(logger.logDir, entries[0].Name()))

	if !strings.Contains(string(content), "Custom message: hello") {
		t.Error("log should contain custom message")
	}

	if !strings.Contains(string(content), "Number: 42") {
		t.Error("log should contain number")
	}
}

func TestStepLogger_ConcurrentWrites(t *testing.T) {
	tmpDir := t.TempDir()
	artifactDir := filepath.Join(tmpDir, ".oda", "artifacts")

	logger, err := NewStepLogger(artifactDir, 300, "test-step", "")
	if err != nil {
		t.Fatalf("NewStepLogger failed: %v", err)
	}

	if err := logger.Start(); err != nil {
		t.Fatalf("Start failed: %v", err)
	}

	done := make(chan bool, 10)
	for i := range 10 {
		go func(n int) {
			logger.Logf("Message %d", n)
			done <- true
		}(i)
	}

	for range 10 {
		<-done
	}

	logger.End(true, "")
	logger.Close()

	entries, _ := os.ReadDir(logger.logDir)
	content, _ := os.ReadFile(filepath.Join(logger.logDir, entries[0].Name()))

	for i := range 10 {
		expected := fmt.Sprintf("Message %d", i)
		if !strings.Contains(string(content), expected) {
			t.Errorf("log should contain '%s'", expected)
		}
	}
}

func TestStepLogger_Truncate(t *testing.T) {
	tests := []struct {
		input    string
		maxLen   int
		expected string
	}{
		{"short", 100, "short"},
		{"exactly ten", 11, "exactly ten"},
		{"this is a very long string that needs truncation", 20, "this is a very long ... (truncated)"},
	}

	for _, tt := range tests {
		result := Truncate(tt.input, tt.maxLen)
		if result != tt.expected {
			t.Errorf("Truncate(%q, %d) = %q, want %q", tt.input, tt.maxLen, result, tt.expected)
		}
	}
}

func TestStepLogger_Write(t *testing.T) {
	tmpDir := t.TempDir()
	artifactDir := filepath.Join(tmpDir, ".oda", "artifacts")

	logger, err := NewStepLogger(artifactDir, 400, "test-step", "")
	if err != nil {
		t.Fatalf("NewStepLogger failed: %v", err)
	}

	if err := logger.Start(); err != nil {
		t.Fatalf("Start failed: %v", err)
	}

	n, err := logger.Write([]byte("streaming chunk 1"))
	if err != nil {
		t.Fatalf("Write failed: %v", err)
	}
	if n != 17 {
		t.Errorf("expected 17 bytes written, got %d", n)
	}

	n, err = logger.Write([]byte(" chunk 2"))
	if err != nil {
		t.Fatalf("Write failed: %v", err)
	}
	if n != 8 {
		t.Errorf("expected 8 bytes written, got %d", n)
	}

	logger.End(true, "")
	logger.Close()

	entries, _ := os.ReadDir(logger.logDir)
	content, _ := os.ReadFile(filepath.Join(logger.logDir, entries[0].Name()))

	if !strings.Contains(string(content), "streaming chunk 1 chunk 2") {
		t.Error("log should contain streamed chunks concatenated")
	}
}

func TestStepLogger_Write_NilSafety(t *testing.T) {
	var logger *StepLogger

	n, err := logger.Write([]byte("test"))
	if err != nil {
		t.Errorf("Write on nil logger should not error, got: %v", err)
	}
	if n != 4 {
		t.Errorf("Write on nil logger should return len(p)=%d, got %d", 4, n)
	}

	logger, _ = NewStepLogger(t.TempDir(), 1, "test", "")

	n, err = logger.Write([]byte("test"))
	if err != nil {
		t.Errorf("Write before Start should not error, got: %v", err)
	}
	if n != 4 {
		t.Errorf("Write before Start should return len(p)=%d, got %d", 4, n)
	}
}

func TestStepLogger_ImplementsIOWriter(_ *testing.T) {
	var _ io.Writer = (*StepLogger)(nil)
}

func TestStepLogger_ProviderModelInLog(t *testing.T) {
	dir := t.TempDir()
	logger, err := NewStepLogger(dir, 42, "implement", "anthropic/claude-sonnet-4")
	if err != nil {
		t.Fatalf("NewStepLogger: %v", err)
	}
	defer logger.Close()

	if err := logger.Start(); err != nil {
		t.Fatalf("Start: %v", err)
	}
	_ = logger.End(true, "")

	files, _ := filepath.Glob(filepath.Join(dir, "logs", "*.log"))
	if len(files) != 1 {
		t.Fatalf("expected 1 log file, got %d", len(files))
	}
	content, _ := os.ReadFile(files[0])
	logStr := string(content)

	if !strings.Contains(logStr, "Provider: anthropic") {
		t.Errorf("log should contain 'Provider: anthropic', got:\n%s", logStr)
	}
	if !strings.Contains(logStr, "Model: claude-sonnet-4") {
		t.Errorf("log should contain 'Model: claude-sonnet-4', got:\n%s", logStr)
	}
}

func TestStepLogger_NoModelOmitsProviderModel(t *testing.T) {
	dir := t.TempDir()
	logger, err := NewStepLogger(dir, 42, "create-pr", "")
	if err != nil {
		t.Fatalf("NewStepLogger: %v", err)
	}
	defer logger.Close()

	if err := logger.Start(); err != nil {
		t.Fatalf("Start: %v", err)
	}
	_ = logger.End(true, "")

	files, _ := filepath.Glob(filepath.Join(dir, "logs", "*.log"))
	if len(files) != 1 {
		t.Fatalf("expected 1 log file, got %d", len(files))
	}
	content, _ := os.ReadFile(files[0])
	logStr := string(content)

	if strings.Contains(logStr, "Provider:") {
		t.Errorf("log should NOT contain 'Provider:' for empty model, got:\n%s", logStr)
	}
	if strings.Contains(logStr, "Model:") {
		t.Errorf("log should NOT contain 'Model:' for empty model, got:\n%s", logStr)
	}
}

func TestStepLogger_ModelWithoutProvider(t *testing.T) {
	dir := t.TempDir()
	logger, err := NewStepLogger(dir, 42, "test-step", "gpt-4o")
	if err != nil {
		t.Fatalf("NewStepLogger: %v", err)
	}
	defer logger.Close()

	if err := logger.Start(); err != nil {
		t.Fatalf("Start: %v", err)
	}
	_ = logger.End(true, "")

	files, _ := filepath.Glob(filepath.Join(dir, "logs", "*.log"))
	if len(files) != 1 {
		t.Fatalf("expected 1 log file, got %d", len(files))
	}
	content, _ := os.ReadFile(files[0])
	logStr := string(content)

	if strings.Contains(logStr, "Provider:") {
		t.Errorf("log should NOT contain 'Provider:' for model without slash, got:\n%s", logStr)
	}
	if !strings.Contains(logStr, "Model: gpt-4o") {
		t.Errorf("log should contain 'Model: gpt-4o', got:\n%s", logStr)
	}
}

func TestStepLogger_ConcurrentWrite(t *testing.T) {
	tmpDir := t.TempDir()
	artifactDir := filepath.Join(tmpDir, ".oda", "artifacts")

	logger, err := NewStepLogger(artifactDir, 401, "test-step", "")
	if err != nil {
		t.Fatalf("NewStepLogger failed: %v", err)
	}

	if err := logger.Start(); err != nil {
		t.Fatalf("Start failed: %v", err)
	}

	done := make(chan bool, 10)
	for i := range 10 {
		go func(n int) {
			chunk := fmt.Sprintf("chunk-%d ", n)
			logger.Write([]byte(chunk))
			done <- true
		}(i)
	}

	for range 10 {
		<-done
	}

	logger.End(true, "")
	logger.Close()

	entries, _ := os.ReadDir(logger.logDir)
	content, _ := os.ReadFile(filepath.Join(logger.logDir, entries[0].Name()))

	for i := range 10 {
		expected := fmt.Sprintf("chunk-%d", i)
		if !strings.Contains(string(content), expected) {
			t.Errorf("log should contain '%s'", expected)
		}
	}
}
