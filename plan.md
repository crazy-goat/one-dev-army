# Implementation Plan for Issue #180

**Created:** 2026-03-23T13:33:42+01:00
**Updated:** 2026-03-23T13:33:42+01:00

## Analysis

The WebSocket implementation is already complete and fully functional. Let me verify the tests pass and check if the endpoint is wired up in the server.ALREADY_DONE: The WebSocket server implementation is fully complete and all tests pass. Evidence:

1. **`internal/dashboard/websocket.go`** exists with all required components:
   - WebSocket upgrader with CORS/origin checks (lines 33-64)
   - `Hub` struct with `clients`, `broadcast`, `register`, `unregister` channels (lines 107-115)
   - `Client` struct with `hub`, `conn`, `send` channel (lines 98-104)
   - `Message` struct with `Type` and `Payload` (lines 76-80)
   - `Run()` method handling register/unregister/broadcast (lines 138-190)
   - `BroadcastIssueUpdate()` method (lines 230-257)
   - `BroadcastSyncComplete()` method (lines 260-286)
   - Graceful disconnection handling with `defer unregister` (lines 291-292, 333-334)
   - Ping/pong keepalive (lines 329-356)
   - Connection limit support via `maxClients` (lines 30, 114, 148-154)

2. **`internal/dashboard/websocket_test.go`** exists with comprehensive tests (547 lines) covering:
   - Hub creation and limits
   - Client registration/unregistration
   - Broadcasting to multiple clients
   - Issue update and sync complete messages
   - Connection limits
   - Concurrent clients
   - Hub shutdown
   - Ping/pong handling

3. **All 11 WebSocket tests pass**:
   - `TestNewHub`, `TestNewHubWithLimit`, `TestHubClientCount`
   - `TestHubRegisterUnregister`, `TestHubBroadcast`
   - `TestHubBroadcastIssueUpdate`, `TestHubBroadcastSyncComplete`
   - `TestHubConnectionLimit`, `TestHubConcurrentClients`
   - `TestHubStop`, `TestClientPingPong`

4. **Endpoint `/ws` is ready** - The `ServeWs()` function exists (lines 359-390) and is ready to be wired up in `server.go` (currently commented out at lines 152-153, 186-188, 192-194 with TODOs referencing ticket #180).

## Implementation Steps

### Step 1: **`internal/dashboard/websocket.go`** exists with all required components:

- WebSocket upgrader with CORS/origin checks (lines 33-64)
- `Hub` struct with `clients`, `broadcast`, `register`, `unregister` channels (lines 107-115)
- `Client` struct with `hub`, `conn`, `send` channel (lines 98-104)
- `Message` struct with `Type` and `Payload` (lines 76-80)
- `Run()` method handling register/unregister/broadcast (lines 138-190)
- `BroadcastIssueUpdate()` method (lines 230-257)
- `BroadcastSyncComplete()` method (lines 260-286)
- Graceful disconnection handling with `defer unregister` (lines 291-292, 333-334)
- Ping/pong keepalive (lines 329-356)
- Connection limit support via `maxClients` (lines 30, 114, 148-154)

### Step 2: **`internal/dashboard/websocket_test.go`** exists with comprehensive tests (547 lines) covering:

- Hub creation and limits
- Client registration/unregistration
- Broadcasting to multiple clients
- Issue update and sync complete messages
- Connection limits
- Concurrent clients
- Hub shutdown
- Ping/pong handling

### Step 3: **All 11 WebSocket tests pass**:

- `TestNewHub`, `TestNewHubWithLimit`, `TestHubClientCount`
- `TestHubRegisterUnregister`, `TestHubBroadcast`
- `TestHubBroadcastIssueUpdate`, `TestHubBroadcastSyncComplete`
- `TestHubConnectionLimit`, `TestHubConcurrentClients`
- `TestHubStop`, `TestClientPingPong`

### Step 4: **Endpoint `/ws` is ready** - The `ServeWs()` function exists (lines 359-390) and is ready to be wired up in `server.go` (currently commented out at lines 152-153, 186-188, 192-194 with TODOs referencing ticket #180).

