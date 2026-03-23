# Implementation Plan for Issue #166

**Created:** 2026-03-23T12:25:19+01:00
**Updated:** 2026-03-23T12:25:19+01:00

## Analysis

Now I have a complete understanding. Let me provide the analysis:

1. **Core requirements**: Remove the Cancel button from the "Select Issue Type" step (lines 34-40 in wizard_new.html) while keeping it in subsequent steps.

2. **Files that need changes**: Only `internal/dashboard/templates/wizard_new.html` - specifically removing lines 34-40 (the `.form-actions` div inside the `{{if .NeedsTypeSelection}}` block).

3. **Implementation approach**: Simple template edit - remove the form-actions div that contains the Cancel button from the type selection section. The Cancel button in the form section (lines 82-92) should remain.

4. **Testing strategy**: Since this is a UI-only change with no logic changes, the existing tests should still pass. The tests in wizard_test.go focus on backend logic, not UI elements. No new tests needed for this template-only change.## Analysis: GitHub Issue #166

### 1. Core Requirements
Remove the redundant "Cancel" button from the **first step** (Select Issue Type) of the wizard creation flow. Users can already close the modal via the X button or browser navigation, making the Cancel button unnecessary on this initial step. The Cancel button must remain in all subsequent wizard steps.

### 2. Files Requiring Changes
- **`internal/dashboard/templates/wizard_new.html`** (lines 34-40)
  - Remove the `.form-actions` div containing the Cancel button from within the `{{if .NeedsTypeSelection}}` block
  - Keep the `.form-actions` div in the `{{else}}` block (lines 82-92) which handles subsequent steps

### 3. Implementation Approach
Simple template modification:
- Delete lines 34-40 (the entire `.form-actions` block inside the type selection conditional)
- No backend changes required - this is purely a presentation layer change
- No JavaScript changes needed - the `closeWizardModal()` function remains for other steps

### 4. Testing Strategy
- **No new tests required** - this is a UI-only template change with no logic modifications
- **Existing tests** in `wizard_test.go` focus on session management and backend logic, not UI elements
- **Manual verification**: 
  - Open wizard in modal mode → Cancel button should NOT appear on type selection
  - Select a type → Cancel button SHOULD appear on description step
  - Verify modal can still be closed via X button and clicking outside
  - Verify page mode (`?page=1`) navigation still works via browser back button

## Implementation Steps

### Step 1: Open wizard in modal mode → Confirm Cancel button does NOT appear on type selection

### Step 2: Select "Feature" or "Bug" → Confirm Cancel button DOES appear on description step

### Step 3: Verify modal can still be closed via X button and clicking outside

### Step 4: Test page mode (`?page=1`) → browser back button still works for navigation

### Step 4: Run Existing Tests
```bash
go test ./internal/dashboard/... -v
```
**Estimated effort:** 5 minutes (single file edit, no logic changes)

