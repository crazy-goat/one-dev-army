# Wizard JSON Response + Remove Done Filter

> **For Claude:** REQUIRED SUB-SKILL: Use superpowers:executing-plans to implement this plan task-by-task.

**Goal:** Simplify wizard to 3 steps (Idea → Review → Create) by merging title+description into one JSON LLM call, change prompt to task-oriented format, and remove Done column filter.

**Architecture:** Replace streaming `SendMessageStream` with synchronous `SendMessageStructured` (already exists) for the wizard refine step. LLM returns `{"title": "...", "description": "..."}` in one call. Title step is removed from the wizard flow. Done filter dropdown and all related logic are deleted.

**Tech Stack:** Go, HTMX, `opencode.SendMessageStructured`, JSON Schema

---

### Task 1: Remove Done column filter

**Files:**
- Modify: `internal/dashboard/handlers.go:47` (remove `DoneFilter` field)
- Modify: `internal/dashboard/handlers.go:110-115` (remove `done_filter` parsing)
- Modify: `internal/dashboard/handlers.go:155-165` (remove filter logic)
- Modify: `internal/dashboard/templates/board.html:271-278` (remove dropdown)
- Modify: `internal/dashboard/handlers_test.go:3320-3400` (remove `TestBuildBoardData_DoneFilter`)

**Step 1: Remove `DoneFilter` field from `boardData` struct**

In `handlers.go`, remove line 47:
```go
DoneFilter     string // Filter for Done column: "all", "merged", "closed"
```

**Step 2: Remove `done_filter` parsing from `buildBoardData`**

In `handlers.go`, remove lines 110-115:
```go
if r != nil {
    data.DoneFilter = r.URL.Query().Get("done_filter")
}
if data.DoneFilter == "" {
    data.DoneFilter = "all"
}
```

**Step 3: Remove Done filter logic from `buildBoardData`**

In `handlers.go`, remove lines 155-165:
```go
if data.DoneFilter != "all" && len(data.Done) > 0 {
    var filteredDone []taskCard
    for _, card := range data.Done {
        if data.DoneFilter == "merged" && card.IsMerged {
            filteredDone = append(filteredDone, card)
        } else if data.DoneFilter == "closed" && !card.IsMerged {
            filteredDone = append(filteredDone, card)
        }
    }
    data.Done = filteredDone
}
```

**Step 4: Remove filter dropdown from `board.html`**

In `board.html`, replace the Done column header section (lines 271-278) — remove the `done-filter` div with select, keep just the column title and cards.

Remove:
```html
<div class="done-filter">
  <label for="done-filter" style="font-size: .75rem; color: var(--muted);">Show:</label>
  <select id="done-filter" name="done_filter" hx-get="/api/board-data" hx-target="#board-container" hx-trigger="change">
    <option value="all" {{if eq .DoneFilter "all"}}selected{{end}}>All Done</option>
    <option value="merged" {{if eq .DoneFilter "merged"}}selected{{end}}>Merged Only</option>
    <option value="closed" {{if eq .DoneFilter "closed"}}selected{{end}}>Closed Only</option>
  </select>
</div>
```

Also remove the `.done-filter` CSS rule from the `<style>` block.

**Step 5: Remove `TestBuildBoardData_DoneFilter` test**

In `handlers_test.go`, delete the entire `TestBuildBoardData_DoneFilter` function.

**Step 6: Run tests**

Run: `go test ./internal/dashboard/ -count=1`
Expected: PASS

**Step 7: Commit**

```bash
git add -A && git commit -m "feat(dashboard): remove Done column filter dropdown and logic"
```

---

### Task 2: New prompt — task-oriented format returning JSON

**Files:**
- Modify: `internal/dashboard/prompts.go` (replace `TechnicalPlanningPromptTemplate`, add `IssueGenerationPromptTemplate`, add JSON schema, update `BuildTechnicalPlanningPrompt`)
- Modify: `internal/dashboard/prompts_test.go` (update tests)

**Step 1: Add JSON response struct and schema**

In `prompts.go`, add after the imports:

```go
// GeneratedIssue is the JSON structure returned by the LLM for issue generation.
type GeneratedIssue struct {
	Title       string `json:"title"`
	Description string `json:"description"`
}

// GeneratedIssueSchema is the JSON schema for structured LLM output.
var GeneratedIssueSchema = json.RawMessage(`{
	"type": "object",
	"properties": {
		"title": {
			"type": "string",
			"description": "Concise GitHub issue title, 5-10 words, max 80 characters. Must start with [Feature] or [Bug] prefix."
		},
		"description": {
			"type": "string",
			"description": "GitHub issue body in markdown format with sections: Description, Tasks, Files to Modify, Acceptance Criteria."
		}
	},
	"required": ["title", "description"],
	"additionalProperties": false
}`)
```

Add `"encoding/json"` to imports.

**Step 2: Replace `TechnicalPlanningPromptTemplate` with task-oriented prompt**

Replace the existing `TechnicalPlanningPromptTemplate` constant with:

```go
const IssueGenerationPromptTemplate = `You are a GitHub issue generator. You produce a JSON object with "title" and "description" fields.

CRITICAL RULE: Output MUST be in %s regardless of input language.

The "title" field:
- 5-10 words, maximum 80 characters
- Must start with [Feature] or [Bug] prefix based on issue type
- Scannable and descriptive

The "description" field is a markdown document with exactly these sections:

## Description
[1-3 sentences: what needs to be done and why]

## Tasks
[Numbered list of concrete implementation steps. Each step is one action a developer can complete in 2-15 minutes. Be specific about file paths.]

## Files to Modify
[List of file paths that need changes, with a brief note on what changes]

## Acceptance Criteria
[2-5 specific, verifiable criteria for completion]

CRITICAL RULES:
- NO implementation code, algorithms, or design patterns
- NO architecture overviews or component dependency analysis
- Focus on WHAT to do, not HOW
- Be specific about file paths
- Tasks should be actionable steps, not abstract descriptions
- Keep it concise — a developer should read this in under 2 minutes

Codebase context (for reference only):
%s

Issue type: %s

Original request:
%s`
```

**Step 3: Update `BuildTechnicalPlanningPrompt` to use new template**

Rename to `BuildIssueGenerationPrompt` and update:

```go
func BuildIssueGenerationPrompt(wizardType WizardType, idea string, codebaseContext string, language string) string {
	if codebaseContext == "" {
		codebaseContext = "No codebase context provided."
	}
	if language == "" {
		language = "en-US"
	}

	var typeLabel string
	if wizardType == WizardTypeBug {
		typeLabel = "Bug"
	} else {
		typeLabel = "Feature"
	}

	return fmt.Sprintf(IssueGenerationPromptTemplate, language, codebaseContext, typeLabel, idea)
}
```

Keep `BuildTechnicalPlanningPrompt` as a wrapper that calls `BuildIssueGenerationPrompt` for backward compatibility (used in tests).

**Step 4: Update tests**

In `prompts_test.go`, update tests that reference `TechnicalPlanningPromptTemplate` to use `IssueGenerationPromptTemplate`. Add test for `GeneratedIssueSchema` being valid JSON.

**Step 5: Run tests**

Run: `go test ./internal/dashboard/ -count=1`
Expected: PASS

**Step 6: Commit**

```bash
git add -A && git commit -m "feat(wizard): replace technical planning prompt with task-oriented JSON prompt"
```

---

### Task 3: Merge refine+title into single JSON LLM call

**Files:**
- Modify: `internal/dashboard/handlers.go` (rewrite `handleWizardRefine` to use `SendMessageStructured`, remove `handleWizardGenerateTitle`, remove `renderTitlePage`, remove `generateMockTitle`)
- Modify: `internal/dashboard/server.go` (remove `/wizard/title` route)

**Step 1: Rewrite `handleWizardRefine` to use structured JSON**

Replace the LLM call section in `handleWizardRefine` (lines ~1218-1298). Instead of:
- `SendMessageStream` → `stripLLMPreamble` → store `TechnicalPlanning`

Do:
- `SendMessageStructured` with `GeneratedIssueSchema` → parse into `GeneratedIssue` → store both `GeneratedTitle` and `TechnicalPlanning`

The handler should:
1. Create LLM session
2. Build prompt with `BuildIssueGenerationPrompt`
3. Call `s.oc.SendMessageStructured(ctx, llmSession.ID, prompt, model, GeneratedIssueSchema, &result)`
4. Store `result.Title` → `session.SetGeneratedTitle(result.Title)`
5. Store `result.Description` → `session.SetTechnicalPlanning(result.Description)`
6. Render `wizard_refine.html` (which now shows both title and description)

Also update the mock response (when `s.oc == nil`) to return both title and description.

**Step 2: Remove `handleWizardGenerateTitle`**

Delete the entire `handleWizardGenerateTitle` function (~lines 1302-1431).

**Step 3: Remove `renderTitlePage`**

Delete the entire `renderTitlePage` function (~lines 1434-1456).

**Step 4: Remove `generateMockTitle`**

Delete the entire `generateMockTitle` function (~lines 1458-1486).

**Step 5: Remove title route from server.go**

In `server.go`, remove:
```go
s.mux.HandleFunc("POST /wizard/title", s.handleWizardGenerateTitle)
```

**Step 6: Update `handleWizardCreate`**

In `handleWizardCreate` / `handleWizardCreateSingle`:
- Read `issue_title` from form (user may have edited it on review page)
- If provided and different from generated, call `session.SetCustomTitle()` + `session.SetUseCustomTitle(true)`
- Remove fallback to `generateMockTitle` — use `session.GetFinalTitle()` directly

**Step 7: Run tests**

Run: `go test ./internal/dashboard/ -count=1`
Expected: Some test failures (tests reference old handlers). Fix in Task 4.

**Step 8: Commit**

```bash
git add -A && git commit -m "feat(wizard): merge title+description into single JSON LLM call"
```

---

### Task 4: Update wizard templates — 3-step flow

**Files:**
- Modify: `internal/dashboard/templates/wizard_steps.html` (3 steps: Idea → Review → Create)
- Modify: `internal/dashboard/templates/wizard_refine.html` (add title input, change "Accept & Continue" to "Accept & Create Issue", form posts to `/wizard/create`)
- Delete: `internal/dashboard/templates/wizard_title.html`
- Modify: `internal/dashboard/templates/wizard_create.html` (update step number)

**Step 1: Update `wizard_steps.html` to 3 steps**

Replace content with 3-step indicator:
- Step 1: Idea
- Step 2: Review
- Step 3: Create

(With 4-step variant for type selection: Type → Idea → Review → Create)

**Step 2: Update `wizard_refine.html`**

- Add editable title input at the top (before the markdown preview), pre-filled with `{{.Title}}`
- Change form action from `/wizard/title` to `/wizard/create`
- Change "Accept & Continue" button text to "Accept & Create Issue"
- Add `issue_title` hidden input or use the title input name
- Remove "Back" button that goes to title step
- Update `CurrentStep` references

**Step 3: Delete `wizard_title.html`**

Remove the file entirely.

**Step 4: Update `wizard_create.html`**

Change `CurrentStep` from 4 to 3 (or 4 if type selection was needed).

**Step 5: Update handler data structs**

In `handleWizardRefine`, add `Title` field to the template data struct so `wizard_refine.html` can display it.

**Step 6: Run tests**

Run: `go test ./internal/dashboard/ -count=1`
Expected: PASS (or fix remaining test issues)

**Step 7: Commit**

```bash
git add -A && git commit -m "feat(wizard): simplify to 3-step flow (Idea → Review → Create)"
```

---

### Task 5: Update and fix all tests

**Files:**
- Modify: `internal/dashboard/handlers_test.go` (remove title-related tests, update wizard flow tests)
- Modify: `internal/dashboard/prompts_test.go` (update prompt tests)
- Modify: `internal/dashboard/wizard_test.go` (if needed)

**Step 1: Remove/update title generation tests**

- Remove tests for `handleWizardGenerateTitle`
- Remove tests for `generateMockTitle`
- Remove tests for `renderTitlePage`
- Update wizard flow integration tests to skip title step

**Step 2: Update wizard refine tests**

- Update mock response expectations to include both title and description
- Update assertions to check that both `GeneratedTitle` and `TechnicalPlanning` are set

**Step 3: Update prompt tests**

- Add test for `BuildIssueGenerationPrompt`
- Add test for `GeneratedIssueSchema` validity
- Update existing tests that reference old prompt names

**Step 4: Run full test suite**

Run: `go test -count=1 ./...`
Expected: ALL PASS

**Step 5: Commit**

```bash
git add -A && git commit -m "test: update wizard tests for 3-step JSON flow"
```

---

### Task 6: Clean up dead code

**Files:**
- Modify: `internal/dashboard/prompts.go` (remove `TitleGenerationPromptTemplate`, `BuildTitleGenerationPrompt`, `stripLLMPreamble` if unused)
- Modify: `internal/dashboard/wizard.go` (remove `WizardStepTitle` constant, clean up unused fields)

**Step 1: Remove `TitleGenerationPromptTemplate` and `BuildTitleGenerationPrompt`**

These are no longer used since title comes from the JSON response.

**Step 2: Remove `stripLLMPreamble`**

No longer needed — structured JSON response doesn't have preamble.

**Step 3: Remove `WizardStepTitle` from wizard.go**

The title step no longer exists in the flow.

**Step 4: Remove legacy fields/methods if unused**

Check if `SkipBreakdown`, `RefinedDescription`, `MigrateOldSession` are still referenced. Remove if dead.

**Step 5: Run full test suite**

Run: `go test -count=1 ./...`
Expected: ALL PASS

**Step 6: Commit**

```bash
git add -A && git commit -m "refactor(wizard): remove dead code from old wizard flow"
```
