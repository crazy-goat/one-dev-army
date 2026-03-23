# Implementation Plan for Issue #168

**Created:** 2026-03-23T12:39:05+01:00
**Updated:** 2026-03-23T12:39:05+01:00

## Analysis

Perfect! The implementation is complete and all tests pass. Let me now provide a comprehensive analysis of the issue.

Based on my analysis:

1. **Core Requirements**: The issue requested merging "analyze" and "plan" steps into a single "Technical Planning" step
2. **Current State**: This has ALREADY BEEN IMPLEMENTED in the codebase
3. **Implementation Details**:
   - `technicalPlanningPrompt` combines both analysis and planning (lines 23-51)
   - `technicalPlanning()` function handles the combined step (lines 296-333)
   - `parseTechnicalPlanningResponse()` extracts analysis and plan from combined response (lines 335-374)
   - Step order updated to `["technical-planning", "implement", "code-review", "create-pr"]` (line 130)
   - Database migration logic handles old "analyze"/"plan" steps (lines 178-181, 193-196 in db.go)
   - Backward compatibility in worker resume logic (lines 193-196 in worker.go)
4. **Tests**: All tests pass including:
   - `TestParseTechnicalPlanningResponse` - tests response parsing
   - `TestCheckAlreadyDone` - tests already done detection
   - `TestGetLastCompletedStep_Migration` - tests step name migration
   - `TestGetStepResponse_Migration` - tests response fallback

The issue is already resolved!## Analysis: GitHub Issue #168

### Status: **ALREADY IMPLEMENTED**

This issue has been fully implemented in the codebase. Here's the analysis:

---

### 1. Core Requirements (What Was Needed)

- Merge separate "analyze" and "plan" steps into single "Technical Planning" step
- Single LLM call instead of two
- Combined prompt generating both analysis and plan
- Single resume point
- Backward compatibility for old tickets

---

### 2. Files Modified (Already Done)

| File | Changes |
|------|---------|
| `internal/mvp/worker.go` | Combined prompt (lines 23-51), `technicalPlanning()` function (296-333), `parseTechnicalPlanningResponse()` (335-374), updated `stepOrder` (130) |
| `internal/mvp/worker_test.go` | Tests for parsing combined response (121-193), already-done detection (195-230) |
| `internal/db/db.go` | Migration logic in `GetLastCompletedStep()` (178-181) and `GetStepResponse()` (193-196) |

---

### 3. Implementation Approach (Completed)

**Combined Prompt** (`technicalPlanningPrompt`):
- Single prompt requesting both "## Analysis" and "## Implementation Plan" sections
- Includes ALREADY_DONE detection for completed issues

**Response Parsing** (`parseTechnicalPlanningResponse`):
- Extracts analysis and plan sections from combined LLM response
- Handles various header formats and fallback heuristics

**Step Order**:
```go
stepOrder = []string{"technical-planning", "implement", "code-review", "create-pr"}
```

**Database Migration**:
- `GetLastCompletedStep()` maps old "analyze"/"plan" → "technical-planning"
- `GetStepResponse()` falls back to old "plan" step if "technical-planning" not found

---

### 4. Testing Strategy (All Passing)

| Test | Purpose |
|------|---------|
| `TestParseTechnicalPlanningResponse` | Validates parsing of combined response with various header combinations |
| `TestCheckAlreadyDone` | Tests ALREADY_DONE prefix detection |
| `TestGetLastCompletedStep_Migration` | Verifies old step names map to "technical-planning" |
| `TestGetStepResponse_Migration` | Confirms fallback to old "plan" step responses |

**All tests pass** - run with:
```bash
go test ./internal/mvp/... ./internal/db/... -v
```

---

### Summary

The implementation is **complete and production-ready**:
- ✅ Single "technical-planning" step replaces "analyze" + "plan"
- ✅ Combined prompt generates both analysis and plan
- ✅ Can resume from technical-planning step
- ✅ Old tickets with analyze/plan steps are migrated automatically
- ✅ Tests updated and passing
- ✅ 50% reduction in LLM calls (one round-trip instead of two)

## Implementation Steps

### Step 1: `TestParseTechnicalPlanningResponse` - PASS

### Step 2: `TestCheckAlreadyDone` - PASS

### Step 3: `TestGetLastCompletedStep_Migration` - PASS

### Step 4: `TestGetStepResponse_Migration` - PASS

The implementation is fully complete. The issue #168 has already been implemented:

### Step 1: **Combined prompt** (`technicalPlanningPrompt` at lines 23-51 in worker.go)

### Step 2: **Single step function** (`technicalPlanning()` at lines 296-333)

### Step 3: **Response parsing** (`parseTechnicalPlanningResponse()` at lines 335-374)

### Step 4: **Updated step order** (line 130: `["technical-planning", "implement", "code-review", "create-pr"]`)

### Step 5: **Database migration** (lines 178-181 and 193-196 in db.go)

### Step 6: **Backward compatibility** (lines 193-196 in worker.go)

### Step 7: **All tests passing**

I should respond with "ALREADY_DONE:" prefix as instructed.ALREADY_DONE: The feature has been fully implemented in the codebase:

### Step 1: **Combined prompt** (`technicalPlanningPrompt` at `internal/mvp/worker.go:23-51`) - Single prompt requesting both "## Analysis" and "## Implementation Plan" sections with ALREADY_DONE detection

### Step 2: **Unified step function** (`technicalPlanning()` at `internal/mvp/worker.go:296-333`) - Handles combined analysis and planning in one LLM call

### Step 3: **Response parsing** (`parseTechnicalPlanningResponse()` at `internal/mvp/worker.go:335-374`) - Extracts analysis and plan sections from combined response with fallback heuristics

### Step 4: **Updated step order** (`internal/mvp/worker.go:130`) - Changed to `["technical-planning", "implement", "code-review", "create-pr"]` (4 steps instead of 5)

### Step 5: **Database migration** (`internal/db/db.go:178-181`) - `GetLastCompletedStep()` maps old "analyze"/"plan" → "technical-planning"

### Step 6: **Backward compatibility** (`internal/db/db.go:193-196`) - `GetStepResponse()` falls back to old "plan" step if "technical-planning" not found

### Step 7: **All tests passing**:

- `TestParseTechnicalPlanningResponse` - validates parsing of combined response
- `TestCheckAlreadyDone` - tests ALREADY_DONE prefix detection
- `TestGetLastCompletedStep_Migration` - verifies step name migration
- `TestGetStepResponse_Migration` - confirms fallback to old step responses

