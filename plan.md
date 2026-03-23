# Implementation Plan for Issue #180

**Created:** 2026-03-23T13:07:49+01:00
**Updated:** 2026-03-23T13:07:49+01:00

## Analysis

### 1. Core Requirements
- Create a WebSocket server at `/ws` endpoint for real-time dashboard updates
- Implement Hub pattern with client management (register/unregister/broadcast)
- Support message types: `issue_update`, `sync_complete`
- Handle multiple concurrent connections with proper lifecycle management
- Implement ping/pong keepalive and graceful disconnections
- Add connection limits and CORS/origin checks
- Provide broadcast methods for issue updates and sync completion

### 2. Files That Need Changes
- **NEW**: `internal/dashboard/websocket.go` - Core WebSocket implementation
- **NEW**: `internal/dashboard/websocket_test.go` - Unit tests for Hub
- **MODIFY**: `internal/dashboard/server.go` - Add WebSocket route and Hub integration
- **MODIFY**: `go.mod` - Add gorilla/websocket dependency

### 3. Implementation Approach
- Use **gorilla/websocket** library (industry standard for Go WebSockets)
- Implement classic Hub pattern with goroutines for concurrency
- Hub runs in background goroutine handling register/unregister/broadcast channels
- Each Client has its own goroutine for reading/writing
- Ping/pong every 30 seconds to keep connections alive
- Connection limit: 100 concurrent clients (configurable)
- JSON message format for all communications

### 4. Testing Strategy
- Unit tests for Hub: client registration, broadcasting, unregistration
- Test concurrent client operations
- Test message marshaling/unmarshaling
- Test connection limits
- Mock WebSocket connections using gorilla/websocket test utilities

