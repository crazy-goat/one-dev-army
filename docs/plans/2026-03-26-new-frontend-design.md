# New Frontend Architecture Design

**Date:** 2026-03-26  
**Branch:** new-fe  
**Status:** Draft  

## Overview

Migration from HTMX-based dashboard to React SPA with REST API, maintaining both versions during transition period with feature flag and user-controlled switch.

## Current State Analysis

### Existing Dashboard Features

1. **Kanban Board** (`/dashboard`)
   - 10 columns: Blocked, Backlog, Plan, Code, AI Review, Check Pipeline, Approve, Merge, Done, Failed
   - Task cards with labels, assignees, PR links
   - Processing panel showing current active task
   - Sprint controls (Start/Pause/Close)
   - Manual sync button
   - Decline modal with reason input
   - Process confirmation modal

2. **Task Detail** (`/task/{id}`)
   - Step-by-step progress view
   - LLM prompt/response display
   - Live streaming via Server-Sent Events
   - Auto-refresh for active tasks

3. **Wizard** (`/wizard`)
   - 3-step form: Type selection → Refine (LLM) → Create
   - Session-based state management
   - Epic breakdown (optional)
   - Sprint assignment

4. **Settings** (`/settings`)
   - LLM model configuration
   - YOLO mode toggle

5. **Real-time Features**
   - WebSocket for live updates
   - Issue updates, worker status, sync completion
   - Sprint closable status

6. **Footer**
   - WebSocket connection status
   - Worker status with tooltip
   - YOLO mode indicator
   - GitHub API rate limit

### Current Endpoints

```
GET  /                    - Board page
GET  /api/board-data      - Board JSON data
GET  /api/current-task    - Current task info
GET  /api/sprint/status   - Sprint status
POST /api/sprint/start    - Start sprint
POST /api/sprint/pause    - Pause sprint
POST /api/sprint/close    - Close sprint
GET  /task/{id}           - Task detail page
GET  /api/task/{id}/stream - SSE stream
POST /approve/{id}        - Approve task
POST /reject/{id}         - Reject to backlog
POST /retry/{id}          - Retry task
POST /retry-fresh/{id}    - Retry fresh
POST /approve-merge/{id}  - Approve & merge
POST /decline/{id}        - Decline with reason
POST /block/{id}          - Block task
POST /unblock/{id}        - Unblock task
POST /api/tickets/{id}/process - Manual process
GET  /wizard              - Wizard page
GET  /wizard/new         - Wizard step 1
POST /wizard/refine      - Wizard step 2
POST /wizard/create      - Wizard step 3
GET  /wizard/logs/{id}    - Wizard logs
GET  /settings            - Settings page
POST /settings            - Save settings
GET  /api/rate-limit      - Rate limit status
POST /api/yolo/toggle     - Toggle YOLO mode
GET  /api/worker-status   - Worker status
GET  /ws                  - WebSocket
```

## Proposed Architecture

### Approach: Full Migration with Feature Flag

**URL Structure:**
```
/               → Feature flag check:
                  - false: Old dashboard (HTMX)
                  - true:  Redirect to /new/
/new/           → New React SPA (always)
/api/v2/*       → New REST API endpoints
/ws             → WebSocket (unchanged)
```

**Feature Flag:**
- Config field: `UseNewFrontend bool` (default: `false`)
- User override: LocalStorage + Cookie
- Footer switch: Toggle between versions

### New API Structure

```
GET  /api/v2/board              → Board data (all columns)
GET  /api/v2/tasks/{id}         → Task details with steps
POST /api/v2/tasks/{id}/actions/{action}  → All task actions
                                 actions: approve, reject, retry, retry-fresh,
                                         merge, decline, block, unblock, process
GET  /api/v2/sprint             → Sprint status
POST /api/v2/sprint/{action}    → Sprint actions: start, pause, close
GET  /api/v2/wizard             → Wizard state
POST /api/v2/wizard/{step}      → Wizard steps: init, refine, create
GET  /api/v2/settings           → Settings
POST /api/v2/settings           → Save settings
GET  /api/v2/rate-limit         → Rate limit
POST /api/v2/yolo/toggle        → Toggle YOLO mode
GET  /api/v2/worker-status      → Worker status
GET  /api/v2/task/{id}/stream   → SSE stream (keep as-is)
```

### React App Structure

```
web/
├── src/
│   ├── components/
│   │   ├── Board/
│   │   │   ├── KanbanBoard.tsx
│   │   │   ├── KanbanColumn.tsx
│   │   │   ├── TaskCard.tsx
│   │   │   └── ProcessingPanel.tsx
│   │   ├── Task/
│   │   │   ├── TaskDetail.tsx
│   │   │   ├── StepList.tsx
│   │   │   ├── StepItem.tsx
│   │   │   └── StreamBox.tsx
│   │   ├── Wizard/
│   │   │   ├── WizardModal.tsx
│   │   │   ├── Step1Type.tsx
│   │   │   ├── Step2Refine.tsx
│   │   │   └── Step3Create.tsx
│   │   ├── Layout/
│   │   │   ├── Layout.tsx
│   │   │   ├── Navbar.tsx
│   │   │   ├── Footer.tsx
│   │   │   └── VersionSwitch.tsx
│   │   └── common/
│   │       ├── Button.tsx
│   │       ├── Badge.tsx
│   │       ├── Modal.tsx
│   │       └── LabelIcon.tsx
│   ├── pages/
│   │   ├── BoardPage.tsx
│   │   ├── TaskPage.tsx
│   │   ├── WizardPage.tsx
│   │   └── SettingsPage.tsx
│   ├── hooks/
│   │   ├── useBoard.ts
│   │   ├── useTask.ts
│   │   ├── useSprint.ts
│   │   ├── useWizard.ts
│   │   ├── useWebSocket.ts
│   │   └── useVersion.ts
│   ├── api/
│   │   ├── client.ts
│   │   ├── board.ts
│   │   ├── tasks.ts
│   │   ├── sprint.ts
│   │   ├── wizard.ts
│   │   └── settings.ts
│   ├── types/
│   │   ├── board.ts
│   │   ├── task.ts
│   │   ├── sprint.ts
│   │   └── wizard.ts
│   ├── store/
│   │   └── index.ts          → Zustand store
│   ├── utils/
│   │   └── helpers.ts
│   ├── App.tsx
│   ├── main.tsx
│   └── index.css
├── public/
├── package.json
├── vite.config.ts
└── tsconfig.json
```

### State Management

**Zustand Store:**
```typescript
interface Store {
  // Board state
  board: BoardData | null
  isLoading: boolean
  error: string | null
  
  // Sprint state
  sprint: SprintStatus | null
  
  // Worker state
  worker: WorkerStatus | null
  
  // WebSocket connection
  wsConnected: boolean
  
  // Actions
  refreshBoard: () => Promise<void>
  updateTask: (taskId: number, updates: Partial<Task>) => void
  setWsConnected: (connected: boolean) => void
}
```

### Technology Stack

- **Framework:** React 18 + TypeScript
- **Build Tool:** Vite
- **Routing:** React Router v6
- **State:** Zustand
- **Data Fetching:** SWR (stale-while-revalidate)
- **Styling:** CSS Modules (zachować obecny design)
- **WebSocket:** Native WebSocket API

### Build & Integration

**Development:**
```bash
# Terminal 1
cd web && npm run dev  # localhost:5173

# Terminal 2
go run ./cmd/oda       # localhost:8080
# (Vite proxy: /api → :8080, /ws → :8080)
```

**Production:**
```bash
# Build frontend
cd web && npm run build  # → web/dist/

# Build Go (embeds web/dist/)
go build ./cmd/oda
```

**Go Integration:**
```go
//go:embed web/dist/*
var webFS embed.FS

func (s *Server) routes() {
    // API routes
    s.mux.HandleFunc("GET /api/v2/...", s.handleAPI)
    
    // WebSocket
    s.mux.HandleFunc("GET /ws", s.handleWebSocket)
    
    // Static files (React app)
    web, _ := fs.Sub(webFS, "web/dist")
    s.mux.Handle("GET /new/assets/", http.FileServer(http.FS(web)))
    
    // SPA fallback for /new/*
    s.mux.HandleFunc("GET /new/{$}", s.serveReactApp)
    s.mux.HandleFunc("GET /new/{path...}", s.serveReactApp)
    
    // Root with feature flag
    s.mux.HandleFunc("GET /{$}", s.handleRoot)
}

func (s *Server) handleRoot(w http.ResponseWriter, r *http.Request) {
    // Check user preference (cookie/localStorage via JS)
    // Check config flag
    // Redirect to /new/ or serve old dashboard
}
```

## Migration Phases

### Phase 1: Setup (1-2 days)
1. Create `web/` directory structure
2. Setup React + Vite + TypeScript
3. Add feature flag to config
4. Create version switch component
5. Setup proxy for dev

### Phase 2: API Layer (2-3 days)
1. Create `/api/v2/*` endpoints
2. Refactor existing handlers to reuse logic
3. Add tests for new endpoints
4. Document API changes

### Phase 3: React Components (5-7 days)
1. Layout (Navbar, Footer with switch)
2. Board page (Kanban, columns, cards)
3. Task page (steps, SSE stream)
4. Wizard (3-step form)
5. Settings page
6. WebSocket integration

### Phase 4: Integration (2 days)
1. Build pipeline
2. Embed in Go binary
3. Feature flag logic
4. Testing both versions

### Phase 5: Stabilization (2 days)
1. Bug fixes
2. Performance optimization
3. User testing
4. Change default to new frontend

### Phase 6: Cleanup (1 day)
1. Remove old dashboard code
2. Remove feature flag
3. Update documentation

## Open Questions

1. **Drag & drop?** - Czy dodać możliwość przenoszenia tasków między kolumnami?
2. **Mobile?** - Czy wymagana jest responsywność mobilna?
3. **Dark mode?** - Czy zachować tylko dark theme czy dodać toggle?
4. **Keyboard shortcuts?** - Czy dodać skróty klawiszowe?

## Success Criteria

- [ ] All existing features work in new frontend
- [ ] WebSocket real-time updates functional
- [ ] User can switch between versions
- [ ] No regression in performance
- [ ] Build produces single binary
- [ ] All tests pass

## Risks & Mitigation

| Risk | Mitigation |
|------|-----------|
| Longer development time | MVP approach - board + task first |
| WebSocket complexity | Keep existing implementation |
| Bundle size | Code splitting, lazy loading |
| User resistance | Feature flag, easy rollback |
| Testing overhead | Maintain both versions temporarily |

## Next Steps

1. Approve this design
2. Create implementation plan with tasks
3. Start Phase 1 (Setup)
