# Implementation Plan for Issue #178

**Created:** 2026-03-23T13:37:17+01:00
**Updated:** 2026-03-23T13:37:17+01:00

## Analysis

### 1. Core Requirements
- Create `SetStageLabel(issueNumber int, stage string) (Issue, error)` method in `labels.go`
- Map stage names to GitHub labels per the issue specification
- Remove all existing stage labels before applying new ones
- Fetch fresh issue data from GitHub API after label changes
- Return updated issue (cache update handled by caller)
- Handle special cases: "Done" closes issue, "Backlog" removes all stage labels

### 2. Files That Need Changes
- `internal/github/labels.go` - Add `SetStageLabel()` method and stage-to-label mapping
- `internal/github/labels_test.go` - Add comprehensive unit tests

### 3. Implementation Approach
- Define stage-to-labels mapping as a map[string][]string
- Define all stage label prefixes for cleanup (stage:*, awaiting-approval, failed, blocked)
- Implementation steps:
  1. Get current issue to check existing labels
  2. Remove all stage-related labels from the issue
  3. Add new label(s) based on stage mapping
  4. For "Done" stage: close the issue via `CloseIssue()`
  5. Fetch fresh issue data via `GetIssue()`
  6. Return updated issue
- Error handling: wrap errors with context, continue on label removal errors (idempotent)

### 4. Testing Strategy
- Mock `gh` command responses for all scenarios
- Test each stage mapping (Backlog, Plan, Code, AI Review, Approve, Done, Failed, Blocked)
- Test label removal logic
- Test error handling (API failures, partial failures)
- Test edge cases (issue already closed, label already exists)

## Implementation Steps

### Step 1: **Phase 1: Implementation** (15 min)

- Add stage mappings and `SetStageLabel()` to `labels.go`
- Add necessary imports

### Step 2: **Phase 2: Testing** (20 min)

- Add unit tests to `labels_test.go`
- Run tests: `go test ./internal/github/... -v`

### Step 3: **Phase 3: Verification** (5 min)

- Verify all stage mappings work correctly
- Verify error handling
- Run full test suite
### 4. Acceptance Criteria Verification
- [ ] `SetStageLabel()` method exists in `labels.go` with correct signature
- [ ] All stage mappings implemented per specification
- [ ] Previous stage labels are removed before adding new ones
- [ ] Fresh issue data fetched and returned
- [ ] "Done" stage closes the issue
- [ ] Unit tests cover all stages and error cases
- [ ] All tests pass

