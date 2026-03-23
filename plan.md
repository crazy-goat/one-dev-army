# Implementation Plan for Issue #184

**Created:** 2026-03-23T14:05:50+01:00
**Updated:** 2026-03-23T14:05:50+01:00

## Analysis

### 1. Core Requirements
The issue requires implementing a startup sequence that populates the cache before serving requests. The key requirements are:
- Initialize database → Create GitHub client → Find active milestone → Fetch issues → Populate cache → Create WebSocket hub → Create and start SyncService → Create and start dashboard server
- Add logging for each step
- Handle errors gracefully (log and continue if possible)
- Handle edge cases: no active milestone, GitHub API errors
- Sync service should start automatically with a 30s ticker

### 2. Files That Need Changes
- `main.go` - Complete restructuring of startup sequence
- `internal/dashboard/server.go` - Update constructor to accept hub and syncService
- `internal/dashboard/sync.go` - **NEEDS CREATION** (exists in commit 14c8b28 but not on current branch)
- `internal/dashboard/sync_test.go` - **NEEDS CREATION**

### 3. Implementation Approach
The sync service implementation exists in commit 14c8b28 but hasn't been merged to this branch yet. I need to:
1. Create the SyncService with background ticker, graceful shutdown, and WebSocket broadcasting
2. Restructure main.go to follow the exact startup sequence specified
3. Update the server constructor to accept and manage the hub and syncService
4. Add comprehensive logging at each startup step
5. Implement error handling that logs but continues where possible

### 4. Testing Strategy
- Unit tests for SyncService (start/stop/syncNow methods, ticker behavior, thread safety)
- Integration test for startup sequence
- Test error scenarios (no milestone, GitHub API failure)

## Implementation Steps

### Step 1: **Initialize database** (lines 158-163) - keep existing

### Step 2: **Create GitHub client** (line 167) - keep existing

### Step 3: **Get active milestone** (lines 193-202) - keep existing, add explicit logging

### Step 4: **Fetch all issues from GitHub for active milestone** - NEW CODE

- If activeMilestone exists, call `gh.ListIssuesForMilestone()`
- Log: "Fetching issues from GitHub for milestone: X"

### Step 5: **Populate issue_cache table** - NEW CODE

- Iterate through issues and call `store.SaveIssueCache()`
- Log: "Cached N issues"
- Handle errors gracefully (log but continue)

### Step 6: **Create WebSocket hub** - NEW CODE

- `hub := dashboard.NewHub()`
- `go hub.Run()`
- Log: "WebSocket hub started"

### Step 7: **Create SyncService with hub reference** - NEW CODE

- `syncService := dashboard.NewSyncService(gh, store, hub)`
- If activeMilestone exists: `syncService.SetActiveMilestone(activeMilestone.Title)`

### Step 8: **Start SyncService** - NEW CODE

- `syncService.Start()` (begins 30s ticker)
- Log: "Sync service started (30s interval)"

### Step 9: **Create dashboard server with hub and syncService** - MODIFY EXISTING

- Change: `srv, err := dashboard.NewServer(cfg.Dashboard.Port, store, pool.Workers, gh, orchestrator, oc, cfg.Planning.LLM, hub, syncService)`

### Step 10: **Start dashboard server** (lines 240-246) - keep existing

Add error handling:
- If no active milestone: log warning, continue with empty cache
- If GitHub API error: log error, continue with empty cache
- All errors logged but startup continues where possible
### Phase 4: Update Server Constructor Call
**File: `main.go` line 231**
Change from:
```go
srv, err := dashboard.NewServer(cfg.Dashboard.Port, store, pool.Workers, gh, orchestrator, oc, cfg.Planning.LLM)
```
To:
```go
srv, err := dashboard.NewServer(cfg.Dashboard.Port, store, pool.Workers, gh, orchestrator, oc, cfg.Planning.LLM, hub, syncService)
```
### Order of Operations

### Step 1: Create `internal/dashboard/sync.go` with full SyncService implementation

### Step 2: Create `internal/dashboard/sync_test.go` with comprehensive tests

### Step 3: Run tests: `go test ./internal/dashboard/... -v`

### Step 4: Update `internal/dashboard/server.go`:

- Add syncService field
- Modify NewServer signature
- Update Shutdown method

### Step 5: Update `main.go`:

- Add hub creation after database init
- Add issue fetching and cache population
- Add syncService creation and start
- Update NewServer call
- Add logging for each step

### Step 6: Run full test suite: `go test ./...`

### Step 7: Build and verify: `go build -o oda`

### Acceptance Criteria Verification
- [x] Startup populates cache before serving requests - achieved by fetching issues before creating server
- [x] No empty board on first load (if issues exist) - cache populated before server starts
- [x] All services start cleanly - proper error handling and logging
- [x] Errors are logged but don't crash startup - graceful error handling
- [x] Works with no active milestone (empty board) - check for nil milestone, log warning, continue
- [x] Sync service starts automatically - syncService.Start() called during startup

