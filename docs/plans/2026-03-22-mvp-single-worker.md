# ODA MVP Sprint Process - Single Worker End-to-End

> **Goal:** Simplified MVP where orchestrator manages exactly ONE worker that implements a complete ticket from A to Z.

## Architecture Overview

**Simplification from full ODA:**
- **Orchestrator:** Manages only 1 worker (not a pool)
- **Worker:** Does complete end-to-end implementation (no pipeline stages)
- **Flow:** Pick ticket → Analyze → Plan → Code → Test → Create PR
- **No stage labels, no complex pipeline, no manual approvals**

## Phase 1: Single Worker Implementation

### Task 1: Create Simple Worker Structure

**Files:**
- Create: `internal/mvp/worker.go`
- Create: `internal/mvp/orchestrator.go`
- Create: `internal/mvp/task.go`

**Step 1: Define MVP Task structure**

```go
// internal/mvp/task.go
package mvp

import "github.com/crazy-goat/one-dev-army/internal/github"

type Task struct {
	Issue     github.Issue
	Milestone string
	Branch    string
	Worktree  string
	Status    TaskStatus
	Result    *TaskResult
}

type TaskStatus string

const (
	StatusPending    TaskStatus = "pending"
	StatusAnalyzing  TaskStatus = "analyzing"
	StatusPlanning   TaskStatus = "planning"
	StatusCoding     TaskStatus = "coding"
	StatusTesting    TaskStatus = "testing"
	StatusCreatingPR TaskStatus = "creating_pr"
	StatusDone       TaskStatus = "done"
	StatusFailed     TaskStatus = "failed"
)

type TaskResult struct {
	PRURL   string
	Error   error
	Summary string
}
```

**Step 2: Create MVP Worker**

```go
// internal/mvp/worker.go
package mvp

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	
	"github.com/crazy-goat/one-dev-army/internal/config"
	"github.com/crazy-goat/one-dev-army/internal/git"
	"github.com/crazy-goat/one-dev-army/internal/github"
	"github.com/crazy-goat/one-dev-army/internal/opencode"
)

type Worker struct {
	id       int
	cfg      *config.Config
	oc       *opencode.Client
	gh       *github.Client
	wtMgr    *git.WorktreeManager
	baseDir  string
}

func NewWorker(id int, cfg *config.Config, oc *opencode.Client, gh *github.Client, wtMgr *git.WorktreeManager) *Worker {
	return &Worker{
		id:      id,
		cfg:     cfg,
		oc:      oc,
		gh:      gh,
		wtMgr:   wtMgr,
		baseDir: wtMgr.BaseDir,
	}
}

// Process implements a complete ticket from A to Z
func (w *Worker) Process(ctx context.Context, task *Task) error {
	task.Status = StatusAnalyzing
	
	// Step 1: Create worktree
	branch := fmt.Sprintf("oda-%d-%s", task.Issue.Number, slug(task.Issue.Title))
	worktreePath, err := w.wtMgr.Create(branch)
	if err != nil {
		task.Status = StatusFailed
		task.Result = &TaskResult{Error: fmt.Errorf("creating worktree: %w", err)}
		return err
	}
	task.Branch = branch
	task.Worktree = worktreePath
	
	// Step 2: Analyze issue
	analysis, err := w.analyze(ctx, task)
	if err != nil {
		task.Status = StatusFailed
		task.Result = &TaskResult{Error: fmt.Errorf("analysis: %w", err)}
		return err
	}
	
	// Step 3: Create implementation plan
	task.Status = StatusPlanning
	plan, err := w.plan(ctx, task, analysis)
	if err != nil {
		task.Status = StatusFailed
		task.Result = &TaskResult{Error: fmt.Errorf("planning: %w", err)}
		return err
	}
	
	// Step 4: Implement code
	task.Status = StatusCoding
	if err := w.implement(ctx, task, plan); err != nil {
		task.Status = StatusFailed
		task.Result = &TaskResult{Error: fmt.Errorf("implementation: %w", err)}
		return err
	}
	
	// Step 5: Run tests
	task.Status = StatusTesting
	if err := w.test(ctx, task); err != nil {
		task.Status = StatusFailed
		task.Result = &TaskResult{Error: fmt.Errorf("testing: %w", err)}
		return err
	}
	
	// Step 6: Create PR
	task.Status = StatusCreatingPR
	prURL, err := w.createPR(ctx, task)
	if err != nil {
		task.Status = StatusFailed
		task.Result = &TaskResult{Error: fmt.Errorf("creating PR: %w", err)}
		return err
	}
	
	task.Status = StatusDone
	task.Result = &TaskResult{
		PRURL:   prURL,
		Summary: fmt.Sprintf("Implemented %s in branch %s", task.Issue.Title, branch),
	}
	
	return nil
}

func slug(title string) string {
	// Simple slug: lowercase, replace spaces with -, limit length
	return "implementation"
}
```

### Task 2: Implement Worker Steps

**Step 1: Analysis with LLM**

```go
// internal/mvp/worker.go (continued)

const analysisPrompt = `You are analyzing a GitHub issue for implementation.

## Issue #%d: %s

%s

## Instructions

Analyze this issue and provide:
1. Core requirements
2. Files that need changes
3. Implementation approach
4. Testing strategy

Be concise. Do NOT ask questions.
`

func (w *Worker) analyze(ctx context.Context, task *Task) (string, error) {
	session, err := w.oc.CreateSession(fmt.Sprintf("analyze-%d", task.Issue.Number))
	if err != nil {
		return "", err
	}
	
	prompt := fmt.Sprintf(analysisPrompt, task.Issue.Number, task.Issue.Title, task.Issue.Body)
	
	msg, err := w.oc.SendMessage(session.ID, prompt, opencode.ParseModelRef(w.cfg.Planning.LLM), os.Stdout)
	if err != nil {
		return "", err
	}
	
	return extractText(msg), nil
}
```

**Step 2: Planning**

```go
const planningPrompt = `You are creating an implementation plan.

## Issue #%d: %s

## Analysis

%s

## Instructions

Create a step-by-step implementation plan:
1. What files to modify
2. What code to write
3. What tests to add

Be specific and actionable.
`

func (w *Worker) plan(ctx context.Context, task *Task, analysis string) (string, error) {
	session, err := w.oc.CreateSession(fmt.Sprintf("plan-%d", task.Issue.Number))
	if err != nil {
		return "", err
	}
	
	prompt := fmt.Sprintf(planningPrompt, task.Issue.Number, task.Issue.Title, analysis)
	
	msg, err := w.oc.SendMessage(session.ID, prompt, opencode.ParseModelRef(w.cfg.Planning.LLM), os.Stdout)
	if err != nil {
		return "", err
	}
	
	return extractText(msg), nil
}
```

**Step 3: Implementation**

```go
const implementationPrompt = `You are implementing a GitHub issue.

## Issue #%d: %s

## Implementation Plan

%s

## Instructions

Implement the changes in the provided codebase. Use tools to:
1. Read relevant files
2. Make necessary changes
3. Create new files if needed
4. Run tests to verify

The codebase is at: %s

CRITICAL: You are in a fully automated pipeline. NEVER ask questions or wait for input.
`

func (w *Worker) implement(ctx context.Context, task *Task, plan string) error {
	session, err := w.oc.CreateSession(fmt.Sprintf("implement-%d", task.Issue.Number))
	if err != nil {
		return err
	}
	
	// Set working directory for this session
	w.oc.SetDirectory(task.Worktree)
	
	prompt := fmt.Sprintf(implementationPrompt, 
		task.Issue.Number, 
		task.Issue.Title, 
		plan,
		task.Worktree,
	)
	
	_, err = w.oc.SendMessage(session.ID, prompt, opencode.ParseModelRef(w.cfg.EpicAnalysis.LLM), os.Stdout)
	return err
}
```

**Step 4: Testing**

```go
func (w *Worker) test(ctx context.Context, task *Task) error {
	// Run configured test command in worktree
	testCmd := w.cfg.Tools.TestCmd
	if testCmd == "" {
		testCmd = "go test ./..."
	}
	
	// Execute test command in worktree
	return w.wtMgr.RunInWorktree(task.Branch, testCmd)
}
```

**Step 5: Create PR**

```go
func (w *Worker) createPR(ctx context.Context, task *Task) (string, error) {
	// Push branch
	if err := w.wtMgr.PushBranch(task.Branch); err != nil {
		return "", fmt.Errorf("pushing branch: %w", err)
	}
	
	// Create PR
	body := fmt.Sprintf("Closes #%d\n\n%s", task.Issue.Number, task.Issue.Body)
	prURL, err := w.gh.CreatePR(task.Branch, task.Issue.Title, body)
	if err != nil {
		return "", fmt.Errorf("creating PR: %w", err)
	}
	
	return prURL, nil
}
```

### Task 3: Create Simple Orchestrator

**File:** `internal/mvp/orchestrator.go`

```go
package mvp

import (
	"context"
	"fmt"
	"time"
	
	"github.com/crazy-goat/one-dev-army/internal/config"
	"github.com/crazy-goat/one-dev-army/internal/git"
	"github.com/crazy-goat/one-dev-army/internal/github"
	"github.com/crazy-goat/one-dev-army/internal/opencode"
)

type Orchestrator struct {
	cfg     *config.Config
	worker  *Worker
	gh      *github.Client
	oc      *opencode.Client
	wtMgr   *git.WorktreeManager
	running bool
}

func NewOrchestrator(cfg *config.Config, gh *github.Client, oc *opencode.Client, wtMgr *git.WorktreeManager) *Orchestrator {
	worker := NewWorker(1, cfg, oc, gh, wtMgr)
	
	return &Orchestrator{
		cfg:    cfg,
		worker: worker,
		gh:     gh,
		oc:     oc,
		wtMgr:  wtMgr,
	}
}

// Run starts the orchestrator and processes one ticket at a time
func (o *Orchestrator) Run(ctx context.Context) error {
	o.running = true
	
	for o.running {
		select {
		case <-ctx.Done():
			return ctx.Err()
		default:
		}
		
		// Get active milestone
		milestone := o.gh.GetActiveMilestone()
		if milestone == nil {
			fmt.Println("No active milestone, waiting...")
			time.Sleep(30 * time.Second)
			continue
		}
		
		// Get open issues from milestone
		issues, err := o.gh.ListIssues(milestone.Title)
		if err != nil {
			fmt.Printf("Error listing issues: %v\n", err)
			time.Sleep(30 * time.Second)
			continue
		}
		
		// Find first unassigned issue
		var nextIssue *github.Issue
		for _, issue := range issues {
			if issue.State == "open" && !hasLabel(issue.Labels, "in-progress") {
				nextIssue = &issue
				break
			}
		}
		
		if nextIssue == nil {
			fmt.Println("No available issues in sprint, waiting...")
			time.Sleep(30 * time.Second)
			continue
		}
		
		// Mark as in-progress
		if err := o.gh.AddLabel(nextIssue.Number, "in-progress"); err != nil {
			fmt.Printf("Error adding label: %v\n", err)
		}
		
		// Process the issue
		task := &Task{
			Issue:     *nextIssue,
			Milestone: milestone.Title,
			Status:    StatusPending,
		}
		
		fmt.Printf("Processing issue #%d: %s\n", nextIssue.Number, nextIssue.Title)
		
		if err := o.worker.Process(ctx, task); err != nil {
			fmt.Printf("Failed to process issue #%d: %v\n", nextIssue.Number, err)
			o.gh.AddLabel(nextIssue.Number, "failed")
		} else {
			fmt.Printf("Successfully processed issue #%d: %s\n", nextIssue.Number, task.Result.PRURL)
			o.gh.AddComment(nextIssue.Number, fmt.Sprintf("Implemented in %s", task.Result.PRURL))
		}
		
		// Small delay between tickets
		time.Sleep(5 * time.Second)
	}
	
	return nil
}

func (o *Orchestrator) Stop() {
	o.running = false
}

func hasLabel(labels []struct{ Name string }, name string) bool {
	for _, l := range labels {
		if l.Name == name {
			return true
		}
	}
	return false
}
```

## Phase 2: Integration with Main

### Task 4: Update main.go to use MVP orchestrator

**File:** `main.go`

Add to imports:
```go
import "github.com/crazy-goat/one-dev-army/internal/mvp"
```

Replace worker pool with orchestrator:
```go
// OLD:
// pool := worker.NewPool(cfg.Workers.Count, &worker.EmptyQueue{}, processor)
// pool.Start(ctx)

// NEW:
orchestrator := mvp.NewOrchestrator(cfg, gh, oc, wtMgr)

// Run orchestrator in background
orchErrCh := make(chan error, 1)
go func() {
	if err := orchestrator.Run(ctx); err != nil {
		orchErrCh <- err
	}
	close(orchErrCh)
}()
```

### Task 5: Dashboard Updates

**File:** `internal/dashboard/handlers.go`

Add endpoint to show current task:
```go
func (s *Server) handleCurrentTask(w http.ResponseWriter, r *http.Request) {
	// Get current task from orchestrator
	// Return HTML with task status
}
```

## Phase 3: Testing

### Task 6: Test MVP Flow

**File:** `internal/mvp/worker_test.go`

```go
package mvp

import (
	"context"
	"testing"
)

func TestWorker_Process(t *testing.T) {
	// Test complete flow with mock dependencies
}
```

## Summary

**MVP Simplifications:**
1. ✅ Single worker instead of pool
2. ✅ Worker does A-Z (no pipeline stages)
3. ✅ No complex stage labels
4. ✅ Simple orchestrator loop
5. ✅ Process one ticket at a time

**Next Steps:**
1. Implement Task 1-3 (worker structure)
2. Wire into main.go
3. Test with real issue
4. Add dashboard view

**Files to Create:**
- `internal/mvp/task.go`
- `internal/mvp/worker.go`
- `internal/mvp/orchestrator.go`
- `internal/mvp/worker_test.go`

**Files to Modify:**
- `main.go` - use orchestrator instead of worker pool
- `internal/dashboard/handlers.go` - add current task endpoint
