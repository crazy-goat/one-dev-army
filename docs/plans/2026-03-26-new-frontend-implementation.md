# New Frontend Implementation Plan

> **For Claude:** REQUIRED SUB-SKILL: Use superpowers:executing-plans to implement this plan task-by-task.

**Goal:** Migrate from HTMX dashboard to React SPA with REST API, maintaining both versions during transition.

**Architecture:** Feature flag controls routing between old (HTMX) and new (React) dashboards. New API at `/api/v2/*`. React app embedded in Go binary via `embed.FS`.

**Tech Stack:** React 18 + TypeScript + Vite + Zustand + SWR + React Router v6

---

## Phase 1: Setup (Day 1)

### Task 1: Create web directory structure

**Files:**
- Create: `web/package.json`
- Create: `web/vite.config.ts`
- Create: `web/tsconfig.json`
- Create: `web/tsconfig.node.json`
- Create: `web/.gitignore`

**Step 1: Create package.json**

```json
{
  "name": "oda-dashboard",
  "private": true,
  "version": "0.0.0",
  "type": "module",
  "scripts": {
    "dev": "vite",
    "build": "tsc && vite build",
    "preview": "vite preview",
    "lint": "eslint . --ext ts,tsx --report-unused-disable-directives --max-warnings 0",
    "typecheck": "tsc --noEmit"
  },
  "dependencies": {
    "react": "^18.2.0",
    "react-dom": "^18.2.0",
    "react-router-dom": "^6.20.0",
    "swr": "^2.2.4",
    "zustand": "^4.4.7"
  },
  "devDependencies": {
    "@types/react": "^18.2.43",
    "@types/react-dom": "^18.2.17",
    "@typescript-eslint/eslint-plugin": "^6.14.0",
    "@typescript-eslint/parser": "^6.14.0",
    "@vitejs/plugin-react": "^4.2.1",
    "eslint": "^8.55.0",
    "eslint-plugin-react-hooks": "^4.6.0",
    "eslint-plugin-react-refresh": "^0.4.5",
    "typescript": "^5.2.2",
    "vite": "^5.0.8"
  }
}
```

**Step 2: Create vite.config.ts**

```typescript
import { defineConfig } from 'vite'
import react from '@vitejs/plugin-react'

export default defineConfig({
  plugins: [react()],
  build: {
    outDir: 'dist',
    emptyOutDir: true,
    rollupOptions: {
      output: {
        manualChunks: undefined,
        entryFileNames: 'assets/[name].js',
        chunkFileNames: 'assets/[name].js',
        assetFileNames: 'assets/[name].[ext]'
      }
    }
  },
  server: {
    port: 5173,
    proxy: {
      '/api': 'http://localhost:8080',
      '/ws': {
        target: 'ws://localhost:8080',
        ws: true
      }
    }
  }
})
```

**Step 3: Create tsconfig.json**

```json
{
  "compilerOptions": {
    "target": "ES2020",
    "useDefineForClassFields": true,
    "lib": ["ES2020", "DOM", "DOM.Iterable"],
    "module": "ESNext",
    "skipLibCheck": true,
    "moduleResolution": "bundler",
    "allowImportingTsExtensions": true,
    "resolveJsonModule": true,
    "isolatedModules": true,
    "noEmit": true,
    "jsx": "react-jsx",
    "strict": true,
    "noUnusedLocals": true,
    "noUnusedParameters": true,
    "noFallthroughCasesInSwitch": true,
    "baseUrl": ".",
    "paths": {
      "@/*": ["src/*"]
    }
  },
  "include": ["src"],
  "references": [{ "path": "./tsconfig.node.json" }]
}
```

**Step 4: Create tsconfig.node.json**

```json
{
  "compilerOptions": {
    "composite": true,
    "skipLibCheck": true,
    "module": "ESNext",
    "moduleResolution": "bundler",
    "allowSyntheticDefaultImports": true
  },
  "include": ["vite.config.ts"]
}
```

**Step 5: Create .gitignore**

```
node_modules
dist
*.local
```

**Step 6: Commit**

```bash
git add web/
git commit -m "chore: setup React project structure"
```

---

### Task 2: Create React app skeleton

**Files:**
- Create: `web/src/main.tsx`
- Create: `web/src/App.tsx`
- Create: `web/src/index.css`
- Create: `web/index.html`

**Step 1: Create main.tsx**

```typescript
import React from 'react'
import ReactDOM from 'react-dom/client'
import App from './App'
import './index.css'

ReactDOM.createRoot(document.getElementById('root')!).render(
  <React.StrictMode>
    <App />
  </React.StrictMode>,
)
```

**Step 2: Create App.tsx**

```typescript
import { BrowserRouter, Routes, Route } from 'react-router-dom'

function App() {
  return (
    <BrowserRouter>
      <Routes>
        <Route path="/" element={<div>ODA Dashboard - Coming Soon</div>} />
      </Routes>
    </BrowserRouter>
  )
}

export default App
```

**Step 3: Create index.css (basic reset + dark theme)**

```css
* {
  margin: 0;
  padding: 0;
  box-sizing: border-box;
}

:root {
  --bg: #0d1117;
  --surface: #161b22;
  --border: #30363d;
  --text: #e6edf3;
  --muted: #8b949e;
  --accent: #58a6ff;
  --green: #3fb950;
  --red: #f85149;
  --orange: #d29922;
  --purple: #bc8cff;
}

html, body, #root {
  height: 100vh;
  overflow: hidden;
}

body {
  font-family: -apple-system, BlinkMacSystemFont, "Segoe UI", Helvetica, Arial, sans-serif;
  background: var(--bg);
  color: var(--text);
  line-height: 1.5;
}
```

**Step 4: Create index.html**

```html
<!DOCTYPE html>
<html lang="en">
  <head>
    <meta charset="UTF-8" />
    <meta name="viewport" content="width=device-width, initial-scale=1.0" />
    <title>ODA Dashboard</title>
  </head>
  <body>
    <div id="root"></div>
    <script type="module" src="/src/main.tsx"></script>
  </body>
</html>
```

**Step 5: Install dependencies and test build**

```bash
cd web && npm install
npm run build
```

Expected: `web/dist/` created with `index.html` and `assets/`

**Step 6: Commit**

```bash
git add web/
git commit -m "feat: add React app skeleton"
```

---

### Task 3: Add feature flag to Go config

**Files:**
- Modify: `internal/config/config.go`
- Modify: `internal/config/config_test.go`

**Step 1: Add UseNewFrontend field to Config struct**

```go
type Config struct {
    // ... existing fields ...
    UseNewFrontend bool `yaml:"use_new_frontend"`
}
```

**Step 2: Set default value in Load function**

```go
func Load(rootDir string) (*Config, error) {
    // ... existing code ...
    
    if cfg.UseNewFrontend == false {
        // Default is false (old dashboard)
        // This is already the zero value, but explicit for clarity
    }
    
    return &cfg, nil
}
```

**Step 3: Add test for new field**

```go
func TestConfig_UseNewFrontend(t *testing.T) {
    cfg := &Config{}
    assert.False(t, cfg.UseNewFrontend, "default should be false")
    
    cfg.UseNewFrontend = true
    assert.True(t, cfg.UseNewFrontend)
}
```

**Step 4: Run tests**

```bash
go test ./internal/config/... -v
```

Expected: All tests pass

**Step 5: Commit**

```bash
git add internal/config/
git commit -m "feat: add UseNewFrontend feature flag to config"
```

---

### Task 4: Create version switch in footer (old dashboard)

**Files:**
- Modify: `internal/dashboard/templates/layout.html`
- Modify: `internal/dashboard/server.go`
- Modify: `internal/dashboard/handlers.go`

**Step 1: Add version switch to layout.html footer**

Add before closing `</footer>` tag:

```html
<div style="margin-left: auto; display: flex; align-items: center; gap: 1rem;">
  <span style="font-size: .75rem; color: var(--muted);">Dashboard:</span>
  <a href="/new/" class="btn" style="font-size: .75rem; padding: .2rem .5rem;">
    Try New Version
  </a>
</div>
```

**Step 2: Add handler for root path with feature flag check**

```go
func (s *Server) handleRoot(w http.ResponseWriter, r *http.Request) {
    // Check if user wants new version (cookie)
    cookie, err := r.Cookie("oda_dashboard_version")
    useNew := s.config.UseNewFrontend
    if err == nil && cookie.Value == "new" {
        useNew = true
    }
    
    if useNew {
        http.Redirect(w, r, "/new/", http.StatusTemporaryRedirect)
        return
    }
    
    // Serve old dashboard
    s.handleBoard(w, r)
}
```

**Step 3: Update routes**

```go
func (s *Server) routes() {
    // Root with feature flag
    s.mux.HandleFunc("GET /{$}", s.handleRoot)
    
    // ... rest of routes ...
}
```

**Step 4: Commit**

```bash
git add internal/dashboard/
git commit -m "feat: add version switch to old dashboard footer"
```

---

## Phase 2: API Layer (Day 2-3)

### Task 5: Create API v2 structure

**Files:**
- Create: `internal/api/v2/routes.go`
- Create: `internal/api/v2/board.go`
- Create: `internal/api/v2/types.go`

**Step 1: Create types.go with shared types**

```go
package v2

import "github.com/crazy-goat/one-dev-army/internal/github"

type TaskCard struct {
    ID       int      `json:"id"`
    Title    string   `json:"title"`
    Status   string   `json:"status"`
    Worker   string   `json:"worker,omitempty"`
    Assignee string   `json:"assignee,omitempty"`
    Labels   []string `json:"labels"`
    PRURL    string   `json:"pr_url,omitempty"`
    IsMerged bool     `json:"is_merged"`
}

type BoardData struct {
    SprintName     string     `json:"sprint_name"`
    Paused         bool       `json:"paused"`
    Processing     bool       `json:"processing"`
    CanCloseSprint bool       `json:"can_close_sprint"`
    CurrentTicket  *TaskInfo  `json:"current_ticket,omitempty"`
    Blocked        []TaskCard `json:"blocked"`
    Backlog        []TaskCard `json:"backlog"`
    Plan           []TaskCard `json:"plan"`
    Code           []TaskCard `json:"code"`
    AIReview       []TaskCard `json:"ai_review"`
    CheckPipeline  []TaskCard `json:"check_pipeline"`
    Approve        []TaskCard `json:"approve"`
    Merge          []TaskCard `json:"merge"`
    Done           []TaskCard `json:"done"`
    Failed         []TaskCard `json:"failed"`
}

type TaskInfo struct {
    Number   int    `json:"number"`
    Title    string `json:"title"`
    Status   string `json:"status"`
    Priority string `json:"priority,omitempty"`
    Type     string `json:"type,omitempty"`
    Size     string `json:"size,omitempty"`
}
```

**Step 2: Create routes.go**

```go
package v2

import (
    "net/http"
    
    "github.com/crazy-goat/one-dev-army/internal/db"
    "github.com/crazy-goat/one-dev-army/internal/github"
    "github.com/crazy-goat/one-dev-army/internal/mvp"
)

type Handler struct {
    store        *db.Store
    gh           *github.Client
    orchestrator *mvp.Orchestrator
}

func NewHandler(store *db.Store, gh *github.Client, orch *mvp.Orchestrator) *Handler {
    return &Handler{
        store:        store,
        gh:           gh,
        orchestrator: orch,
    }
}

func (h *Handler) Register(mux *http.ServeMux) {
    mux.HandleFunc("GET /api/v2/board", h.GetBoard)
    mux.HandleFunc("GET /api/v2/tasks/{id}", h.GetTask)
    mux.HandleFunc("POST /api/v2/tasks/{id}/actions/{action}", h.TaskAction)
    mux.HandleFunc("GET /api/v2/sprint", h.GetSprint)
    mux.HandleFunc("POST /api/v2/sprint/{action}", h.SprintAction)
    mux.HandleFunc("GET /api/v2/worker-status", h.GetWorkerStatus)
}
```

**Step 3: Create board.go with GetBoard handler**

```go
package v2

import (
    "encoding/json"
    "net/http"
    "strings"
)

func (h *Handler) GetBoard(w http.ResponseWriter, r *http.Request) {
    w.Header().Set("Content-Type", "application/json")
    
    data := h.buildBoardData()
    
    if err := json.NewEncoder(w).Encode(data); err != nil {
        http.Error(w, err.Error(), http.StatusInternalServerError)
        return
    }
}

func (h *Handler) buildBoardData() BoardData {
    // Reuse logic from dashboard/handlers.go
    // ... implementation ...
    return BoardData{}
}
```

**Step 4: Commit**

```bash
git add internal/api/
git commit -m "feat: create API v2 structure and board endpoint"
```

---

### Task 6: Implement remaining API v2 endpoints

**Files:**
- Create: `internal/api/v2/tasks.go`
- Create: `internal/api/v2/sprint.go`
- Create: `internal/api/v2/worker.go`

**Step 1: Implement tasks.go**

```go
package v2

import (
    "encoding/json"
    "net/http"
    "strconv"
)

func (h *Handler) GetTask(w http.ResponseWriter, r *http.Request) {
    idStr := r.PathValue("id")
    id, err := strconv.Atoi(idStr)
    if err != nil {
        http.Error(w, "invalid task id", http.StatusBadRequest)
        return
    }
    
    // Get task from store
    // Return task with steps
}

func (h *Handler) TaskAction(w http.ResponseWriter, r *http.Request) {
    idStr := r.PathValue("id")
    action := r.PathValue("action")
    
    id, err := strconv.Atoi(idStr)
    if err != nil {
        http.Error(w, "invalid task id", http.StatusBadRequest)
        return
    }
    
    // Handle different actions
    switch action {
    case "approve":
        // ...
    case "reject":
        // ...
    // ... etc
    default:
        http.Error(w, "unknown action", http.StatusBadRequest)
        return
    }
    
    w.WriteHeader(http.StatusOK)
    json.NewEncoder(w).Encode(map[string]bool{"success": true})
}
```

**Step 2: Implement sprint.go and worker.go**

Similar pattern - wrap existing logic from dashboard/handlers.go

**Step 3: Wire up in server.go**

```go
func (s *Server) routes() {
    // API v2
    v2Handler := v2.NewHandler(s.store, s.gh, s.orchestrator)
    v2Handler.Register(s.mux)
    
    // ... rest of routes
}
```

**Step 4: Commit**

```bash
git add internal/api/
git commit -m "feat: implement all API v2 endpoints"
```

---

## Phase 3: React Components (Day 4-10)

### Task 7: Create API client and types

**Files:**
- Create: `web/src/api/client.ts`
- Create: `web/src/api/board.ts`
- Create: `web/src/types/board.ts`

**Step 1: Create client.ts**

```typescript
const API_BASE = '/api/v2'

export async function fetchAPI<T>(path: string, options?: RequestInit): Promise<T> {
  const response = await fetch(`${API_BASE}${path}`, {
    headers: {
      'Content-Type': 'application/json',
    },
    ...options,
  })
  
  if (!response.ok) {
    throw new Error(`API error: ${response.status}`)
  }
  
  return response.json()
}
```

**Step 2: Create types**

```typescript
// web/src/types/board.ts
export interface TaskCard {
  id: number
  title: string
  status: string
  worker?: string
  assignee?: string
  labels: string[]
  pr_url?: string
  is_merged: boolean
}

export interface BoardData {
  sprint_name: string
  paused: boolean
  processing: boolean
  can_close_sprint: boolean
  current_ticket?: TaskInfo
  blocked: TaskCard[]
  backlog: TaskCard[]
  plan: TaskCard[]
  code: TaskCard[]
  ai_review: TaskCard[]
  check_pipeline: TaskCard[]
  approve: TaskCard[]
  merge: TaskCard[]
  done: TaskCard[]
  failed: TaskCard[]
}

export interface TaskInfo {
  number: number
  title: string
  status: string
  priority?: string
  type?: string
  size?: string
}
```

**Step 3: Create board API**

```typescript
// web/src/api/board.ts
import { fetchAPI } from './client'
import { BoardData } from '../types/board'

export const boardAPI = {
  getBoard: () => fetchAPI<BoardData>('/board'),
}
```

**Step 4: Commit**

```bash
git add web/src/api web/src/types
git commit -m "feat: add API client and types"
```

---

### Task 8: Create Zustand store

**Files:**
- Create: `web/src/store/index.ts`

**Step 1: Create store**

```typescript
import { create } from 'zustand'
import { BoardData, TaskCard } from '../types/board'

interface Store {
  // Board state
  board: BoardData | null
  isLoading: boolean
  error: string | null
  
  // Worker state
  wsConnected: boolean
  workerStatus: {
    active: boolean
    paused: boolean
    step: string
    issue_id: number
    issue_title: string
  } | null
  
  // Actions
  setBoard: (board: BoardData) => void
  setLoading: (loading: boolean) => void
  setError: (error: string | null) => void
  updateTask: (taskId: number, updates: Partial<TaskCard>) => void
  setWsConnected: (connected: boolean) => void
  setWorkerStatus: (status: Store['workerStatus']) => void
}

export const useStore = create<Store>((set) => ({
  board: null,
  isLoading: false,
  error: null,
  wsConnected: false,
  workerStatus: null,
  
  setBoard: (board) => set({ board }),
  setLoading: (isLoading) => set({ isLoading }),
  setError: (error) => set({ error }),
  updateTask: (taskId, updates) => set((state) => {
    if (!state.board) return state
    // Update task in appropriate column
    return state
  }),
  setWsConnected: (wsConnected) => set({ wsConnected }),
  setWorkerStatus: (workerStatus) => set({ workerStatus }),
}))
```

**Step 2: Commit**

```bash
git add web/src/store
git commit -m "feat: add Zustand store"
```

---

### Task 9: Create Layout components

**Files:**
- Create: `web/src/components/Layout/Layout.tsx`
- Create: `web/src/components/Layout/Navbar.tsx`
- Create: `web/src/components/Layout/Footer.tsx`

**Step 1: Create Layout.tsx**

```typescript
import { Navbar } from './Navbar'
import { Footer } from './Footer'

interface LayoutProps {
  children: React.ReactNode
}

export function Layout({ children }: LayoutProps) {
  return (
    <div style={{ display: 'flex', flexDirection: 'column', height: '100vh' }}>
      <Navbar />
      <main style={{ flex: 1, overflow: 'auto', padding: '1rem' }}>
        {children}
      </main>
      <Footer />
    </div>
  )
}
```

**Step 2: Create Navbar.tsx**

```typescript
import { Link } from 'react-router-dom'

export function Navbar() {
  return (
    <nav style={{
      background: 'var(--surface)',
      borderBottom: '1px solid var(--border)',
      padding: '.75rem 1.5rem',
      display: 'flex',
      alignItems: 'center',
      gap: '2rem'
    }}>
      <span style={{ fontWeight: 700, fontSize: '1.1rem' }}>ODA</span>
      <div style={{ display: 'flex', gap: '1rem' }}>
        <Link to="/" style={{ color: 'var(--text)', textDecoration: 'none' }}>
          Sprint Board
        </Link>
        <Link to="/settings" style={{ color: 'var(--muted)', textDecoration: 'none' }}>
          Settings
        </Link>
      </div>
    </nav>
  )
}
```

**Step 3: Create Footer.tsx with version switch**

```typescript
export function Footer() {
  const switchToOld = () => {
    document.cookie = 'oda_dashboard_version=old; path=/'
    window.location.href = '/'
  }
  
  return (
    <footer style={{
      background: 'var(--surface)',
      borderTop: '1px solid var(--border)',
      padding: '.5rem 1.5rem',
      display: 'flex',
      justifyContent: 'flex-end',
      alignItems: 'center',
      gap: '1rem'
    }}>
      <span style={{ fontSize: '.75rem', color: 'var(--muted)' }}>
        New Dashboard (Beta)
      </span>
      <button 
        onClick={switchToOld}
        style={{
          fontSize: '.75rem',
          padding: '.2rem .5rem',
          background: 'var(--surface)',
          border: '1px solid var(--border)',
          borderRadius: '4px',
          color: 'var(--text)',
          cursor: 'pointer'
        }}
      >
        Back to Old Version
      </button>
    </footer>
  )
}
```

**Step 4: Commit**

```bash
git add web/src/components/Layout
git commit -m "feat: add Layout components with version switch"
```

---

### Task 10: Create Board components

**Files:**
- Create: `web/src/components/Board/KanbanBoard.tsx`
- Create: `web/src/components/Board/KanbanColumn.tsx`
- Create: `web/src/components/Board/TaskCard.tsx`
- Create: `web/src/components/Board/ProcessingPanel.tsx`

**Step 1: Create KanbanBoard.tsx**

```typescript
import useSWR from 'swr'
import { boardAPI } from '../../api/board'
import { KanbanColumn } from './KanbanColumn'
import { ProcessingPanel } from './ProcessingPanel'

export function KanbanBoard() {
  const { data: board, error, isLoading } = useSWR('board', boardAPI.getBoard, {
    refreshInterval: 5000, // Poll every 5s as fallback
  })
  
  if (isLoading) return <div>Loading...</div>
  if (error) return <div>Error: {error.message}</div>
  if (!board) return <div>No data</div>
  
  return (
    <div style={{ display: 'flex', flexDirection: 'column', height: '100%' }}>
      <div style={{ 
        display: 'grid', 
        gridTemplateColumns: '15% 1fr 15%', 
        gap: '1rem',
        flex: 1 
      }}>
        {/* Left column */}
        <div style={{ display: 'flex', flexDirection: 'column', gap: '.5rem' }}>
          <KanbanColumn title="Backlog" count={board.backlog.length} tasks={board.backlog} />
          <KanbanColumn title="Blocked" count={board.blocked.length} tasks={board.blocked} variant="blocked" />
        </div>
        
        {/* Center column */}
        <div style={{ display: 'flex', flexDirection: 'column', gap: '1rem' }}>
          <div style={{ 
            display: 'grid', 
            gridTemplateColumns: 'repeat(6, 1fr)', 
            gap: '1rem',
            height: '180px'
          }}>
            <KanbanColumn title="Plan" count={board.plan.length} tasks={board.plan} variant="plan" />
            <KanbanColumn title="Code" count={board.code.length} tasks={board.code} variant="code" />
            <KanbanColumn title="AI Review" count={board.ai_review.length} tasks={board.ai_review} />
            <KanbanColumn title="Check Pipeline" count={board.check_pipeline.length} tasks={board.check_pipeline} variant="pipeline" />
            <KanbanColumn title="Approve" count={board.approve.length} tasks={board.approve} variant="approve" />
            <KanbanColumn title="Merge" count={board.merge.length} tasks={board.merge} variant="merge" />
          </div>
          <ProcessingPanel currentTicket={board.current_ticket} />
        </div>
        
        {/* Right column */}
        <div style={{ display: 'flex', flexDirection: 'column', gap: '.5rem' }}>
          <KanbanColumn title="Done" count={board.done.length} tasks={board.done} />
          <KanbanColumn title="Failed" count={board.failed.length} tasks={board.failed} variant="failed" />
        </div>
      </div>
    </div>
  )
}
```

**Step 2: Create KanbanColumn.tsx**

```typescript
import { TaskCard } from './TaskCard'
import { TaskCard as TaskCardType } from '../../types/board'

interface KanbanColumnProps {
  title: string
  count: number
  tasks: TaskCardType[]
  variant?: 'default' | 'blocked' | 'plan' | 'code' | 'pipeline' | 'approve' | 'merge' | 'failed'
}

const variantStyles = {
  default: {},
  blocked: { borderColor: 'var(--red)' },
  plan: { borderColor: '#f39c12', background: 'rgba(243,156,18,0.05)' },
  code: { borderColor: '#3498db', background: 'rgba(52,152,219,0.05)' },
  pipeline: { borderColor: '#17a2b8', background: 'rgba(23,162,184,0.05)' },
  approve: { borderColor: '#9b59b6', background: 'rgba(155,89,182,0.05)' },
  merge: { borderColor: '#6f42c1', background: 'rgba(111,66,193,0.05)' },
  failed: { borderColor: '#e74c3c', background: 'rgba(231,76,60,0.05)' },
}

export function KanbanColumn({ title, count, tasks, variant = 'default' }: KanbanColumnProps) {
  const styles = variantStyles[variant]
  
  return (
    <div style={{
      background: 'var(--surface)',
      border: '1px solid var(--border)',
      borderRadius: '8px',
      padding: '.75rem',
      height: '100%',
      overflowY: 'auto',
      ...styles
    }}>
      <div style={{
        fontSize: '.8rem',
        fontWeight: 600,
        textTransform: 'uppercase',
        letterSpacing: '.05em',
        color: 'var(--muted)',
        marginBottom: '.75rem',
        display: 'flex',
        justifyContent: 'space-between'
      }}>
        {title}
        <span style={{
          background: 'var(--border)',
          padding: '.1rem .5rem',
          borderRadius: '10px',
          fontSize: '.75rem'
        }}>{count}</span>
      </div>
      
      {tasks.map(task => (
        <TaskCard key={task.id} task={task} />
      ))}
      
      {tasks.length === 0 && (
        <div style={{ color: 'var(--muted)', fontSize: '.85rem', textAlign: 'center', padding: '2rem', fontStyle: 'italic' }}>
          No tickets
        </div>
      )}
    </div>
  )
}
```

**Step 3: Create TaskCard.tsx**

```typescript
import { Link } from 'react-router-dom'
import { TaskCard as TaskCardType } from '../../types/board'

interface TaskCardProps {
  task: TaskCardType
}

export function TaskCard({ task }: TaskCardProps) {
  return (
    <div style={{
      background: 'var(--bg)',
      border: '1px solid var(--border)',
      borderRadius: '6px',
      padding: '.6rem .75rem',
      marginBottom: '.5rem',
      fontSize: '.85rem'
    }}>
      <div style={{ color: 'var(--muted)', fontSize: '.75rem' }}>
        <Link to={`/task/${task.id}`} style={{ color: 'var(--muted)', textDecoration: 'none' }}>
          #{task.id}
        </Link>
      </div>
      <div style={{ marginTop: '.2rem' }}>{task.title}</div>
      
      {task.labels.length > 0 && (
        <div style={{ display: 'flex', gap: '.25rem', marginTop: '.3rem', flexWrap: 'wrap' }}>
          {task.labels.map(label => (
            <span key={label} style={{
              fontSize: '.65rem',
              padding: '.1rem .4rem',
              borderRadius: '4px',
              background: 'var(--surface)',
              border: '1px solid var(--border)'
            }}>{label}</span>
          ))}
        </div>
      )}
      
      {task.assignee && (
        <div style={{ color: 'var(--accent)', fontSize: '.75rem', marginTop: '.3rem' }}>
          @{task.assignee}
        </div>
      )}
    </div>
  )
}
```

**Step 4: Create ProcessingPanel.tsx**

```typescript
import { Link } from 'react-router-dom'
import { TaskInfo } from '../../types/board'

interface ProcessingPanelProps {
  currentTicket?: TaskInfo
}

export function ProcessingPanel({ currentTicket }: ProcessingPanelProps) {
  const isActive = !!currentTicket
  
  return (
    <div style={{
      background: isActive ? 'rgba(52,152,219,0.08)' : 'rgba(108,117,125,0.08)',
      border: `1px solid ${isActive ? 'rgba(52,152,219,0.2)' : 'rgba(108,117,125,0.2)'}`,
      borderRadius: '8px',
      padding: '.5rem 1rem',
      display: 'flex',
      alignItems: 'center',
      minHeight: '100px'
    }}>
      <span style={{
        width: '8px',
        height: '8px',
        borderRadius: '50%',
        background: isActive ? '#3498db' : '#6c757d',
        marginRight: '1rem',
        animation: isActive ? 'pulse 2s infinite' : 'none'
      }} />
      
      {currentTicket ? (
        <div>
          <Link to={`/task/${currentTicket.number}`} style={{ textDecoration: 'none', color: 'var(--text)' }}>
            <div style={{ color: 'var(--muted)', fontSize: '.8rem' }}>#{currentTicket.number}</div>
            <div style={{ fontSize: '1.1rem', fontWeight: 600 }}>{currentTicket.title}</div>
          </Link>
          <div style={{ marginTop: '.5rem', display: 'flex', gap: '.5rem' }}>
            {currentTicket.priority && (
              <span style={{
                fontSize: '.7rem',
                padding: '.15rem .5rem',
                borderRadius: '4px',
                background: 'var(--surface)',
                border: '1px solid var(--border)'
              }}>
                {currentTicket.priority}
              </span>
            )}
            {currentTicket.type && (
              <span style={{
                fontSize: '.7rem',
                padding: '.15rem .5rem',
                borderRadius: '4px',
                background: 'var(--surface)',
                border: '1px solid var(--border)'
              }}>
                {currentTicket.type}
              </span>
            )}
          </div>
        </div>
      ) : (
        <span style={{ color: 'var(--muted)', fontSize: '.85rem' }}>
          No active ticket — Worker ready
        </span>
      )}
    </div>
  )
}
```

**Step 5: Commit**

```bash
git add web/src/components/Board
git commit -m "feat: add Board components (KanbanBoard, Column, TaskCard, ProcessingPanel)"
```

---

### Task 11: Create BoardPage and wire up routing

**Files:**
- Create: `web/src/pages/BoardPage.tsx`
- Modify: `web/src/App.tsx`

**Step 1: Create BoardPage.tsx**

```typescript
import { Layout } from '../components/Layout/Layout'
import { KanbanBoard } from '../components/Board/KanbanBoard'

export function BoardPage() {
  return (
    <Layout>
      <KanbanBoard />
    </Layout>
  )
}
```

**Step 2: Update App.tsx**

```typescript
import { BrowserRouter, Routes, Route } from 'react-router-dom'
import { BoardPage } from './pages/BoardPage'

function App() {
  return (
    <BrowserRouter>
      <Routes>
        <Route path="/" element={<BoardPage />} />
        <Route path="*" element={<div>Not Found</div>} />
      </Routes>
    </BrowserRouter>
  )
}

export default App
```

**Step 3: Test dev build**

```bash
cd web && npm run build
```

Expected: Build succeeds, `web/dist/` created

**Step 4: Commit**

```bash
git add web/src/pages web/src/App.tsx
git commit -m "feat: add BoardPage and routing"
```

---

## Phase 4: Integration (Day 11-12)

### Task 12: Embed React app in Go binary

**Files:**
- Modify: `internal/dashboard/server.go`
- Modify: `internal/dashboard/routes.go` (new file)

**Step 1: Add embed directive**

```go
//go:embed web/dist/*
var webFS embed.FS
```

**Step 2: Add serveReactApp handler**

```go
func (s *Server) serveReactApp(w http.ResponseWriter, r *http.Request) {
    // Serve index.html for all /new/* routes (SPA fallback)
    w.Header().Set("Content-Type", "text/html")
    
    data, err := webFS.ReadFile("web/dist/index.html")
    if err != nil {
        http.Error(w, "Failed to load app", http.StatusInternalServerError)
        return
    }
    
    w.Write(data)
}
```

**Step 3: Add static file serving**

```go
func (s *Server) routes() {
    // API v2
    v2Handler := v2.NewHandler(s.store, s.gh, s.orchestrator)
    v2Handler.Register(s.mux)
    
    // Static files from React build
    web, _ := fs.Sub(webFS, "web/dist")
    s.mux.Handle("GET /new/assets/", http.StripPrefix("/new/", http.FileServer(http.FS(web))))
    
    // React SPA fallback
    s.mux.HandleFunc("GET /new/{$}", s.serveReactApp)
    s.mux.HandleFunc("GET /new/{path...}", s.serveReactApp)
    
    // Root with feature flag
    s.mux.HandleFunc("GET /{$}", s.handleRoot)
    
    // ... rest of old dashboard routes ...
}
```

**Step 4: Test build**

```bash
cd web && npm run build
cd .. && go build ./cmd/oda
```

Expected: Binary builds successfully with embedded frontend

**Step 5: Commit**

```bash
git add internal/dashboard/
git commit -m "feat: embed React app in Go binary"
```

---

### Task 13: Add WebSocket integration

**Files:**
- Create: `web/src/hooks/useWebSocket.ts`
- Modify: `web/src/store/index.ts`

**Step 1: Create useWebSocket hook**

```typescript
import { useEffect, useRef } from 'react'
import { useStore } from '../store'

export function useWebSocket() {
  const wsRef = useRef<WebSocket | null>(null)
  const { setWsConnected, setWorkerStatus, updateTask } = useStore()
  
  useEffect(() => {
    const wsUrl = `${window.location.protocol === 'https:' ? 'wss:' : 'ws:'}//${window.location.host}/ws`
    const ws = new WebSocket(wsUrl)
    wsRef.current = ws
    
    ws.onopen = () => {
      console.log('[WebSocket] Connected')
      setWsConnected(true)
    }
    
    ws.onclose = () => {
      console.log('[WebSocket] Disconnected')
      setWsConnected(false)
    }
    
    ws.onmessage = (event) => {
      const msg = JSON.parse(event.data)
      
      switch (msg.type) {
        case 'worker_update':
          setWorkerStatus(JSON.parse(msg.payload))
          break
        case 'issue_update':
          // Trigger board refresh
          break
        case 'sync_complete':
          // Trigger board refresh
          break
      }
    }
    
    return () => {
      ws.close()
    }
  }, [setWsConnected, setWorkerStatus, updateTask])
  
  return wsRef.current
}
```

**Step 2: Use hook in App.tsx**

```typescript
import { useWebSocket } from './hooks/useWebSocket'

function App() {
  useWebSocket() // Initialize WebSocket connection
  
  return (
    <BrowserRouter>
      <Routes>
        <Route path="/" element={<BoardPage />} />
      </Routes>
    </BrowserRouter>
  )
}
```

**Step 3: Commit**

```bash
git add web/src/hooks web/src/App.tsx
git commit -m "feat: add WebSocket integration"
```

---

## Phase 5: Remaining Features (Day 13-15)

### Task 14: Create Task Detail page

**Files:**
- Create: `web/src/pages/TaskPage.tsx`
- Create: `web/src/components/Task/StepList.tsx`
- Create: `web/src/components/Task/StreamBox.tsx`
- Modify: `web/src/App.tsx`

**Step 1: Create types for task detail**

```typescript
// web/src/types/task.ts
export interface TaskStep {
  id: number
  step_name: string
  status: 'pending' | 'running' | 'done' | 'failed'
  prompt?: string
  response?: string
  error_msg?: string
  started_at?: string
  finished_at?: string
}

export interface TaskDetail {
  issue_number: number
  issue_title: string
  status: string
  is_active: boolean
  steps: TaskStep[]
}
```

**Step 2: Create TaskPage.tsx**

```typescript
import { useParams } from 'react-router-dom'
import useSWR from 'swr'
import { Layout } from '../components/Layout/Layout'
import { StepList } from '../components/Task/StepList'
import { tasksAPI } from '../api/tasks'

export function TaskPage() {
  const { id } = useParams<{ id: string }>()
  const { data: task, error, isLoading } = useSWR(
    id ? `task-${id}` : null,
    () => tasksAPI.getTask(parseInt(id!))
  )
  
  if (isLoading) return <Layout><div>Loading...</div></Layout>
  if (error) return <Layout><div>Error: {error.message}</div></Layout>
  if (!task) return <Layout><div>Task not found</div></Layout>
  
  return (
    <Layout>
      <div>
        <h1>#{task.issue_number}: {task.issue_title}</h1>
        <StepList steps={task.steps} isActive={task.is_active} issueNumber={task.issue_number} />
      </div>
    </Layout>
  )
}
```

**Step 3: Add route**

```typescript
<Route path="/task/:id" element={<TaskPage />} />
```

**Step 4: Commit**

```bash
git add web/src/pages/TaskPage.tsx web/src/components/Task web/src/api/tasks.ts web/src/types/task.ts
git commit -m "feat: add Task Detail page with steps and SSE stream"
```

---

### Task 15: Create Sprint controls

**Files:**
- Create: `web/src/components/Board/SprintControls.tsx`
- Modify: `web/src/components/Board/KanbanBoard.tsx`

**Step 1: Create SprintControls.tsx**

```typescript
import useSWR from 'swr'
import { sprintAPI } from '../../api/sprint'

export function SprintControls() {
  const { data: sprint, mutate } = useSWR('sprint', sprintAPI.getSprint)
  
  const handleAction = async (action: 'start' | 'pause' | 'close') => {
    await sprintAPI.action(action)
    mutate() // Refresh data
  }
  
  if (!sprint) return null
  
  return (
    <div style={{ display: 'flex', gap: '.5rem', marginBottom: '1rem' }}>
      {sprint.paused ? (
        <button onClick={() => handleAction('start')} className="btn btn-success">
          Start Sprint
        </button>
      ) : (
        <button onClick={() => handleAction('pause')} className="btn btn-danger">
          Pause Sprint
        </button>
      )}
      
      {sprint.can_close_sprint && (
        <button onClick={() => handleAction('close')} className="btn btn-success">
          Close Sprint
        </button>
      )}
    </div>
  )
}
```

**Step 2: Add to KanbanBoard**

```typescript
import { SprintControls } from './SprintControls'

// In KanbanBoard component, before the grid:
<SprintControls />
```

**Step 3: Commit**

```bash
git add web/src/components/Board/SprintControls.tsx web/src/api/sprint.ts
git commit -m "feat: add Sprint controls"
```

---

## Phase 6: Testing & Polish (Day 16-17)

### Task 16: Add error handling and loading states

**Files:**
- Modify: All components with API calls
- Create: `web/src/components/common/ErrorBoundary.tsx`
- Create: `web/src/components/common/Loading.tsx`

**Step 1: Create ErrorBoundary**

```typescript
import { Component, ReactNode } from 'react'

interface Props {
  children: ReactNode
  fallback?: ReactNode
}

interface State {
  hasError: boolean
  error?: Error
}

export class ErrorBoundary extends Component<Props, State> {
  state: State = { hasError: false }
  
  static getDerivedStateFromError(error: Error): State {
    return { hasError: true, error }
  }
  
  render() {
    if (this.state.hasError) {
      return this.props.fallback || <div>Something went wrong</div>
    }
    
    return this.props.children
  }
}
```

**Step 2: Create Loading component**

```typescript
export function Loading() {
  return (
    <div style={{ 
      display: 'flex', 
      justifyContent: 'center', 
      alignItems: 'center',
      height: '100%'
    }}>
      <div>Loading...</div>
    </div>
  )
}
```

**Step 3: Update components to use ErrorBoundary and Loading**

**Step 4: Commit**

```bash
git add web/src/components/common
git commit -m "feat: add error handling and loading states"
```

---

### Task 17: Final testing and build verification

**Step 1: Run all tests**

```bash
# Go tests
go test ./... -race

# Frontend lint
cd web && npm run lint

# Frontend typecheck
cd web && npm run typecheck

# Build frontend
cd web && npm run build

# Build Go binary
cd .. && go build ./cmd/oda

# Test binary
./oda --help
```

**Step 2: Manual testing checklist**

- [ ] Old dashboard works (feature flag = false)
- [ ] New dashboard loads (go to /new/)
- [ ] Version switch works both ways
- [ ] Board displays correctly
- [ ] Task cards show all info
- [ ] Processing panel shows current task
- [ ] WebSocket connects and receives updates
- [ ] Sprint controls work
- [ ] Task detail page loads
- [ ] SSE stream works for active tasks

**Step 3: Commit any fixes**

```bash
git add .
git commit -m "fix: address testing issues"
```

---

## Phase 7: Documentation & Cleanup (Day 18)

### Task 18: Update documentation

**Files:**
- Modify: `docs/architecture.md`
- Modify: `docs/development.md`
- Create: `web/README.md`

**Step 1: Update architecture.md**

Add section about new frontend architecture.

**Step 2: Update development.md**

Add instructions for frontend development.

**Step 3: Create web/README.md**

```markdown
# ODA Dashboard Frontend

React SPA for ODA dashboard.

## Development

```bash
npm install
npm run dev
```

## Build

```bash
npm run build
```

## Tech Stack

- React 18
- TypeScript
- Vite
- Zustand
- SWR
- React Router
```

**Step 4: Commit**

```bash
git add docs/ web/README.md
git commit -m "docs: update documentation for new frontend"
```

---

## Summary

**Total Tasks:** 18  
**Estimated Time:** 18 days (with buffer)  
**Key Milestones:**
- Day 3: API v2 complete
- Day 10: React components complete
- Day 12: Integration working
- Day 17: All features working
- Day 18: Documentation complete

**Next Steps After Completion:**
1. User testing period (1 week)
2. Change default feature flag to `true`
3. Remove old dashboard code
4. Remove feature flag
5. Update CI/CD for frontend build

---

## Appendix: File Structure

```
web/
├── src/
│   ├── api/
│   │   ├── client.ts
│   │   ├── board.ts
│   │   ├── tasks.ts
│   │   ├── sprint.ts
│   │   └── wizard.ts
│   ├── components/
│   │   ├── Board/
│   │   │   ├── KanbanBoard.tsx
│   │   │   ├── KanbanColumn.tsx
│   │   │   ├── TaskCard.tsx
│   │   │   ├── ProcessingPanel.tsx
│   │   │   └── SprintControls.tsx
│   │   ├── Task/
│   │   │   ├── StepList.tsx
│   │   │   ├── StepItem.tsx
│   │   │   └── StreamBox.tsx
│   │   ├── Layout/
│   │   │   ├── Layout.tsx
│   │   │   ├── Navbar.tsx
│   │   │   └── Footer.tsx
│   │   └── common/
│   │       ├── ErrorBoundary.tsx
│   │       └── Loading.tsx
│   ├── hooks/
│   │   └── useWebSocket.ts
│   ├── pages/
│   │   ├── BoardPage.tsx
│   │   └── TaskPage.tsx
│   ├── store/
│   │   └── index.ts
│   ├── types/
│   │   ├── board.ts
│   │   └── task.ts
│   ├── App.tsx
│   ├── main.tsx
│   └── index.css
├── package.json
├── vite.config.ts
└── tsconfig.json

internal/api/v2/
├── routes.go
├── types.go
├── board.go
├── tasks.go
├── sprint.go
└── worker.go
```
