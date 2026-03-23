# Implementation Plan for Issue #164

**Created:** 2026-03-23T12:27:10+01:00
**Updated:** 2026-03-23T12:27:10+01:00

## Analysis

Now I have a comprehensive understanding of the codebase. Let me provide a concise analysis of the issue.

## Analysis Summary

### 1. Core Requirements
Split the "In Progress" column into two separate columns:
- **Plan** column (yellow/orange): Contains `analyzing` + `planning` states
- **Code** column (blue): Contains `coding` state only

New column order: Blocked → Backlog → Plan → Code → AI Review → Approve → Done → Failed

### 2. Files That Need Changes

1. **`internal/github/project.go`** (lines 10-17):
   - Update `ProjectColumns` array to replace "In Progress" with "Plan" and "Code"
   - Update `columnColors` map (lines 168-175) with new column colors

2. **`internal/dashboard/handlers.go`**:
   - Update `boardData` struct (lines 30-43): Replace `Progress []taskCard` with `Plan []taskCard` and `Code []taskCard`
   - Update `placeholderBoard()` (lines 63-84): Replace Progress with Plan/Code
   - Update `inferColumnFromIssue()` (lines 205-239): Map state labels to new columns
   - Update `addCardToColumn()` (lines 241-271): Handle new column names

3. **`internal/dashboard/templates/board.html`**:
   - Replace "In Progress" column section (lines 99-112) with two new column sections
   - Add CSS styling for `.col-plan` and `.col-code` classes
   - Update grid template to accommodate 8 columns instead of 7 (line 7)

### 3. Implementation Approach

**Phase 1: Data Model Changes**
- Update `ProjectColumns` in `project.go` to define the new column structure
- Modify `boardData` struct to replace single `Progress` field with separate `Plan` and `Code` fields

**Phase 2: Logic Updates**
- Refactor `inferColumnFromIssue()` to route issues to the appropriate column based on their state labels
- Update `addCardToColumn()` to populate the new data structures correctly

**Phase 3: UI Changes**
- Redesign the board template to display the two new columns with distinct visual styling
- Adjust the CSS grid layout to support the additional column

**Phase 4: State Label Mapping**
- `state:analyze` and `state:plan` labels → Plan column
- `state:coding` label → Code column

### 4. Testing Strategy

**Unit Tests to Update:**
- `handlers_test.go`: Refresh tests that check the board data structure and column mappings
- `project_test.go`: Validate the new column configuration and color assignments

**Integration Tests:**
- Verify that issues with different state labels appear in the correct columns
- Test the complete flow from issue creation through state transitions
- Ensure the dashboard renders correctly with the new 8-column layout

**Manual Verification:**
- Confirm the column order matches the specification
- Check that color coding is applied correctly to each column
- Validate responsive behavior on different screen sizes## Analysis: GitHub Issue #164 - Split In Progress Column

### 1. Core Requirements

Split the "In Progress" column into two distinct columns:
- **Plan** (yellow/orange): Tickets in `analyzing` or `planning` state
- **Code** (blue): Tickets in `coding` state only

New column order: Blocked → Backlog → **Plan** → **Code** → AI Review → Approve → Done → Failed

### 2. Files That Need Changes

| File | Lines | Changes Needed |
|------|-------|----------------|
| `internal/github/project.go` | 10-17, 168-175 | Update `ProjectColumns` array and `columnColors` map |
| `internal/dashboard/handlers.go` | 30-43, 63-84, 205-239, 241-271 | Update `boardData` struct, `placeholderBoard()`, `inferColumnFromIssue()`, `addCardToColumn()` |
| `internal/dashboard/templates/board.html` | 7, 99-112 | Update grid to 8 columns, replace In Progress section with Plan and Code sections, add CSS |

### 3. Implementation Approach

**Data Layer:**
- Replace `Progress []taskCard` with `Plan []taskCard` and `Code []taskCard` in `boardData` struct
- Update `ProjectColumns` from `["Backlog", "In Progress", "AI Review", "Approve", "Done", "Blocked"]` to `["Blocked", "Backlog", "Plan", "Code", "AI Review", "Approve", "Done", "Failed"]`

**Logic Layer:**
- Modify `inferColumnFromIssue()` to map labels:
  - `state:analyze` or `state:plan` → "Plan"
  - `state:coding` → "Code"
  - Remove old `in-progress` label mapping
- Update `addCardToColumn()` switch statement to handle "Plan" and "Code" cases

**Presentation Layer:**
- Change CSS grid from `repeat(7,1fr)` to `repeat(8,1fr)`
- Add two new column sections with styling:
  - `.col-plan`: Yellow/orange (#f39c12) border and title
  - `.col-code`: Blue (#3498db) border and title

### 4. Testing Strategy

**Unit Tests:**
- Update `handlers_test.go` tests that reference `data.Progress` → `data.Plan`/`data.Code`
- Add tests for `inferColumnFromIssue()` with new state labels
- Verify `addCardToColumn()` correctly routes to Plan vs Code columns

**Integration Tests:**
- Test board renders with 8 columns in correct order
- Verify tickets with `state:analyze` appear in Plan column
- Verify tickets with `state:coding` appear in Code column
- Test responsive layout on mobile viewport

**Edge Cases:**
- Tickets with both `state:plan` and `state:coding` labels (should go to Code - more specific)
- Tickets with no state labels (default to Backlog)
- Existing tickets with old `in-progress` label (need migration or backward compatibility)

