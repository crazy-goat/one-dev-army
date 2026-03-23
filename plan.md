# Implementation Plan for Issue #168

**Created:** 2026-03-23T12:41:24+01:00
**Updated:** 2026-03-23T12:41:24+01:00

## Analysis

Excellent! All tests are passing. Let me now provide a comprehensive analysis of the implementation.## Analysis: Issue #168 - Merge analyze and plan steps

### Status: ✅ **ALREADY IMPLEMENTED**

The feature has been fully implemented on branch `oda-168-merge-analyze-and-plan-steps-into-single` (commit `4b0f71e`).

---

### 1. Core Requirements (Completed)

| Requirement | Status | Implementation |
|-------------|--------|----------------|
| Single "technical-planning" step | ✅ | `worker.go:130` - stepOrder updated |
| Combined prompt | ✅ | `worker.go:23-51` - technicalPlanningPrompt |
| Parse analysis + plan from response | ✅ | `worker.go:335-374` - parseTechnicalPlanningResponse() |
| Resume from technical-planning | ✅ | `worker.go:186-198` - backward compatible resume |
| Migrate old analyze/plan steps | ✅ | `db.go:178-181` - GetLastCompletedStep() mapping |
| Tests updated | ✅ | All tests passing |

---

### 2. Files Modified

- **`internal/mvp/worker.go`** - Main implementation
  - Combined prompt (`technicalPlanningPrompt`)
  - New `technicalPlanning()` function (replaces separate analyze/plan)
  - Response parser with fallback heuristics
  - Updated step order: 4 steps instead of 5
  
- **`internal/db/db.go`** - Database migration
  - `GetLastCompletedStep()` maps "analyze"/"plan" → "technical-planning"
  - `GetStepResponse()` fallback to old "plan" step
  
- **`internal/mvp/worker_test.go`** - Tests
  - `TestParseTechnicalPlanningResponse` - 5 test cases for parsing
  
- **`internal/db/db_test.go`** - Migration tests
  - `TestGetLastCompletedStep_Migration`
  - `TestGetStepResponse_Migration`

---

### 3. Implementation Strategy

**Before:** 2 LLM calls (analyze → plan) with separate resume points
**After:** 1 LLM call with combined prompt, single resume point

**Key Design Decisions:**
1. **Combined Prompt** - Merges analysis requirements + implementation planning into one structured output
2. **Response Parsing** - Robust parser handles various LLM output formats (headers, heuristics, fallback)
3. **Backward Compatibility** - Database layer transparently maps old step names to new
4. **Plan.md Generation** - Creates GitHub attachment with both analysis and plan sections

---

### 4. Testing Strategy (Implemented)

| Test | Coverage |
|------|----------|
| `TestParseTechnicalPlanningResponse` | Response parsing with/without headers |
| `TestGetLastCompletedStep_Migration` | Old step name → new mapping |
| `TestGetStepResponse_Migration` | Fallback to legacy "plan" step |
| `TestCheckAlreadyDone` | ALREADY_DONE detection |
| Integration tests | Full workflow validation |

**All tests passing** ✅

---

### Benefits Achieved

1. **Performance**: 50% reduction in LLM calls for planning phase (2 → 1)
2. **Simplicity**: 4 pipeline steps instead of 5
3. **Consistency**: Matches wizard behavior from issue #148
4. **Resilience**: Backward compatible with in-flight tickets using old step names

## Implementation Steps

### Step 1: In `worker.go`:

- Line 130: `stepOrder` is set to `["technical-planning", "implement", "code-review", "create-pr"]` - 4 steps instead of 5
- Lines 23-51: `technicalPlanningPrompt` constant with combined analysis and plan sections
- Lines 296-333: `technicalPlanning()` function that handles the combined step
- Lines 335-374: `parseTechnicalPlanningResponse()` function to parse the combined response
- Lines 174-198: Resume logic that handles both new "technical-planning" step and old "analyze"/"plan" steps

### Step 2: In `db.go`:

- Lines 178-181: `GetLastCompletedStep()` maps old "analyze" and "plan" step names to "technical-planning"
- Lines 194-196: `GetStepResponse()` has fallback to old "plan" step for backward compatibility
The analysis provided by the user is correct - this feature is ALREADY IMPLEMENTED. All the requirements from issue #168 have been met:
- Single "technical-planning" step ✅
- Combined prompt with Analysis and Implementation Plan sections ✅
- Response parsing to extract both sections ✅
- Resume from technical-planning ✅
- Migration of old analyze/plan steps ✅
- Tests (mentioned in the analysis) ✅
I should respond with "ALREADY_DONE:" followed by the concrete evidence.ALREADY_DONE: The feature is fully implemented in internal/mvp/worker.go:130 (stepOrder with 4 steps), worker.go:23-51 (technicalPlanningPrompt constant), worker.go:296-333 (technicalPlanning function), worker.go:335-374 (parseTechnicalPlanningResponse), and internal/db/db.go:178-181 (migration mapping old "analyze"/"plan" → "technical-planning").

