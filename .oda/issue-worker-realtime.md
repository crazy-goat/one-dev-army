## Problem: Dashboard doesn't show worker status changes in real-time

### Current Behavior
When a worker starts processing a ticket or changes stage (Plan → Code → Review), the dashboard board page doesn't update automatically. Users must manually refresh the page to see:
- Which ticket is currently being processed
- What stage the worker is in
- Worker elapsed time

### Expected Behavior
Dashboard should update automatically via WebSocket when worker status changes, similar to how issue updates are broadcast.

### Technical Analysis

**Current WebSocket message types** (`internal/dashboard/websocket.go:71-74`):
```go
MessageTypeIssueUpdate  MessageType = "issue_update"
MessageTypeSyncComplete MessageType = "sync_complete"
MessageTypePing         MessageType = "ping"
MessageTypePong         MessageType = "pong"
```

**Missing:** `MessageTypeWorkerUpdate`

**Worker status changes occur in:**
- `internal/worker/worker.go:50-73` - `SetTask()`, `SetStage()`, `SetIdle()`
- `internal/mvp/worker.go:135-296` - Worker calls `setStageLabel()` which broadcasts via orchestrator
- `internal/mvp/orchestrator.go:381-393` - `BroadcastStageUpdate()` updates GitHub labels and broadcasts issue update

**The gap:** While `BroadcastStageUpdate()` sends `issue_update` messages, there's no dedicated worker status broadcast. The dashboard board page shows worker info from `WorkerInfo` struct but only refreshes on:
- Manual page refresh
- HTMX refresh trigger (every 30s from sync)
- Issue update WebSocket (but this doesn't include worker status)

### Proposed Solution

Add WebSocket support for real-time worker status updates:

1. **Add new message type** (`internal/dashboard/websocket.go`):
```go
MessageTypeWorkerUpdate MessageType = "worker_update"
```

2. **Add broadcast method** (`internal/dashboard/websocket.go`):
```go
func (h *Hub) BroadcastWorkerUpdate(worker WorkerInfo)
```

3. **Broadcast on worker changes** (`internal/mvp/orchestrator.go`):
   - When worker starts: `worker.SetTask()` → broadcast
   - When worker changes stage: `worker.SetStage()` → broadcast  
   - When worker finishes: `worker.SetIdle()` → broadcast

4. **Add JavaScript handler** (`internal/dashboard/templates/board.html`):
   - Listen for `worker_update` messages
   - Update worker status display in real-time
   - Show current ticket being processed
   - Update elapsed time

### Implementation Tasks

- [ ] Add `MessageTypeWorkerUpdate` constant
- [ ] Implement `BroadcastWorkerUpdate()` method in Hub
- [ ] Add worker status tracking to Orchestrator
- [ ] Broadcast worker updates on SetTask/SetStage/SetIdle
- [ ] Add JavaScript WebSocket handler for worker_update messages
- [ ] Update board.html to show real-time worker status
- [ ] Test with active worker processing

### Acceptance Criteria

1. When worker starts a ticket, dashboard shows it immediately without refresh
2. When worker changes stage (Plan→Code→Review), stage indicator updates in real-time
3. Elapsed time counter updates live while worker is active
4. When worker finishes, status clears automatically
5. Multiple dashboard clients see updates simultaneously

### Related Code

- Worker status: `internal/worker/worker.go`
- Worker processing: `internal/mvp/worker.go`
- Orchestrator: `internal/mvp/orchestrator.go:381-393`
- WebSocket hub: `internal/dashboard/websocket.go`
- Board template: `internal/dashboard/templates/board.html:147`
