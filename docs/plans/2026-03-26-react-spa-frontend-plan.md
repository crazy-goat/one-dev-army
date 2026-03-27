# React SPA Frontend — Implementation Plan

> **For Claude:** REQUIRED SUB-SKILL: Use superpowers:executing-plans to implement this plan task-by-task.

**Goal:** Replace the HTMX dashboard with a React SPA communicating via JSON API and WebSocket.

**Architecture:** React 19 SPA under `/new/*`, new `/api/v2/*` JSON endpoints, existing WebSocket/SSE reused.

**Tech Stack:** React 19, TypeScript, React Router v7, TanStack Query, Tailwind CSS v4, Vite

**Design doc:** `docs/plans/2026-03-26-react-spa-frontend-design.md`

---

## Phase 1: Foundation — Scaffold & API

### Task 1: Clean `web/` and scaffold React + Vite + Tailwind project

**Files:**
- Delete: `web/` (entire directory)
- Create: `web/package.json`
- Create: `web/vite.config.ts`
- Create: `web/tsconfig.json`
- Create: `web/tsconfig.app.json`
- Create: `web/tailwind.config.ts`
- Create: `web/index.html`
- Create: `web/src/main.tsx`
- Create: `web/src/App.tsx`
- Create: `web/src/index.css`
- Create: `web/.gitignore`

**Step 1: Delete old `web/` directory**

```bash
rm -rf web/
```

**Step 2: Create Vite React TypeScript project**

```bash
mkdir -p web/src
```

Create `web/package.json`:
```json
{
  "name": "oda-dashboard",
  "private": true,
  "version": "0.0.0",
  "type": "module",
  "scripts": {
    "dev": "vite",
    "build": "tsc -b && vite build",
    "preview": "vite preview",
    "lint": "eslint ."
  },
  "dependencies": {
    "react": "^19.0.0",
    "react-dom": "^19.0.0",
    "react-router": "^7.0.0",
    "@tanstack/react-query": "^5.0.0"
  },
  "devDependencies": {
    "@types/react": "^19.0.0",
    "@types/react-dom": "^19.0.0",
    "@vitejs/plugin-react": "^4.0.0",
    "typescript": "^5.7.0",
    "vite": "^6.0.0",
    "@tailwindcss/vite": "^4.0.0",
    "tailwindcss": "^4.0.0"
  }
}
```

Create `web/vite.config.ts`:
```typescript
import { defineConfig } from 'vite'
import react from '@vitejs/plugin-react'
import tailwindcss from '@tailwindcss/vite'

export default defineConfig({
  plugins: [react(), tailwindcss()],
  base: '/new/',
  build: {
    outDir: 'dist',
    rollupOptions: {
      output: {
        // Single JS and CSS bundle — no code splitting
        manualChunks: undefined,
        entryFileNames: 'assets/app.js',
        chunkFileNames: 'assets/app.js',
        assetFileNames: 'assets/app[extname]',
      },
    },
  },
  server: {
    proxy: {
      '/api': 'http://localhost:3000',
      '/ws': { target: 'ws://localhost:3000', ws: true },
    },
  },
})
```

Create `web/tsconfig.json`:
```json
{
  "files": [],
  "references": [{ "path": "./tsconfig.app.json" }]
}
```

Create `web/tsconfig.app.json`:
```json
{
  "compilerOptions": {
    "target": "ES2022",
    "lib": ["ES2023", "DOM", "DOM.Iterable"],
    "module": "ESNext",
    "skipLibCheck": true,
    "moduleResolution": "bundler",
    "allowImportingTsExtensions": true,
    "isolatedModules": true,
    "moduleDetection": "force",
    "noEmit": true,
    "jsx": "react-jsx",
    "strict": true,
    "noUnusedLocals": true,
    "noUnusedParameters": true,
    "noFallthroughCasesInSwitch": true,
    "noUncheckedIndexedAccess": true,
    "forceConsistentCasingInFileNames": true
  },
  "include": ["src"]
}
```

Create `web/index.html`:
```html
<!DOCTYPE html>
<html lang="en" class="dark">
  <head>
    <meta charset="UTF-8" />
    <meta name="viewport" content="width=device-width, initial-scale=1.0" />
    <title>ODA Dashboard</title>
  </head>
  <body class="bg-gray-950 text-gray-100 min-h-screen">
    <div id="root"></div>
    <script type="module" src="/src/main.tsx"></script>
  </body>
</html>
```

Create `web/src/index.css`:
```css
@import "tailwindcss";
```

Create `web/src/main.tsx`:
```tsx
import { StrictMode } from 'react'
import { createRoot } from 'react-dom/client'
import { BrowserRouter } from 'react-router'
import { QueryClient, QueryClientProvider } from '@tanstack/react-query'
import App from './App'
import './index.css'

const queryClient = new QueryClient({
  defaultOptions: {
    queries: {
      staleTime: 5_000,
      refetchOnWindowFocus: true,
      retry: 1,
    },
  },
})

createRoot(document.getElementById('root')!).render(
  <StrictMode>
    <QueryClientProvider client={queryClient}>
      <BrowserRouter basename="/new">
        <App />
      </BrowserRouter>
    </QueryClientProvider>
  </StrictMode>,
)
```

Create `web/src/App.tsx`:
```tsx
import { Routes, Route } from 'react-router'

function Placeholder({ name }: { name: string }) {
  return (
    <div className="flex items-center justify-center min-h-screen">
      <div className="text-center">
        <h1 className="text-3xl font-bold text-white mb-2">ODA Dashboard</h1>
        <p className="text-gray-400">{name} — coming soon</p>
        <a href="/" className="text-blue-400 hover:text-blue-300 mt-4 inline-block">
          ← Back to classic dashboard
        </a>
      </div>
    </div>
  )
}

export default function App() {
  return (
    <Routes>
      <Route path="/" element={<Placeholder name="Board" />} />
      <Route path="/task/:id" element={<Placeholder name="Task Detail" />} />
      <Route path="/wizard" element={<Placeholder name="Wizard" />} />
      <Route path="/settings" element={<Placeholder name="Settings" />} />
      <Route path="/sprint/close" element={<Placeholder name="Sprint Close" />} />
    </Routes>
  )
}
```

Create `web/.gitignore`:
```
node_modules/
```

**Step 3: Install dependencies and verify build**

```bash
cd web && npm install && npm run build
```
Expected: Build succeeds, `web/dist/` contains `index.html`, `assets/app.js`, `assets/app.css`

**Step 4: Commit**

```bash
git add web/
git commit -m "feat: scaffold React SPA with Vite, Tailwind, React Router, TanStack Query"
```

---

### Task 2: SPA serving from Go — embed and route under `/new/`

**Files:**
- Create: `internal/dashboard/spa.go`
- Modify: `internal/dashboard/server.go` — add SPA routes and embed directive
- Modify: `internal/dashboard/templates/layout.html` — add footer toggle link

**Step 1: Create `internal/dashboard/spa.go`**

```go
package dashboard

import (
	"embed"
	"io/fs"
	"net/http"
	"strings"
)

//go:embed all:web/dist
var spaFS embed.FS

// serveSPA registers routes to serve the React SPA under /new/.
// Static assets (JS, CSS) are served directly from embed.FS.
// All other paths return index.html for client-side routing.
func (s *Server) serveSPA() {
	distFS, err := fs.Sub(spaFS, "web/dist")
	if err != nil {
		return
	}

	fileServer := http.FileServer(http.FS(distFS))

	s.mux.HandleFunc("GET /new/", func(w http.ResponseWriter, r *http.Request) {
		// Strip /new/ prefix for file lookup
		path := strings.TrimPrefix(r.URL.Path, "/new/")
		if path == "" || path == "/" {
			path = "index.html"
		}

		// Try to serve static file
		if _, err := fs.Stat(distFS, path); err == nil {
			// Rewrite URL path for file server
			r2 := new(http.Request)
			*r2 = *r
			r2.URL = new(url.URL)
			*r2.URL = *r.URL
			r2.URL.Path = "/" + path
			fileServer.ServeHTTP(w, r2)
			return
		}

		// SPA fallback — return index.html for client-side routing
		indexBytes, err := fs.ReadFile(distFS, "index.html")
		if err != nil {
			http.Error(w, "index.html not found", http.StatusInternalServerError)
			return
		}
		w.Header().Set("Content-Type", "text/html; charset=utf-8")
		w.Write(indexBytes)
	})
}
```

Note: The embed directive path needs to be relative to the file location. Since `spa.go` is in `internal/dashboard/` but `web/dist/` is at the project root, we need to adjust. The embed will actually be in `server.go` or a file at the project root. The agent implementing this should verify the correct embed path — it may need to be in `main.go` and passed to the dashboard server, or use a build step to copy `web/dist/` into `internal/dashboard/web/dist/`.

**Alternative approach (recommended):** Put the embed in `main.go` and pass the `fs.FS` to the dashboard server constructor.

**Step 2: Add SPA route registration to `server.go`**

In `routes()` method, add at the end:
```go
// SPA routes (new React dashboard)
s.serveSPA()
```

**Step 3: Add footer toggle link to `layout.html`**

Find the footer section in `internal/dashboard/templates/layout.html` and add:
```html
<a href="/new/" style="color: #60a5fa; text-decoration: none; margin-left: 16px;">
  🚀 Try new dashboard
</a>
```

**Step 4: Build and verify**

```bash
cd web && npm run build
cd .. && go build ./...
```
Expected: Go binary compiles with embedded SPA files.

**Step 5: Run and verify**

```bash
go run . serve  # or however the app starts
# Visit http://localhost:3000/new/ — should see React placeholder
# Visit http://localhost:3000/ — should see old dashboard with footer link
```

**Step 6: Commit**

```bash
git add internal/dashboard/spa.go internal/dashboard/server.go internal/dashboard/templates/layout.html
git commit -m "feat: serve React SPA under /new/ with footer toggle link"
```

---

### Task 3: API v2 — Board endpoint

**Files:**
- Create: `internal/dashboard/api_v2.go`
- Create: `internal/dashboard/api_v2_test.go`
- Modify: `internal/dashboard/server.go` — register v2 routes

**Step 1: Write the failing test**

Create `internal/dashboard/api_v2_test.go`:
```go
package dashboard

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
)

func TestHandleBoardV2_ReturnsJSON(t *testing.T) {
	s := &Server{
		mux: http.NewServeMux(),
	}
	s.mux.HandleFunc("GET /api/v2/board", s.handleBoardV2)

	req := httptest.NewRequest(http.MethodGet, "/api/v2/board", nil)
	w := httptest.NewRecorder()
	s.mux.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d", w.Code)
	}

	ct := w.Header().Get("Content-Type")
	if ct != "application/json" {
		t.Fatalf("expected application/json, got %s", ct)
	}

	var result map[string]any
	if err := json.Unmarshal(w.Body.Bytes(), &result); err != nil {
		t.Fatalf("invalid JSON response: %v", err)
	}

	// Verify expected top-level keys
	for _, key := range []string{"sprint_name", "paused", "processing", "columns", "total_tickets"} {
		if _, ok := result[key]; !ok {
			t.Errorf("missing key %q in response", key)
		}
	}
}
```

**Step 2: Run test to verify it fails**

```bash
go test ./internal/dashboard/ -run TestHandleBoardV2 -v
```
Expected: FAIL — `handleBoardV2` not defined.

**Step 3: Write `internal/dashboard/api_v2.go`**

```go
package dashboard

import (
	"encoding/json"
	"fmt"
	"log"
	"net/http"
	"strconv"
)

// --- JSON response types for API v2 ---

type boardResponseV2 struct {
	SprintName     string              `json:"sprint_name"`
	Paused         bool                `json:"paused"`
	Processing     bool                `json:"processing"`
	CanCloseSprint bool                `json:"can_close_sprint"`
	CanPlanSprint  bool                `json:"can_plan_sprint"`
	YoloMode       bool                `json:"yolo_mode"`
	TotalTickets   int                 `json:"total_tickets"`
	WorkerCount    int                 `json:"worker_count"`
	CurrentTicket  *currentTicketV2    `json:"current_ticket,omitempty"`
	Columns        map[string][]cardV2 `json:"columns"`
}

type currentTicketV2 struct {
	Number   int    `json:"number"`
	Title    string `json:"title"`
	Status   string `json:"status"`
	Priority string `json:"priority,omitempty"`
	Type     string `json:"type,omitempty"`
	Size     string `json:"size,omitempty"`
}

type cardV2 struct {
	ID       int      `json:"id"`
	Title    string   `json:"title"`
	Status   string   `json:"status"`
	Worker   string   `json:"worker,omitempty"`
	Assignee string   `json:"assignee,omitempty"`
	Labels   []string `json:"labels"`
	PRURL    string   `json:"pr_url,omitempty"`
	IsMerged bool     `json:"is_merged"`
}

func (s *Server) handleBoardV2(w http.ResponseWriter, r *http.Request) {
	data := s.buildBoardData(r)

	resp := boardResponseV2{
		SprintName:     data.SprintName,
		Paused:         data.Paused,
		Processing:     data.Processing,
		CanCloseSprint: data.CanCloseSprint,
		CanPlanSprint:  data.CanPlanSprint,
		YoloMode:       data.YoloMode,
		TotalTickets:   data.TotalTickets,
		WorkerCount:    data.WorkerCount,
		Columns:        make(map[string][]cardV2),
	}

	if data.CurrentTicket != nil {
		resp.CurrentTicket = &currentTicketV2{
			Number:   data.CurrentTicket.Number,
			Title:    data.CurrentTicket.Title,
			Status:   data.CurrentTicket.Status,
			Priority: data.CurrentTicket.Priority,
			Type:     data.CurrentTicket.Type,
			Size:     data.CurrentTicket.Size,
		}
	}

	// Convert columns
	columnMap := map[string][]taskCard{
		"backlog":        data.Backlog,
		"blocked":        data.Blocked,
		"plan":           data.Plan,
		"code":           data.Code,
		"ai_review":      data.AIReview,
		"check_pipeline": data.CheckPipeline,
		"approve":        data.Approve,
		"merge":          data.Merge,
		"done":           data.Done,
		"failed":         data.Failed,
	}

	for colName, cards := range columnMap {
		v2Cards := make([]cardV2, len(cards))
		for i, c := range cards {
			v2Cards[i] = cardV2{
				ID:       c.ID,
				Title:    c.Title,
				Status:   c.Status,
				Worker:   c.Worker,
				Assignee: c.Assignee,
				Labels:   c.Labels,
				PRURL:    c.PRURL,
				IsMerged: c.IsMerged,
			}
		}
		resp.Columns[colName] = v2Cards
	}

	writeJSON(w, http.StatusOK, resp)
}

// writeJSON is a helper to write JSON responses with proper headers and error handling.
func writeJSON(w http.ResponseWriter, status int, v any) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	if err := json.NewEncoder(w).Encode(v); err != nil {
		log.Printf("[API v2] Error encoding JSON: %v", err)
	}
}

// writeError is a helper to write JSON error responses.
func writeError(w http.ResponseWriter, status int, msg string) {
	writeJSON(w, status, map[string]string{"error": msg})
}

// parseIssueID extracts and validates an issue number from the URL path.
func parseIssueID(r *http.Request) (int, error) {
	idStr := r.PathValue("id")
	n, err := strconv.Atoi(idStr)
	if err != nil || n <= 0 {
		return 0, fmt.Errorf("invalid issue ID: %q", idStr)
	}
	return n, nil
}
```

**Step 4: Register route in `server.go`**

Add to `routes()`:
```go
// API v2 routes (JSON API for React SPA)
s.mux.HandleFunc("GET /api/v2/board", s.handleBoardV2)
```

**Step 5: Run test to verify it passes**

```bash
go test ./internal/dashboard/ -run TestHandleBoardV2 -v
```
Expected: PASS

**Step 6: Commit**

```bash
git add internal/dashboard/api_v2.go internal/dashboard/api_v2_test.go internal/dashboard/server.go
git commit -m "feat: add /api/v2/board JSON endpoint for React SPA"
```

---

### Task 4: API v2 — Issue detail, sprint, workers, sync, rate-limit endpoints

**Files:**
- Modify: `internal/dashboard/api_v2.go` — add remaining read endpoints
- Modify: `internal/dashboard/api_v2_test.go` — add tests
- Modify: `internal/dashboard/server.go` — register routes

**Step 1: Write failing tests for each endpoint**

Add to `api_v2_test.go`:
- `TestHandleIssueDetailV2_ReturnsJSON`
- `TestHandleSprintStatusV2_ReturnsJSON`
- `TestHandleWorkersV2_ReturnsJSON`
- `TestHandleRateLimitV2_ReturnsJSON`

**Step 2: Run tests to verify they fail**

```bash
go test ./internal/dashboard/ -run TestHandle.*V2 -v
```

**Step 3: Implement handlers in `api_v2.go`**

Add the following handlers:

```go
// GET /api/v2/issues/{id}
func (s *Server) handleIssueDetailV2(w http.ResponseWriter, r *http.Request) { ... }

// GET /api/v2/issues/{id}/steps
func (s *Server) handleIssueStepsV2(w http.ResponseWriter, r *http.Request) { ... }

// GET /api/v2/sprint/status
func (s *Server) handleSprintStatusV2(w http.ResponseWriter, r *http.Request) { ... }

// GET /api/v2/workers
func (s *Server) handleWorkersV2(w http.ResponseWriter, r *http.Request) { ... }

// GET /api/v2/rate-limit
func (s *Server) handleRateLimitV2(w http.ResponseWriter, r *http.Request) { ... }

// GET /api/v2/settings
func (s *Server) handleSettingsV2(w http.ResponseWriter, r *http.Request) { ... }
```

Each handler should:
1. Reuse existing server methods to get data (e.g., `s.buildBoardData()`, `s.store.GetSteps()`)
2. Convert to JSON-serializable response types
3. Call `writeJSON()` or `writeError()`

**Step 4: Register routes in `server.go`**

```go
s.mux.HandleFunc("GET /api/v2/issues/{id}", s.handleIssueDetailV2)
s.mux.HandleFunc("GET /api/v2/issues/{id}/steps", s.handleIssueStepsV2)
s.mux.HandleFunc("GET /api/v2/sprint/status", s.handleSprintStatusV2)
s.mux.HandleFunc("GET /api/v2/workers", s.handleWorkersV2)
s.mux.HandleFunc("GET /api/v2/rate-limit", s.handleRateLimitV2)
s.mux.HandleFunc("GET /api/v2/settings", s.handleSettingsV2)
```

**Step 5: Run tests**

```bash
go test ./internal/dashboard/ -run TestHandle.*V2 -v
```

**Step 6: Commit**

```bash
git add internal/dashboard/api_v2.go internal/dashboard/api_v2_test.go internal/dashboard/server.go
git commit -m "feat: add /api/v2/ read endpoints (issues, sprint, workers, settings, rate-limit)"
```

---

### Task 5: API v2 — Ticket action endpoints (approve, reject, retry, block, etc.)

**Files:**
- Modify: `internal/dashboard/api_v2.go` — add action endpoints
- Modify: `internal/dashboard/api_v2_test.go` — add tests
- Modify: `internal/dashboard/server.go` — register routes

**Step 1: Write failing tests**

Test each action endpoint returns JSON (not redirect):
- `TestHandleApproveV2_ReturnsJSON`
- `TestHandleRejectV2_ReturnsJSON`
- etc.

**Step 2: Implement action handlers**

Each action handler follows the same pattern:
1. Parse issue ID from path
2. Validate preconditions (orchestrator exists, etc.)
3. Execute the action (reuse existing logic from old handlers)
4. Return JSON `{"success": true}` or `{"error": "message"}`

```go
// POST /api/v2/issues/{id}/approve
func (s *Server) handleApproveV2(w http.ResponseWriter, r *http.Request) {
	issueNum, err := parseIssueID(r)
	if err != nil {
		writeError(w, http.StatusBadRequest, err.Error())
		return
	}
	if s.orchestrator == nil {
		writeError(w, http.StatusServiceUnavailable, "orchestrator not configured")
		return
	}
	if err := s.orchestrator.ChangeStage(issueNum, github.StageApprove, github.ReasonManualApprove); err != nil {
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"success": true})
}
```

Implement similarly for: reject, retry, retry-fresh, approve-merge, decline, block, unblock, process.

**Step 3: Register routes**

```go
s.mux.HandleFunc("POST /api/v2/issues/{id}/approve", s.handleApproveV2)
s.mux.HandleFunc("POST /api/v2/issues/{id}/reject", s.handleRejectV2)
s.mux.HandleFunc("POST /api/v2/issues/{id}/retry", s.handleRetryV2)
s.mux.HandleFunc("POST /api/v2/issues/{id}/retry-fresh", s.handleRetryFreshV2)
s.mux.HandleFunc("POST /api/v2/issues/{id}/approve-merge", s.handleApproveMergeV2)
s.mux.HandleFunc("POST /api/v2/issues/{id}/decline", s.handleDeclineV2)
s.mux.HandleFunc("POST /api/v2/issues/{id}/block", s.handleBlockV2)
s.mux.HandleFunc("POST /api/v2/issues/{id}/unblock", s.handleUnblockV2)
s.mux.HandleFunc("POST /api/v2/issues/{id}/process", s.handleProcessV2)
```

**Step 4: Run tests and lint**

```bash
go test ./internal/dashboard/ -run TestHandle.*V2 -v
golangci-lint run ./internal/dashboard/
```

**Step 5: Commit**

```bash
git commit -am "feat: add /api/v2/ ticket action endpoints (approve, reject, retry, block, etc.)"
```

---

### Task 6: API v2 — Sprint actions, settings, sync, YOLO toggle

**Files:**
- Modify: `internal/dashboard/api_v2.go`
- Modify: `internal/dashboard/api_v2_test.go`
- Modify: `internal/dashboard/server.go`

**Step 1: Implement handlers**

```go
// POST /api/v2/sprint/start
func (s *Server) handleSprintStartV2(w http.ResponseWriter, r *http.Request) { ... }

// POST /api/v2/sprint/pause
func (s *Server) handleSprintPauseV2(w http.ResponseWriter, r *http.Request) { ... }

// POST /api/v2/sprint/close/preview
func (s *Server) handleSprintClosePreviewV2(w http.ResponseWriter, r *http.Request) { ... }

// POST /api/v2/sprint/close/confirm
func (s *Server) handleSprintCloseConfirmV2(w http.ResponseWriter, r *http.Request) { ... }

// PUT /api/v2/settings
func (s *Server) handleSaveSettingsV2(w http.ResponseWriter, r *http.Request) { ... }

// POST /api/v2/settings/yolo
func (s *Server) handleYoloToggleV2(w http.ResponseWriter, r *http.Request) { ... }

// POST /api/v2/sync
func (s *Server) handleSyncV2(w http.ResponseWriter, r *http.Request) { ... }

// POST /api/v2/workers/toggle
func (s *Server) handleWorkerToggleV2(w http.ResponseWriter, r *http.Request) { ... }
```

Key difference from old handlers: these read JSON body (not form data) and return JSON (not redirect).

**Step 2: Register routes and test**

**Step 3: Commit**

```bash
git commit -am "feat: add /api/v2/ sprint, settings, sync, worker toggle endpoints"
```

---

### Task 7: API v2 — Wizard endpoints

**Files:**
- Modify: `internal/dashboard/api_v2.go`
- Modify: `internal/dashboard/api_v2_test.go`
- Modify: `internal/dashboard/server.go`

**Step 1: Implement wizard handlers**

```go
// POST /api/v2/wizard/sessions — create new wizard session
func (s *Server) handleWizardCreateSessionV2(w http.ResponseWriter, r *http.Request) { ... }

// GET /api/v2/wizard/sessions/{id} — get session state
func (s *Server) handleWizardGetSessionV2(w http.ResponseWriter, r *http.Request) { ... }

// DELETE /api/v2/wizard/sessions/{id} — cancel session
func (s *Server) handleWizardDeleteSessionV2(w http.ResponseWriter, r *http.Request) { ... }

// POST /api/v2/wizard/sessions/{id}/refine — send idea to LLM
func (s *Server) handleWizardRefineV2(w http.ResponseWriter, r *http.Request) { ... }

// POST /api/v2/wizard/sessions/{id}/create — create GitHub issue
func (s *Server) handleWizardCreateIssueV2(w http.ResponseWriter, r *http.Request) { ... }

// GET /api/v2/wizard/sessions/{id}/logs — get LLM logs
func (s *Server) handleWizardLogsV2(w http.ResponseWriter, r *http.Request) { ... }
```

These reuse existing `WizardSessionStore` and wizard logic but accept/return JSON.

**Step 2: Register routes**

```go
s.mux.HandleFunc("POST /api/v2/wizard/sessions", s.handleWizardCreateSessionV2)
s.mux.HandleFunc("GET /api/v2/wizard/sessions/{id}", s.handleWizardGetSessionV2)
s.mux.HandleFunc("DELETE /api/v2/wizard/sessions/{id}", s.handleWizardDeleteSessionV2)
s.mux.HandleFunc("POST /api/v2/wizard/sessions/{id}/refine", s.handleWizardRefineV2)
s.mux.HandleFunc("POST /api/v2/wizard/sessions/{id}/create", s.handleWizardCreateIssueV2)
s.mux.HandleFunc("GET /api/v2/wizard/sessions/{id}/logs", s.handleWizardLogsV2)
```

**Step 3: Test and commit**

```bash
go test ./internal/dashboard/ -run TestWizard.*V2 -v
git commit -am "feat: add /api/v2/ wizard endpoints (sessions, refine, create, logs)"
```

---

### Task 8: API v2 — SSE streaming endpoints

**Files:**
- Modify: `internal/dashboard/api_v2.go`
- Modify: `internal/dashboard/server.go`

**Step 1: Implement SSE handlers**

These are thin wrappers around existing SSE logic:

```go
// GET /api/v2/issues/{id}/stream — live LLM output (SSE)
func (s *Server) handleTaskStreamV2(w http.ResponseWriter, r *http.Request) {
	// Reuse existing handleTaskStream logic
	s.handleTaskStream(w, r)
}

// GET /api/v2/issues/{id}/logs/stream — log file tailing (SSE)
func (s *Server) handleLogStreamV2(w http.ResponseWriter, r *http.Request) {
	// Reuse existing handleLogStream logic, mapping {id} to {issue}
	s.handleLogStream(w, r)
}
```

Note: The path parameter name changes from `{issue}` to `{id}` for consistency. The handler needs to adapt.

**Step 2: Register routes**

```go
s.mux.HandleFunc("GET /api/v2/issues/{id}/stream", s.handleTaskStreamV2)
s.mux.HandleFunc("GET /api/v2/issues/{id}/logs/stream", s.handleLogStreamV2)
```

**Step 3: Commit**

```bash
git commit -am "feat: add /api/v2/ SSE streaming endpoints for task output and logs"
```

---

## Phase 2: React Pages

### Task 9: TypeScript types and API client

**Files:**
- Create: `web/src/api/types.ts`
- Create: `web/src/api/client.ts`
- Create: `web/src/api/queries.ts`

**Step 1: Create TypeScript types matching Go structs**

`web/src/api/types.ts` — define interfaces for all API responses:
- `Board`, `Card`, `CurrentTicket`
- `IssueDetail`, `TaskStep`
- `SprintStatus`
- `WorkerStatus`
- `Settings`, `ProviderModel`
- `WizardSession`, `CreatedIssue`
- `RateLimit`
- WebSocket message types: `WSMessage`, `IssueUpdatePayload`, `WorkerUpdatePayload`, etc.

**Step 2: Create fetch wrapper**

`web/src/api/client.ts` — typed fetch wrapper with error handling:
```typescript
const BASE = '/api/v2'

async function request<T>(path: string, options?: RequestInit): Promise<T> {
  const res = await fetch(`${BASE}${path}`, {
    headers: { 'Content-Type': 'application/json', ...options?.headers },
    ...options,
  })
  if (!res.ok) {
    const body = await res.json().catch(() => ({ error: res.statusText }))
    throw new Error(body.error || res.statusText)
  }
  return res.json()
}

export const api = {
  getBoard: () => request<Board>('/board'),
  getIssue: (id: number) => request<IssueDetail>(`/issues/${id}`),
  getIssueSteps: (id: number) => request<TaskStep[]>(`/issues/${id}/steps`),
  getSprintStatus: () => request<SprintStatus>('/sprint/status'),
  getWorkers: () => request<WorkerStatus[]>('/workers'),
  getSettings: () => request<Settings>('/settings'),
  getRateLimit: () => request<RateLimit>('/rate-limit'),
  // ... actions
  approveIssue: (id: number) => request<{success: boolean}>(`/issues/${id}/approve`, { method: 'POST' }),
  // ... etc
}
```

**Step 3: Create TanStack Query hooks**

`web/src/api/queries.ts`:
```typescript
import { useQuery, useMutation, useQueryClient } from '@tanstack/react-query'
import { api } from './client'

export function useBoard() {
  return useQuery({ queryKey: ['board'], queryFn: api.getBoard, refetchInterval: 30_000 })
}

export function useIssue(id: number) {
  return useQuery({ queryKey: ['issue', id], queryFn: () => api.getIssue(id) })
}

// ... etc
```

**Step 4: Build and verify types compile**

```bash
cd web && npm run build
```

**Step 5: Commit**

```bash
git commit -am "feat: add TypeScript types, API client, and TanStack Query hooks"
```

---

### Task 10: WebSocket hook

**Files:**
- Create: `web/src/lib/websocket.ts`
- Create: `web/src/hooks/useWebSocket.ts`

**Step 1: Create WebSocket client class**

`web/src/lib/websocket.ts` — reconnecting WebSocket with exponential backoff:
- Auto-reconnect with 1s → 30s backoff
- Ping/pong keep-alive
- Message type parsing
- Event callbacks

**Step 2: Create React hook**

`web/src/hooks/useWebSocket.ts`:
```typescript
export function useWebSocketUpdates() {
  const queryClient = useQueryClient()
  
  useEffect(() => {
    const ws = new OdaWebSocket(wsUrl)
    ws.onMessage((msg) => {
      switch (msg.type) {
        case 'issue_update':
        case 'sync_complete':
          queryClient.invalidateQueries({ queryKey: ['board'] })
          break
        case 'worker_update':
          queryClient.setQueryData(['workers'], ...)
          break
        case 'can_close_sprint':
          queryClient.invalidateQueries({ queryKey: ['sprint'] })
          break
      }
    })
    return () => ws.close()
  }, [queryClient])
}
```

**Step 3: Commit**

```bash
git commit -am "feat: add WebSocket client with auto-reconnect and TanStack Query integration"
```

---

### Task 11: Layout components (Navbar, Footer)

**Files:**
- Create: `web/src/components/layout/Navbar.tsx`
- Create: `web/src/components/layout/Footer.tsx`
- Modify: `web/src/App.tsx` — wrap routes in layout

**Step 1: Create Navbar**

Port the navigation from `layout.html` — links to Board, Wizard, Settings. Show sprint name, worker count, pause/resume button.

**Step 2: Create Footer**

Include "← Back to classic dashboard" link and rate limit display.

**Step 3: Update App.tsx**

Wrap routes in layout with Navbar + Footer. Add `useWebSocketUpdates()` at the App level.

**Step 4: Build and verify**

```bash
cd web && npm run build
```

**Step 5: Commit**

```bash
git commit -am "feat: add Navbar and Footer layout components with classic dashboard toggle"
```

---

### Task 12: Board page

**Files:**
- Create: `web/src/pages/BoardPage.tsx`
- Create: `web/src/components/board/Column.tsx`
- Create: `web/src/components/board/TaskCard.tsx`
- Create: `web/src/components/board/ProcessingPanel.tsx`
- Modify: `web/src/App.tsx` — replace placeholder

**Step 1: Implement BoardPage**

Uses `useBoard()` query. Renders 10 columns (Backlog, Blocked, Plan, Code, AI Review, Pipeline, Approve, Merge, Done, Failed). Each column shows cards with title, labels (emoji badges), assignee, PR link.

**Step 2: Implement Column component**

Renders column header with count badge and list of TaskCards.

**Step 3: Implement TaskCard component**

Shows issue number, title, label badges (priority, type, size emojis), assignee. Clickable — navigates to `/task/{id}`.

Action buttons per column:
- Approve column: Approve+Merge, Decline buttons
- Failed column: Retry, Retry Fresh buttons
- Blocked column: Unblock button
- Backlog column: Process button

**Step 4: Implement ProcessingPanel**

Shows current ticket being processed, worker status, elapsed time. Uses `useWorkers()` query.

**Step 5: Build and verify**

```bash
cd web && npm run build
```

**Step 6: Commit**

```bash
git commit -am "feat: implement Board page with Kanban columns, task cards, and processing panel"
```

---

### Task 13: Task detail page

**Files:**
- Create: `web/src/pages/TaskPage.tsx`
- Create: `web/src/components/task/StepList.tsx`
- Create: `web/src/components/task/LogViewer.tsx`
- Create: `web/src/components/task/ActionButtons.tsx`
- Create: `web/src/hooks/useSSE.ts`

**Step 1: Implement TaskPage**

Uses `useIssue(id)` and `useIssueSteps(id)` queries. Shows issue title, status, pipeline steps with expand/collapse for prompt/response content.

**Step 2: Implement StepList**

Renders pipeline steps (analysis, planning, coding, etc.) with timestamps, duration, expand/collapse for prompt and response text.

**Step 3: Implement LogViewer**

Connects to SSE endpoint `/api/v2/issues/{id}/stream` for live LLM output. Shows streaming text with auto-scroll. Falls back to WebSocket `log_stream` messages.

**Step 4: Implement useSSE hook**

```typescript
export function useSSE(url: string, onEvent: (data: any) => void) {
  useEffect(() => {
    const source = new EventSource(url)
    source.onmessage = (e) => onEvent(JSON.parse(e.data))
    return () => source.close()
  }, [url])
}
```

**Step 5: Implement ActionButtons**

Context-aware action buttons based on issue status (same as board card actions).

**Step 6: Build and commit**

```bash
cd web && npm run build
git commit -am "feat: implement Task detail page with pipeline steps, live log viewer, and SSE streaming"
```

---

### Task 14: Settings page

**Files:**
- Create: `web/src/pages/SettingsPage.tsx`
- Create: `web/src/components/settings/ModelSelector.tsx`
- Create: `web/src/components/settings/YoloToggle.tsx`

**Step 1: Implement SettingsPage**

Uses `useSettings()` query. Shows 5 model selectors (Setup, Planning, Orchestration, Code, CodeHeavy) with autocomplete dropdown from available models list. YOLO mode toggle. Sprint auto-start toggle. Save button.

**Step 2: Implement ModelSelector**

Dropdown/combobox with search. Shows model name and provider. Selected model highlighted.

**Step 3: Implement YoloToggle**

Toggle switch with confirmation dialog ("Are you sure? YOLO mode skips human approval").

**Step 4: Build and commit**

```bash
cd web && npm run build
git commit -am "feat: implement Settings page with model selectors and YOLO toggle"
```

---

### Task 15: Wizard page

**Files:**
- Create: `web/src/pages/WizardPage.tsx`
- Create: `web/src/components/wizard/StepIndicator.tsx`
- Create: `web/src/components/wizard/IdeaForm.tsx`
- Create: `web/src/components/wizard/RefinePreview.tsx`
- Create: `web/src/components/wizard/CreateConfirm.tsx`

**Step 1: Implement WizardPage**

Multi-step wizard flow:
1. **Step 1 — Idea:** Type selector (Feature/Bug), text area for idea, language selector, "Add to sprint" checkbox
2. **Step 2 — Review:** Shows LLM-generated title (editable), description (markdown preview), priority/complexity badges
3. **Step 3 — Confirm:** Shows created GitHub issue with link

Uses wizard API endpoints. Manages wizard state locally with `useState`.

**Step 2: Implement each step component**

**Step 3: Build and commit**

```bash
cd web && npm run build
git commit -am "feat: implement Wizard page with 3-step issue creation flow"
```

---

### Task 16: Sprint close page

**Files:**
- Create: `web/src/pages/SprintClosePage.tsx`

**Step 1: Implement SprintClosePage**

- Version bump selector (Major/Minor/Patch radio buttons)
- Current version → New version display
- Release notes preview (fetched from `/api/v2/sprint/close/preview`)
- Editable title and description
- Confirm button

**Step 2: Build and commit**

```bash
cd web && npm run build
git commit -am "feat: implement Sprint Close page with version bump and release notes preview"
```

---

## Phase 3: Integration & Polish

### Task 17: Full build pipeline — Go embeds React dist

**Files:**
- Modify: `Makefile` or build script — add `cd web && npm run build` before `go build`
- Verify: `go:embed` picks up `web/dist/*` correctly
- Test: full binary serves SPA at `/new/`

**Step 1: Update build process**

Ensure `web/dist/` is built before `go build`. Add to Makefile:
```makefile
.PHONY: build-web
build-web:
	cd web && npm ci && npm run build

.PHONY: build
build: build-web
	go build -o oda .
```

**Step 2: Verify end-to-end**

```bash
make build
./oda serve
# Visit /new/ — full React SPA
# Visit / — old dashboard with footer link
```

**Step 3: Commit**

```bash
git commit -am "feat: integrate React build into Go build pipeline with embed"
```

---

### Task 18: Run full test suite and lint

**Files:**
- All modified files

**Step 1: Run Go tests**

```bash
go test -race ./...
```

**Step 2: Run Go linter**

```bash
golangci-lint run ./...
```

**Step 3: Run frontend build**

```bash
cd web && npm run build
```

**Step 4: Fix any issues and commit**

```bash
git commit -am "fix: resolve lint and test issues from SPA integration"
```

---

### Task 19: Push branch

**Step 1: Push feature branch**

```bash
git push -u origin feat/react-spa-dashboard
```
