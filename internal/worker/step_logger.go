package worker

import (
	"fmt"
	"os"
	"path/filepath"
	"strconv"
	"sync"
	"time"

	"github.com/crazy-goat/one-dev-army/internal/opencode"
)

type StepLogger struct {
	file      *os.File
	issueNum  int
	stepName  string
	startTime time.Time
	logDir    string
	mu        sync.Mutex
}

func NewStepLogger(artifactDir string, issueNum int, stepName string) (*StepLogger, error) {
	logDir := filepath.Join(artifactDir, strconv.Itoa(issueNum), "logs")
	if err := os.MkdirAll(logDir, 0o755); err != nil {
		return nil, fmt.Errorf("creating log directory %s: %w", logDir, err)
	}

	return &StepLogger{
		issueNum: issueNum,
		stepName: stepName,
		logDir:   logDir,
	}, nil
}

func (l *StepLogger) Start() error {
	if l == nil {
		return nil
	}

	l.mu.Lock()
	defer l.mu.Unlock()

	if l.file != nil {
		return nil
	}

	l.startTime = time.Now()
	timestamp := l.startTime.Format("20060102_150405")
	filename := fmt.Sprintf("%s_%s.log", timestamp, l.stepName)
	filepath := filepath.Join(l.logDir, filename)

	file, err := os.Create(filepath)
	if err != nil {
		return fmt.Errorf("creating log file %s: %w", filepath, err)
	}

	l.file = file
	l.logf("STEP START: %s", l.stepName)
	l.logf("Issue: #%d", l.issueNum)
	l.logf("Start Time: %s", l.startTime.Format("2006-01-02 15:04:05"))
	l.logf("---")

	return nil
}

func (l *StepLogger) LogLLMResponse(msg *opencode.Message) {
	if l == nil || msg == nil {
		return
	}

	l.mu.Lock()
	defer l.mu.Unlock()

	if l.file == nil {
		return
	}

	l.logf("---")
	l.logf("LLM Response:")

	for _, part := range msg.Parts {
		switch part.Type {
		case "text":
			if part.Text != "" {
				l.logf("Text: %s", part.Text)
			}
		case "tool_call":
			if part.ToolCall != nil {
				l.logf("Tool Call: %s (ID: %s)", part.ToolCall.Name, part.ToolCall.ID)
				if len(part.ToolCall.Arguments) > 0 {
					l.logf("  Arguments: %s", string(part.ToolCall.Arguments))
				}
			}
		case "tool_result":
			if part.ToolResult != nil {
				if part.ToolResult.Error != "" {
					l.logf("Tool Result: ERROR (ID: %s)", part.ToolResult.ID)
					l.logf("  Error: %s", part.ToolResult.Error)
				} else {
					l.logf("Tool Result: SUCCESS (ID: %s)", part.ToolResult.ID)
					if part.ToolResult.Output != "" {
						l.logf("  Output: %s", Truncate(part.ToolResult.Output, 1000))
					}
				}
			}
		}
	}

	l.logf("---")
}

func (l *StepLogger) Logf(format string, args ...any) {
	if l == nil {
		return
	}

	l.mu.Lock()
	defer l.mu.Unlock()

	if l.file == nil {
		return
	}

	l.logf(format, args...)
}

func (l *StepLogger) End(success bool, output string) error {
	if l == nil {
		return nil
	}

	l.mu.Lock()
	defer l.mu.Unlock()

	if l.file == nil {
		return nil
	}

	duration := time.Since(l.startTime)
	status := "SUCCESS"
	if !success {
		status = "FAILURE"
	}

	l.logf("---")
	l.logf("STEP END: %s", l.stepName)
	l.logf("Status: %s", status)
	l.logf("Duration: %s", duration.Round(time.Millisecond))
	if output != "" {
		l.logf("Output: %s", output)
	}

	return nil
}

func (l *StepLogger) Close() error {
	if l == nil {
		return nil
	}

	l.mu.Lock()
	defer l.mu.Unlock()

	if l.file == nil {
		return nil
	}

	err := l.file.Close()
	l.file = nil
	return err
}

func (l *StepLogger) logf(format string, args ...any) {
	timestamp := time.Now().Format("2006-01-02 15:04:05")
	line := fmt.Sprintf("[%s] %s\n", timestamp, fmt.Sprintf(format, args...))
	_, _ = l.file.WriteString(line)
}

func Truncate(s string, maxLen int) string {
	if len(s) <= maxLen {
		return s
	}
	return s[:maxLen] + "... (truncated)"
}
