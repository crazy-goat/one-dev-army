# LLM Config Propagation Implementation Plan

> **For Claude:** REQUIRED SUB-SKILL: Use superpowers:executing-plans to implement this plan task-by-task.

**Goal:** When a user changes LLM models in the dashboard settings, propagate the changes immediately to the Router, Worker, and Orchestrator so they take effect without restarting ODA.

**Architecture:** Add an `UpdateConfig()` method to Orchestrator (mirroring Worker's pattern), make the Orchestrator implement `ConfigAwareWorker`, wire the Router update into the `ConfigPropagator` via a wrapper, and add a `configPropagator` reference to the Dashboard `Server` so `handleSaveSettings` can trigger immediate propagation instead of waiting for the 5-second file-poll.

**Tech Stack:** Go, `sync/atomic`, `sync.Mutex`, existing `config.ConfigPropagator` infrastructure.

**GitHub Issue:** #307

---

## Analysis

### Current State

| Component | Has UpdateConfig? | Receives live updates? | Config access pattern |
|-----------|------------------|----------------------|----------------------|
| Worker | Yes (atomic) | Yes (via ConfigPropagator, 5s delay) | `w.cfg.Load()` — atomic, safe |
| Router | Yes (mutex) | **NO** — not registered with propagator | `r.cfg` — pointer swap under mutex |
| Orchestrator | **NO** | **NO** | `o.cfg` — plain pointer, never updated |
| Dashboard | N/A | N/A | Reads from disk each request |

### Propagation Chain Gaps

1. `handleSaveSettings()` → `SaveConfig()` → writes to disk → **STOP** (no in-memory propagation)
2. `ReloadManager` (5s poll) → detects file change → `ConfigPropagator.Propagate()` → only updates Worker
3. Router is **never** updated after initial creation
4. Orchestrator's `cfg` field is **never** updated after initial creation

### Key Files

- `internal/mvp/orchestrator.go:28-42` — Orchestrator struct, `cfg` is plain `*config.Config`
- `internal/mvp/orchestrator.go:300` — Only place Orchestrator reads `o.cfg.LLM.*` (fallback for router)
- `internal/mvp/worker.go:757-765` — Worker's existing `UpdateConfig()` pattern to follow
- `internal/llm/router.go:103-115` — Router's existing `UpdateConfig(*config.LLMConfig)` (different signature)
- `internal/config/reload.go:220-306` — `ConfigPropagator` and `ConfigAwareWorker` interface
- `internal/dashboard/server.go:24-42` — Dashboard Server struct (no propagator reference)
- `internal/dashboard/handlers.go:1768-1855` — `handleSaveSettings` (saves to disk only)
- `main.go:351-370` — Wiring: only Worker registered with propagator

---

## Task 1: Add `UpdateConfig()` to Orchestrator

**Files:**
- Modify: `internal/mvp/orchestrator.go:28-42` (struct) and append method near line 104
- Test: `internal/mvp/orchestrator_test.go`

### Step 1: Write the failing test

Add to `internal/mvp/orchestrator_test.go`:

```go
func TestOrchestrator_UpdateConfig(t *testing.T) {
	initialCfg := &config.Config{
		LLM: config.LLMConfig{
			Orchestration: config.ModelConfig{Model: "old-provider/old-model"},
		},
		YoloMode: false,
	}

	o := &Orchestrator{}
	o.cfg.Store(initialCfg)

	if o.cfg.Load().LLM.Orchestration.Model != "old-provider/old-model" {
		t.Fatalf("initial model = %q, want %q", o.cfg.Load().LLM.Orchestration.Model, "old-provider/old-model")
	}

	newCfg := &config.Config{
		LLM: config.LLMConfig{
			Orchestration: config.ModelConfig{Model: "new-provider/new-model"},
		},
		YoloMode: true,
	}
	o.UpdateConfig(newCfg)

	got := o.cfg.Load()
	if got.LLM.Orchestration.Model != "new-provider/new-model" {
		t.Errorf("updated model = %q, want %q", got.LLM.Orchestration.Model, "new-provider/new-model")
	}
	if !got.YoloMode {
		t.Error("updated YoloMode should be true")
	}
}

func TestOrchestrator_ImplementsConfigAwareWorker(t *testing.T) {
	var _ config.ConfigAwareWorker = (*Orchestrator)(nil)
}

func TestOrchestrator_UpdateConfig_PropagesToWorker(t *testing.T) {
	initialCfg := &config.Config{
		LLM: config.LLMConfig{
			Code: config.ModelConfig{Model: "old-provider/old-code-model"},
		},
	}

	o := &Orchestrator{}
	o.cfg.Store(initialCfg)

	w := &Worker{id: 1, decisionCh: make(chan UserDecision, 1)}
	w.cfg.Store(initialCfg)
	o.worker = w

	newCfg := &config.Config{
		LLM: config.LLMConfig{
			Code: config.ModelConfig{Model: "new-provider/new-code-model"},
		},
	}
	o.UpdateConfig(newCfg)

	if o.worker.cfg.Load().LLM.Code.Model != "new-provider/new-code-model" {
		t.Errorf("worker model = %q, want %q", o.worker.cfg.Load().LLM.Code.Model, "new-provider/new-code-model")
	}
}
```

### Step 2: Run test to verify it fails

Run: `go test ./internal/mvp/ -run TestOrchestrator_UpdateConfig -v`
Expected: FAIL — `o.cfg` is `*config.Config` not `atomic.Pointer`, no `UpdateConfig` method

### Step 3: Implement changes

**3a. Change `cfg` field from `*config.Config` to `atomic.Pointer[config.Config]` in Orchestrator struct.**

In `internal/mvp/orchestrator.go`, change the struct definition (line 28-42):

```go
type Orchestrator struct {
	cfg         atomic.Pointer[config.Config]  // was: *config.Config
	worker      *Worker
	gh          *github.Client
	oc          *opencode.Client
	brMgr       *git.BranchManager
	store       *db.Store
	hub         StageBroadcaster
	router      *llm.Router
	running     bool
	paused      bool
	processing  bool
	currentTask *Task
	mu          sync.Mutex
}
```

Add `"sync/atomic"` to imports if not already present. Note: `sync/atomic` is not needed as an import — `atomic.Pointer` is in `sync/atomic` which is already available via `sync`. Actually, `atomic` is a sub-package: add `"sync/atomic"` to imports.

**3b. Update constructor** (line 44-57):

```go
func NewOrchestrator(cfg *config.Config, gh *github.Client, oc *opencode.Client, brMgr *git.BranchManager, store *db.Store, hub StageBroadcaster, router *llm.Router) *Orchestrator {
	o := &Orchestrator{
		gh:     gh,
		oc:     oc,
		brMgr:  brMgr,
		store:  store,
		hub:    hub,
		router: router,
		paused: true,
	}
	o.cfg.Store(cfg)
	o.worker = NewWorker(1, cfg, oc.Clone(), gh, brMgr, store, o, router)
	return o
}
```

**3c. Update all `o.cfg.` references to `o.cfg.Load().`**

There is only ONE place (line 300):
```go
// Before:
llmModel := o.cfg.LLM.Orchestration.Model
// After:
llmModel := o.cfg.Load().LLM.Orchestration.Model
```

**3d. Add `UpdateConfig` method** (append after `GetWorker()`, around line 104):

```go
// UpdateConfig updates the orchestrator's configuration atomically and propagates to the worker and router.
func (o *Orchestrator) UpdateConfig(cfg *config.Config) {
	o.cfg.Store(cfg)
	if o.router != nil {
		o.router.UpdateConfig(&cfg.LLM)
	}
	if o.worker != nil {
		o.worker.UpdateConfig(cfg)
	}
	log.Printf("[Orchestrator] Configuration updated (YoloMode=%v)", cfg.YoloMode)
}
```

**3e. Add compile-time interface check** (append at end of file):

```go
var _ config.ConfigAwareWorker = (*Orchestrator)(nil)
```

### Step 4: Run tests to verify they pass

Run: `go test ./internal/mvp/ -run TestOrchestrator -v`
Expected: PASS

### Step 5: Commit

```bash
git add internal/mvp/orchestrator.go internal/mvp/orchestrator_test.go
git commit -m "feat: add UpdateConfig to Orchestrator for live LLM config propagation (#307)"
```

---

## Task 2: Register Orchestrator with ConfigPropagator in main.go

**Files:**
- Modify: `main.go:363-370`

### Step 1: No separate test needed

This is wiring — covered by the integration test in Task 4.

### Step 2: Update main.go wiring

In `main.go`, change lines 363-370 from:

```go
configPropagator := config.NewConfigPropagator(0)
configPropagator.RegisterWorker(orchestrator.GetWorker())
reloadManager.OnReload(func(cfg *config.Config) {
    configPropagator.Propagate(cfg)
})
```

To:

```go
configPropagator := config.NewConfigPropagator(0)
configPropagator.RegisterWorker(orchestrator)
reloadManager.OnReload(func(cfg *config.Config) {
    configPropagator.Propagate(cfg)
})
```

**Why:** The Orchestrator now implements `ConfigAwareWorker` and its `UpdateConfig` cascades to both the Router and Worker. Registering the Worker separately would cause double-updates.

### Step 3: Verify build

Run: `go build ./...`
Expected: Success

### Step 4: Commit

```bash
git add main.go
git commit -m "refactor: register Orchestrator instead of Worker with ConfigPropagator (#307)"
```

---

## Task 3: Add ConfigPropagator to Dashboard Server for Immediate Propagation

**Files:**
- Modify: `internal/dashboard/server.go:24-42` (struct), `internal/dashboard/server.go:44` (constructor)
- Modify: `internal/dashboard/handlers.go:1831-1837` (handleSaveSettings)
- Modify: `main.go:402` (NewServer call)
- Test: `internal/dashboard/handlers_test.go`

### Step 1: Write the failing test

Add to `internal/dashboard/handlers_test.go`:

```go
func TestHandleSaveSettings_PropagatesConfig(t *testing.T) {
	tmpDir := t.TempDir()
	odaDir := filepath.Join(tmpDir, ".oda")
	if err := os.MkdirAll(odaDir, 0755); err != nil {
		t.Fatalf("failed to create .oda directory: %v", err)
	}

	configContent := `llm:
  setup:
    model: old-provider/old-setup
  planning:
    model: old-provider/old-planning
  orchestration:
    model: old-provider/old-orch
  code:
    model: old-provider/old-code
  code_heavy:
    model: old-provider/old-code-heavy
`
	configPath := filepath.Join(odaDir, "config.yaml")
	if err := os.WriteFile(configPath, []byte(configContent), 0644); err != nil {
		t.Fatalf("failed to write config file: %v", err)
	}

	srv := createTestServerWithTemplates(t)
	srv.rootDir = tmpDir
	defer srv.wizardStore.Stop()

	propagated := make(chan *config.Config, 1)
	srv.configPropagator = &config.ConfigPropagator{}
	// We can't easily use a real propagator in unit tests, so we use a mock approach.
	// Instead, we'll verify the propagator's Propagate method is called by registering
	// a test worker.
	testWorker := &testConfigWorker{ch: propagated}
	srv.configPropagator = config.NewConfigPropagator(0)
	srv.configPropagator.RegisterWorker(testWorker)

	form := url.Values{}
	form.Set("setup_model", "new-provider/new-setup")
	form.Set("planning_model", "new-provider/new-planning")
	form.Set("orchestration_model", "new-provider/new-orch")
	form.Set("code_model", "new-provider/new-code")
	form.Set("code_heavy_model", "new-provider/new-code-heavy")

	req := httptest.NewRequest(http.MethodPost, "/settings", strings.NewReader(form.Encode()))
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	rec := httptest.NewRecorder()

	srv.handleSaveSettings(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("expected status 200, got %d: %s", rec.Code, rec.Body.String())
	}

	select {
	case cfg := <-propagated:
		if cfg.LLM.Code.Model != "new-provider/new-code" {
			t.Errorf("propagated code model = %q, want %q", cfg.LLM.Code.Model, "new-provider/new-code")
		}
	case <-time.After(2 * time.Second):
		t.Fatal("config was not propagated within timeout")
	}
}

type testConfigWorker struct {
	ch chan *config.Config
}

func (w *testConfigWorker) UpdateConfig(cfg *config.Config) {
	w.ch <- cfg
}
```

Note: Check if `time` is already imported in the test file. Add it if not.

### Step 2: Run test to verify it fails

Run: `go test ./internal/dashboard/ -run TestHandleSaveSettings_PropagatesConfig -v`
Expected: FAIL — `srv.configPropagator` field does not exist

### Step 3: Implement changes

**3a. Add `configPropagator` field to Server struct** in `internal/dashboard/server.go`:

```go
type Server struct {
	port             int
	webPort          int
	tmpls            map[string]*template.Template
	store            *db.Store
	pool             func() []worker.WorkerInfo
	gh               *github.Client
	orchestrator     *mvp.Orchestrator
	mux              *http.ServeMux
	httpSrv          *http.Server
	wizardStore      *WizardSessionStore
	oc               *opencode.Client
	wizardLLM        string
	hub              *Hub
	syncService      *SyncService
	rateLimitService *RateLimitService
	rootDir          string
	modelsCache      []opencode.ProviderModel
	configPropagator *config.ConfigPropagator
}
```

Add `"github.com/crazy-goat/one-dev-army/internal/config"` to imports in `server.go` if not already present.

**3b. Add `configPropagator` parameter to `NewServer`** in `internal/dashboard/server.go`:

Change the constructor signature (line 44):

```go
func NewServer(port int, webPort int, store *db.Store, pool func() []worker.WorkerInfo, gh *github.Client, orchestrator *mvp.Orchestrator, oc *opencode.Client, wizardLLM string, hub *Hub, syncService *SyncService, rootDir string, configPropagator *config.ConfigPropagator) (*Server, error) {
```

Add to the struct literal inside:

```go
	s := &Server{
		// ... existing fields ...
		configPropagator: configPropagator,
	}
```

**3c. Add propagation call in `handleSaveSettings`** in `internal/dashboard/handlers.go`.

After line 1837 (`log.Printf("[Dashboard] LLM configuration saved successfully")`), add:

```go
	if s.configPropagator != nil {
		s.configPropagator.Propagate(cfg)
		log.Printf("[Dashboard] LLM configuration propagated to running components")
	}
```

**3d. Update `main.go` call to `NewServer`** (line 402):

```go
srv, err := dashboard.NewServer(cfg.Dashboard.Port, cfg.OpenCode.WebPort, store, pool.Workers, gh, orchestrator, oc, cfg.LLM.Planning.Model, hub, syncService, dir, configPropagator)
```

### Step 4: Run tests to verify they pass

Run: `go test ./internal/dashboard/ -run TestHandleSaveSettings -v`
Expected: PASS (all settings tests including the new propagation test)

Also run: `go test ./... -count=1`
Expected: All tests pass

### Step 5: Commit

```bash
git add internal/dashboard/server.go internal/dashboard/handlers.go internal/dashboard/handlers_test.go main.go
git commit -m "feat: propagate LLM config changes immediately from dashboard settings (#307)"
```

---

## Task 4: Integration Test — Full Propagation Flow

**Files:**
- Test: `internal/mvp/orchestrator_test.go`

### Step 1: Write the integration test

Add to `internal/mvp/orchestrator_test.go`:

```go
func TestConfigPropagation_EndToEnd(t *testing.T) {
	initialCfg := &config.Config{
		LLM: config.LLMConfig{
			Setup:         config.ModelConfig{Model: "provider/setup-v1"},
			Planning:      config.ModelConfig{Model: "provider/planning-v1"},
			Orchestration: config.ModelConfig{Model: "provider/orch-v1"},
			Code:          config.ModelConfig{Model: "provider/code-v1"},
			CodeHeavy:     config.ModelConfig{Model: "provider/code-heavy-v1"},
		},
		YoloMode: false,
	}

	router := llm.NewRouter(&initialCfg.LLM)

	o := &Orchestrator{router: router}
	o.cfg.Store(initialCfg)

	w := &Worker{id: 1, decisionCh: make(chan UserDecision, 1)}
	w.cfg.Store(initialCfg)
	o.worker = w

	propagator := config.NewConfigPropagator(0)
	propagator.RegisterWorker(o)

	newCfg := &config.Config{
		LLM: config.LLMConfig{
			Setup:         config.ModelConfig{Model: "provider/setup-v2"},
			Planning:      config.ModelConfig{Model: "provider/planning-v2"},
			Orchestration: config.ModelConfig{Model: "provider/orch-v2"},
			Code:          config.ModelConfig{Model: "provider/code-v2"},
			CodeHeavy:     config.ModelConfig{Model: "provider/code-heavy-v2"},
		},
		YoloMode: true,
	}

	propagator.Propagate(newCfg)

	// Propagate dispatches goroutines, give them time to complete
	time.Sleep(100 * time.Millisecond)

	// Verify orchestrator received update
	oCfg := o.cfg.Load()
	if oCfg.LLM.Orchestration.Model != "provider/orch-v2" {
		t.Errorf("orchestrator model = %q, want %q", oCfg.LLM.Orchestration.Model, "provider/orch-v2")
	}
	if !oCfg.YoloMode {
		t.Error("orchestrator YoloMode should be true")
	}

	// Verify worker received update (cascaded from orchestrator)
	wCfg := w.cfg.Load()
	if wCfg.LLM.Code.Model != "provider/code-v2" {
		t.Errorf("worker code model = %q, want %q", wCfg.LLM.Code.Model, "provider/code-v2")
	}

	// Verify router received update (cascaded from orchestrator)
	selectedModel := router.SelectModel(config.CategoryCode, config.ComplexityMedium, nil)
	if selectedModel != "provider/code-v2" {
		t.Errorf("router code model = %q, want %q", selectedModel, "provider/code-v2")
	}
}
```

Note: Add `"time"` and `llm` import (`"github.com/crazy-goat/one-dev-army/internal/llm"`) to the test file imports.

### Step 2: Run test

Run: `go test ./internal/mvp/ -run TestConfigPropagation_EndToEnd -v`
Expected: PASS

### Step 3: Commit

```bash
git add internal/mvp/orchestrator_test.go
git commit -m "test: add end-to-end config propagation integration test (#307)"
```

---

## Task 5: Final Verification

### Step 1: Run all tests

Run: `go test -race ./...`
Expected: All tests pass with no race conditions

### Step 2: Run linter

Run: `golangci-lint run ./...`
Expected: No errors

### Step 3: Final commit (if any fixes needed)

```bash
git add -A
git commit -m "fix: address lint/test issues from config propagation (#307)"
```

---

## Summary of All Changes

| File | Change |
|------|--------|
| `internal/mvp/orchestrator.go` | Change `cfg` to `atomic.Pointer[config.Config]`, add `UpdateConfig()`, add interface check |
| `internal/mvp/orchestrator_test.go` | Add `TestOrchestrator_UpdateConfig`, `TestOrchestrator_ImplementsConfigAwareWorker`, `TestOrchestrator_UpdateConfig_PropagesToWorker`, `TestConfigPropagation_EndToEnd` |
| `internal/dashboard/server.go` | Add `configPropagator` field, add parameter to `NewServer` |
| `internal/dashboard/handlers.go` | Call `s.configPropagator.Propagate(cfg)` after save |
| `internal/dashboard/handlers_test.go` | Add `TestHandleSaveSettings_PropagatesConfig` with mock worker |
| `main.go` | Register `orchestrator` (not worker) with propagator, pass propagator to `NewServer` |

## Potential Breaking Changes

- `dashboard.NewServer()` signature changes (adds `configPropagator` parameter). Any callers outside `main.go` must be updated. Search for other callers — the test helper `createTestServerWithTemplates` in `handlers_test.go` does NOT call `NewServer` (it constructs `Server` directly), so it should be unaffected.
- `Orchestrator.cfg` changes from `*config.Config` to `atomic.Pointer[config.Config]`. Any code that accesses `o.cfg` directly (outside the package) will break. Since the field is unexported, this only affects code within `package mvp`. The only usage is `o.cfg.LLM.Orchestration.Model` at line 300, which we update.

## Complexity Estimate

**Small** (2-4 hours). The patterns already exist (Worker's `UpdateConfig`, Router's `UpdateConfig`, `ConfigPropagator`). This is primarily wiring and one struct field type change.
