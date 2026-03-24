# Centralize Stage Changes in Orchestrator — Implementation Plan

> **For Claude:** REQUIRED SUB-SKILL: Use superpowers:executing-plans to implement this plan task-by-task.

**Goal:** Make orchestrator the single owner of all stage transitions. Dashboard and worker request changes via orchestrator's public API — they never touch GitHub labels, cache, or ledger directly.

**Architecture:** Orchestrator exposes `ChangeStage(issueNumber, stage, reason)` as a public, synchronous method. Dashboard handlers call it instead of `StageManager.ChangeStage()`. Worker processor calls it instead of `gh.SetStageLabel()` + `store.SaveStageChange()`. The `StageChangeReason` enum moves from `dashboard/stage.go` to `github/reasons.go` (alongside `Stage` enum) so all packages can use it without importing dashboard. `StageManager` struct is deleted entirely.

**Tech Stack:** Go 1.24, existing `github`, `mvp`, `dashboard`, `worker` packages.

---

### Task 1: Create `github/reasons.go` — StageChangeReason enum

**Files:**
- Create: `internal/github/reasons.go`
- Create: `internal/github/reasons_test.go`

**What:**
Move `StageChangeReason` type from `dashboard/stage.go` to `github/reasons.go`. Add missing worker pipeline reasons. Improve `String()` descriptions.

**Enum values:**

| Constant | DB value | String() description |
|---|---|---|
| `ReasonManualApprove` | `manual_approve` | `User approved issue via dashboard` |
| `ReasonManualReject` | `manual_reject` | `User rejected issue via dashboard, moved to backlog` |
| `ReasonManualRetry` | `manual_retry` | `User requested retry via dashboard` |
| `ReasonManualRetryFresh` | `manual_retry_fresh` | `User requested fresh retry via dashboard, PR closed and steps cleared` |
| `ReasonManualBlock` | `manual_block` | `User blocked issue via dashboard` |
| `ReasonManualUnblock` | `manual_unblock` | `User unblocked issue via dashboard, moved to backlog` |
| `ReasonManualDecline` | `manual_decline` | `User declined PR via dashboard, sent back for fixes` |
| `ReasonManualMerge` | `manual_merge` | `User approved and merged PR via dashboard` |
| `ReasonManualMergeFailed` | `manual_merge_failed` | `Merge failed (likely conflict), PR closed` |
| `ReasonWorkerAlreadyDone` | `worker_already_done` | `Worker detected ticket already completed, closing` |
| `ReasonWorkerFailed` | `worker_failed` | `Worker pipeline failed` |
| `ReasonWorkerApprove` | `worker_approve` | `Worker completed pipeline, awaiting manual approval` |
| `ReasonWorkerCompletedAnalysis` | `worker_completed_analysis` | `Worker completed analysis, advancing to coding` |
| `ReasonWorkerCompletedCoding` | `worker_completed_coding` | `Worker completed coding, advancing to code-review` |
| `ReasonWorkerCompletedCodeReview` | `worker_completed_code_review` | `Worker completed code-review, advancing to create-pr` |
| `ReasonWorkerCompletedCreatePR` | `worker_completed_create_pr` | `Worker completed PR creation, advancing to approval` |
| `ReasonWorkerNeedsUser` | `worker_needs_user` | `Worker needs user intervention` |
| `ReasonWorkerBlocked` | `worker_blocked` | `Worker pipeline blocked` |
| `ReasonSyncInitial` | `sync_initial` | `Initial sync from GitHub` |
| `ReasonSyncPeriodic` | `sync_periodic` | `Periodic sync from GitHub` |
| `ReasonSyncManual` | `sync_manual` | `Manual sync from GitHub` |

**Step 1:** Create `internal/github/reasons.go` with the full enum, `Label() string` method (returns DB value), and `String() string` method (returns description).

**Step 2:** Create `internal/github/reasons_test.go` — test that all reasons have non-empty Label() and String(), test round-trip.

**Step 3:** Run `go test ./internal/github/...` — verify pass.

**Step 4:** Commit: `feat(github): add StageChangeReason enum to github package`

---

### Task 2: Make orchestrator's `changeStage` public and accept `StageChangeReason`

**Files:**
- Modify: `internal/mvp/orchestrator.go`

**What:**
- Rename `changeStage` → `ChangeStage` (public)
- Change signature: `reason string` → `reason github.StageChangeReason`
- Update `SaveStageChange` call to use `reason.Label()` for DB value
- Update all internal call sites in orchestrator (`Run()` post-process, `HandleWorkerEvent`, `BroadcastStageUpdate`)
- `decideNextStage` returns `(github.Stage, github.StageChangeReason, bool)` instead of `(github.Stage, bool)` — so the reason is determined by the state machine, not hardcoded at call site

**Updated `decideNextStage` mapping:**

| event.Stage | event.Status | → next Stage | → Reason |
|---|---|---|---|
| `analysis` | success | `StageCode` | `ReasonWorkerCompletedAnalysis` |
| `coding` | success | `StageReview` | `ReasonWorkerCompletedCoding` |
| `code-review` | success | `StageCreatePR` | `ReasonWorkerCompletedCodeReview` |
| `create-pr` | success | `StageApprove` | `ReasonWorkerCompletedCreatePR` |
| any | failed | `StageFailed` | `ReasonWorkerFailed` |
| any | blocked | `StageNeedsUser` | `ReasonWorkerBlocked` |

**Step 1:** Update `changeStage` → `ChangeStage`, update signature, update all internal callers.

**Step 2:** Update `decideNextStage` to return 3 values.

**Step 3:** Update `HandleWorkerEvent` to use 3-value return.

**Step 4:** Update `BroadcastStageUpdate` to accept `github.StageChangeReason`.

**Step 5:** Run `go build ./...` — verify compilation.

**Step 6:** Commit: `refactor(orchestrator): make ChangeStage public with StageChangeReason enum`

---

### Task 3: Dashboard handlers call `orchestrator.ChangeStage()` instead of `stageManager.ChangeStage()`

**Files:**
- Modify: `internal/dashboard/handlers.go`
- Modify: `internal/dashboard/server.go`

**What:**
Replace every `s.stageManager.ChangeStage(...)` call with `s.orchestrator.ChangeStage(...)`. The orchestrator is already available as `s.orchestrator *mvp.Orchestrator` on the Server struct.

**Handler mapping:**

| Handler | Stage | Reason | Extra logic (stays in handler) |
|---|---|---|---|
| `handleApprove` | `StageApprove` | `ReasonManualApprove` | — |
| `handleReject` | `StageBacklog` | `ReasonManualReject` | — |
| `handleRetry` | `StageCode` | `ReasonManualRetry` | — |
| `handleRetryFresh` | `StageBacklog` | `ReasonManualRetryFresh` | Close PR, delete steps (before stage change) |
| `handleBlock` | `StageBlocked` | `ReasonManualBlock` | — |
| `handleUnblock` | `StageBacklog` | `ReasonManualUnblock` | — |
| `handleDecline` | `StageCode` | `ReasonManualDecline` | Add comment, delete steps (after stage change) |
| `handleApproveMerge` | `StageMerge` → merge → `StageDone` or `StageFailed` | `ReasonManualMerge` / `ReasonManualMergeFailed` | Find PR, merge PR, close PR on failure, add comment |

**Note on `handleApproveMerge`:** This handler has complex business logic (find PR → set merge stage → attempt merge → on success set done / on failure close PR + set failed). The stage changes go through orchestrator, but the PR merge logic stays in the handler (it's dashboard-specific user action, not state machine logic).

**Step 1:** Replace all `s.stageManager.ChangeStage(...)` with `s.orchestrator.ChangeStage(...)` using `github.ReasonXxx` constants.

**Step 2:** Remove `stageManager` field from `Server` struct and its initialization in `NewServer`.

**Step 3:** Run `go build ./...` — verify compilation.

**Step 4:** Commit: `refactor(dashboard): route all stage changes through orchestrator`

---

### Task 4: Delete `StageManager` and reason enum from `dashboard/stage.go`

**Files:**
- Modify: `internal/dashboard/stage.go`

**What:**
Remove `StageManager` struct, `NewStageManager`, `ChangeStage` method, `getStageFromIssue` method, and the entire `StageChangeReason` enum (now lives in `github/reasons.go`). The file should only contain imports if anything else references it, or be deleted entirely if empty.

**Step 1:** Remove all `StageManager`-related code and the `StageChangeReason` enum from `dashboard/stage.go`. If the file becomes empty, delete it.

**Step 2:** Run `go build ./...` — verify compilation.

**Step 3:** Commit: `refactor(dashboard): remove StageManager, reason enum moved to github package`

---

### Task 5: Worker processor calls `orchestrator.ChangeStage()` instead of direct GitHub API

**Files:**
- Modify: `internal/worker/processor.go`

**What:**
The `Processor` currently calls `p.gh.SetStageLabel()` + `p.store.SaveStageChange()` directly in two places (lines 195-201 and 207-213). Replace with orchestrator call.

**Problem:** `Processor` doesn't have access to orchestrator. It has `gh`, `store`, `brMgr`, but not orchestrator (circular dependency risk: orchestrator → worker → orchestrator).

**Solution:** Add a `StageChanger` interface to `worker` package:

```go
type StageChanger interface {
    ChangeStage(issueNumber int, stage github.Stage, reason github.StageChangeReason) error
}
```

Pass it to `NewProcessor`. Orchestrator implements this interface. No circular dependency because the interface is defined in `worker` package.

**Step 1:** Add `StageChanger` interface to `worker/processor.go`. Add field to `Processor` struct. Update `NewProcessor` signature.

**Step 2:** Replace `p.gh.SetStageLabel()` + `p.store.SaveStageChange()` with `p.stageChanger.ChangeStage()`.

**Step 3:** Update `NewWorker` / wherever `NewProcessor` is called to pass orchestrator.

**Step 4:** Run `go build ./...` — verify compilation.

**Step 5:** Commit: `refactor(worker): route stage changes through orchestrator via StageChanger interface`

---

### Task 6: Clean up orchestrator's `BroadcastStageUpdate` 

**Files:**
- Modify: `internal/mvp/orchestrator.go`

**What:**
`BroadcastStageUpdate` currently duplicates the full `ChangeStage` logic (get from cache, set label, save ledger, broadcast). Now that `ChangeStage` is public, `BroadcastStageUpdate` should just call `ChangeStage`. Check if it's still called anywhere — if not, remove it.

**Step 1:** Check all callers of `BroadcastStageUpdate`. If none, delete it. If some exist, replace body with `o.ChangeStage(...)` call.

**Step 2:** Run `go build ./...` and `go test ./...` — verify everything passes.

**Step 3:** Commit: `refactor(orchestrator): remove redundant BroadcastStageUpdate`

---

### Task 7: Update tests

**Files:**
- Modify: `internal/github/labels_test.go` (if needed)
- Modify: `internal/mvp/orchestrator_test.go` (if exists)
- Verify: `internal/dashboard/handlers_test.go` (pre-existing failures, don't fix)

**Step 1:** Run `go test ./...` — identify any new failures from our changes.

**Step 2:** Fix any test compilation errors caused by removed `StageManager` or changed signatures.

**Step 3:** Run `go test ./...` — verify all pass (except pre-existing `sync_test.go` / `handlers_test.go` failures).

**Step 4:** Run `go vet ./...` — verify clean.

**Step 5:** Commit: `test: update tests for centralized stage changes`

---

## Summary of changes

| Before | After |
|---|---|
| 3 places change stages (orchestrator, dashboard StageManager, worker processor) | 1 place: `orchestrator.ChangeStage()` |
| `StageChangeReason` in `dashboard/stage.go` | `StageChangeReason` in `github/reasons.go` |
| Orchestrator uses bare strings for reasons | Orchestrator uses `github.StageChangeReason` enum |
| Worker calls `gh.SetStageLabel()` directly | Worker calls `StageChanger.ChangeStage()` interface |
| `StageManager` struct in dashboard | Deleted |
| `changeStage` private method | `ChangeStage` public method |
