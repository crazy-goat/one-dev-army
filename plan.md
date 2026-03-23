# Implementation Plan for Issue #179

**Created:** 2026-03-23T13:53:41+01:00
**Updated:** 2026-03-23T13:53:41+01:00

## Analysis

### 1. Core Requirements
- Create a `SyncService` struct that runs background synchronization every 30 seconds
- Sync issues from GitHub API to SQLite `issue_cache` table for the active milestone only
- Provide lifecycle methods: `Start()`, `Stop()`, `SyncNow()`
- Support dynamic milestone switching via `SetActiveMilestone()`
- Broadcast sync completion via WebSocket hub
- Handle errors gracefully without crashing

### 2. Files That Need Changes
- **NEW**: `internal/dashboard/sync.go` - Main sync service implementation
- **MODIFY**: `internal/dashboard/server.go` - Integrate SyncService into Server

### 3. Implementation Approach
- Use `time.Ticker` for periodic 30s sync intervals
- Use goroutine with proper shutdown signaling via `context.Context` or `chan struct{}`
- Leverage existing `github.Client.ListIssuesForMilestone()` for fetching
- Leverage existing `db.Store.SaveIssueCache()` for persistence
- Leverage existing `Hub.BroadcastSyncComplete()` for WebSocket notifications
- Thread-safe milestone switching with mutex protection

### 4. Testing Strategy
- Unit tests for `SyncService` with mocked GitHub client and store
- Test start/stop lifecycle
- Test manual sync trigger
- Test error handling (GitHub API failures)
- Test milestone switching during active sync

---

## Implementation Steps

### Step 1: Add `syncService *SyncService` field to `Server` struct

### Step 2: Initialize in `NewServer()` after hub creation

### Step 3: Start sync service in `NewServer()` if `gh` and `store` are available

### Step 4: Stop sync service in `Shutdown()`

**Code changes:**
- Line 33: Add `syncService *SyncService` to Server struct
- Line 65 (after hub.Run()): Initialize and start sync service
- Line 174-179 (Shutdown): Add sync service stop call
### Phase 3: Testing (`internal/dashboard/sync_test.go`)
Create comprehensive tests:
- `TestSyncService_StartStop` - lifecycle management
- `TestSyncService_SyncNow` - manual sync trigger
- `TestSyncService_SetActiveMilestone` - milestone switching
- `TestSyncService_GitHubError` - error handling
- `TestSyncService_Broadcast` - WebSocket notification
### Order of Operations

### Step 1: **Create** `internal/dashboard/sync.go` with full implementation

### Step 2: **Modify** `internal/dashboard/server.go` to integrate SyncService

### Step 3: **Create** `internal/dashboard/sync_test.go` with unit tests

### Step 4: **Run** tests: `go test ./internal/dashboard/... -v`

### Step 5: **Verify** integration: Run server and check logs for sync activity

### Acceptance Criteria Verification
- ✅ Sync runs every 30s automatically (ticker-based)
- ✅ Manual sync works via `SyncNow()` (public method)
- ✅ Errors are logged but don't crash service (error handling in performSync)
- ✅ Can be started/stopped cleanly (context-based lifecycle)
- ✅ Broadcasts update after sync (hub.BroadcastSyncComplete)
- ✅ Only syncs active milestone issues (milestone filtering in ListIssuesForMilestone)

