# New React SPA Dashboard

## Overview

The ODA dashboard has been rewritten as a React SPA that communicates with the Go backend exclusively via JSON API (`/api/v2/*`) and WebSocket (`/ws`). The frontend is fully separated from the backend — no server-side rendering, no HTML templates.

The SPA is embedded in the Go binary via `go:embed` and served under `/new/`. The old HTMX dashboard remains at `/` during the migration period. A footer toggle link allows switching between the two.

## Tech Stack

- **React 19** + TypeScript (strict mode)
- **React Router v7** — client-side routing, `basename="/new"`
- **TanStack Query v5** — server state management, caching, auto-refetch
- **Tailwind CSS v4** — utility-first styling, dark theme
- **Vite 6** — bundler, dev server with proxy to Go backend

## Build Output

Three files embedded in the Go binary:

```
web/dist/
├── index.html        (0.4 KB)
└── assets/
    ├── app.js         (~330 KB)
    └── app.css        (~34 KB)
```

No code splitting — single bundle for simplicity of embedding.

## URLs

| URL | What |
|-----|------|
| `/` | Old HTMX dashboard (default, will be swapped later) |
| `/new/` | New React SPA |
| `/new/task/:id` | Task detail page |
| `/new/wizard` | Issue creation wizard |
| `/new/settings` | LLM model configuration |
| `/new/sprint/close` | Sprint close with version bump |
| `/api/v2/*` | JSON API (33 endpoints) |
| `/ws` | WebSocket (unchanged, shared by both dashboards) |

## API Endpoints (`/api/v2/`)

### Board & Issues
- `GET /api/v2/board` — full board data (columns, sprint info, status)
- `GET /api/v2/issues/{id}` — issue detail with pipeline steps
- `GET /api/v2/issues/{id}/steps` — pipeline steps only

### Sprint
- `GET /api/v2/sprint/status` — paused/processing/current issue
- `POST /api/v2/sprint/start` — start orchestrator
- `POST /api/v2/sprint/pause` — pause orchestrator
- `POST /api/v2/sprint/close/preview` — preview release notes
- `POST /api/v2/sprint/close/confirm` — close sprint, create tag/release

### Ticket Actions
- `POST /api/v2/issues/{id}/approve`
- `POST /api/v2/issues/{id}/reject`
- `POST /api/v2/issues/{id}/retry`
- `POST /api/v2/issues/{id}/retry-fresh`
- `POST /api/v2/issues/{id}/approve-merge`
- `POST /api/v2/issues/{id}/decline` — body: `{reason}`
- `POST /api/v2/issues/{id}/block`
- `POST /api/v2/issues/{id}/unblock`
- `POST /api/v2/issues/{id}/process`

### Workers
- `GET /api/v2/workers` — all worker statuses
- `POST /api/v2/workers/toggle` — pause/resume

### Settings
- `GET /api/v2/settings` — LLM config + available models
- `PUT /api/v2/settings` — save LLM config
- `POST /api/v2/settings/yolo` — toggle YOLO mode

### Sync & Monitoring
- `POST /api/v2/sync` — trigger GitHub sync
- `GET /api/v2/rate-limit` — GitHub API rate limit

### Wizard
- `POST /api/v2/wizard/sessions` — create session
- `GET /api/v2/wizard/sessions/{id}` — get session state
- `DELETE /api/v2/wizard/sessions/{id}` — cancel session
- `POST /api/v2/wizard/sessions/{id}/refine` — send idea to LLM
- `POST /api/v2/wizard/sessions/{id}/create` — create GitHub issue
- `GET /api/v2/wizard/sessions/{id}/logs` — LLM interaction logs

### Streaming (SSE)
- `GET /api/v2/issues/{id}/stream` — live LLM output
- `GET /api/v2/issues/{id}/logs/stream` — log file tailing

## Real-time Updates

The SPA uses two real-time channels:

1. **WebSocket (`/ws`)** — receives `issue_update`, `sync_complete`, `worker_update`, `can_close_sprint`, `log_stream` messages. On receipt, TanStack Query caches are invalidated to trigger automatic refetch.

2. **SSE** — used on the Task Detail page for live LLM output streaming. Connected via `EventSource` to `/api/v2/issues/{id}/stream`.

## Frontend Structure

```
web/src/
├── main.tsx                          # Entry point
├── App.tsx                           # Routes + layout + WebSocket
├── api/
│   ├── types.ts                      # TypeScript interfaces (30 types)
│   ├── client.ts                     # Fetch wrapper (33 methods)
│   └── queries.ts                    # TanStack Query hooks (23 hooks)
├── hooks/
│   ├── useWebSocket.ts               # WebSocket + query invalidation
│   └── useSSE.ts                     # Server-Sent Events hook
├── lib/
│   └── websocket.ts                  # Reconnecting WebSocket client
├── pages/
│   ├── BoardPage.tsx                 # Kanban board
│   ├── TaskPage.tsx                  # Task detail + live logs
│   ├── SettingsPage.tsx              # LLM model config
│   ├── WizardPage.tsx                # 3-step issue creation
│   └── SprintClosePage.tsx           # Version bump + release notes
└── components/
    ├── layout/
    │   ├── Navbar.tsx                # Top navigation bar
    │   └── Footer.tsx                # Footer with classic dashboard link
    ├── board/
    │   ├── Column.tsx                # Kanban column
    │   ├── TaskCard.tsx              # Issue card with actions
    │   └── ProcessingPanel.tsx       # Current task display
    ├── task/
    │   ├── StepList.tsx              # Pipeline steps (expandable)
    │   ├── LogViewer.tsx             # Live SSE log viewer
    │   └── ActionButtons.tsx         # Context-aware actions
    ├── wizard/
    │   ├── StepIndicator.tsx         # Step progress (1/2/3)
    │   ├── IdeaForm.tsx              # Type + idea input
    │   ├── RefinePreview.tsx         # LLM result review
    │   └── CreateConfirm.tsx         # Created issue confirmation
    └── settings/
        ├── ModelSelector.tsx         # Searchable model dropdown
        └── YoloToggle.tsx            # Toggle switch
```

## Go-side Embedding

In `main.go`:
```go
//go:embed all:web/dist
var spaDistFS embed.FS
```

The `fs.FS` is passed to `dashboard.NewServer()` which registers a handler for `GET /new/{path...}`. Static files are served directly; all other paths return `index.html` for client-side routing.

## Development

```bash
# Frontend dev server (hot reload, proxies API to Go backend)
cd web && npm run dev

# Build for embedding
cd web && npm run build

# Full build (Go + embedded SPA)
go build ./...
```

The Vite dev server proxies `/api` and `/ws` to `http://localhost:3000` (the Go backend).

## Migration Plan

1. **Current state:** Old dashboard at `/`, new SPA at `/new/`, footer toggle to switch
2. **Next:** After testing, swap defaults — SPA at `/`, old at `/legacy/`
3. **Final:** Remove old HTMX templates, handlers, and CDN dependencies
