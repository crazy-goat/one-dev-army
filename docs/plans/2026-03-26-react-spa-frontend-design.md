# React SPA Frontend — Design Document

> **For Claude:** REQUIRED SUB-SKILL: Use superpowers:executing-plans to implement this plan task-by-task.

**Goal:** Replace the HTMX server-rendered dashboard with a React SPA that communicates exclusively via JSON API and WebSocket, fully separating frontend from backend while remaining embedded in the Go binary.

**Architecture:** React 19 SPA bundled by Vite into 3 files (index.html + app.js + app.css), embedded via `go:embed` and served by the Go HTTP server. New `/api/v2/*` JSON endpoints wrap existing backend logic. WebSocket (`/ws`) and SSE endpoints remain unchanged. Both old and new dashboards coexist during migration with a footer toggle.

**Tech Stack:** React 19, TypeScript, React Router v7, TanStack Query, Tailwind CSS v4, Vite

---

## Decisions

| Aspect | Decision |
|--------|----------|
| Framework | React 19 + TypeScript |
| Routing | React Router v7 |
| Server state | TanStack Query |
| UI state | `useState` / `useContext` |
| Styling | Tailwind CSS v4 |
| Bundler | Vite |
| Output | 3 files: `index.html` + `app.js` + `app.css` |
| Directory | `web/` (clean and reuse) |
| Embed | `//go:embed web/dist/*` |
| Backend API | New endpoints under `/api/v2/` |
| Real-time | Existing WebSocket `/ws` + SSE unchanged |
| Migration | Both dashboards coexist, footer toggle to switch |

---

## Architecture — FE/BE Separation

```
+-----------------------------------------------------+
|                   Go Binary (oda)                    |
|                                                      |
|  +--------------+  +-------------+  +------------+  |
|  | embed.FS     |  | /api/v2/*   |  | /ws        |  |
|  | web/dist/*   |  | JSON API    |  | WebSocket  |  |
|  |              |  |             |  | (existing) |  |
|  | index.html   |  | Board       |  |            |  |
|  | app.js       |  | Tasks       |  | issue_upd  |  |
|  | app.css      |  | Sprint      |  | worker_upd |  |
|  |              |  | Wizard      |  | sync_compl |  |
|  | SPA ---------+--+ Settings    |  | log_stream |  |
|  | (React)      |  | Workers     |  | sprint_cls |  |
|  +--------------+  +-------------+  +------------+  |
|                                                      |
|  +----------------------------------------------+   |
|  | Old HTMX dashboard (coexists during migration)|   |
|  | GET /        -> old (default initially)       |   |
|  | GET /new/*   -> new React SPA                 |   |
|  +----------------------------------------------+   |
+-----------------------------------------------------+
```

The SPA loads from embed.FS, then communicates **exclusively** through JSON API (`/api/v2/*`) and WebSocket (`/ws`). Zero server-side rendering.

---

## JSON API v2

All endpoints return JSON. Convention: `Content-Type: application/json`, errors as `{"error": "message"}`, standard HTTP status codes.

### Board & Issues
| Method | Endpoint | Description |
|--------|----------|-------------|
| `GET` | `/api/v2/board` | Full board data: columns with cards, sprint info, status |
| `GET` | `/api/v2/issues/{id}` | Issue details + pipeline steps |
| `GET` | `/api/v2/issues/{id}/steps` | Pipeline steps with prompts/responses |

### Sprint
| Method | Endpoint | Description |
|--------|----------|-------------|
| `GET` | `/api/v2/sprint/status` | Sprint status (paused, processing, can_close, name) |
| `POST` | `/api/v2/sprint/start` | Start orchestrator |
| `POST` | `/api/v2/sprint/pause` | Pause orchestrator |
| `POST` | `/api/v2/sprint/close/preview` | Preview release notes (body: `{bump_type}`) |
| `POST` | `/api/v2/sprint/close/confirm` | Close sprint (body: `{bump_type, release_notes}`) |

### Ticket Actions
| Method | Endpoint | Description |
|--------|----------|-------------|
| `POST` | `/api/v2/issues/{id}/approve` | Approve ticket |
| `POST` | `/api/v2/issues/{id}/reject` | Reject -> backlog |
| `POST` | `/api/v2/issues/{id}/retry` | Retry (close PR, keep branch) |
| `POST` | `/api/v2/issues/{id}/retry-fresh` | Retry fresh (close PR, delete branch) |
| `POST` | `/api/v2/issues/{id}/approve-merge` | Approve + merge PR |
| `POST` | `/api/v2/issues/{id}/decline` | Decline with reason (body: `{reason}`) |
| `POST` | `/api/v2/issues/{id}/block` | Block ticket |
| `POST` | `/api/v2/issues/{id}/unblock` | Unblock ticket |
| `POST` | `/api/v2/issues/{id}/process` | Manually trigger processing |

### Workers
| Method | Endpoint | Description |
|--------|----------|-------------|
| `GET` | `/api/v2/workers` | Status of all workers |
| `POST` | `/api/v2/workers/toggle` | Pause/resume workers |

### Sync
| Method | Endpoint | Description |
|--------|----------|-------------|
| `POST` | `/api/v2/sync` | Trigger manual sync |

### Settings
| Method | Endpoint | Description |
|--------|----------|-------------|
| `GET` | `/api/v2/settings` | Current LLM config + available models |
| `PUT` | `/api/v2/settings` | Save LLM configuration |
| `POST` | `/api/v2/settings/yolo` | Toggle YOLO mode |

### Wizard
| Method | Endpoint | Description |
|--------|----------|-------------|
| `POST` | `/api/v2/wizard/sessions` | Create new session (body: `{type}`) |
| `GET` | `/api/v2/wizard/sessions/{id}` | Get session state |
| `DELETE` | `/api/v2/wizard/sessions/{id}` | Cancel session |
| `POST` | `/api/v2/wizard/sessions/{id}/refine` | Send idea to LLM (body: `{idea, language}`) |
| `POST` | `/api/v2/wizard/sessions/{id}/create` | Create issue on GitHub (body: `{title, add_to_sprint}`) |
| `GET` | `/api/v2/wizard/sessions/{id}/logs` | LLM interaction logs |

### Monitoring
| Method | Endpoint | Description |
|--------|----------|-------------|
| `GET` | `/api/v2/rate-limit` | GitHub API rate limit status |

### Streaming (SSE — unchanged)
| Method | Endpoint | Description |
|--------|----------|-------------|
| `GET` | `/api/v2/issues/{id}/stream` | Live LLM output (SSE proxy) |
| `GET` | `/api/v2/issues/{id}/logs/stream` | Log file tailing (SSE) |

---

## React SPA Structure

```
web/
├── src/
│   ├── main.tsx                    # Entry point, React Router setup
│   ├── App.tsx                     # Root layout (nav, footer, WebSocket provider)
│   ├── api/
│   │   ├── client.ts               # Fetch wrapper with error handling
│   │   ├── queries.ts              # TanStack Query hooks (useBoard, useWorkers, etc.)
│   │   └── types.ts                # TypeScript types matching Go structs
│   ├── hooks/
│   │   ├── useWebSocket.ts         # WebSocket connection + auto-reconnect
│   │   └── useSSE.ts               # SSE hook for streaming
│   ├── pages/
│   │   ├── BoardPage.tsx           # Kanban board
│   │   ├── TaskPage.tsx            # Task detail + live logs
│   │   ├── WizardPage.tsx          # Issue creation wizard
│   │   ├── SettingsPage.tsx        # LLM config
│   │   └── SprintClosePage.tsx     # Sprint close + release notes
│   ├── components/
│   │   ├── layout/
│   │   │   ├── Navbar.tsx
│   │   │   └── Footer.tsx
│   │   ├── board/
│   │   │   ├── Column.tsx
│   │   │   ├── TaskCard.tsx
│   │   │   └── ProcessingPanel.tsx
│   │   ├── task/
│   │   │   ├── StepList.tsx
│   │   │   ├── LogViewer.tsx
│   │   │   └── ActionButtons.tsx
│   │   ├── wizard/
│   │   │   ├── StepIndicator.tsx
│   │   │   ├── IdeaForm.tsx
│   │   │   ├── RefinePreview.tsx
│   │   │   └── CreateConfirm.tsx
│   │   ├── settings/
│   │   │   ├── ModelSelector.tsx
│   │   │   └── YoloToggle.tsx
│   │   └── common/
│   │       ├── Badge.tsx
│   │       ├── Button.tsx
│   │       └── Modal.tsx
│   └── lib/
│       ├── websocket.ts            # WebSocket client class
│       └── constants.ts            # Stage names, label mappings, etc.
├── dist/                           # Build output (3 files)
│   ├── index.html
│   └── assets/
│       ├── app-[hash].js
│       └── app-[hash].css
├── index.html                      # Vite entry HTML
├── package.json
├── vite.config.ts
├── tailwind.config.ts
├── tsconfig.json
└── tsconfig.app.json
```

---

## Real-time Updates — WebSocket + TanStack Query Integration

```typescript
// Strategy: WebSocket invalidates TanStack Query cache

function useWebSocketUpdates() {
  const queryClient = useQueryClient();
  
  useWebSocket('/ws', {
    onMessage: (msg) => {
      switch (msg.type) {
        case 'issue_update':
        case 'sync_complete':
          // Invalidate board query -> automatic refetch
          queryClient.invalidateQueries({ queryKey: ['board'] });
          break;
        case 'worker_update':
          // Optimistic update worker data (frequent, skip refetch)
          queryClient.setQueryData(['workers'], (old) => 
            updateWorker(old, msg.payload)
          );
          break;
        case 'can_close_sprint':
          queryClient.invalidateQueries({ queryKey: ['sprint'] });
          break;
        case 'log_stream':
          // Append to log buffer (streaming, not query-based)
          appendLog(msg.payload);
          break;
      }
    }
  });
}
```

**Key decision:** WebSocket does not replace API — it signals "data changed". TanStack Query does the refetch. This gives consistency (single source of truth = API response) and simplicity (zero manual state sync).

Exceptions: `worker_update` and `log_stream` — updated directly (optimistic update / append) because they arrive frequently and refetch would be too slow.

---

## Embed and Serving in Go

```go
// internal/dashboard/spa.go

//go:embed web/dist/*
var spaFS embed.FS

func (s *Server) serveSPA() {
    distFS, _ := fs.Sub(spaFS, "web/dist")
    fileServer := http.FileServer(http.FS(distFS))
    
    // Catch-all: unknown paths return index.html (SPA routing)
    s.mux.HandleFunc("GET /new/", func(w http.ResponseWriter, r *http.Request) {
        // Strip /new/ prefix for file lookup
        path := strings.TrimPrefix(r.URL.Path, "/new/")
        if path == "" {
            path = "index.html"
        }
        if _, err := fs.Stat(distFS, path); err == nil {
            // Serve static file
            r.URL.Path = "/" + path
            fileServer.ServeHTTP(w, r)
            return
        }
        // SPA fallback — return index.html for client-side routing
        index, _ := distFS.ReadFile("index.html")
        w.Header().Set("Content-Type", "text/html")
        w.Write(index)
    })
}
```

---

## Migration Strategy with Footer Toggle

### Phase 1: Coexistence
Both dashboards run simultaneously:
- `GET /` -> old HTMX dashboard (default)
- `GET /new/*` -> new React SPA

React Router configured with `basename="/new"`.

### Footer Toggle
**Old dashboard footer** gets a link:
```html
<a href="/new/">Try new dashboard</a>
```

**New React dashboard footer** gets a link:
```html
<a href="/">Back to classic dashboard</a>
```

Simple navigation link. No shared state needed — both dashboards read the same data from API/WebSocket.

### Phase 2: Swap Default
After all pages are implemented and tested:
- `GET /` -> React SPA (new default)
- `GET /legacy/*` -> old HTMX dashboard

### Phase 3: Remove Legacy
Remove old templates, HTMX handlers, CDN dependencies.

---

## Migration Order

1. Scaffold React app in `web/`, build API v2 endpoints (JSON wrappers over existing logic)
2. Implement React pages one by one: Board -> Task -> Settings -> Wizard -> Sprint Close
3. Connect WebSocket + SSE
4. Add footer toggle links to both dashboards
5. Swap default serving to SPA
6. Remove old templates, HTMX handlers, CDN dependencies
