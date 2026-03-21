# ODA (One Dev Army) - Design Document

## Vision

A single Go binary that turns a solo developer into a full scrum team. The agent acts as Scrum Master + Product Owner, orchestrating workers (goroutines) that communicate with opencode serve via HTTP API. Dashboard built with HTMX + Go templates. GitHub as source of truth, SQLite for metrics.

## Key Assumptions

- **Solo developer** - one programmer, agent handles the rest
- **Lean scrum** - MVP: sprint planning + backlog refinement
- **Sprint = batch of tasks** - no fixed time frames, sprint is N tasks
- **One repo = one agent** - separate instance per project
- **Config and SQLite** - stored in `.oda/` directory within the project

## Architecture

```
┌──────────────────────────────────────────────────┐
│                  ODA Agent (Go binary)            │
│                                                   │
│  ┌──────────┐  ┌───────────┐  ┌───────────────┐  │
│  │ Dashboard │  │ Scheduler │  │ GitHub Client │  │
│  │ HTMX+Go  │  │ (Sprint   │  │ (gh CLI)      │  │
│  │ templates │  │  Planner) │  │ (Issues, PRs, │  │
│  │ :8080    │  │           │  │  Projects,    │  │
│  └──────────┘  └───────────┘  │  Milestones)  │  │
│                               └───────────────┘  │
│  ┌──────────────────────────────────────────┐    │
│  │            Worker Pool                    │    │
│  │  ┌────────┐  ┌────────┐  ┌────────┐      │    │
│  │  │Worker 1│  │Worker 2│  │Worker N│      │    │
│  │  │goroutin│  │goroutin│  │goroutin│      │    │
│  │  └───┬────┘  └───┬────┘  └───┬────┘      │    │
│  │      │           │           │            │    │
│  │      ▼           ▼           ▼            │    │
│  │   git worktree per worker                 │    │
│  └──────────────────────────────────────────┘    │
│                                                   │
│  ┌──────────┐  ┌─────────────────────────────┐   │
│  │ SQLite   │  │ opencode serve (HTTP API)   │   │
│  │ (metrics)│  │ (single instance, multi     │   │
│  │          │  │  session)                   │   │
│  └──────────┘  └─────────────────────────────┘   │
└──────────────────────────────────────────────────┘
```

## Initialization

### `oda init`

One-time setup command. LLM scans the repo and adapts to what it finds.

#### Empty repo

1. Asks user for project description
2. LLM analyzes description, suggests stack and structure
3. Creates `.oda/config.yaml` with defaults
4. Configures GitHub (labels, project board, milestone)
5. Generates GitHub Actions CI (LLM adapts to chosen stack)
6. Creates first sprint - LLM breaks project description into tasks, creates issues

#### Existing repo

1. LLM scans codebase - detects stack, structure, test framework, lint tools
2. Creates `.oda/config.yaml` with detected settings (lint_cmd, test_cmd, etc.)
3. Configures GitHub (labels, project board, milestone)
4. Generates/updates GitHub Actions CI (LLM adapts to detected stack)
5. Imports existing GitHub issues into backlog
6. Optionally asks user if they want to plan first sprint

### `oda init` is one-time only

Config file `.oda/config.yaml` is created once. GitHub configuration (labels, project board, actions, milestones) is verified and repaired programmatically on every `oda` startup - not via `oda init`.

## Startup

```bash
oda
```

Looks for `.oda/config.yaml` in the current directory.

### Preflight Checks (ordered)

1. Check git repo exists in current directory
2. Check `gh` is installed and authenticated
3. Check opencode serve is reachable
4. Check `.oda/config.yaml` exists (error if not - run `oda init` first)

### Missing git repo

Agent shows error and instructions:
- `git init && git remote add origin <url>` for new projects
- `git clone <url>` for existing projects

### Missing opencode

If opencode serve is not reachable, agent detects the platform and shows installation instructions:
- **macOS**: `brew install opencode`
- **Linux**: `curl -fsSL https://opencode.ai/install | sh`
- **Windows**: `scoop install opencode`
- Plus instructions to start: `opencode serve`

### Missing or unauthenticated gh CLI

Agent checks for `gh` binary and authentication status (`gh auth status`). If missing or not logged in:

**Not installed:**
- **macOS**: `brew install gh`
- **Linux**: instructions for apt/dnf/pacman depending on detected distro
- **Windows**: `scoop install gh` or `winget install GitHub.cli`

**Not authenticated:**
- Shows: `Run: gh auth login` with step-by-step guidance

### GitHub Setup Verification (every startup)

Agent programmatically verifies and repairs GitHub repo configuration:
- GitHub Project board exists (create if missing)
- Required labels exist: `sprint`, `insight`, `size:S`, `size:M`, `size:L`, `size:XL`, stage labels
- Pipeline stage labels: `stage:analysis`, `stage:planning`, `stage:plan-review`, `stage:coding`, `stage:testing`, `stage:code-review`, `stage:needs-user`
- Issues are enabled on the repo
- Agent has sufficient permissions (read/write issues, PRs, projects)

If anything is missing, agent fixes it automatically.

### Normal Startup

1. Preflight checks pass
2. GitHub setup verification and repair
3. Sync GitHub -> SQLite (issues, milestones, project board state)
4. Initialize worker pool (goroutines)
5. Create git worktrees per worker
6. Start dashboard HTTP server

### Graceful Shutdown (Ctrl+C)

Agent prompts user: "Workers are running. Force quit or wait?"
- **Wait** - workers finish current pipeline stage, save state to SQLite, then shutdown
- **Force** - abort opencode sessions, save state to SQLite, shutdown
- On next startup, agent reads state from GitHub and resumes from last completed stage

## Configuration

File: `.oda/config.yaml`

```yaml
github:
  repo: "owner/repo"  # auto-detected from git remote, can override

dashboard:
  port: 8080

workers:
  count: 3

opencode:
  url: "http://localhost:4096"

tools:
  lint_cmd: "make lint"
  test_cmd: "make test"
  e2e_cmd: "make e2e"

pipeline:
  stages:
    - name: analysis
      llm: claude-sonnet-4
    - name: planning
      llm: claude-opus-4
    - name: plan-review
      llm: claude-opus-4
    - name: coding
      llm: claude-sonnet-4
    - name: testing
      llm: claude-sonnet-4
    - name: code-review
      llm: claude-opus-4
    - name: merge
      manual_approval: true

  max_retries: 5

planning:
  llm: claude-opus-4

epic_analysis:
  llm: claude-sonnet-4

sprint:
  tasks_per_sprint: 10
```

## Opencode Integration

### Single opencode serve instance

ODA connects to one running opencode serve instance. All workers create separate sessions on it. Opencode natively supports multi-session.

### Key API endpoints used

| Endpoint | Purpose |
|---|---|
| `GET /global/health` | Preflight health check |
| `POST /session` | Create new session per pipeline stage |
| `POST /session/:id/message` | Send prompt with `model` param for LLM selection |
| `POST /session/:id/prompt_async` | Async prompt (non-blocking) |
| `GET /session/:id/message` | Read results |
| `POST /session/:id/abort` | Abort session on force quit |
| `GET /event` | SSE stream for monitoring progress |

### Agent as orchestrator

- Agent creates a new opencode session for each pipeline stage of each task
- Agent manages branches and worktrees before starting work
- Agent merges and closes PRs after task completion
- Agent can use its own LLM for meta-tasks (commit messages, PR descriptions)
- Workers respond in JSON format for easy result parsing
- Coding and review stages receive tool commands (lint, test, e2e) as part of the prompt context

## Task Pipeline

Every task (GitHub Issue) goes through the pipeline:

```
GitHub Issue
    │
    ▼
┌─ analysis ──── LLM: configurable ───────────┐
│  Analyze requirements, code context          │
└──────────────────────────────────────────────┘
    │
    ▼
┌─ planning ──── LLM: configurable ───────────┐
│  Implementation plan, steps, files to change │
└──────────────────────────────────────────────┘
    │
    ▼
┌─ plan-review ── LLM: configurable ──────────┐
│  Plan code review                            │
│  REJECT -> retry (max 5, then blocked)       │
└──────────────────────────────────────────────┘
    │
    ▼
┌─ coding ─────── LLM: configurable ──────────┐
│  Implementation via opencode serve           │
│  Receives lint/test/e2e commands in prompt   │
└──────────────────────────────────────────────┘
    │
    ▼
┌─ testing ────── LLM: configurable ──────────┐
│  Run unit tests + e2e tests                  │
│  FAIL -> retry coding (max 5, then blocked)  │
└──────────────────────────────────────────────┘
    │
    ▼
┌─ code-review ── LLM: configurable ──────────┐
│  Code review                                 │
│  REJECT -> retry coding (max 5, then blocked)│
└──────────────────────────────────────────────┘
    │
    ▼
┌─ merge ─────────────────────────────────────┐
│  Creates PR on GitHub                        │
│  manual_approval: true -> waits for user     │
│  manual_approval: false -> auto merge        │
└──────────────────────────────────────────────┘
```

### Retry Logic

- Review stages (plan-review, code-review) and testing can reject/fail
- Rejection -> returns to previous stage (planning or coding)
- Max 5 attempts per review/test cycle
- After 5 attempts -> task moves to Blocked with `stage:needs-user` label, waits for user decision on dashboard

### Merge Conflict Handling

Workers operate in parallel on separate branches/worktrees. Conflicts can occur on merge:
1. Worker attempts rebase on main before merge
2. If conflict -> worker tries to resolve (max 3 attempts)
3. If still unresolved -> worker restarts the entire task from scratch (new branch, fresh implementation)
4. Every task must pass unit + e2e tests to validate correctness after conflict resolution

### Worker Flow

1. Worker picks next available task from sprint queue (respects dependencies)
2. Creates branch `task/{issue_id}-{slug}`
3. Works on dedicated git worktree
4. Creates new opencode session for each pipeline stage
5. Goes through pipeline stages
6. On completion -> writes insights as comment in PR/issue
7. Writes metrics as YAML comment in issue/PR + SQLite
8. Cleans up worktree after merge
9. Automatically picks next task from queue

### Task Dependencies

- Defined via native GitHub issue links (linked issues)
- Worker checks if all dependencies are in Done state before picking a task
- If dependencies not met -> worker skips task and picks next available one

## Task State Machine

GitHub Project board columns serve as the state machine. Detailed pipeline stage tracked via labels.

### Project Board Columns (6)

```
Backlog | In Progress | Review | Merging | Done | Blocked
```

### State Mapping

| Pipeline stage | Column | Label |
|---|---|---|
| queued | Backlog | - |
| analysis | In Progress | `stage:analysis` |
| planning | In Progress | `stage:planning` |
| plan-review | Review | `stage:plan-review` |
| coding | In Progress | `stage:coding` |
| testing | In Progress | `stage:testing` |
| code-review | Review | `stage:code-review` |
| merging | Merging | - |
| done | Done | - |
| blocked (deps) | Blocked | - |
| blocked (5 retries) | Blocked | `stage:needs-user` |
| cancelled | Done | `stage:cancelled` |

Agent moves cards between columns and updates labels as tasks progress. On restart, agent reads state from GitHub Project board and resumes.

## Task Management

### Task Sources

1. **Dashboard** - form where user enters idea/epic
2. **GitHub** - user creates issue manually, agent detects on sync

### Epic -> Tasks

User submits idea/epic -> LLM (epic_analysis) breaks it down into tasks -> creates GitHub Issues with:
- Title
- Technical description (architecture, API, data models - no implementation details)
- Acceptance criteria
- Estimated size (S/M/L/XL)
- Dependencies (as GitHub issue links)
- Labels

### GitHub Sync

- Full sync during sprint planning
- "Sync now" button on dashboard

## Sprint on GitHub

- **Milestone** - sprint = milestone, issues assigned to milestone
- **GitHub Project (board)** - columns: Backlog, In Progress, Review, Merging, Done, Blocked
- Agent creates and synchronizes both

## Metrics and Insights

### Dual Write - GitHub + SQLite

On task completion, worker writes metrics as YAML comment in issue/PR:

```yaml
# ODA Metrics
task_id: 123
stage_metrics:
  analysis:
    llm: claude-sonnet-4
    tokens_in: 1200
    tokens_out: 800
    cost_usd: 0.012
    duration_s: 15
  planning:
    llm: claude-opus-4
    tokens_in: 2500
    tokens_out: 1500
    cost_usd: 0.045
    duration_s: 32
  plan-review:
    llm: claude-opus-4
    tokens_in: 1800
    tokens_out: 600
    cost_usd: 0.028
    duration_s: 12
  coding:
    llm: claude-sonnet-4
    tokens_in: 5000
    tokens_out: 3000
    cost_usd: 0.048
    duration_s: 90
  testing:
    llm: claude-sonnet-4
    tokens_in: 800
    tokens_out: 400
    cost_usd: 0.007
    duration_s: 45
  code-review:
    llm: claude-opus-4
    tokens_in: 4000
    tokens_out: 1200
    cost_usd: 0.058
    duration_s: 25
total:
  tokens: 23200
  cost_usd: 0.210
  duration_s: 219
  retries: 1
```

Same data stored in SQLite (`.oda/metrics.db`) for fast dashboard queries.

### Insights (Golden Nuggets)

1. Worker generates insights on task completion -> comment in PR/issue on GitHub
2. After sprint -> planner LLM analyzes all insights from sprint issues/PRs
3. Concrete ideas -> new GitHub issues with `insight` label
4. General observations -> comment in sprint meta-issue on GitHub
5. Insights feed into future sprint planning and refinement

## Dashboard (MVP)

### Main View: Sprint Board
- Columns: Backlog | In Progress | Review | Merging | Done | Blocked
- Task cards with status, assigned worker, pipeline stage label

### Worker Status
- Visible in real-time on dashboard
- Each worker: name, current task, pipeline stage, elapsed time

### Costs
- Tokens and $ per task, per sprint, per stage
- Visible on dashboard

### Drill-down
- Click on task -> pipeline logs, metrics, insights
- Click on active worker -> live logs

### Menu
- Sprint Board (main)
- Backlog (separate page)
- Costs/Reports

### Dashboard Actions
- Add epic/idea (form)
- Sync with GitHub (button)
- Plan sprint (button)
- Approve/Reject (blocked tasks and manual merge gate)

## Project Directory Structure

```
.oda/
├── config.yaml          # agent configuration
├── metrics.db           # SQLite with metrics
└── worktrees/           # git worktrees per worker
    ├── worker-1/
    ├── worker-2/
    └── worker-3/
```

## Tech Stack

- **Language**: Go
- **Frontend**: HTMX + Go html/template
- **Database**: SQLite (via go-sqlite3 or modernc.org/sqlite)
- **GitHub**: gh CLI (wrapping `gh` commands)
- **Git**: exec git CLI (worktrees, branches, merge)
- **HTTP server**: net/http (stdlib)
- **opencode**: HTTP client to opencode serve API

## Out of Scope (MVP)

- Daily standup
- Sprint review
- Sprint retro
- Burndown/velocity charts
- Definition of Done automation
- Multi-repo support
- Dashboard authentication
- Docker deployment
