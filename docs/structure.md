# Repository Structure

## Top-Level Layout

```
main.go                     # Entry point: CLI parsing, preflight checks, server bootstrap
go.mod / go.sum             # Go 1.25 module definition (github.com/crazy-goat/one-dev-army)
AGENTS.md                   # Project overview for AI agents (hub with links to docs/)
README.md                   # User-facing README with quick start guide
internal/                   # All private application packages (see below)
api/                        # OpenAPI spec for opencode (reference only, gitignored)
docs/                       # Project documentation
  architecture.md           # Orchestrator, state machine, worker pool, LLM routing, dashboard
  configuration.md          # CLI commands and .oda/config.yaml reference
  development.md            # Build, pre-commit checklist, CI/linting, testing & code conventions
  structure.md              # This file — repository structure with package descriptions
  workflow.md               # Ticket lifecycle flowchart (Mermaid)
  state-machine.md          # Full state machine specification with all transitions
  plans/                    # Architecture decision records and implementation plans
.oda/                       # Runtime directory (created by `oda init`, partially gitignored)
  config.yaml               # Project configuration (tracked)
  metrics.db                # SQLite metrics database (gitignored)
  oda.db                    # SQLite task progress database (gitignored)
  worktrees/                # Legacy worktree directory (gitignored)
.github/
  workflows/ci.yml          # CI pipeline: build + test + golangci-lint
```

## `main.go` — Entry Point

The single `main.go` file handles:

- **CLI parsing** — `flag` package, two commands: `oda` (serve) and `oda init`
- **Preflight checks** — verifies git repo, `gh` auth, opencode reachability
- **Auto-start opencode** — if opencode is not running, starts `opencode serve` as a subprocess
- **Bootstrap sequence** — loads config, opens SQLite, creates GitHub client, validates LLM models, ensures labels/milestones, populates issue cache, starts WebSocket hub, sync service, worker pool, orchestrator, and dashboard server
- **Graceful shutdown** — handles SIGINT/SIGTERM, waits for workers, shuts down HTTP server, stops opencode subprocess

## `internal/` — Application Packages

### `config/` — Configuration Management

**Files:** `config.go`, `llm.go`, `reload.go`

- **`config.go`** — Loads `.oda/config.yaml` via `gopkg.in/yaml.v3`. Defines all config structs: `GitHub`, `Dashboard`, `Workers`, `OpenCode`, `Tools`, `Pipeline`, `Planning`, `EpicAnalysis`, `Sprint`, `LLMConfig`. Applies LLM defaults for missing fields.
- **`llm.go`** — Multi-model LLM configuration. Defines 5 task categories (`code`, `code-heavy`, `planning`, `orchestration`, `setup`), complexity levels (`low`/`medium`/`high`), routing rules with complexity thresholds. Includes legacy migration from old strong/weak model format and model validation against available models with automatic fallback.
- **`reload.go`** — Hot-reload support. `ReloadManager` watches `config.yaml` for changes (polling-based `FileWatcher`, default 5s interval), reloads config, and notifies registered listeners. `ConfigPropagator` distributes config updates to workers implementing `ConfigAwareWorker` interface.

### `dashboard/` — HTMX Dashboard & WebSocket

**Files:** `server.go`, `handlers.go`, `websocket.go`, `sync.go`, `wizard.go`, `prompts.go`, `health.go`, `ratelimit.go`, `webserver.go`
**Templates:** `templates/*.html` (13 embedded HTML templates)

- **`server.go`** — HTTP server using stdlib `net/http` + `http.ServeMux`. Embeds templates via `//go:embed`. Registers all routes, creates `WizardSessionStore`, sets up rate limiting. Serves the Kanban board, worker status, task details, wizard pages, and API endpoints.
- **`handlers.go`** — HTMX request handlers for board rendering, issue actions (approve, decline, retry, cancel, block, unblock), worker status, task detail views. Returns HTML fragments for HTMX partial updates.
- **`websocket.go`** — `Hub` manages WebSocket connections. Broadcasts real-time issue updates and worker status changes to all connected clients. Supports debug logging via `--debug-websocket` flag.
- **`sync.go`** — `SyncService` periodically (30s) fetches issues from GitHub and updates the local SQLite cache. Detects label changes and broadcasts updates via WebSocket. Compares timestamps to avoid overwriting newer local data with stale GitHub CDN data.
- **`wizard.go`** — Epic decomposition wizard. Multi-step flow: describe epic → LLM generates sub-issues → user refines → create issues on GitHub. Uses `WizardSessionStore` for in-memory session management.
- **`prompts.go`** — LLM prompt templates for the wizard (epic analysis, issue generation).
- **`health.go`** — Health check endpoint.
- **`ratelimit.go`** — `RateLimitService` with per-endpoint token bucket rate limiting.
- **`webserver.go`** — Reverse proxy for the opencode web UI (serves on a separate port).

### `db/` — SQLite Storage

**Files:** `db.go`, `migrations.go`

- **`db.go`** — `Store` wraps `modernc.org/sqlite` (pure-Go SQLite, no CGO). Uses WAL journal mode, foreign keys, 5s busy timeout. All writes go through a single-writer goroutine via a buffered job channel (capacity 100) to avoid SQLite lock contention. Provides CRUD for three tables:
  - `stage_metrics` — LLM token usage, cost, duration per pipeline stage
  - `task_steps` — step-by-step progress for each issue (prompt, response, session ID, plan attachment URL)
  - `issue_cache` — local cache of GitHub issues (labels, state, milestone, PR merge status)
  - `stage_change_ledger` — audit log of all stage transitions (from/to stage, reason, changed_by)
- **`migrations.go`** — Sequential migration list with `CREATE TABLE IF NOT EXISTS` and `ALTER TABLE ADD COLUMN` statements. Handles idempotent column additions by checking `pragma_table_info`.

### `git/` — Branch Management

**Files:** `worktree.go`

- **`BranchManager`** — manages git branches for worker isolation. Creates branches (`git checkout -b`), removes branches (switches to main/master first, then `git branch -D`), pushes branches (`git push -u --force-with-lease` with fallback to regular push).
- Includes legacy worktree cleanup — removes leftover `git worktree` entries from previous ODA versions.
- Legacy type aliases (`WorktreeManager`, `NewWorktreeManager`, `Worktree`, `RunInWorktree`) maintained for backward compatibility.

### `github/` — GitHub Client

**Files:** `client.go`, `issues.go`, `labels.go`, `milestones.go`, `project.go`, `reasons.go`

- All GitHub interactions go through the `gh` CLI (no direct REST/GraphQL client library). The `Client` struct wraps `exec.Command("gh", ...)` with `-R owner/repo`.
- **`client.go`** — Base client with `gh()`, `ghNoRepo()`, `ghJSON()` helpers. Tracks active milestone and project board IDs.
- **`issues.go`** — List issues (by milestone, open/closed), get issue details, create issues, add comments, close issues, set labels, create PRs, merge PRs.
- **`labels.go`** — `EnsureLabels()` creates all required `stage:*` labels. `SetStageLabel()` is the universal stage transition method — atomically removes all existing `stage:*` labels and adds the new one. Special case: "Done" also closes the issue.
- **`milestones.go`** — `EnsureMilestone()` creates a sprint milestone if none exists. `GetOldestOpenMilestone()` finds the active sprint.
- **`project.go`** — GitHub Projects v2 integration (optional). Creates project board, manages status field and column options via GraphQL mutations through `gh api graphql`.
- **`reasons.go`** — Structured reason strings for stage transitions (used in the stage change ledger).

### `initialize/` — Project Scaffolding

**Files:** `init.go`

- Implements `oda init`. Detects the GitHub repo from git remote, creates `.oda/` directory, generates default `config.yaml` with sensible defaults. If the repo is empty, prompts for a project description. Detects project language for template generation.

### `llm/` — LLM Router

**Files:** `router.go`, `complexity.go`

- **`router.go`** — `Router` selects the appropriate LLM model based on task category. Maps pipeline stages to categories (`analysis`/`planning`/`plan-review` → planning, `coding`/`testing`/`code-review` → code, etc.). Supports config hot-reload. Also provides `EstimateComplexity()` which analyzes issue text for complexity indicators.
- **`complexity.go`** — Heuristic complexity estimation. Counts lines, detects complexity indicator patterns (refactor, concurrency, security, database migration, etc.), counts affected files. Returns `low`/`medium`/`high` based on configurable thresholds.

### `metrics/` — Metrics Writer

**Files:** `writer.go`

- `Writer` saves stage metrics to SQLite and posts YAML-formatted metric summaries as GitHub issue comments. Tracks tokens in/out, cost in USD, duration, and retry count per stage.

### `mvp/` — Core Orchestrator & Worker

**Files:** `orchestrator.go`, `worker.go`, `task.go`, `decision.go`, `chat_history.go`, `worker_events.go`

- **`orchestrator.go`** — Central control loop. Polls GitHub for candidate issues (open, no `stage:*` label, in active milestone). Uses an LLM session to pick the next ticket based on dependency analysis and priority labels. Creates a `Worker` and delegates the full ticket lifecycle. Supports pause/resume, tracks processing state. Broadcasts stage changes via WebSocket hub.
- **`worker.go`** — `Worker` owns a ticket from pickup to terminal state. Runs the pipeline: analysis → coding → code review → PR creation → approval → merge. Blocks on `decisionCh` at the approval stage waiting for user input (approve/decline). On decline, fixes code and loops back. On approve, merges PR. Returns only when done, failed, or context cancelled.
- **`task.go`** — `Task` struct holds issue data, branch name, status, session ID, and chat history. Defines all task statuses (`pending`, `analyzing`, `planning`, `coding`, `reviewing`, `creating_pr`, `awaiting_approval`, `merging`, `done`, `failed`).
- **`decision.go`** — `UserDecision` struct with action (`approve`/`decline`) and optional decline reason.
- **`chat_history.go`** — Thread-safe ring buffer (default max 1000 messages) storing user/assistant messages with timestamps. Used for displaying conversation history in the dashboard.
- **`worker_events.go`** — Event broadcasting helpers for worker status updates.

### `opencode/` — opencode Client & Server

**Files:** `client.go`, `server.go`

- **`client.go`** — HTTP client for the opencode API. Creates sessions, sends messages (with SSE streaming), lists messages, manages models. Supports structured JSON output format with retry. Handles model references (`provider/model` format). Parses SSE event streams for real-time response streaming. ~815 lines covering the full opencode HTTP/SSE API surface.
- **`server.go`** — Manages a spawned `opencode serve` subprocess. Starts the process, waits for health check (`/global/health`), and provides `Stop()` for graceful shutdown.

### `pipeline/` — Stage Machine

**Files:** `pipeline.go`, `stage.go`

- **`pipeline.go`** — Generic pipeline runner with retry logic. Executes stages sequentially via a `StageExecutor` interface. On failure, retries up to `maxRetries` times, falling back to the stage's `RetryTarget()`. Calls `onStageChange` callback on each transition.
- **`stage.go`** — Defines all pipeline stages as a `Stage` type with constants: `queued`, `analysis`, `coding`, `code-review`, `create-pr`, `awaiting-approval`, `merging`, `done`, `failed`, `blocked`. Each stage knows its:
  - `Next()` stage in the happy path
  - `Column()` mapping to dashboard columns
  - `Label()` for the corresponding GitHub `stage:*` label
  - `RetryTarget()` — all retries go back to `coding`

### `plan/` — Implementation Plan

**Files:** `plan.go`, `attachment.go`

- **`plan.go`** — `Plan` struct with analysis text, implementation steps (ordered, with affected files and details), and test plan. Converts to Markdown format. Parses step descriptions from LLM output. Saves plans as `.md` files in the repo.
- **`attachment.go`** — `AttachmentManager` creates implementation plans and posts them as GitHub issue comments. Parses structured plan content from LLM responses, formats as a technical plan comment with analysis, steps, and file lists.

### `preflight/` — Startup Checks

**Files:** `preflight.go`

- Runs startup validation before ODA can operate:
  - `CheckGitRepo()` — verifies `.git` directory exists
  - `CheckGhCLI()` — verifies `gh` is in PATH
  - `CheckGhAuth()` — verifies `gh auth status` succeeds
  - `CheckOpencode()` — verifies opencode API is reachable (`/global/health`)
  - `CheckOdaConfig()` — verifies `.oda/config.yaml` exists
  - `CheckOpencodeInstalled()` — verifies `opencode` binary is in PATH
- Platform detection (macOS, Linux apt/dnf/pacman, Windows) for install instructions.
- `RunAll()` executes all checks with a progress callback.

### `scheduler/` — Sprint Planning

**Files:** `planner.go`, `epic.go`

- **`planner.go`** — `Planner` creates sprints by fetching open issues, sending them to an LLM for prioritization and grouping, and creating a milestone. Uses structured JSON output for reliable parsing. All prompts include the `automatedPipelineNotice` to prevent LLM from asking questions.
- **`epic.go`** — Epic decomposition. Takes a high-level epic description, uses an LLM to break it into concrete implementation issues, and creates them on GitHub with proper labels and milestone assignment.

### `setup/` — One-Time Setup

**Files:** `setup.go`

- `Setup.CheckAndGenerate()` runs on every ODA start. Checks for `AGENTS.md` — if missing, generates a template based on detected project language. Checks for GitHub Actions CI workflow — if missing, generates a basic CI pipeline. Uses LLM for intelligent template generation when available.

### `worker/` — Worker Pool & Processor

**Files:** `pool.go`, `processor.go`, `worker.go`

- **`pool.go`** — `Pool` manages a fixed number of `Worker` goroutines. Each worker pulls tasks from a `TaskQueue` interface and processes them via a `TaskProcessor` interface. Provides `Workers()` for status reporting.
- **`processor.go`** — `Processor` implements `TaskProcessor`. Executes each pipeline stage by constructing prompts and sending them to opencode sessions. Stage-specific logic:
  - **Analysis** — sends issue details, expects structured JSON analysis (requirements, affected files, risks, complexity)
  - **Coding** — sends issue + plan + tool commands, expects implementation with tests
  - **Code Review** — sends diff for AI review, expects approve/reject decision
  - **Create PR** — pushes branch, creates PR via `gh pr create`
  - **Merge** — merges PR via `gh pr merge`
  - All prompts include `automatedPipelineNotice` to prevent LLM from blocking on questions
- **`worker.go`** — `Worker` struct with status tracking (`idle`/`working`), task assignment, elapsed time. Thread-safe via `sync.RWMutex`.

## Dependencies

Key external dependencies (from `go.mod`):

| Module | Purpose |
|--------|---------|
| `github.com/google/uuid` | UUID generation for session IDs |
| `github.com/gorilla/websocket` | WebSocket connections for real-time dashboard updates |
| `gopkg.in/yaml.v3` | YAML config parsing |
| `modernc.org/sqlite` | Pure-Go SQLite driver (no CGO required) |
