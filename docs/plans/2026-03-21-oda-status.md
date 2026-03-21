# ODA (One Dev Army) - Current Status

## Summary

MVP skeleton is built. All components exist, compile, and pass tests. The project needs one more integration round to work end-to-end with real opencode serve and GitHub.

## What Works

| Component | Status | Notes |
|---|---|---|
| Config loader | Done | Reads `.oda/config.yaml`, all fields parsed |
| Preflight checks | Done | git, gh, opencode, config detection with platform-specific instructions |
| SQLite metrics | Done | Save/query stage metrics, sprint costs |
| Git worktree manager | Done | Create/remove/list worktrees, run commands in worktree |
| Opencode HTTP client | Done | Health check, sessions, messages (sync/async), abort |
| Pipeline state machine | Done | Full stage progression, retry logic, blocking after 5 retries |
| Worker pool | Done | Goroutine-based, context cancellation, task queue interface |
| Task processor | Done | Connects pipeline to opencode sessions, records metrics |
| Dashboard | Partial | Renders pages with HTMX, but uses placeholder data |
| `oda init` | Partial | Creates config with defaults, no LLM scanning |
| Sprint planner | Partial | Logic exists, not wired to dashboard |
| Epic analyzer | Partial | Logic exists, not wired to dashboard |
| Metrics writer | Done | Dual write to SQLite + GitHub YAML comments |
| Graceful shutdown | Done | Two-stage Ctrl+C (graceful then force) |
| Integration tests | Done | 4 integration tests with mock opencode server |

## What Needs Work

### Priority 1: Core Integration (required for end-to-end)

1. **Task queue from GitHub** - Implement `TaskQueue` interface that reads tasks from GitHub milestone, respects dependencies (linked issues), and skips blocked tasks. Currently only a mock queue exists.

2. **Dashboard wiring** - Connect dashboard handlers to real data:
   - `GET /` should read sprint tasks from GitHub via gh client
   - `GET /api/workers` already works with real pool
   - `POST /epic` should call `EpicAnalyzer.Analyze()` + `CreateIssues()`
   - `POST /plan-sprint` should call `Planner.PlanSprint()`
   - `POST /approve/{id}` should unblock task and re-queue
   - `POST /reject/{id}` should cancel task
   - `POST /sync` should trigger GitHub -> SQLite sync

3. **GitHub Project board sync** - Move cards between columns as tasks progress through pipeline stages. Currently labels are created but cards are not moved.

### Priority 2: LLM Integration (required for useful output)

4. **`oda init` LLM scanning** - Use opencode session to scan codebase, detect stack, suggest config values, generate GitHub Actions CI.

5. **Prompt engineering** - Current prompts are basic templates. Need testing and refinement with real opencode serve to produce useful analysis, plans, reviews, and code.

6. **JSON response parsing hardening** - LLM responses are unpredictable. The `extractJSON` helper exists but needs more edge case handling.

### Priority 3: Robustness (required for production use)

7. **Merge conflict handling** - Pipeline has retry logic, but no code for `git rebase` + conflict resolution + task restart from scratch after 3 failed attempts.

8. **GitHub Actions generation** - `oda init` should generate CI workflow adapted to detected stack. Currently skipped.

9. **Sprint completion flow** - After all tasks in sprint are done: run `Planner.AnalyzeInsights()`, create sprint summary, close milestone.

10. **Dashboard live logs** - Click on task/worker should show pipeline logs. Currently drill-down pages are placeholder.

11. **Cost reporting** - Dashboard costs page exists but shows placeholder data. Wire to `store.GetSprintCost()` and per-task metrics.

## Architecture

```
internal/
├── config/        Config loader (.oda/config.yaml)
├── preflight/     Environment checks (git, gh, opencode)
├── db/            SQLite metrics storage
├── github/        gh CLI wrapper (issues, labels, projects, milestones)
├── opencode/      HTTP client for opencode serve API
├── git/           Git worktree manager
├── pipeline/      Stage state machine + executor interface
├── worker/        Worker pool + task processor
├── dashboard/     HTMX + Go templates HTTP server
├── initialize/    oda init command
├── scheduler/     Sprint planner + epic analyzer
├── metrics/       Dual write (SQLite + GitHub comments)
└── integration_test.go
```

## Test Coverage

11 packages, all passing:
- `config` - 7 tests
- `preflight` - 10 tests
- `db` - 4 tests
- `opencode` - 11 tests
- `git` - 3 tests
- `pipeline` - 8 tests
- `worker` - 11 tests (4 pool + 7 processor)
- `initialize` - 4 tests
- `metrics` - 5 tests
- `scheduler` - 18 tests
- `integration` - 4 tests

## Next Steps

To get ODA working end-to-end:
1. Implement GitHub-backed TaskQueue
2. Wire dashboard to real data
3. Test with real opencode serve instance
4. Iterate on prompts until pipeline produces useful code
