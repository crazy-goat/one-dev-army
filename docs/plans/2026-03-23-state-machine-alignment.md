# State Machine Alignment Implementation Plan

> **For Claude:** REQUIRED SUB-SKILL: Use superpowers:executing-plans to implement this plan task-by-task.

**Goal:** Align the codebase (labels, worker, dashboard, pipeline) with the state machine defined in `docs/state-machine.md`, ensuring all stages use `stage:*` prefixed labels and the dashboard/worker correctly handle all transitions.

**Architecture:** The state machine defines 10 states with `stage:*` labels. The single source of truth for label management is `SetStageLabel()` in `internal/github/labels.go`. The MVP worker (`internal/mvp/`) is the active system; the pipeline worker (`internal/worker/` + `internal/pipeline/`) must be kept in sync but is not wired to the orchestrator. The dashboard reads labels to infer columns and provides action buttons for manual transitions.

**Tech Stack:** Go 1.24, HTML templates, GitHub API via `gh` CLI

---

## Key Design Decisions

1. **All stage labels use `stage:` prefix** — `stage:failed`, `stage:blocked`, `stage:awaiting-approval`, `stage:backlog`, `stage:create-pr`, `stage:merging` (matching state-machine.md)
2. **`Plan` stage uses single label `stage:analysis`** — remove `stage:planning` (not in state machine, was a pipeline-only concept)
3. **`Code` stage uses single label `stage:coding`** — remove `stage:testing` (not in state machine, was a pipeline-only concept)
4. **`Create PR` and `Merge` are new stages** in `StageToLabels` but NOT separate dashboard columns (they map to AI Review and Approve columns respectively, as the state machine suggests)
5. **Legacy labels** (`in-progress`, `stage:planning`, `stage:testing`, `stage:plan-review`, `stage:needs-user`, `stage:cancelled`) are removed from `RequiredLabels` but kept in `inferColumnFromIssue` for backward compatibility with existing issues

---

### Task 1: Update `StageToLabels` and `StageLabelPrefixes` in `labels.go`

**Files:**
- Modify: `internal/github/labels.go:11-28`
- Test: `internal/github/labels_test.go`

**Step 1: Update `StageToLabels` map**

Replace the current map with:

```go
var StageToLabels = map[string][]string{
	"Backlog":   {},
	"Plan":      {"stage:analysis"},
	"Code":      {"stage:coding"},
	"AI Review": {"stage:code-review"},
	"Create PR": {"stage:create-pr"},
	"Approve":   {"stage:awaiting-approval"},
	"Merge":     {"stage:merging"},
	"Done":      {},
	"Failed":    {"stage:failed"},
	"Blocked":   {"stage:blocked"},
}
```

Changes:
- `Plan`: removed `stage:planning` (single label now)
- `Code`: removed `stage:testing` (single label now)
- `Approve`: changed from `awaiting-approval` to `stage:awaiting-approval`
- `Failed`: changed from `failed` to `stage:failed`
- `Blocked`: changed from `blocked` to `stage:blocked`
- Added `"Create PR"` stage
- Added `"Merge"` stage

**Step 2: Simplify `StageLabelPrefixes`**

Replace with:

```go
var StageLabelPrefixes = []string{
	"stage:",
}
```

Since ALL stage labels now use `stage:` prefix, we only need one prefix. The old bare labels (`awaiting-approval`, `failed`, `blocked`) are no longer stage labels — but we need backward compatibility in `getStageLabelsToRemove` to also clean up old bare labels during transition.

Actually, for safety during migration, keep the old prefixes too:

```go
var StageLabelPrefixes = []string{
	"stage:",
	"awaiting-approval",
	"failed",
	"blocked",
	"in-progress",
}
```

This ensures `SetStageLabel` will clean up both old and new label formats.

**Step 3: Update `RequiredLabels`**

Replace the full list with:

```go
var RequiredLabels = []Label{
	{Name: "sprint", Color: "0E8A16"},
	{Name: "insight", Color: "D93F0B"},
	{Name: "size:S", Color: "C2E0C6"},
	{Name: "size:M", Color: "BFDADC"},
	{Name: "size:L", Color: "BFD4F2"},
	{Name: "size:XL", Color: "D4C5F9"},
	{Name: "stage:backlog", Color: "EEEEEE"},
	{Name: "stage:analysis", Color: "FBCA04"},
	{Name: "stage:coding", Color: "1D76DB"},
	{Name: "stage:code-review", Color: "1D76DB"},
	{Name: "stage:create-pr", Color: "1D76DB"},
	{Name: "stage:awaiting-approval", Color: "0E8A16"},
	{Name: "stage:merging", Color: "0E8A16"},
	{Name: "stage:failed", Color: "D93F0B"},
	{Name: "stage:blocked", Color: "B60205"},
	{Name: "priority:high", Color: "B60205"},
	{Name: "priority:medium", Color: "FBCA04"},
	{Name: "priority:low", Color: "0E8A16"},
	{Name: "epic", Color: "5319E7"},
	{Name: "wizard", Color: "7C3AED"},
	{Name: "merge-failed", Color: "D93F0B"},
}
```

Removed: `in-progress`, `failed` (bare), `stage:planning`, `stage:plan-review`, `stage:testing`, `stage:needs-user`, `stage:cancelled`, `awaiting-approval` (bare), `blocked` (bare)

Added: `stage:backlog`, `stage:create-pr`, `stage:awaiting-approval`, `stage:merging`, `stage:failed`, `stage:blocked`

**Step 4: Run tests to see what breaks**

Run: `go test ./internal/github/ -v -run TestStage 2>&1 | head -60`
Expected: Multiple test failures (tests reference old label names)

**Step 5: Update tests in `labels_test.go`**

Update all test cases to match new label names. Key changes:
- `TestStageToLabelsMapping`: Update expected values
- `TestIsStageLabel`: Update expected values (all `stage:*` are true, bare labels are false except during migration)
- `TestGetStageFromLabels`: Update expected values
- `TestStageLabelPrefixes`: Update expected prefixes
- `TestRequiredLabelsCount`: Update count
- `TestLabelStructure`: Update expected labels
- `TestStageTransitions`: Update expected adds/removes
- `TestStageLabelsExistInRequiredLabels`: Should pass with new labels
- `TestStageMappingsUseValidLabels`: Should pass with new labels
- Remove tests for bare labels: `TestAwaitingApprovalLabel`, `TestBlockedLabel`

**Step 6: Run tests to verify they pass**

Run: `go test ./internal/github/ -v 2>&1 | tail -10`
Expected: PASS

**Step 7: Commit**

```bash
git add internal/github/labels.go internal/github/labels_test.go
git commit -m "refactor: align stage labels with state machine - all use stage: prefix"
```

---

### Task 2: Fix MVP Worker stage label calls

**Files:**
- Modify: `internal/mvp/worker.go:195,223,231,240,294`
- Modify: `internal/mvp/orchestrator.go:226-294`
- Test: `internal/mvp/worker_test.go`

**Step 1: Fix `setStageLabel` calls in `worker.go`**

Current calls and their fixes:

| Line | Current | New | Reason |
|------|---------|-----|--------|
| 195 | `w.setStageLabel("Plan")` | `w.setStageLabel("Plan")` | OK, no change |
| 223 | `w.setStageLabel("stage:coding")` | `w.setStageLabel("Code")` | Was using raw label, should use stage name |
| 231 | `w.setStageLabel("stage:testing")` | DELETE this line | `stage:testing` no longer exists; Code stage covers implementation+testing |
| 240 | `w.setStageLabel("stage:code-review")` | `w.setStageLabel("AI Review")` | Was using raw label |
| 294 | `w.setStageLabel("Approve")` | `w.setStageLabel("Create PR")` | Should be Create PR stage, then Approve after PR is created |

Also add a `setStageLabel("Approve")` call AFTER the PR is successfully created (after line 302).

**Step 2: Fix orchestrator post-processing in `orchestrator.go`**

Current orchestrator uses `AddLabel`/`RemoveLabel` directly instead of `SetStageLabel`. Fix:

Lines 226-228 (picking up ticket):
```go
// OLD:
if err := o.gh.AddLabel(nextIssue.Number, "in-progress"); err != nil {
// NEW:
// Remove in-progress label usage - the worker sets stage labels via setStageLabel()
```

Remove the `AddLabel("in-progress")` call entirely. The worker already calls `setStageLabel("Plan")` at the start.

Lines 266-268 (already done):
```go
// OLD:
if err := o.gh.RemoveLabel(nextIssue.Number, "in-progress"); err != nil {
// NEW: Use SetStageLabel
if _, err := o.gh.SetStageLabel(nextIssue.Number, "Done"); err != nil {
```

Lines 276-281 (failed):
```go
// OLD:
if err := o.gh.RemoveLabel(nextIssue.Number, "in-progress"); err != nil { ... }
if err := o.gh.AddLabel(nextIssue.Number, "failed"); err != nil { ... }
// NEW:
if _, err := o.gh.SetStageLabel(nextIssue.Number, "Failed"); err != nil {
    log.Printf("[Orchestrator] Error setting Failed stage for #%d: %v", nextIssue.Number, err)
}
```

Lines 289-294 (success):
```go
// OLD:
if err := o.gh.RemoveLabel(nextIssue.Number, "in-progress"); err != nil { ... }
if err := o.gh.AddLabel(nextIssue.Number, "awaiting-approval"); err != nil { ... }
// NEW:
if _, err := o.gh.SetStageLabel(nextIssue.Number, "Approve"); err != nil {
    log.Printf("[Orchestrator] Error setting Approve stage for #%d: %v", nextIssue.Number, err)
}
```

Lines 229-231 (remove merge-failed):
```go
// Keep this - merge-failed is not a stage label, it's informational
if err := o.gh.RemoveLabel(nextIssue.Number, "merge-failed"); err != nil {
```

Lines 185 (blocking check):
```go
// OLD:
if hasLabel(issues[i], "awaiting-approval") || hasLabel(issues[i], "failed") {
// NEW:
if hasLabel(issues[i], "stage:awaiting-approval") || hasLabel(issues[i], "stage:failed") {
```

**Step 3: Run tests**

Run: `go test ./internal/mvp/ -v 2>&1 | tail -10`
Expected: PASS

**Step 4: Commit**

```bash
git add internal/mvp/worker.go internal/mvp/orchestrator.go
git commit -m "refactor: MVP worker uses stage names instead of raw labels"
```

---

### Task 3: Update Dashboard `inferColumnFromIssue` and handlers

**Files:**
- Modify: `internal/dashboard/handlers.go:189-228`
- Test: `internal/dashboard/handlers_test.go`

**Step 1: Rewrite `inferColumnFromIssue`**

Replace the function with a clean version that prioritizes `stage:*` labels:

```go
func inferColumnFromIssue(issue github.Issue) string {
	labels := issue.GetLabelNames()
	labelSet := make(map[string]bool)
	for _, l := range labels {
		labelSet[strings.ToLower(l)] = true
	}

	// Priority order matches state-machine.md Column Mapping
	if labelSet["stage:blocked"] || labelSet["blocked"] {
		return "Blocked"
	}
	if labelSet["stage:failed"] || labelSet["failed"] {
		return "Failed"
	}
	if labelSet["stage:merging"] {
		return "Approve" // Merge is part of Approve column
	}
	if labelSet["stage:awaiting-approval"] || labelSet["awaiting-approval"] {
		return "Approve"
	}
	if labelSet["stage:create-pr"] {
		return "AI Review" // Create PR is part of AI Review column
	}
	if labelSet["stage:code-review"] {
		return "AI Review"
	}
	if labelSet["stage:coding"] || labelSet["stage:testing"] || labelSet["in-progress"] {
		return "Code"
	}
	if labelSet["stage:analysis"] || labelSet["stage:planning"] {
		return "Plan"
	}

	if strings.EqualFold(issue.State, "CLOSED") {
		return "Done"
	}

	return "Backlog"
}
```

Note: Old bare labels (`blocked`, `failed`, `awaiting-approval`, `in-progress`, `stage:testing`, `stage:planning`) are kept for backward compatibility with existing issues that haven't been migrated yet.

**Step 2: Run tests**

Run: `go test ./internal/dashboard/ -v 2>&1 | tail -10`
Expected: PASS

**Step 3: Commit**

```bash
git add internal/dashboard/handlers.go
git commit -m "refactor: dashboard inferColumnFromIssue supports new stage: labels"
```

---

### Task 4: Fix Blocked column actions in `board.html`

**Files:**
- Modify: `internal/dashboard/templates/board.html:165-168`
- Modify: `internal/dashboard/server.go:139-181` (add new routes)
- Modify: `internal/dashboard/handlers.go` (add new handlers)

**Step 1: Add `handleUnblock` handler**

Add to `handlers.go`:

```go
func (s *Server) handleUnblock(w http.ResponseWriter, r *http.Request) {
	id := r.PathValue("id")
	issueNum := 0
	fmt.Sscanf(id, "%d", &issueNum)
	if issueNum == 0 || s.gh == nil {
		http.Redirect(w, r, "/", http.StatusSeeOther)
		return
	}

	// Set stage label to "Backlog" (removes blocked label)
	updatedIssue, err := s.gh.SetStageLabel(issueNum, "Backlog")
	if err != nil {
		log.Printf("[Dashboard] Error setting Backlog stage on #%d: %v", issueNum, err)
		http.Redirect(w, r, "/", http.StatusSeeOther)
		return
	}

	// Update cache
	if s.store != nil {
		milestone := s.activeSprintName()
		if err := s.store.SaveIssueCache(updatedIssue, milestone); err != nil {
			log.Printf("[Dashboard] Error saving issue cache for #%d: %v", issueNum, err)
		}
	}

	// Broadcast update via WebSocket
	if s.hub != nil {
		s.hub.BroadcastIssueUpdate(updatedIssue)
	}

	log.Printf("[Dashboard] Unblocked #%d — moved to Backlog", issueNum)
	http.Redirect(w, r, "/", http.StatusSeeOther)
}
```

**Step 2: Add `handleBlock` handler**

Add to `handlers.go`:

```go
func (s *Server) handleBlock(w http.ResponseWriter, r *http.Request) {
	id := r.PathValue("id")
	issueNum := 0
	fmt.Sscanf(id, "%d", &issueNum)
	if issueNum == 0 || s.gh == nil {
		http.Redirect(w, r, "/", http.StatusSeeOther)
		return
	}

	// Set stage label to "Blocked"
	updatedIssue, err := s.gh.SetStageLabel(issueNum, "Blocked")
	if err != nil {
		log.Printf("[Dashboard] Error setting Blocked stage on #%d: %v", issueNum, err)
		http.Redirect(w, r, "/", http.StatusSeeOther)
		return
	}

	// Update cache
	if s.store != nil {
		milestone := s.activeSprintName()
		if err := s.store.SaveIssueCache(updatedIssue, milestone); err != nil {
			log.Printf("[Dashboard] Error saving issue cache for #%d: %v", issueNum, err)
		}
	}

	// Broadcast update via WebSocket
	if s.hub != nil {
		s.hub.BroadcastIssueUpdate(updatedIssue)
	}

	log.Printf("[Dashboard] Blocked #%d", issueNum)
	http.Redirect(w, r, "/", http.StatusSeeOther)
}
```

**Step 3: Register new routes in `server.go`**

Add after line 161:
```go
s.mux.HandleFunc("POST /block/{id}", s.handleBlock)
s.mux.HandleFunc("POST /unblock/{id}", s.handleUnblock)
```

**Step 4: Fix Blocked column in `board.html`**

Replace lines 165-168:
```html
<!-- OLD -->
<form method="post" action="/approve/{{.ID}}"><button type="submit" class="btn btn-success">Approve</button></form>
<form method="post" action="/reject/{{.ID}}"><button type="submit" class="btn btn-danger">Reject</button></form>
<!-- NEW -->
<form method="post" action="/unblock/{{.ID}}"><button type="submit" class="btn btn-success">Unblock</button></form>
```

**Step 5: Add Block button to Backlog, Plan, Code, AI Review cards**

For each column that should have a Block button, add inside the card div (after labels):
```html
<div class="card-actions">
  <form method="post" action="/block/{{.ID}}"><button type="submit" class="btn btn-sm" title="Block this ticket">Block</button></form>
</div>
```

Add to: Backlog cards (line ~183), Plan cards (line ~197), Code cards (line ~212)

**Step 6: Rename "Retry Fresh" to "Cancel" in Failed column**

In `board.html` line 290, change:
```html
<!-- OLD -->
<form method="post" action="/retry-fresh/{{.ID}}"><button type="submit" class="btn btn-primary">Retry Fresh</button></form>
<!-- NEW -->
<form method="post" action="/retry-fresh/{{.ID}}"><button type="submit" class="btn btn-primary">Cancel</button></form>
```

**Step 7: Run tests and build**

Run: `go test ./internal/dashboard/ -v 2>&1 | tail -10`
Run: `go build ./... 2>&1`
Expected: PASS

**Step 8: Commit**

```bash
git add internal/dashboard/handlers.go internal/dashboard/server.go internal/dashboard/templates/board.html
git commit -m "feat: add Block/Unblock actions, fix Blocked column buttons"
```

---

### Task 5: Update `handleApproveMerge` to use Merge stage

**Files:**
- Modify: `internal/dashboard/handlers.go` (handleApproveMerge function)

**Step 1: Add Merge stage transition**

In `handleApproveMerge`, before the actual merge call, set the stage to "Merge":

```go
// Set stage to Merge before attempting merge
mergingIssue, err := s.gh.SetStageLabel(issueNum, "Merge")
if err != nil {
    log.Printf("[Dashboard] Error setting Merge stage on #%d: %v", issueNum, err)
}
// Update cache and broadcast
if s.store != nil {
    milestone := s.activeSprintName()
    _ = s.store.SaveIssueCache(mergingIssue, milestone)
}
if s.hub != nil {
    s.hub.BroadcastIssueUpdate(mergingIssue)
}
```

On merge failure, set to "Failed" instead of "Backlog":
```go
// OLD: s.gh.SetStageLabel(issueNum, "Backlog")
// NEW:
s.gh.SetStageLabel(issueNum, "Failed")
```

**Step 2: Run tests**

Run: `go test ./internal/dashboard/ -v 2>&1 | tail -10`
Expected: PASS

**Step 3: Commit**

```bash
git add internal/dashboard/handlers.go
git commit -m "feat: handleApproveMerge transitions through Merge stage"
```

---

### Task 6: Sync Pipeline with State Machine

**Files:**
- Modify: `internal/pipeline/stage.go`

**Step 1: Update pipeline stages to match state machine**

Remove stages not in state machine: `StagePlanning`, `StagePlanReview`, `StageTesting`
Add missing stages: `StageCreatePR`, `StageApprove`, `StageFailed`

```go
const (
	StageQueued     Stage = "queued"
	StageAnalysis   Stage = "analysis"
	StageCoding     Stage = "coding"
	StageCodeReview Stage = "code-review"
	StageCreatePR   Stage = "create-pr"
	StageApprove    Stage = "awaiting-approval"
	StageMerging    Stage = "merging"
	StageDone       Stage = "done"
	StageFailed     Stage = "failed"
	StageBlocked    Stage = "blocked"
)
```

Update `Column()`:
```go
func (s Stage) Column() Column {
	switch s {
	case StageQueued:
		return ColumnBacklog
	case StageAnalysis:
		return ColumnPlan
	case StageCoding:
		return ColumnCode
	case StageCodeReview, StageCreatePR:
		return ColumnAIReview
	case StageApprove, StageMerging:
		return ColumnApprove
	case StageDone:
		return ColumnDone
	case StageFailed:
		return ColumnFailed
	case StageBlocked:
		return ColumnBlocked
	default:
		return ColumnBacklog
	}
}
```

Add `ColumnFailed`:
```go
const (
	ColumnBacklog  Column = "Backlog"
	ColumnPlan     Column = "Plan"
	ColumnCode     Column = "Code"
	ColumnAIReview Column = "AI Review"
	ColumnApprove  Column = "Approve"
	ColumnDone     Column = "Done"
	ColumnFailed   Column = "Failed"
	ColumnBlocked  Column = "Blocked"
)
```

Update `Label()`:
```go
func (s Stage) Label() string {
	switch s {
	case StageAnalysis:
		return "stage:analysis"
	case StageCoding:
		return "stage:coding"
	case StageCodeReview:
		return "stage:code-review"
	case StageCreatePR:
		return "stage:create-pr"
	case StageApprove:
		return "stage:awaiting-approval"
	case StageMerging:
		return "stage:merging"
	case StageFailed:
		return "stage:failed"
	case StageBlocked:
		return "stage:blocked"
	default:
		return ""
	}
}
```

Update `stageOrder`, `Next()`, `RetryTarget()`.

**Step 2: Update `processor.go` to match**

Remove references to `StagePlanning`, `StagePlanReview`, `StageTesting`.
Add `StageCreatePR` handling.

**Step 3: Run tests**

Run: `go test ./internal/pipeline/ ./internal/worker/ -v 2>&1 | tail -10`
Expected: PASS (may need test updates)

**Step 4: Commit**

```bash
git add internal/pipeline/stage.go internal/worker/processor.go
git commit -m "refactor: sync pipeline stages with state machine"
```

---

### Task 7: Update state-machine.md to reflect final implementation

**Files:**
- Modify: `docs/state-machine.md`

**Step 1: Update the document**

- Remove `stage:backlog` from the label list (Backlog has no label, or optionally `stage:backlog`)
- Confirm all label names match the code
- Add version history entry

**Step 2: Commit**

```bash
git add docs/state-machine.md
git commit -m "docs: update state machine to match implementation"
```

---

### Task 8: Final verification

**Step 1: Run all tests**

Run: `go test ./... 2>&1 | tail -20`
Expected: ALL PASS

**Step 2: Build**

Run: `go build ./... 2>&1`
Expected: No errors

**Step 3: Verify label consistency**

Check that every label in `StageToLabels` exists in `RequiredLabels`:
Run: `go test ./internal/github/ -run TestStageMappingsUseValidLabels -v`
Expected: PASS
