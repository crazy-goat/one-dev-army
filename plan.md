# Implementation Plan for Issue #168

**Created:** 2026-03-23T12:33:47+01:00
**Updated:** 2026-03-23T12:33:47+01:00

## Analysis

Now I have a good understanding of the codebase. Let me provide a concise analysis of the issue:

1. **Core Requirements:**
   - Merge "analyze" and "plan" steps into single "technical-planning" step
   - Combine the two LLM prompts into one
   - Update stepOrder from ["analyze", "plan", "implement", "code-review", "create-pr"] to ["technical-planning", "implement", "code-review", "create-pr"]
   - Create a new technicalPlanning() function that returns both analysis and plan
   - Handle resume logic migration from old steps
   - Update tests

2. **Files that need changes:**
   - `internal/mvp/worker.go` - Main implementation (lines 23-53 for prompts, 132 for stepOrder, 309-373 for analyze/plan functions, 176-211 for Process method)
   - `internal/mvp/worker_test.go` - Update tests
   - `internal/db/db.go` - May need migration logic for old step names (lines 165-178 for GetLastCompletedStep, 180-193 for GetStepResponse)

3. **Implementation approach:**
   - Create combined `technicalPlanningPrompt` that merges analysisPrompt and planningPrompt
   - Create `technicalPlanning()` function that makes single LLM call and returns (analysis, plan string, err error)
   - Parse combined response to extract both analysis and plan sections
   - Update `Process()` to call technicalPlanning() instead of separate analyze() and plan() calls
   - Update resume logic to handle migration: if last completed step is "analyze" or "plan", treat as "technical-planning" completed
   - Keep backward compatibility by checking for old step names in database

4. **Testing strategy:**
   - Update existing tests to use new "technical-planning" step name
   - Add tests for technicalPlanning() function
   - Add tests for resume logic migration from old "analyze"/"plan" steps
   - Test that combined prompt generates both analysis and plan sections
   - Verify backward compatibility with old tickets## Analysis of GitHub Issue #168

### 1. Core Requirements

Merge the separate `analyze` and `plan` steps into a single `technical-planning` step:

- **Combine prompts**: Merge `analysisPrompt` (lines 23-34) and `planningPrompt` (lines 36-53) into one unified prompt
- **Single LLM call**: Replace two separate `llmStep()` calls with one call that returns both analysis and plan
- **Update step order**: Change `stepOrder` (line 132) from `["analyze", "plan", "implement", "code-review", "create-pr"]` to `["technical-planning", "implement", "code-review", "create-pr"]`
- **Resume migration**: Handle tickets that have old "analyze" or "plan" steps completed
- **Plan.md updates**: The combined step should still create and update plan.md (currently done separately in `analyze()` lines 316-336 and `plan()` lines 351-370)

### 2. Files That Need Changes

| File | Lines | Changes |
|------|-------|---------|
| `internal/mvp/worker.go` | 23-53 | Create combined `technicalPlanningPrompt` |
| `internal/mvp/worker.go` | 132 | Update `stepOrder` array |
| `internal/mvp/worker.go` | 176-211 | Replace separate analyze/plan calls in `Process()` |
| `internal/mvp/worker.go` | 309-373 | Replace `analyze()` and `plan()` with `technicalPlanning()` |
| `internal/mvp/worker.go` | 143-157 | Update resume logic for migration |
| `internal/mvp/worker_test.go` | All | Update tests for new step name |
| `internal/db/db.go` | 165-193 | Consider migration for `GetLastCompletedStep` and `GetStepResponse` |

### 3. Implementation Approach

**Prompt merging strategy:**
- Create `technicalPlanningPrompt` that asks LLM to output both analysis and plan in structured format (similar to wizard's `TechnicalPlanningPromptTemplate` in `internal/dashboard/prompts.go:71-100`)
- Use clear section markers (e.g., `## Analysis` and `## Implementation Plan`) so response can be parsed

**Function structure:**
```go
func (w *Worker) technicalPlanning(ctx context.Context, task *Task) (analysis, plan string, err error)
```

**Response parsing:**
- Split LLM response on section headers to extract analysis and plan separately
- Handle `ALREADY_DONE:` detection (currently in `checkAlreadyDone()` at line 617)

**Resume migration:**
- In `GetLastCompletedStep()` check: if last step is "analyze" or "plan", return "technical-planning"
- In `Process()`, when resuming from old steps, fetch both responses and combine

**Plan.md handling:**
- Single call to create/update plan.md with both analysis and plan content (merge the two existing plan.md operations)

### 4. Testing Strategy

**Unit tests to add/update:**
- `TestTechnicalPlanning()` - Test new function makes single LLM call and parses response correctly
- `TestTechnicalPlanningAlreadyDone()` - Verify `ALREADY_DONE:` detection still works
- `TestProcessResumeFromOldSteps()` - Test migration from "analyze"/"plan" to "technical-planning"
- Update `TestSlug`, `TestExtractText` tests if they reference step names

**Integration considerations:**
- Test that old tickets with completed "analyze" step can resume correctly
- Test that plan.md is created with combined content
- Verify step storage in database uses new "technical-planning" name

**Key risk:** The wizard implementation (`internal/dashboard/prompts.go`) already has a unified technical planning prompt template that could be adapted for the worker.

