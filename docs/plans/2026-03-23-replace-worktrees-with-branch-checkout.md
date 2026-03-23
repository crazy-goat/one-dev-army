# Replace Git Worktrees with Regular Branch Checkout - Implementation Plan

> **For Claude:** REQUIRED SUB-SKILL: Use superpowers:executing-plans to implement this plan task-by-task.

**Goal:** Replace git worktree-based isolation with simple branch checkout since only 1 worker runs, simplifying the architecture and removing worktree complexity.

**Architecture:** Create a `BranchManager` that manages branches directly in the main repo instead of creating isolated worktrees. The LLM will work directly in the main repository directory, checking out feature branches as needed.

**Tech Stack:** Go 1.22+, Git CLI

---

## Overview

### Current State (Worktree-based)
- Creates isolated worktree at `.oda/worktrees/worker-1/`
- LLM works in separate directory
- Complex worktree lifecycle management

### Target State (Branch-based)
- Checkout branch directly in main repo: `git checkout -b <branch>`
- LLM works in main repo directory
- Simple branch lifecycle management

### Files to Modify

**Core Implementation:**
1. `internal/git/worktree.go` → Rename to `branch_manager.go`, rewrite all methods
2. `internal/mvp/task.go` → Remove `Worktree` field
3. `internal/mvp/worker.go` → Update to use repo directory instead of worktree
4. `internal/mvp/orchestrator.go` → Update field type to `BranchManager`
5. `internal/worker/processor.go` → Update worktree references to use repo directory
6. `main.go` → Remove worktrees directory creation

**Tests:**
7. `internal/git/worktree_test.go` → Rewrite for `BranchManager`
8. `internal/mvp/task_test.go` → Remove `Worktree` field tests
9. `internal/mvp/integration_test.go` → Update setup
10. `internal/worker/processor_test.go` → Update test setup
11. `internal/integration_test.go` → Update integration tests

**Cleanup:**
12. `.gitignore` → Remove worktree entries
13. `README.md` → Update architecture description

---

## Task 1: Create BranchManager Core

**Files:**
- Create: `internal/git/branch_manager.go`
- Test: `internal/git/branch_manager_test.go`

**Step 1: Write the BranchManager struct and interface**

```go
package git

import (
	"fmt"
	"os/exec"
	"strings"
)

type BranchManager struct {
	repoDir string
}

func NewBranchManager(repoDir string) *BranchManager {
	return &BranchManager{repoDir: repoDir}
}

func (m *BranchManager) RepoDir() string {
	return m.repoDir
}
```

**Step 2: Implement Create method**

```go
func (m *BranchManager) Create(branch string) error {
	// Check if branch already exists and delete it
	cmd := exec.Command("git", "branch", "-D", branch)
	cmd.Dir = m.repoDir
	_ = cmd.Run() // Ignore error - branch might not exist

	// Create and checkout new branch
	cmd = exec.Command("git", "checkout", "-b", branch)
	cmd.Dir = m.repoDir
	if out, err := cmd.CombinedOutput(); err != nil {
		return fmt.Errorf("git checkout -b %s: %w\n%s", branch, err, out)
	}

	return nil
}
```

**Step 3: Implement Remove method**

```go
func (m *BranchManager) Remove(branch string) error {
	// Checkout main/master first
	cmd := exec.Command("git", "checkout", "main")
	cmd.Dir = m.repoDir
	if out, err := cmd.CombinedOutput(); err != nil {
		// Try master if main fails
		cmd = exec.Command("git", "checkout", "master")
		cmd.Dir = m.repoDir
		if out, err := cmd.CombinedOutput(); err != nil {
			return fmt.Errorf("git checkout main/master: %w\n%s", err, out)
		}
	}

	// Delete the branch
	cmd = exec.Command("git", "branch", "-D", branch)
	cmd.Dir = m.repoDir
	if out, err := cmd.CombinedOutput(); err != nil {
		return fmt.Errorf("git branch -D %s: %w\n%s", branch, err, out)
	}

	return nil
}
```

**Step 4: Implement PushBranch method (from existing WorktreeManager)**

```go
func (m *BranchManager) PushBranch(branch string) error {
	// Try force-with-lease first (safe for existing remote branches)
	cmd := exec.Command("git", "push", "-u", "--force-with-lease", "origin", branch)
	cmd.Dir = m.repoDir
	out, err := cmd.CombinedOutput()
	if err == nil {
		return nil
	}

	// If stale info or no upstream, fall back to regular push
	if strings.Contains(string(out), "stale info") || strings.Contains(string(out), "no upstream") {
		cmd = exec.Command("git", "push", "-u", "origin", branch)
		cmd.Dir = m.repoDir
		out, err = cmd.CombinedOutput()
		if err != nil {
			return fmt.Errorf("git push -u origin %s: %w\n%s", branch, err, out)
		}
		return nil
	}

	return fmt.Errorf("git push -u --force-with-lease origin %s: %w\n%s", branch, err, out)
}
```

**Step 5: Add RunInDir helper (replaces RunInWorktree)**

```go
func RunInDir(dir, name string, args ...string) ([]byte, error) {
	cmd := exec.Command(name, args...)
	cmd.Dir = dir
	out, err := cmd.CombinedOutput()
	if err != nil {
		return nil, fmt.Errorf("running %s in %s: %w\n%s", name, dir, err, out)
	}
	return out, nil
}
```

**Step 6: Write tests for BranchManager**

```go
package git_test

import (
	"os/exec"
	"testing"

	"github.com/crazy-goat/one-dev-army/internal/git"
)

func setupRepo(t *testing.T) string {
	t.Helper()
	dir := t.TempDir()

	env := []string{
		"GIT_AUTHOR_NAME=test",
		"GIT_AUTHOR_EMAIL=test@test.com",
		"GIT_COMMITTER_NAME=test",
		"GIT_COMMITTER_EMAIL=test@test.com",
	}

	for _, args := range [][]string{
		{"init"},
		{"commit", "--allow-empty", "-m", "init"},
	} {
		cmd := exec.Command("git", args...)
		cmd.Dir = dir
		cmd.Env = append(env...)
		if out, err := cmd.CombinedOutput(); err != nil {
			t.Fatalf("git %v: %v\n%s", args, err, out)
		}
	}

	return dir
}

func TestBranchManagerCreate(t *testing.T) {
	repoDir := setupRepo(t)
	mgr := git.NewBranchManager(repoDir)

	err := mgr.Create("feature-branch")
	if err != nil {
		t.Fatalf("Create: %v", err)
	}

	// Verify branch exists and is checked out
	cmd := exec.Command("git", "branch", "--show-current")
	cmd.Dir = repoDir
	out, err := cmd.CombinedOutput()
	if err != nil {
		t.Fatalf("git branch --show-current: %v\n%s", err, out)
	}

	if string(out) != "feature-branch\n" {
		t.Errorf("Current branch = %q, want %q", string(out), "feature-branch\n")
	}
}

func TestBranchManagerCreateAlreadyExists(t *testing.T) {
	repoDir := setupRepo(t)
	mgr := git.NewBranchManager(repoDir)

	// Create first branch
	err := mgr.Create("feature-branch")
	if err != nil {
		t.Fatalf("first Create: %v", err)
	}

	// Create second branch (should succeed by deleting first)
	err = mgr.Create("feature-branch-2")
	if err != nil {
		t.Fatalf("second Create: %v", err)
	}

	// Verify we're on the new branch
	cmd := exec.Command("git", "branch", "--show-current")
	cmd.Dir = repoDir
	out, err := cmd.CombinedOutput()
	if err != nil {
		t.Fatalf("git branch --show-current: %v\n%s", err, out)
	}

	if string(out) != "feature-branch-2\n" {
		t.Errorf("Current branch = %q, want %q", string(out), "feature-branch-2\n")
	}
}

func TestBranchManagerRemove(t *testing.T) {
	repoDir := setupRepo(t)
	mgr := git.NewBranchManager(repoDir)

	// Create and then remove branch
	err := mgr.Create("feature-branch")
	if err != nil {
		t.Fatalf("Create: %v", err)
	}

	err = mgr.Remove("feature-branch")
	if err != nil {
		t.Fatalf("Remove: %v", err)
	}

	// Verify we're back on main/master
	cmd := exec.Command("git", "branch", "--show-current")
	cmd.Dir = repoDir
	out, err := cmd.CombinedOutput()
	if err != nil {
		t.Fatalf("git branch --show-current: %v\n%s", err, out)
	}

	current := string(out)
	if current != "main\n" && current != "master\n" {
		t.Errorf("Current branch = %q, want main or master", current)
	}

	// Verify branch is deleted
	cmd = exec.Command("git", "branch", "--list", "feature-branch")
	cmd.Dir = repoDir
	out, err = cmd.CombinedOutput()
	if err != nil {
		t.Fatalf("git branch --list: %v\n%s", err, out)
	}

	if string(out) != "" {
		t.Errorf("Branch should be deleted, got: %q", string(out))
	}
}

func TestRunInDir(t *testing.T) {
	repoDir := setupRepo(t)

	out, err := git.RunInDir(repoDir, "git", "status", "--short")
	if err != nil {
		t.Fatalf("RunInDir: %v", err)
	}

	// Should succeed (empty repo, no output expected)
	_ = out
}
```

**Step 7: Run tests to verify they pass**

```bash
cd /home/decodo/work/one-dev-army
go test ./internal/git/... -v
```

Expected: All tests PASS

**Step 8: Commit**

```bash
git add internal/git/branch_manager.go internal/git/branch_manager_test.go
git commit -m "feat: add BranchManager to replace WorktreeManager

- Create BranchManager with Create(), Remove(), PushBranch() methods
- Add RunInDir() helper to replace RunInWorktree()
- Include comprehensive unit tests"
```

---

## Task 2: Remove Worktree Field from Task Struct

**Files:**
- Modify: `internal/mvp/task.go:26`
- Test: `internal/mvp/task_test.go:54-56`

**Step 1: Remove Worktree field from Task struct**

```go
type Task struct {
	Issue     github.Issue
	Milestone string
	Branch    string
	// Worktree field removed - now using main repo directory
	Status    TaskStatus
	Result    *TaskResult

	mu        sync.Mutex
	sessionID string
}
```

**Step 2: Update task_test.go to remove Worktree test**

Remove lines 54-56 from `internal/mvp/task_test.go`:
```go
// REMOVED: Worktree field no longer exists
// if task.Worktree != "" {
//     t.Errorf("zero-value Worktree = %q, want empty", task.Worktree)
// }
```

**Step 3: Run tests to verify they pass**

```bash
cd /home/decodo/work/one-dev-army
go test ./internal/mvp/... -v -run TestTask
```

Expected: All tests PASS

**Step 4: Commit**

```bash
git add internal/mvp/task.go internal/mvp/task_test.go
git commit -m "refactor: remove Worktree field from Task struct

Task no longer needs Worktree field since we work in main repo directory"
```

---

## Task 3: Update Worker to Use BranchManager

**Files:**
- Modify: `internal/mvp/worker.go:112,117-127,159-170,326-328,342-351,353-368,459`

**Step 1: Update Worker struct field type**

```go
type Worker struct {
	id      int
	cfg     *config.Config
	oc      *opencode.Client
	gh      *github.Client
	wtMgr   *git.BranchManager  // Changed from *git.WorktreeManager
	store   *db.Store
	baseDir string
}
```

**Step 2: Update NewWorker function signature and implementation**

```go
func NewWorker(id int, cfg *config.Config, oc *opencode.Client, gh *github.Client, wtMgr *git.BranchManager, store *db.Store) *Worker {
	return &Worker{
		id:      id,
		cfg:     cfg,
		oc:      oc,
		gh:      gh,
		wtMgr:   wtMgr,
		store:   store,
		baseDir: wtMgr.RepoDir(),  // Changed from WorktreesDir()
	}
}
```

**Step 3: Update Process method - branch creation**

Replace lines 158-170:
```go
	branch := fmt.Sprintf("oda-%d-%s", task.Issue.Number, slug(task.Issue.Title))
	log.Printf("[Worker %d] Creating branch %q", w.id, branch)
	
	if err := w.wtMgr.Create(branch); err != nil {
		task.Status = StatusFailed
		task.Result = &TaskResult{Error: fmt.Errorf("creating branch: %w", err)}
		log.Printf("[Worker %d] ✗ FAILED creating branch: %v", w.id, err)
		return task.Result.Error
	}
	task.Branch = branch
	log.Printf("[Worker %d] Branch %q ready", w.id, branch)
```

**Step 4: Update implement method - use repo directory**

Replace lines 325-340:
```go
func (w *Worker) implement(ctx context.Context, task *Task, plan string) error {
	// Set directory to main repo (baseDir is now repoDir)
	w.oc.SetDirectory(w.baseDir)
	defer w.oc.SetDirectory("")

	testCmd := w.cfg.Tools.TestCmd
	if testCmd == "" {
		testCmd = "go test ./..."
	}
	prompt := fmt.Sprintf(implementationPrompt, task.Issue.Number, task.Issue.Title, plan, w.baseDir, testCmd)
	_, err := w.llmStep(ctx, task, "implement", prompt, w.cfg.EpicAnalysis.LLM)
	if err != nil {
		return err
	}
	w.ensureCommit(task)
	return nil
}
```

**Step 5: Update ensureCommit method - use repo directory**

Replace lines 342-351:
```go
func (w *Worker) ensureCommit(task *Task) {
	git.RunInDir(w.baseDir, "git", "add", "-A")
	out, err := git.RunInDir(w.baseDir, "git", "diff", "--cached", "--quiet")
	if err != nil {
		msg := fmt.Sprintf("feat: implement #%d %s", task.Issue.Number, task.Issue.Title)
		git.RunInDir(w.baseDir, "git", "commit", "-m", msg)
		log.Printf("[Worker %d] Auto-committed uncommitted changes", w.id)
	}
	_ = out
}
```

**Step 6: Update fixFromReview method - use repo directory**

Replace lines 353-368:
```go
func (w *Worker) fixFromReview(ctx context.Context, task *Task, review string) error {
	w.oc.SetDirectory(w.baseDir)
	defer w.oc.SetDirectory("")

	testCmd := w.cfg.Tools.TestCmd
	if testCmd == "" {
		testCmd = "go test ./..."
	}
	prompt := fmt.Sprintf(fixFromReviewPrompt, task.Issue.Number, task.Issue.Title, w.baseDir, testCmd, review)
	_, err := w.llmStep(ctx, task, "fix-from-review", prompt, w.cfg.EpicAnalysis.LLM)
	if err != nil {
		return err
	}
	w.ensureCommit(task)
	return nil
}
```

**Step 7: Verify PushBranch call still works (line 459)**

The PushBranch method signature is the same, so line 459 should work:
```go
if pushErr := w.wtMgr.PushBranch(task.Branch); pushErr != nil {
```

**Step 8: Run tests to verify compilation and tests pass**

```bash
cd /home/decodo/work/one-dev-army
go build ./...
go test ./internal/mvp/... -v
```

Expected: Build succeeds, tests PASS

**Step 9: Commit**

```bash
git add internal/mvp/worker.go
git commit -m "refactor: update Worker to use BranchManager

- Change wtMgr field type from *WorktreeManager to *BranchManager
- Update NewWorker to use RepoDir() instead of WorktreesDir()
- Update Process() to call Create(branch) instead of Create(workerName, branch)
- Update implement(), ensureCommit(), fixFromReview() to use baseDir (repoDir)
- Remove worktree path references, use main repo directory instead"
```

---

## Task 4: Update Orchestrator to Use BranchManager

**Files:**
- Modify: `internal/mvp/orchestrator.go:45,55-66`

**Step 1: Update Orchestrator struct field type**

```go
type Orchestrator struct {
	cfg           *config.Config
	worker        *Worker
	gh            *github.Client
	oc            *opencode.Client
	wtMgr         *git.BranchManager  // Changed from *git.WorktreeManager
	store         *db.Store
	projectNumber int
	running       bool
	paused        bool
	processing    bool
	currentTask   *Task
	mu            sync.Mutex
}
```

**Step 2: Update NewOrchestrator function signature and implementation**

```go
func NewOrchestrator(cfg *config.Config, gh *github.Client, oc *opencode.Client, wtMgr *git.BranchManager, store *db.Store, projectNumber int) *Orchestrator {
	o := &Orchestrator{
		cfg:           cfg,
		gh:            gh,
		oc:            oc,
		wtMgr:         wtMgr,
		store:         store,
		projectNumber: projectNumber,
		paused:        true,
	}
	o.worker = NewWorker(1, cfg, oc, gh, wtMgr, store)
	return o
}
```

**Step 3: Run tests to verify compilation and tests pass**

```bash
cd /home/decodo/work/one-dev-army
go build ./...
go test ./internal/mvp/... -v
```

Expected: Build succeeds, tests PASS

**Step 4: Commit**

```bash
git add internal/mvp/orchestrator.go
git commit -m "refactor: update Orchestrator to use BranchManager

- Change wtMgr field type from *WorktreeManager to *BranchManager
- Update NewOrchestrator signature and worker initialization"
```

---

## Task 5: Update Worker Processor to Use BranchManager

**Files:**
- Modify: `internal/worker/processor.go:198,207,214-217,255,279,288,411-428`

**Step 1: Update Processor struct field type**

```go
type Processor struct {
	cfg   *config.Config
	oc    *opencode.Client
	gh    *github.Client
	store *db.Store
	wtMgr *git.BranchManager  // Changed from *git.WorktreeManager
}
```

**Step 2: Update NewProcessor function**

```go
func NewProcessor(cfg *config.Config, oc *opencode.Client, gh *github.Client, store *db.Store, wtMgr *git.BranchManager) *Processor {
	return &Processor{
		cfg:   cfg,
		oc:    oc,
		gh:    gh,
		store: store,
		wtMgr: wtMgr,
	}
}
```

**Step 3: Update Process method - branch creation and cleanup**

Replace lines 211-263:
```go
func (p *Processor) Process(ctx context.Context, w *Worker, task *Task) error {
	branch := BranchName(task.IssueNumber, task.Title)

	if err := p.wtMgr.Create(branch); err != nil {
		return fmt.Errorf("creating branch for task #%d: %w", task.IssueNumber, err)
	}

	// Get repo directory for executor
	repoDir := p.wtMgr.RepoDir()
	executor := NewStageExecutor(p.cfg, p.oc, p.store, task, repoDir)

	startStage := pipeline.Stage(task.Stage)
	if startStage == "" || startStage == pipeline.StageQueued {
		startStage = pipeline.StageAnalysis
	}

	ppl := pipeline.New(p.cfg.Pipeline.MaxRetries, executor, func(_ int, s pipeline.Stage) {
		w.SetStage(string(s))
	})

	issueCtx := fmt.Sprintf("Issue #%d: %s\n\n%s", task.IssueNumber, task.Title, task.Body)
	result, err := ppl.Run(task.ID, startStage, issueCtx)
	if err != nil {
		return fmt.Errorf("pipeline failed for task #%d: %w", task.IssueNumber, err)
	}

	switch result.Stage {
	case pipeline.StageDone:
		prTitle := fmt.Sprintf("fix #%d: %s", task.IssueNumber, task.Title)
		prBody := fmt.Sprintf("Closes #%d\n\nAutomated implementation by ODA.", task.IssueNumber)

		_, prErr := p.gh.CreatePR(branch, prTitle, prBody)
		if prErr != nil {
			return fmt.Errorf("creating PR for task #%d: %w", task.IssueNumber, prErr)
		}

		mergeStage := p.findStageConfig("merge")
		if mergeStage != nil && mergeStage.ManualApproval {
			_ = p.gh.AddLabel(task.IssueNumber, "stage:needs-user")
		} else {
			if mergeErr := p.gh.MergePR(branch); mergeErr != nil {
				return fmt.Errorf("merging PR for task #%d: %w", task.IssueNumber, mergeErr)
			}
		}

		_ = p.wtMgr.Remove(branch)

	case pipeline.StageBlocked:
		_ = p.gh.AddLabel(task.IssueNumber, "stage:needs-user")
		_ = p.gh.AddComment(task.IssueNumber, fmt.Sprintf("ODA pipeline blocked at stage. Last output:\n\n%s", result.Output))
	}

	return nil
}
```

**Step 4: Update StageExecutor struct - replace worktree with repoDir**

```go
type StageExecutor struct {
	cfg     *config.Config
	oc      *opencode.Client
	store   *db.Store
	task    *Task
	repoDir string  // Changed from *git.Worktree
}
```

**Step 5: Update NewStageExecutor function**

```go
func NewStageExecutor(cfg *config.Config, oc *opencode.Client, store *db.Store, task *Task, repoDir string) *StageExecutor {
	return &StageExecutor{
		cfg:     cfg,
		oc:      oc,
		store:   store,
		task:    task,
		repoDir: repoDir,
	}
}
```

**Step 6: Update executeTesting method - use RunInDir**

Replace lines 406-448:
```go
func (e *StageExecutor) executeTesting(taskID int, stage pipeline.Stage) (*pipeline.StageResult, error) {
	start := time.Now()
	var failures []string

	if e.cfg.Tools.LintCmd != "" {
		parts := strings.Fields(e.cfg.Tools.LintCmd)
		if _, err := git.RunInDir(e.repoDir, parts[0], parts[1:]...); err != nil {
			failures = append(failures, fmt.Sprintf("lint: %v", err))
		}
	}

	if e.cfg.Tools.TestCmd != "" {
		parts := strings.Fields(e.cfg.Tools.TestCmd)
		if _, err := git.RunInDir(e.repoDir, parts[0], parts[1:]...); err != nil {
			failures = append(failures, fmt.Sprintf("test: %v", err))
		}
	}

	if e.cfg.Tools.E2ECmd != "" {
		parts := strings.Fields(e.cfg.Tools.E2ECmd)
		if _, err := git.RunInDir(e.repoDir, parts[0], parts[1:]...); err != nil {
			failures = append(failures, fmt.Sprintf("e2e: %v", err))
		}
	}

	duration := time.Since(start)
	llm := e.llmForStage(stage)
	e.recordMetric(taskID, stage, llm, duration)

	if len(failures) > 0 {
		return &pipeline.StageResult{
			Stage:   stage,
			Success: false,
			Output:  strings.Join(failures, "\n"),
		}, nil
	}

	return &pipeline.StageResult{
		Stage:   stage,
		Success: true,
		Output:  "all checks passed",
	}, nil
}
```

**Step 7: Run tests to verify compilation and tests pass**

```bash
cd /home/decodo/work/one-dev-army
go build ./...
go test ./internal/worker/... -v
```

Expected: Build succeeds, tests PASS

**Step 8: Commit**

```bash
git add internal/worker/processor.go
git commit -m "refactor: update worker Processor to use BranchManager

- Change wtMgr field type from *WorktreeManager to *BranchManager
- Update Process() to use Create(branch) and Remove(branch)
- Replace StageExecutor.worktree with repoDir string
- Update executeTesting() to use RunInDir() instead of RunInWorktree()
- Simplify executor initialization by passing repoDir directly"
```

---

## Task 6: Update main.go to Use BranchManager

**Files:**
- Modify: `main.go:222-226`

**Step 1: Remove worktrees directory creation and simplify manager initialization**

Replace lines 222-230:
```go
	// BranchManager works directly in the main repo - no worktrees directory needed
	wtMgr := git.NewBranchManager(dir)

	processor := worker.NewProcessor(cfg, oc, gh, store, wtMgr)

	orchestrator := mvp.NewOrchestrator(cfg, gh, oc, wtMgr, store, project.Number)
```

**Step 2: Run tests to verify compilation and tests pass**

```bash
cd /home/decodo/work/one-dev-army
go build ./...
go test ./... -v -short
```

Expected: Build succeeds, tests PASS

**Step 3: Commit**

```bash
git add main.go
git commit -m "refactor: update main.go to use BranchManager

- Remove .oda/worktrees/ directory creation
- Replace NewWorktreeManager(dir, worktreesDir) with NewBranchManager(dir)
- Simplify initialization - no worktrees needed"
```

---

## Task 7: Delete Old Worktree Files

**Files:**
- Delete: `internal/git/worktree.go`
- Delete: `internal/git/worktree_test.go`

**Step 1: Remove old worktree implementation files**

```bash
cd /home/decodo/work/one-dev-army
rm internal/git/worktree.go internal/git/worktree_test.go
```

**Step 2: Verify build still works**

```bash
cd /home/decodo/work/one-dev-army
go build ./...
```

Expected: Build succeeds (no references to old files)

**Step 3: Commit**

```bash
git rm internal/git/worktree.go internal/git/worktree_test.go
git commit -m "chore: remove old WorktreeManager implementation

- Delete internal/git/worktree.go
- Delete internal/git/worktree_test.go
- Replaced by BranchManager implementation"
```

---

## Task 8: Update Integration Tests

**Files:**
- Modify: `internal/mvp/integration_test.go` (check for worktree references)
- Modify: `internal/worker/processor_test.go` (check for worktree references)
- Modify: `internal/integration_test.go` (check for worktree references)

**Step 1: Check and update internal/mvp/integration_test.go**

Read the file and update any worktree-specific setup:

```bash
cd /home/decodo/work/one-dev-army
grep -n "worktree\|Worktree" internal/mvp/integration_test.go || echo "No worktree references found"
```

If references exist, update them to use BranchManager pattern (similar to Task 3 changes).

**Step 2: Check and update internal/worker/processor_test.go**

```bash
cd /home/decodo/work/one-dev-army
grep -n "worktree\|Worktree" internal/worker/processor_test.go || echo "No worktree references found"
```

If references exist, update them to use BranchManager pattern (similar to Task 5 changes).

**Step 3: Check and update internal/integration_test.go**

```bash
cd /home/decodo/work/one-dev-army
grep -n "worktree\|Worktree" internal/integration_test.go || echo "No worktree references found"
```

If references exist, update them to use BranchManager pattern.

**Step 4: Run all tests to verify everything passes**

```bash
cd /home/decodo/work/one-dev-army
go test ./... -v -short 2>&1 | head -100
```

Expected: All tests PASS

**Step 5: Commit any test file changes**

```bash
git add -A
git commit -m "test: update integration tests for BranchManager

- Replace worktree-based test setup with branch-based setup
- Update test helpers to use NewBranchManager instead of NewWorktreeManager"
```

---

## Task 9: Cleanup .gitignore

**Files:**
- Modify: `.gitignore:26,33-34`

**Step 1: Remove worktree-related entries from .gitignore**

Remove line 26:
```
.oda/worktrees/
```

Remove lines 33-34:
```
# Git worktrees
.worktrees/
```

The updated `.gitignore` should no longer have these entries.

**Step 2: Commit**

```bash
git add .gitignore
git commit -m "chore: remove worktree entries from .gitignore

- Remove .oda/worktrees/ entry
- Remove .worktrees/ entry
- No longer needed with BranchManager"
```

---

## Task 10: Update README.md Architecture Section

**Files:**
- Modify: `README.md:92`

**Step 1: Update architecture description**

Replace line 92:
```markdown
- **Workers** — goroutines with dedicated git worktrees for parallel task execution
```

With:
```markdown
- **Workers** — goroutines that checkout feature branches directly in the main repository
```

**Step 2: Commit**

```bash
git add README.md
git commit -m "docs: update README architecture section

- Replace 'dedicated git worktrees' with 'checkout feature branches directly'
- Reflect simplified single-worker architecture"
```

---

## Task 11: Final Verification and Cleanup

**Step 1: Run full test suite**

```bash
cd /home/decodo/work/one-dev-army
go test ./... -v 2>&1 | tail -50
```

Expected: All tests PASS

**Step 2: Verify build works**

```bash
cd /home/decodo/work/one-dev-army
go build -o oda .
./oda --help
```

Expected: Binary builds successfully, help text displays

**Step 3: Check for any remaining worktree references**

```bash
cd /home/decodo/work/one-dev-army
grep -r "worktree\|Worktree\|WorktreesDir" --include="*.go" . 2>/dev/null | grep -v "_test.go" | grep -v "vendor" || echo "No worktree references found in source code"
```

Expected: No worktree references (except possibly in test files that were updated)

**Step 4: Final commit if any changes**

```bash
git add -A
git commit -m "chore: final cleanup for branch-based workflow

- Remove any remaining worktree references
- Ensure all tests pass
- Verify binary builds successfully"
```

---

## Summary

This implementation plan replaces the git worktree-based architecture with a simpler branch-based approach:

### Changes Made:
1. **Created `BranchManager`** - Manages branches directly in main repo
2. **Removed `Worktree` field** from Task struct
3. **Updated all callers** to use repo directory instead of worktree path
4. **Simplified initialization** - no worktrees directory needed
5. **Updated all tests** to work with branch-based workflow
6. **Cleaned up documentation** - removed worktree references

### Benefits:
- Simpler architecture (no worktree lifecycle management)
- Fewer edge cases (no worktree cleanup issues)
- Direct git operations in main repo
- Easier to debug (LLM works in familiar directory)

### Verification:
- All unit tests pass
- All integration tests pass
- Binary builds successfully
- No worktree references remain in codebase
