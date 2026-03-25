# Architecture

## Orchestrator Loop

The orchestrator (`internal/mvp/orchestrator.go`) is the central control loop:

1. Polls the oldest open GitHub milestone for candidate issues (no `stage:*` label = backlog)
2. Uses an LLM to pick the highest-priority ticket that unblocks the most work
3. Hands the ticket to a `Worker` which owns the full lifecycle until a terminal state (done/failed)
4. Worker blocks on a `decisionCh` channel at the approval stage, waiting for user input from the dashboard

## State Machine

Tickets flow through stages tracked by GitHub labels with the `stage:` prefix:

```
Backlog -> Plan -> Code -> AI Review -> Create PR -> Check Pipeline -> Approve -> Merge -> Done
```
Backlog -> Plan -> Code -> AI Review -> Create PR -> Approve -> Merge -> Done
                                                                  |
Any State -----> Failed -----> [Retry] -----> Backlog
```

All stage transitions go through `github.Client.SetStageLabel()` which atomically removes old `stage:*` labels and adds the new one. See [state-machine.md](state-machine.md) for the full specification.

## Worker Pool

- Workers are goroutines managed by `internal/worker/pool.go`
- Each worker gets a dedicated git branch for isolation
- The processor (`internal/worker/processor.go`) executes each pipeline stage by sending prompts to opencode sessions
- Configurable worker count (default: 3)

## LLM Routing

The LLM router (`internal/llm/router.go`) supports multi-model configuration with 5 independent modes:

- **setup** — project scaffolding and initialization
- **planning** — analysis, plan creation, plan review
- **orchestration** — ticket selection, sprint planning
- **code** — implementation (standard complexity)
- **code-heavy** — implementation (high complexity)

Each mode has strong/weak model variants. Routing rules determine which variant to use based on code size, file count, and complexity thresholds.

## Dashboard

The HTMX dashboard (`internal/dashboard/`) provides:

- Real-time Kanban board (Backlog, Plan, Code, AI Review, Check Pipeline, Approve, Merge, Done, Failed, Blocked)
- Worker status with elapsed time
- Issue actions (approve/decline PR, retry, cancel, block/unblock)
- Wizard for epic decomposition (LLM-powered issue generation)
- WebSocket-based live updates (no polling)
- Sync service that periodically refreshes GitHub state (30s interval)
