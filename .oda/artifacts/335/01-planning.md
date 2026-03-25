# Technical Planning: Issue #335 — Update technical planning prompt to save analysis to artifact file

## Analysis

1. **Core requirements — what exactly needs to be done**
   - Update the technical planning prompt template (`internal/prompts/mvp/technical_planning.md`) to instruct the LLM to save its detailed analysis to `.oda/artifacts/{ticket-number}/01-planning.md` using file write tools
   - Add a fourth `%d` format specifier in the prompt for the ticket number in the artifact path
   - Update the `technicalPlanning()` method in `internal/mvp/worker.go` to pass `task.Issue.Number` as the fourth argument to `fmt.Sprintf`
   - The prompt must instruct the LLM to return only a brief 2-3 sentence summary to the orchestrator (instead of the full analysis)

2. **Files that likely need changes (based on reading the actual codebase)**
   - `internal/prompts/mvp/technical_planning.md` — replace entire prompt with new version including artifact instructions
   - `internal/mvp/worker.go:392` — add fourth argument (`task.Issue.Number`) to `fmt.Sprintf` call
   - `internal/mvp/worker_test.go` — update `TestParseTechnicalPlanningResponse` if response format changes (the response will now be a brief summary, but the parsing still looks for `## Analysis` and `## Implementation Plan` headers — this needs consideration)

3. **Implementation approach — high-level strategy**
   - The prompt change is the primary deliverable. The LLM will be instructed to:
     1. Write the full analysis to `.oda/artifacts/%d/01-planning.md` using file write tools
     2. Return only a brief summary to the orchestrator
   - The worker code change is minimal: add one more argument to `fmt.Sprintf`
   - The `parseTechnicalPlanningResponse()` function currently expects `## Analysis` and `## Implementation Plan` headers in the LLM response. Since the new prompt tells the LLM to return only a brief summary, the response will no longer contain these headers. However, the `technicalPlanning()` method still uses the parsed `analysis` and `implPlan` to create a GitHub comment via `plan.AttachmentManager.CreateFullPlan()`. This means either:
     - (a) The LLM response still includes the full analysis (artifact is a copy), OR
     - (b) The worker reads the artifact file after the LLM step to get the full analysis
   - Based on the issue description, option (a) is implied: the prompt tells the LLM to save to artifact AND return a brief summary. The full analysis lives in the artifact file. The worker's existing `parseTechnicalPlanningResponse` + `CreateFullPlan` flow may need to be adapted to read from the artifact instead. However, the issue specifically says "RESPONSE to orchestrator: Return ONLY a brief summary" — so the existing parsing will get empty strings for analysis/plan since the headers won't be in the brief response.
   - **Decision**: The issue is specifically about updating the prompt and the Sprintf call. The downstream parsing changes (reading artifact instead of parsing response) are a separate concern. For this ticket, we update the prompt and Sprintf. The `parseTechnicalPlanningResponse` will return empty strings for the brief summary response, but the artifact file will contain the full analysis. The `CreateFullPlan` call will post an empty/minimal comment, which is acceptable as the artifact file is now the source of truth.

4. **Testing strategy — what tests to write or update**
   - Verify the prompt template has exactly 4 format specifiers (`%d`, `%s`, `%s`, `%d`) by adding a test that calls `fmt.Sprintf` with the template and 4 arguments
   - The existing `TestParseTechnicalPlanningResponse` tests remain valid — they test the parser function independently
   - Add a test that verifies the prompt contains the artifact path pattern `.oda/artifacts/%d/01-planning.md`
   - Integration test (`TestWorkerProcessEndToEnd`) may need the mock response adjusted if the brief summary format causes issues

5. **Complexity estimate — small (hours)**
   - Two files to change, minimal code modifications
   - Primary change is the prompt template content (text replacement)
   - Secondary change is adding one argument to a `fmt.Sprintf` call

6. **Potential breaking changes**
   - The LLM response format changes from full analysis to brief summary. This means `parseTechnicalPlanningResponse()` will return empty strings for `analysis` and `implPlan` when the LLM follows the new prompt correctly. The `CreateFullPlan` call on line 421 will create a GitHub comment with empty content.
   - This is acceptable per the issue: the artifact file replaces the response as the source of truth. Downstream stages will read from `.oda/artifacts/{number}/01-planning.md` instead of parsing the response.
   - The `ALREADY_DONE:` detection on line 405 is unaffected — it checks the response prefix before parsing.

## Implementation Plan

### Step 1: Update prompt template

**File:** `internal/prompts/mvp/technical_planning.md`

Replace the entire file content with the new prompt from the issue. Key changes:
- Add `ARTIFACT — Save detailed analysis to file:` section with `.oda/artifacts/%d/01-planning.md` path (4th format specifier)
- Add `CRITICAL: Save the complete analysis to the artifact file using file write tool` requirement
- Add `RESPONSE to orchestrator:` section instructing brief 2-3 sentence summary
- Preserve the `CRITICAL FIRST STEP` / `ALREADY_DONE:` detection mechanism
- Preserve all 6 analysis points and 5 implementation plan points

### Step 2: Update worker.go Sprintf call

**File:** `internal/mvp/worker.go:392`

Change:
```go
prompt := fmt.Sprintf(prompts.MustGet(prompts.MVPTechnicalPlanning), task.Issue.Number, task.Issue.Title, task.Issue.Body)
```
To:
```go
prompt := fmt.Sprintf(prompts.MustGet(prompts.MVPTechnicalPlanning), task.Issue.Number, task.Issue.Title, task.Issue.Body, task.Issue.Number)
```

### Step 3: Add prompt format verification test

**File:** `internal/mvp/worker_test.go`

Add a test that verifies the prompt template can be formatted with 4 arguments (3 original + ticket number for artifact path) and contains the expected artifact path pattern.

### Step 4: Run lint and tests

```bash
golangci-lint run ./...
go test -race ./...
```

### Coding Conventions

Based on examining the codebase:
- Go 1.25, standard library preferred
- Tests use table-driven pattern with `t.Run` subtests
- No comments unless explaining non-obvious logic
- `log.Printf` with `[Worker %d]` prefix for worker logging
- Error wrapping with `fmt.Errorf("context: %w", err)`
- Prompt templates are embedded `.md` files with `%d`/`%s` format specifiers

### Order of Operations

1. Update `internal/prompts/mvp/technical_planning.md` (prompt template)
2. Update `internal/mvp/worker.go:392` (add 4th Sprintf argument)
3. Add test in `internal/mvp/worker_test.go` (verify prompt format)
4. Run `golangci-lint run ./...` and `go test -race ./...`
