# Implementation Prompt Coding Artifact Plan

> **For Claude:** REQUIRED SUB-SKILL: Use superpowers:executing-plans to implement this plan task-by-task.

**Goal:** Update the implementation/coding prompt to instruct the LLM to save detailed coding notes to `.oda/artifacts/{ticket}/02-coding.md`, and to read the planning artifact before coding.

**Architecture:** The prompt template (`implementation.md`) gets new sections for artifact read/write. The `implement()` function in `worker.go` passes `task.Issue.Number` additional times to fill the new `%d` format specifiers in the template. A new test validates the prompt formatting.

**Tech Stack:** Go, embedded prompt templates via `//go:embed`, `fmt.Sprintf` for template interpolation.

---

### Task 1: Update the implementation prompt template

**Files:**
- Modify: `internal/prompts/mvp/implementation.md`

**Step 1: Replace the prompt template**

Replace the entire content of `internal/prompts/mvp/implementation.md` with the new version below. The template adds:
- An `ARTIFACT` section instructing the LLM to save coding notes to `.oda/artifacts/%d/02-coding.md`
- A new workflow step 2 to read the planning artifact `.oda/artifacts/%d/01-planning.md`
- A new workflow step 8 to save the coding artifact
- Updated critical rules including the artifact save instruction

The new template has **8 format specifiers** total:
1. `%d` — issue number (title line)
2. `%s` — issue title (title line)
3. `%s` — plan string
4. `%s` — working directory
5. `%s` — test command
6. `%d` — issue number (artifact save path in ARTIFACT section)
7. `%d` — issue number (read planning artifact path in step 2)
8. `%d` — issue number (save coding artifact path in step 8)

```markdown
Implement the following plan for GitHub issue #%d: %s

Implementation plan:
%s

Working directory: %s

Test command: %s

ARTIFACT — Save implementation notes to file:
After completing implementation, save detailed notes to:
.oda/artifacts/%d/02-coding.md

The artifact must include:

## What Was Implemented
- List of files created/modified
- Key changes made in each file
- Any deviations from the original plan and why

## Implementation Decisions
- Why specific approaches were chosen
- Technical trade-offs considered
- Coding patterns and conventions followed
- Any challenges encountered and how they were solved

## Testing & Verification
- Test results (pass/fail)
- Lint results
- Any test modifications needed

## Notes for Reviewer
- Areas that need careful review
- Potential edge cases
- Known limitations or technical debt

STEP-BY-STEP WORKFLOW:

1. READ AGENTS.md FIRST
   - Read the AGENTS.md file in the working directory for project-specific rules
   - Note the exact lint command and any other requirements

2. READ PLANNING ARTIFACT
   - Read .oda/artifacts/%d/01-planning.md for context

3. ESTABLISH BASELINE
   - Run the test command BEFORE making any changes
   - Verify tests pass in the current state (baseline)

4. IMPLEMENT CHANGES
   - Read existing files before modifying them
   - Make all necessary code changes according to the plan
   - Create new files as needed
   - Verify your implementation matches the plan

5. TEST-FIX-LINT-FIX LOOP (REPEAT UNTIL CLEAN)
   - Run the test command
   - If tests fail, fix the code and re-run tests
   - Run the lint command (from AGENTS.md)
   - If lint errors exist, fix them and re-run lint
   - Do NOT proceed until both tests AND lint pass

6. ATOMIC COMMITS
   - Make focused, atomic commits (one logical change per commit)
   - Use descriptive commit messages referencing the issue number
   - Example: "feat: add user authentication for #123"

7. PUSH CHANGES
   - Push the branch: git push
   - Verify the push succeeded

8. SAVE ARTIFACT
   - Write implementation notes to .oda/artifacts/%d/02-coding.md

9. FINAL VERIFICATION
   - Run "git status" and verify the working tree is clean
   - If there are any uncommitted changes, commit and push them
   - Run tests one final time to confirm everything passes

CRITICAL RULES:
- You are in a fully automated pipeline. NEVER ask questions or wait for input.
- Make your best judgment and proceed immediately.
- Do NOT use git worktrees. Work directly in the provided working directory. Do NOT run "git worktree" commands.
- The branch is already created and checked out — do not create new branches.
- CRITICAL: Save the implementation notes artifact using file write tool
```

**Step 2: Verify the template has exactly 8 format specifiers**

Run: `grep -c '%[ds]' internal/prompts/mvp/implementation.md`
Expected: 8 (five original + three new `%d` for issue number in artifact paths)

---

### Task 2: Update worker.go to pass additional format arguments

**Files:**
- Modify: `internal/mvp/worker.go:497`

**Step 1: Update the fmt.Sprintf call**

In `internal/mvp/worker.go`, line 497, change from:

```go
prompt := fmt.Sprintf(prompts.MustGet(prompts.MVPImplementation), task.Issue.Number, task.Issue.Title, planStr, task.Worktree, testCmd)
```

To:

```go
prompt := fmt.Sprintf(prompts.MustGet(prompts.MVPImplementation), task.Issue.Number, task.Issue.Title, planStr, task.Worktree, testCmd, task.Issue.Number, task.Issue.Number, task.Issue.Number)
```

The three additional `task.Issue.Number` arguments correspond to:
- Arg 6: ARTIFACT section artifact save path (`%d` → `.oda/artifacts/%d/02-coding.md`)
- Arg 7: Step 2 planning artifact read path (`%d` → `.oda/artifacts/%d/01-planning.md`)
- Arg 8: Step 8 coding artifact save path (`%d` → `.oda/artifacts/%d/02-coding.md`)

**Step 2: Verify compilation**

Run: `go build ./...`
Expected: No errors

---

### Task 3: Add test for implementation prompt format

**Files:**
- Modify: `internal/mvp/worker_test.go` (append new test after `TestTechnicalPlanningPromptFormat`)

**Step 1: Write the test**

Add the following test function at the end of `internal/mvp/worker_test.go`:

```go
func TestImplementationPromptFormat(t *testing.T) {
	promptTemplate := prompts.MustGet(prompts.MVPImplementation)

	formatted := fmt.Sprintf(promptTemplate, 123, "Test Title", "Test Plan", "/tmp/worktree", "go test ./...", 123, 123, 123)

	expectedCodingArtifact := ".oda/artifacts/123/02-coding.md"
	if !strings.Contains(formatted, expectedCodingArtifact) {
		t.Errorf("formatted prompt does not contain expected coding artifact path %q", expectedCodingArtifact)
	}

	expectedPlanningArtifact := ".oda/artifacts/123/01-planning.md"
	if !strings.Contains(formatted, expectedPlanningArtifact) {
		t.Errorf("formatted prompt does not contain expected planning artifact path %q", expectedPlanningArtifact)
	}

	if strings.Contains(formatted, "%!") {
		t.Errorf("formatted prompt contains unreplaced format specifiers: %s", formatted)
	}

	if !strings.Contains(formatted, "ARTIFACT") {
		t.Error("formatted prompt does not contain 'ARTIFACT' section")
	}
	if !strings.Contains(formatted, "CRITICAL: Save the implementation notes artifact") {
		t.Error("formatted prompt does not contain artifact save instruction")
	}
	if !strings.Contains(formatted, "SAVE ARTIFACT") {
		t.Error("formatted prompt does not contain 'SAVE ARTIFACT' workflow step")
	}
}
```

**Step 2: Run the test**

Run: `go test -race -run TestImplementationPromptFormat ./internal/mvp/`
Expected: PASS

**Step 3: Run all tests**

Run: `go test -race ./...`
Expected: All PASS

**Step 4: Run linter**

Run: `golangci-lint run ./...`
Expected: No errors

**Step 5: Commit**

```bash
git add internal/prompts/mvp/implementation.md internal/mvp/worker.go internal/mvp/worker_test.go
git commit -m "feat: add coding artifact to implementation prompt (#336)"
```

---

## Verification Checklist

- [ ] `implementation.md` has ARTIFACT section with `02-coding.md` path
- [ ] `implementation.md` workflow step 2 reads `01-planning.md`
- [ ] `implementation.md` workflow step 8 saves `02-coding.md`
- [ ] `worker.go` passes `task.Issue.Number` 3 additional times (8 args total)
- [ ] `TestImplementationPromptFormat` passes
- [ ] All existing tests pass (`go test -race ./...`)
- [ ] Lint passes (`golangci-lint run ./...`)
