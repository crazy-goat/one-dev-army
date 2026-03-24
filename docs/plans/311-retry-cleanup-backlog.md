# Plan: Issue #311 — Retry should cleanup local branch and move to backlog

## Analysis

### 1. Core Requirements

The "Retry" button (`handleRetry` in `handlers.go:333`) currently moves a failed ticket directly to `stage:coding` without any cleanup. This causes two problems:
- Two tickets can appear "in progress" simultaneously (the one the worker is processing + the retried one)
- Stale local branches accumulate

The fix: make `handleRetry` behave more like `handleRetryFresh` — cleanup PR/branch, clear steps, and move to `stage:backlog` instead of `stage:coding`.

### 2. Files That Need Changes

| File | Change |
|------|--------|
| `internal/dashboard/server.go` | Add `brMgr *git.BranchManager` field to `Server` struct; update `NewServer` signature |
| `internal/dashboard/handlers.go` | Rewrite `handleRetry` to cleanup PR, delete local branch, clear steps, move to backlog |
| `internal/dashboard/handlers.go` | Add local branch cleanup to `handleRetryFresh` |
| `internal/git/worktree.go` | Add `FindBranchByPrefix(prefix string) string` method to find local branches matching `oda-{issueNum}-*` |
| `internal/dashboard/templates/board.html` | Update button labels for clarity |
| `main.go` | Pass `brMgr` to `NewServer` |
| `internal/dashboard/handlers_test.go` | Add tests for retry with branch cleanup |
| `docs/state-machine.md` | Update retry transition: Failed → Backlog (not Code) |

### 3. Implementation Approach

**Key insight**: The dashboard `Server` currently has no access to `git.BranchManager`. The orchestrator has it (`orchestrator.go:33`), but doesn't expose it. Two options:

- **Option A**: Add `brMgr` directly to `Server` struct (simpler, follows existing pattern where `Server` already has `gh`, `store`, etc.)
- **Option B**: Add a `CleanupTicket(issueNum)` method to `Orchestrator` that delegates

**Chosen: Option A** — it's simpler and consistent with how `Server` already holds `gh` and `store` directly.

**Branch name discovery**: The branch format is `oda-{issueNum}-{slug}` (worker.go:103). Since we don't know the slug from the dashboard, we need a method to find local branches matching `oda-{issueNum}-*`. We'll add `FindBranchByPrefix` to `BranchManager`.

### 4. Testing Strategy

- **Unit test**: `handleRetry` with mock `BranchManager` — verify it calls cleanup and moves to backlog
- **Unit test**: `handleRetryFresh` — verify it also cleans up local branch
- **Unit test**: `FindBranchByPrefix` — verify it finds matching branches
- **Existing tests**: `TestCreateAndRemoveBranch` and `TestRemoveBranchNonExistent` in `worktree_test.go` cover `RemoveBranch`

Test patterns from existing code:
- `handlers_test.go` uses `httptest.NewRecorder`, `http.NewRequest`, direct handler calls
- `worktree_test.go` uses real git repos created in `t.TempDir()`

### 5. Complexity Estimate

**Small** (hours) — straightforward plumbing changes, no new concepts.

### 6. Potential Breaking Changes

- `NewServer` signature changes (adds `brMgr` parameter) — callers in `main.go` and integration tests need updating
- Retry behavior changes from "immediate coding" to "goes to backlog" — this is the intended behavioral change
- No database migrations needed
- No API changes

---

## Implementation Plan

### Step 1: Add `FindBranchByPrefix` to `BranchManager`

**File**: `internal/git/worktree.go`

Add a new method after `RemoveBranch` (~line 112):

```go
// FindBranchByPrefix returns the first local branch name matching the given prefix.
// Returns empty string if no matching branch is found.
func (m *BranchManager) FindBranchByPrefix(prefix string) string {
	cmd := exec.Command("git", "branch", "--list", prefix+"*")
	cmd.Dir = m.repoDir
	out, err := cmd.CombinedOutput()
	if err != nil {
		return ""
	}
	for line := range strings.SplitSeq(strings.TrimSpace(string(out)), "\n") {
		branch := strings.TrimSpace(strings.TrimPrefix(line, "*"))
		if branch != "" {
			return branch
		}
	}
	return ""
}
```

**Test file**: `internal/git/worktree_test.go` — add `TestFindBranchByPrefix`

### Step 2: Add `brMgr` to dashboard `Server`

**File**: `internal/dashboard/server.go`

1. Add field to struct (line 41, before `modelsCache`):
   ```go
   brMgr           *git.BranchManager
   ```

2. Add import: `"github.com/crazy-goat/one-dev-army/internal/git"`

3. Update `NewServer` signature (line 44) — add `brMgr *git.BranchManager` parameter after `rootDir`:
   ```go
   func NewServer(port int, webPort int, store *db.Store, pool func() []worker.WorkerInfo, gh *github.Client, orchestrator *mvp.Orchestrator, oc *opencode.Client, wizardLLM string, hub *Hub, syncService *SyncService, rootDir string, brMgr *git.BranchManager) (*Server, error) {
   ```

4. Set field in constructor (line 70, after `rootDir`):
   ```go
   brMgr:       brMgr,
   ```

### Step 3: Rewrite `handleRetry`

**File**: `internal/dashboard/handlers.go` (line 333-354)

Replace the current implementation with:

```go
func (s *Server) handleRetry(w http.ResponseWriter, r *http.Request) {
	id := r.PathValue("id")
	issueNum := 0
	if _, err := fmt.Sscanf(id, "%d", &issueNum); err != nil {
		log.Printf("[Dashboard] Error parsing issue ID %q: %v", id, err)
	}
	if issueNum == 0 || s.gh == nil {
		http.Redirect(w, r, "/", http.StatusSeeOther)
		return
	}

	// Close any open PR and delete remote branch
	if branch, err := s.gh.FindPRBranch(issueNum); err == nil {
		log.Printf("[Dashboard] Closing PR for #%d (branch: %s)", issueNum, branch)
		if closeErr := s.gh.ClosePR(branch); closeErr != nil {
			log.Printf("[Dashboard] Error closing PR for #%d: %v", issueNum, closeErr)
		}
	}

	// Delete local branch (best-effort — log errors but don't block)
	if s.brMgr != nil {
		prefix := fmt.Sprintf("oda-%d-", issueNum)
		if branch := s.brMgr.FindBranchByPrefix(prefix); branch != "" {
			log.Printf("[Dashboard] Deleting local branch %q for #%d", branch, issueNum)
			if err := s.brMgr.RemoveBranch(branch); err != nil {
				log.Printf("[Dashboard] Error deleting local branch for #%d: %v", issueNum, err)
			}
		}
	}

	// Set stage label to backlog (not coding) via orchestrator
	err := s.orchestrator.ChangeStage(issueNum, github.StageBacklog, github.ReasonManualRetry)
	if err != nil {
		log.Printf("[Dashboard] Error setting Backlog stage on #%d: %v", issueNum, err)
		http.Redirect(w, r, "/", http.StatusSeeOther)
		return
	}

	// Clear DB steps so ticket starts fresh
	if s.store != nil {
		if err := s.store.DeleteSteps(issueNum); err != nil {
			log.Printf("[Dashboard] Error deleting steps for #%d: %v", issueNum, err)
		}
	}

	log.Printf("[Dashboard] Retry #%d — PR closed, branch cleaned, steps cleared, moved to Backlog", issueNum)
	http.Redirect(w, r, "/", http.StatusSeeOther)
}
```

### Step 4: Add local branch cleanup to `handleRetryFresh`

**File**: `internal/dashboard/handlers.go` (line 356-392)

Add local branch deletion after the PR close block (after line 373, before the `ChangeStage` call):

```go
	// Delete local branch (best-effort)
	if s.brMgr != nil {
		prefix := fmt.Sprintf("oda-%d-", issueNum)
		if branch := s.brMgr.FindBranchByPrefix(prefix); branch != "" {
			log.Printf("[Dashboard] Deleting local branch %q for #%d", branch, issueNum)
			if err := s.brMgr.RemoveBranch(branch); err != nil {
				log.Printf("[Dashboard] Error deleting local branch for #%d: %v", issueNum, err)
			}
		}
	}
```

### Step 5: Update `main.go`

**File**: `main.go` (line 402)

Pass `brMgr` to `NewServer`:

```go
srv, err := dashboard.NewServer(cfg.Dashboard.Port, cfg.OpenCode.WebPort, store, pool.Workers, gh, orchestrator, oc, cfg.LLM.Planning.Model, hub, syncService, dir, brMgr)
```

### Step 6: Update integration tests calling `NewServer`

**File**: `internal/integration_test.go` (lines 575, 626)

Add `nil` as the last argument for `brMgr`:

```go
srv, err := dashboard.NewServer(0, 5001, store, poolFn, nil, nil, nil, "", nil, nil, tmpDir, nil)
```

### Step 7: Update dashboard button labels

**File**: `internal/dashboard/templates/board.html` (lines 287-288)

Change:
```html
<form method="post" action="/retry/{{.ID}}"><button type="submit" class="btn btn-success">Retry</button></form>
<form method="post" action="/retry-fresh/{{.ID}}"><button type="submit" class="btn btn-primary">Cancel</button></form>
```

To:
```html
<form method="post" action="/retry/{{.ID}}"><button type="submit" class="btn btn-success">Retry (backlog)</button></form>
<form method="post" action="/retry-fresh/{{.ID}}"><button type="submit" class="btn btn-primary">Cancel (backlog)</button></form>
```

### Step 8: Update state machine documentation

**File**: `docs/state-machine.md`

Update the retry transition descriptions:
- Line 34-37: Change `Failed → [Retry] → Code` to `Failed → [Retry] → Backlog`
- Line 149: Change target from `Code` to `Backlog`
- Line 152: Update note about retry always going to Code
- Line 199: Change `Failed | Retry | → Code` to `Failed | Retry | → Backlog`
- Line 210-214: Update retry behavior section
- Line 299: Add changelog entry

### Step 9: Add unit tests

**File**: `internal/dashboard/handlers_test.go`

Add tests following existing patterns in the file:

1. `TestHandleRetry_CleansUpAndMovesToBacklog` — verify that:
   - Local branch is deleted (mock `BranchManager`)
   - PR is closed (mock `gh`)
   - Steps are cleared (mock `store`)
   - Stage changes to backlog (mock `orchestrator`)
   - Response is redirect to `/`

2. `TestHandleRetryFresh_CleansUpLocalBranch` — verify local branch cleanup added

3. `TestHandleRetry_BranchCleanupErrorDoesNotBlock` — verify that branch cleanup errors are logged but don't prevent the retry

**File**: `internal/git/worktree_test.go`

Add `TestFindBranchByPrefix`:
- Create a branch with `oda-42-test-slug` name
- Verify `FindBranchByPrefix("oda-42-")` returns it
- Verify `FindBranchByPrefix("oda-99-")` returns empty string

### Step 10: Verify

Run:
```bash
golangci-lint run ./...
go test -race ./...
```

---

## Order of Operations

1. `internal/git/worktree.go` — Add `FindBranchByPrefix` + test
2. `internal/dashboard/server.go` — Add `brMgr` field and update constructor
3. `main.go` — Pass `brMgr` to `NewServer`
4. `internal/integration_test.go` — Fix `NewServer` calls with `nil` brMgr
5. `internal/dashboard/handlers.go` — Rewrite `handleRetry`, update `handleRetryFresh`
6. `internal/dashboard/templates/board.html` — Update button labels
7. `internal/dashboard/handlers_test.go` — Add retry tests
8. `docs/state-machine.md` — Update documentation
9. Run lint + tests
