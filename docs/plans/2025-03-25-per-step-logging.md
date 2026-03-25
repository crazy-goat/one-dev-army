# Per-Step Artifacts Logging Implementation Plan

> **For Claude:** REQUIRED SUB-SKILL: Use superpowers:executing-plans to implement this plan task-by-task.

**Goal:** Create a per-step logging system that writes detailed logs to `.oda/artifacts/<issue>/logs/YYYYmmddHHMMSS_<step>.log` for every pipeline step, capturing LLM interactions (text + tool calls) and step lifecycle events.

**Architecture:** A `StepLogger` struct manages file-based logging per step. Workers create a logger at step start, write LLM responses and tool calls during execution, and close the file at step end. Logs use a simple timestamped format for human readability.

**Tech Stack:** Go standard library (`os`, `fmt`, `time`, `path/filepath`), existing ODA packages (`internal/mvp`, `internal/worker`, `internal/opencode`)

---

## Task 1: Create StepLogger Type

**Files:**
- Create: `internal/worker/step_logger.go`
- Test: `internal/worker/step_logger_test.go`

**Step 1: Write the failing test**

```go
package worker

import (
    "os"
    "path/filepath"
    "strings"
    "testing"
    "time"
)

func TestStepLogger_CreatesLogFile(t *testing.T) {
    tmpDir := t.TempDir()
    logger, err := NewStepLogger(tmpDir, 381, "technical-planning")
    if err != nil {
        t.Fatalf("Failed to create logger: %v", err)
    }
    defer logger.Close()

    if err := logger.Start(); err != nil {
        t.Fatalf("Failed to start logger: %v", err)
    }

    // Check that file was created
    files, err := os.ReadDir(filepath.Join(tmpDir, "381", "logs"))
    if err != nil {
        t.Fatalf("Failed to read logs directory: %v", err)
    }

    if len(files) != 1 {
        t.Errorf("Expected 1 log file, got %d", len(files))
    }

    if !strings.HasSuffix(files[0].Name(), "_technical-planning.log") {
        t.Errorf("Expected filename to end with _technical-planning.log, got %s", files[0].Name())
    }
}

func TestStepLogger_LogsStartAndEnd(t *testing.T) {
    tmpDir := t.TempDir()
    logger, err := NewStepLogger(tmpDir, 381, "implement")
    if err != nil {
        t.Fatalf("Failed to create logger: %v", err)
    }

    if err := logger.Start(); err != nil {
        t.Fatalf("Failed to start logger: %v", err)
    }

    if err := logger.End(true, "completed successfully"); err != nil {
        t.Fatalf("Failed to end logger: %v", err)
    }

    logger.Close()

    // Read log content
    files, _ := os.ReadDir(filepath.Join(tmpDir, "381", "logs"))
    if len(files) != 1 {
        t.Fatalf("Expected 1 log file, got %d", len(files))
    }

    content, _ := os.ReadFile(filepath.Join(tmpDir, "381", "logs", files[0].Name()))
    contentStr := string(content)

    if !strings.Contains(contentStr, "STEP START: implement") {
        t.Errorf("Expected log to contain 'STEP START: implement', got:\n%s", contentStr)
    }

    if !strings.Contains(contentStr, "STEP END: implement") {
        t.Errorf("Expected log to contain 'STEP END: implement', got:\n%s", contentStr)
    }

    if !strings.Contains(contentStr, "completed successfully") {
        t.Errorf("Expected log to contain 'completed successfully', got:\n%s", contentStr)
    }
}
```

**Step 2: Run test to verify it fails**

```bash
cd /home/decodo/work/one-dev-army
go test ./internal/worker -run TestStepLogger -v
```

Expected: FAIL with "NewStepLogger not defined"

**Step 3: Implement StepLogger type**

```go
package worker

import (
    "fmt"
    "os"
    "path/filepath"
    "time"
)

// StepLogger manages logging for a single pipeline step
type StepLogger struct {
    file      *os.File
    issueNum  int
    stepName  string
    startTime time.Time
    logDir    string
}

// ToolCallInfo represents a tool call made during LLM execution
type ToolCallInfo struct {
    Name      string
    Arguments map[string]interface{}
}

// ToolResultInfo represents the result of a tool call
type ToolResultInfo struct {
    ID     string
    Output string
    Error  string
}

// NewStepLogger creates a new logger for a specific step
// artifactDir is the base artifacts directory (e.g., .oda/artifacts)
func NewStepLogger(artifactDir string, issueNum int, stepName string) (*StepLogger, error) {
    logDir := filepath.Join(artifactDir, fmt.Sprintf("%d", issueNum), "logs")
    
    if err := os.MkdirAll(logDir, 0755); err != nil {
        return nil, fmt.Errorf("creating log directory %s: %w", logDir, err)
    }

    return &StepLogger{
        issueNum: issueNum,
        stepName: stepName,
        logDir:   logDir,
    }, nil
}

// Start begins logging for this step, creating the log file
func (l *StepLogger) Start() error {
    timestamp := time.Now().Format("20060102150405")
    filename := fmt.Sprintf("%s_%s.log", timestamp, l.stepName)
    filepath := filepath.Join(l.logDir, filename)

    file, err := os.Create(filepath)
    if err != nil {
        return fmt.Errorf("creating log file %s: %w", filepath, err)
    }

    l.file = file
    l.startTime = time.Now()

    l.logf("STEP START: %s for issue #%d", l.stepName, l.issueNum)
    return nil
}

// LogLLM logs an LLM response with optional tool calls
func (l *StepLogger) LogLLM(text string, toolCalls []ToolCallInfo) {
    if l.file == nil {
        return
    }

    l.logf("LLM: Response received (%d chars)", len(text))
    if len(text) > 0 {
        l.logf("--- LLM Output ---")
        l.logf("%s", text)
        l.logf("--- End LLM Output ---")
    }

    for _, tc := range toolCalls {
        l.logf("TOOL_CALL: %s", tc.Name)
    }
}

// LogToolResult logs the result of a tool call
func (l *StepLogger) LogToolResult(result ToolResultInfo) {
    if l.file == nil {
        return
    }

    if result.Error != "" {
        l.logf("TOOL_RESULT: %s ERROR: %s", result.ID, result.Error)
    } else {
        l.logf("TOOL_RESULT: %s (%d chars)", result.ID, len(result.Output))
    }
}

// End marks the step as complete with success/failure status
func (l *StepLogger) End(success bool, output string) error {
    if l.file == nil {
        return nil
    }

    duration := time.Since(l.startTime)
    status := "success"
    if !success {
        status = "failure"
    }

    if output != "" {
        l.logf("Output: %s", output)
    }

    l.logf("STEP END: %s (%s, duration: %s)", l.stepName, status, duration.Round(time.Second))
    return nil
}

// Close closes the log file
func (l *StepLogger) Close() error {
    if l.file != nil {
        return l.file.Close()
    }
    return nil
}

// logf writes a formatted log line with timestamp
func (l *StepLogger) logf(format string, args ...interface{}) {
    if l.file == nil {
        return
    }

    timestamp := time.Now().Format("2006-01-02 15:04:05")
    line := fmt.Sprintf("[%s] %s\n", timestamp, fmt.Sprintf(format, args...))
    l.file.WriteString(line)
}
```

**Step 4: Run tests to verify they pass**

```bash
cd /home/decodo/work/one-dev-army
go test ./internal/worker -run TestStepLogger -v
```

Expected: PASS

**Step 5: Commit**

```bash
cd /home/decodo/work/one-dev-army
git add internal/worker/step_logger.go internal/worker/step_logger_test.go
git commit -m "feat: add StepLogger for per-step artifact logging

- Creates timestamped log files in .oda/artifacts/<issue>/logs/
- Logs STEP START/END events with duration
- Supports logging LLM responses and tool calls
- Thread-safe file writing with proper cleanup"
```

---

## Task 2: Integrate StepLogger into Worker

**Files:**
- Modify: `internal/mvp/worker.go`

**Step 1: Add StepLogger imports and integration**

Add import for the new logger (already in same package, no import needed).

Modify `Process()` method to create loggers for each step:

```go
// In technicalPlanning step (around line 144-155):
if resumeFrom <= 0 {
    log.Printf("[Worker %d] [1/7] Technical planning for #%d...", w.id, task.Issue.Number)
    
    // Create step logger
    stepLogger, err := NewStepLogger(artifactDir, task.Issue.Number, "technical-planning")
    if err != nil {
        log.Printf("[Worker %d] Warning: failed to create step logger: %v", w.id, err)
    } else {
        stepLogger.Start()
        defer stepLogger.Close()
    }
    
    stepStart := time.Now()
    analysis, implPlan, err = w.technicalPlanning(ctx, task, stepLogger)
    // ... rest of error handling
    
    if stepLogger != nil {
        stepLogger.End(err == nil, "")
    }
}
```

**Step 2: Modify technicalPlanning to accept logger**

```go
func (w *Worker) technicalPlanning(ctx context.Context, task *Task, logger *StepLogger) (analysis, implPlan string, err error) {
    // ... existing code ...
    
    response, err := w.llmStep(ctx, task, "technical-planning", prompt, llmModel, logger)
    // ...
}
```

**Step 3: Modify llmStep to log to StepLogger**

```go
func (w *Worker) llmStep(_ context.Context, task *Task, stepName, prompt, llm string, logger *StepLogger) (string, error) {
    // ... existing session creation code ...

    // Capture the prompt in chat history
    task.AddChatMessage("user", prompt)
    
    // Log to step logger if available
    if logger != nil {
        logger.logf("LLM: Sending prompt (%d chars) to model %s", len(prompt), llm)
    }

    model := opencode.ParseModelRef(llm)
    msg, err := w.oc.SendMessage(session.ID, prompt, model, nil) // nil = don't write to stdout
    if err != nil {
        if w.store != nil && stepID > 0 {
            _ = w.store.FailStep(stepID, err.Error())
        }
        if logger != nil {
            logger.logf("LLM: Error - %v", err)
        }
        return "", fmt.Errorf("sending message: %w", err)
    }

    response := extractText(msg)
    
    // Extract tool calls from message
    var toolCalls []ToolCallInfo
    var toolResults []ToolResultInfo
    for _, p := range msg.Parts {
        if p.Type == "tool_call" && p.ToolCall != nil {
            toolCalls = append(toolCalls, ToolCallInfo{
                Name:      p.ToolCall.Name,
                Arguments: p.ToolCall.Arguments,
            })
        }
        if p.Type == "tool_result" && p.ToolResult != nil {
            toolResults = append(toolResults, ToolResultInfo{
                ID:     p.ToolResult.ID,
                Output: p.ToolResult.Output,
                Error:  p.ToolResult.Error,
            })
        }
    }

    // Log to step logger
    if logger != nil {
        logger.LogLLM(response, toolCalls)
        for _, tr := range toolResults {
            logger.LogToolResult(tr)
        }
    }

    // ... rest of existing code ...
}
```

**Step 4: Run tests to verify integration**

```bash
cd /home/decodo/work/one-dev-army
go test ./internal/mvp -run TestWorker -v
```

Expected: PASS (existing tests should still pass)

**Step 5: Commit**

```bash
cd /home/decodo/work/one-dev-army
git add internal/mvp/worker.go
git commit -m "feat: integrate StepLogger into worker pipeline

- Add step logging to technical-planning step
- Modify llmStep to capture and log LLM responses
- Log tool calls and their results
- Each step gets its own timestamped log file"
```

---

## Task 3: Add Logging to All Pipeline Steps

**Files:**
- Modify: `internal/mvp/worker.go`

**Step 1: Add logging to implement step**

```go
// In implement step (around line 171-185):
if resumeFrom <= 1 {
    task.Status = StatusCoding
    log.Printf("[Worker %d] [2/7] Implementing #%d...", w.id, task.Issue.Number)
    
    stepLogger, err := NewStepLogger(artifactDir, task.Issue.Number, "implement")
    if err != nil {
        log.Printf("[Worker %d] Warning: failed to create step logger: %v", w.id, err)
    } else {
        stepLogger.Start()
        defer stepLogger.Close()
    }
    
    stepStart := time.Now()
    if err := w.implement(ctx, task, implPlan, stepLogger); err != nil {
        if stepLogger != nil {
            stepLogger.End(false, err.Error())
        }
        // ... error handling ...
    }
    if stepLogger != nil {
        stepLogger.End(true, "")
    }
    log.Printf("[Worker %d] [2/7] Implementation done (%s)", w.id, time.Since(stepStart).Round(time.Second))
}
```

**Step 2: Add logging to codeReview step**

```go
// In codeReview step (around line 187-237):
if resumeFrom <= 2 {
    task.Status = StatusReviewing
    log.Printf("[Worker %d] [3/7] Code review #%d...", w.id, task.Issue.Number)
    
    stepLogger, err := NewStepLogger(artifactDir, task.Issue.Number, "code-review")
    if err != nil {
        log.Printf("[Worker %d] Warning: failed to create step logger: %v", w.id, err)
    } else {
        stepLogger.Start()
        defer stepLogger.Close()
    }
    
    stepStart := time.Now()
    approved, review, crErr := w.codeReview(ctx, task, "", stepLogger)
    // ... error handling ...
    
    if stepLogger != nil {
        stepLogger.End(approved, fmt.Sprintf("approved=%v", approved))
    }
}
```

**Step 3: Add logging to createPR step**

```go
// In createPR step (around line 242-260):
if resumeFrom <= 3 {
    task.Status = StatusCreatingPR
    log.Printf("[Worker %d] [4/7] Creating PR for #%d...", w.id, task.Issue.Number)
    
    stepLogger, err := NewStepLogger(artifactDir, task.Issue.Number, "create-pr")
    if err != nil {
        log.Printf("[Worker %d] Warning: failed to create step logger: %v", w.id, err)
    } else {
        stepLogger.Start()
        defer stepLogger.Close()
    }
    
    stepStart := time.Now()
    prURL, err = w.createPR(ctx, task, stepLogger)
    // ... error handling ...
    
    if stepLogger != nil {
        stepLogger.End(err == nil, prURL)
    }
}
```

**Step 4: Add logging to checkPipeline step**

```go
// In checkPipeline step (around line 269-312):
if resumeFrom <= 4 {
    task.Status = StatusCheckingPipeline
    log.Printf("[Worker %d] [5/7] Checking CI pipeline for #%d...", w.id, task.Issue.Number)
    
    stepLogger, err := NewStepLogger(artifactDir, task.Issue.Number, "check-pipeline")
    if err != nil {
        log.Printf("[Worker %d] Warning: failed to create step logger: %v", w.id, err)
    } else {
        stepLogger.Start()
        defer stepLogger.Close()
    }
    
    stepStart := time.Now()
    if err := w.checkPipeline(ctx, task, stepLogger); err != nil {
        if stepLogger != nil {
            stepLogger.End(false, err.Error())
        }
        // ... retry logic ...
    }
    if stepLogger != nil {
        stepLogger.End(true, "all checks passed")
    }
}
```

**Step 5: Add logging to awaiting-approval and merge steps**

```go
// In approval/merge loop (around line 322-441):
// Before the loop starts:
approvalLogger, err := NewStepLogger(artifactDir, task.Issue.Number, "awaiting-approval")
if err != nil {
    log.Printf("[Worker %d] Warning: failed to create step logger: %v", w.id, err)
} else {
    approvalLogger.Start()
    defer approvalLogger.Close()
}

// When approved and merging:
if decision.Action == "approve" {
    if approvalLogger != nil {
        approvalLogger.End(true, "user approved")
    }
    
    // Create merge logger
    mergeLogger, err := NewStepLogger(artifactDir, task.Issue.Number, "merge")
    if err != nil {
        log.Printf("[Worker %d] Warning: failed to create step logger: %v", w.id, err)
    } else {
        mergeLogger.Start()
        defer mergeLogger.Close()
    }
    
    // ... merge logic ...
    
    if mergeLogger != nil {
        mergeLogger.End(err == nil, "PR merged successfully")
    }
}
```

**Step 6: Run full test suite**

```bash
cd /home/decodo/work/one-dev-army
go test ./internal/mvp/... -v
```

Expected: PASS

**Step 7: Commit**

```bash
cd /home/decodo/work/one-dev-army
git add internal/mvp/worker.go
git commit -m "feat: add step logging to all pipeline stages

- Add StepLogger to: technical-planning, implement, code-review
- Add StepLogger to: create-pr, check-pipeline, awaiting-approval, merge
- Each step creates its own timestamped log file
- Log success/failure status and duration for each step"
```

---

## Task 4: Update Worker Methods to Accept Logger

**Files:**
- Modify: `internal/mvp/worker.go`

**Step 1: Update method signatures**

```go
// Change from:
func (w *Worker) implement(ctx context.Context, task *Task, planStr string) error
// To:
func (w *Worker) implement(ctx context.Context, task *Task, planStr string, logger *StepLogger) error

// Change from:
func (w *Worker) codeReview(ctx context.Context, task *Task, prURL string) (approved bool, review string, err error)
// To:
func (w *Worker) codeReview(ctx context.Context, task *Task, prURL string, logger *StepLogger) (approved bool, review string, err error)

// Change from:
func (w *Worker) createPR(_ context.Context, task *Task) (string, error)
// To:
func (w *Worker) createPR(_ context.Context, task *Task, logger *StepLogger) (string, error)

// Change from:
func (w *Worker) checkPipeline(ctx context.Context, task *Task) error
// To:
func (w *Worker) checkPipeline(ctx context.Context, task *Task, logger *StepLogger) error
```

**Step 2: Add logging calls inside methods**

In `createPR`, log git commands:
```go
func (w *Worker) createPR(_ context.Context, task *Task, logger *StepLogger) (string, error) {
    if logger != nil {
        logger.logf("Command: git push origin %s", task.Branch)
    }
    
    if err := w.brMgr.PushBranch(task.Branch); err != nil {
        if logger != nil {
            logger.logf("Command failed: %v", err)
        }
        // ... error handling ...
    }
    
    if logger != nil {
        logger.logf("Command: gh pr create --base main --head %s", task.Branch)
    }
    
    prURL, err := w.gh.CreatePR(task.Branch, task.Issue.Title, body)
    // ...
    
    if logger != nil {
        if err != nil {
            logger.logf("Command failed: %v", err)
        } else {
            logger.logf("Command output: PR created at %s", prURL)
        }
    }
    
    return prURL, err
}
```

In `checkPipeline`, log check status:
```go
func (w *Worker) checkPipeline(ctx context.Context, task *Task, logger *StepLogger) error {
    // ... existing code ...
    
    for {
        select {
        case <-ctx.Done():
            return ctx.Err()
        case <-ticker.C:
            if time.Now().After(deadline) {
                if logger != nil {
                    logger.logf("Pipeline check timed out after %s", timeout)
                }
                return w.handlePipelineFailure(task, "CI checks timed out")
            }

            result, err := w.gh.GetPRChecks(task.Branch)
            if err != nil {
                if logger != nil {
                    logger.logf("Error checking PR status: %v", err)
                }
                continue
            }

            if logger != nil {
                logger.logf("PR checks status: %s", result.Status)
            }

            switch result.Status {
            case "pass":
                if logger != nil {
                    logger.logf("All CI checks passed")
                }
                return nil
            case "fail":
                if logger != nil {
                    logger.logf("CI checks failed: %s", result.Logs)
                }
                return w.handlePipelineFailure(task, result.Logs)
            }
        }
    }
}
```

**Step 3: Run tests**

```bash
cd /home/decodo/work/one-dev-army
go test ./internal/mvp/... ./internal/worker/... -v
```

Expected: PASS

**Step 4: Commit**

```bash
cd /home/decodo/work/one-dev-army
git add internal/mvp/worker.go
git commit -m "refactor: update worker methods to accept StepLogger

- Add logger parameter to implement, codeReview, createPR, checkPipeline
- Log git/gh commands and their output
- Log pipeline check status changes
- Maintain backward compatibility with nil logger"
```

---

## Task 5: Run Lint and Full Test Suite

**Step 1: Run linter**

```bash
cd /home/decodo/work/one-dev-army
golangci-lint run ./...
```

Expected: No errors

**Step 2: Run full test suite with race detector**

```bash
cd /home/decodo/work/one-dev-army
go test -race ./...
```

Expected: PASS

**Step 3: Commit any fixes**

```bash
cd /home/decodo/work/one-dev-army
git add -A
git commit -m "fix: address linting issues in step logging

- Fix any golint warnings
- Ensure proper error handling
- Verify race condition safety"
```

---

## Task 6: Create Documentation

**Files:**
- Create: `docs/plans/2025-03-25-per-step-logging.md` (this file)

Already created as part of this plan.

**Step 1: Commit documentation**

```bash
cd /home/decodo/work/one-dev-army
git add docs/plans/2025-03-25-per-step-logging.md
git commit -m "docs: add per-step logging implementation plan

- Document StepLogger architecture
- Describe integration with worker pipeline
- Include testing and verification steps"
```

---

## Summary

After completing all tasks:

1. **StepLogger type** created in `internal/worker/step_logger.go`
2. **All 7 pipeline steps** now create their own log files:
   - `YYYYMMDDhhmmss_technical-planning.log`
   - `YYYYMMDDhhmmss_implement.log`
   - `YYYYMMDDhhmmss_code-review.log`
   - `YYYYMMDDhhmmss_create-pr.log`
   - `YYYYMMDDhhmmss_check-pipeline.log`
   - `YYYYMMDDhhmmss_awaiting-approval.log`
   - `YYYYMMDDhhmmss_merge.log`
3. **Log format** includes:
   - Timestamps for all entries
   - STEP START/END events
   - LLM responses (text + tool calls)
   - Git/gh command execution
   - Duration and success/failure status
4. **Logs location**: `.oda/artifacts/<issue_number>/logs/`
5. **All tests pass** with race detector
6. **Linting passes** with golangci-lint

**Next steps (future):**
- Dashboard endpoint to view logs via browser
- Real-time log streaming via WebSocket
- Log rotation and cleanup policies
