# Implementation Plan for Issue #168

**Created:** 2026-03-23T12:38:47+01:00
**Updated:** 2026-03-23T12:38:47+01:00

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

