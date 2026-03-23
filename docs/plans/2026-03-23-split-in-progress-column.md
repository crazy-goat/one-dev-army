# Split In Progress Column into Plan and Code - Implementation Plan

> **For Claude:** REQUIRED SUB-SKILL: Use superpowers:executing-plans to implement this plan task-by-task.

**Goal:** Split the single "In Progress" column into two distinct columns: "Plan" (for analyzing/planning states) and "Code" (for coding state), updating the data model, business logic, and UI.

**Architecture:** Update the GitHub project column definitions, modify the dashboard board data structure to separate Plan and Code fields, update the issue-to-column inference logic to route based on state labels, and redesign the board template to display the two new columns with distinct styling.

**Tech Stack:** Go 1.24, HTML templates, GitHub Projects API

---

## Overview

This plan splits the existing "In Progress" column into two columns:
- **Plan** (yellow/orange): Contains issues with `state:analyze` or `state:plan` labels
- **Code** (blue): Contains issues with `state:coding` label

New column order: Blocked → Backlog → **Plan** → **Code** → AI Review → Approve → Done → Failed

---

## Task 1: Update Project Column Definitions

**Files:**
- Modify: `internal/github/project.go:10-17` (ProjectColumns array)
- Modify: `internal/github/project.go:168-175` (columnColors map)

**Step 1: Update ProjectColumns array**

Replace the current 6-column array with the new 8-column order:

```go
var ProjectColumns = []string{
	"Blocked",
	"Backlog",
	"Plan",
	"Code",
	"AI Review",
	"Approve",
	"Done",
	"Failed",
}
```

**Step 2: Update columnColors map**

Replace the current map to include new columns with appropriate colors:

```go
var columnColors = map[string]string{
	"Blocked":   "RED",
	"Backlog":   "GRAY",
	"Plan":      "YELLOW",
	"Code":      "BLUE",
	"AI Review": "YELLOW",
	"Approve":   "PURPLE",
	"Done":      "GREEN",
	"Failed":    "RED",
}
```

**Step 3: Verify changes compile**

Run: `go build ./internal/github/...`
Expected: No errors

**Step 4: Commit**

```bash
git add internal/github/project.go
git commit -m "feat: update ProjectColumns to split In Progress into Plan and Code

- Reorder columns: Blocked, Backlog, Plan, Code, AI Review, Approve, Done, Failed
- Assign colors: Plan=YELLOW, Code=BLUE"
```

---

## Task 2: Update Dashboard Board Data Structure

**Files:**
- Modify: `internal/dashboard/handlers.go:30-43` (boardData struct)
- Modify: `internal/dashboard/handlers.go:63-84` (placeholderBoard function)

**Step 1: Update boardData struct**

Replace the Progress field with separate Plan and Code fields:

```go
type boardData struct {
	Active       string
	SprintName   string
	Paused       bool
	Processing   bool
	CurrentIssue string
	Blocked      []taskCard
	Backlog      []taskCard
	Plan         []taskCard  // NEW: Replaces Progress for analyzing/planning
	Code         []taskCard  // NEW: For coding state
	AIReview     []taskCard
	Approve      []taskCard
	Done         []taskCard
	Failed       []taskCard
}
```

**Step 2: Update placeholderBoard function**

Replace the placeholder data to use new fields:

```go
func placeholderBoard() boardData {
	return boardData{
		Active: "board",
		Blocked: []taskCard{
			{ID: 6, Title: "Deploy to staging", Status: "blocked"},
		},
		Backlog: []taskCard{
			{ID: 1, Title: "Set up CI pipeline", Status: "backlog"},
			{ID: 2, Title: "Add logging middleware", Status: "backlog"},
		},
		Plan: []taskCard{
			{ID: 3, Title: "Design auth service", Status: "plan", Worker: "worker-1"},
		},
		Code: []taskCard{
			{ID: 7, Title: "Implement auth service", Status: "coding", Worker: "worker-2"},
		},
		AIReview: []taskCard{
			{ID: 4, Title: "Database migrations", Status: "ai_review"},
		},
		Approve: []taskCard{},
		Done: []taskCard{
			{ID: 5, Title: "Project skeleton", Status: "done"},
		},
		Failed: []taskCard{},
	}
}
```

**Step 3: Verify changes compile**

Run: `go build ./internal/dashboard/...`
Expected: No errors (will have unused field warnings for now)

**Step 4: Commit**

```bash
git add internal/dashboard/handlers.go
git commit -m "feat: update boardData struct with Plan and Code columns

- Replace Progress field with separate Plan and Code fields
- Update placeholderBoard with sample data for both columns"
```

---

## Task 3: Update Issue-to-Column Inference Logic

**Files:**
- Modify: `internal/dashboard/handlers.go:205-239` (inferColumnFromIssue function)

**Step 1: Replace inferColumnFromIssue function**

Update the function to route issues based on state labels to the new columns:

```go
func inferColumnFromIssue(issue github.Issue) string {
	// Check labels first
	labels := issue.GetLabelNames()
	labelSet := make(map[string]bool)
	for _, l := range labels {
		labelSet[strings.ToLower(l)] = true
	}

	// Map labels to columns
	if labelSet["failed"] {
		return "Failed"
	}
	if labelSet["blocked"] || labelSet["blocker"] {
		return "Blocked"
	}
	
	// NEW: Route to Plan column for analyzing/planning states
	if labelSet["state:analyze"] || labelSet["state:plan"] || 
	   labelSet["analyzing"] || labelSet["planning"] {
		return "Plan"
	}
	
	// NEW: Route to Code column for coding state
	if labelSet["state:coding"] || labelSet["coding"] {
		return "Code"
	}
	
	// DEPRECATED: Keep for backward compatibility during transition
	if labelSet["in-progress"] || labelSet["in progress"] || labelSet["wip"] || labelSet["working"] {
		return "Code" // Default old in-progress to Code
	}
	
	if labelSet["review"] || labelSet["in-review"] || labelSet["pr-ready"] {
		return "AI Review"
	}
	if labelSet["awaiting-approval"] || labelSet["approve"] || labelSet["merge-ready"] {
		return "Approve"
	}
	if labelSet["done"] || labelSet["completed"] || labelSet["finished"] {
		return "Done"
	}

	if strings.EqualFold(issue.State, "CLOSED") {
		return "Done"
	}

	// Default to Backlog
	return "Backlog"
}
```

**Step 2: Verify changes compile**

Run: `go build ./internal/dashboard/...`
Expected: No errors

**Step 3: Commit**

```bash
git add internal/dashboard/handlers.go
git commit -m "feat: update inferColumnFromIssue to route to Plan and Code columns

- Add routing for state:analyze, state:plan → Plan column
- Add routing for state:coding → Code column
- Map deprecated in-progress labels to Code for backward compatibility"
```

---

## Task 4: Update Card-to-Column Assignment Logic

**Files:**
- Modify: `internal/dashboard/handlers.go:241-271` (addCardToColumn function)

**Step 1: Replace addCardToColumn function**

Update the switch statement to handle the new column names:

```go
func (s *Server) addCardToColumn(data *boardData, col string, issue github.Issue) {
	card := taskCard{
		ID:       issue.Number,
		Title:    issue.Title,
		Status:   col,
		Assignee: issue.GetAssignee(),
		Labels:   issue.GetLabelNames(),
	}

	switch col {
	case "Blocked":
		data.Blocked = append(data.Blocked, card)
	case "Backlog":
		data.Backlog = append(data.Backlog, card)
	case "Plan":
		data.Plan = append(data.Plan, card)
	case "Code":
		data.Code = append(data.Code, card)
	case "AI Review":
		data.AIReview = append(data.AIReview, card)
	case "Approve":
		if s.store != nil {
			if prURL, err := s.store.GetStepResponse(issue.Number, "create-pr"); err == nil && prURL != "" {
				card.PRURL = prURL
			}
		}
		data.Approve = append(data.Approve, card)
	case "Done":
		data.Done = append(data.Done, card)
	case "Failed":
		data.Failed = append(data.Failed, card)
	}
}
```

**Step 2: Verify changes compile**

Run: `go build ./internal/dashboard/...`
Expected: No errors

**Step 3: Commit**

```bash
git add internal/dashboard/handlers.go
git commit -m "feat: update addCardToColumn to handle Plan and Code columns

- Add cases for Plan and Code columns
- Remove deprecated In Progress case"
```

---

## Task 5: Update Board Template - Grid and CSS

**Files:**
- Modify: `internal/dashboard/templates/board.html:7` (grid CSS)
- Modify: `internal/dashboard/templates/board.html:23-30` (column color CSS)

**Step 1: Update grid to 8 columns**

Change line 7 from:
```css
.board{display:grid;grid-template-columns:repeat(7,1fr);gap:1rem;margin-bottom:2rem}
```

To:
```css
.board{display:grid;grid-template-columns:repeat(8,1fr);gap:1rem;margin-bottom:2rem}
```

**Step 2: Add CSS for new column colors**

After line 30 (after `.card .card-pr a:hover` styles), add:

```css
.col-plan .card{border-color:#f39c12;background:rgba(243,156,18,0.05)}
.col-plan .column-title{color:#f39c12}
.col-code .card{border-color:#3498db;background:rgba(52,152,219,0.05)}
.col-code .column-title{color:#3498db}
```

**Step 3: Verify template syntax**

Run: `go build ./internal/dashboard/...`
Expected: No errors

**Step 4: Commit**

```bash
git add internal/dashboard/templates/board.html
git commit -m "feat: update board template grid and CSS for Plan and Code columns

- Change grid from 7 to 8 columns
- Add .col-plan styling with yellow/orange (#f39c12)
- Add .col-code styling with blue (#3498db)"
```

---

## Task 6: Update Board Template - Replace In Progress Section

**Files:**
- Modify: `internal/dashboard/templates/board.html:99-112` (In Progress column section)

**Step 1: Replace In Progress section with Plan and Code sections**

Replace lines 99-112:

```html
  <div class="column">
    <div class="column-title">In Progress <span class="count">{{len .Progress}}</span></div>
    {{range .Progress}}
    <div class="card">
      <div class="card-id"><a href="/task/{{.ID}}">#{{.ID}}</a></div>
      <div class="card-title">{{.Title}}</div>
      {{if .Labels}}<div class="card-meta"><div class="card-labels">{{range .Labels}}<span class="label">{{.}}</span>{{end}}</div></div>{{end}}
      {{if .Assignee}}<div class="card-meta"><div class="card-assignee">@{{.Assignee}}</div></div>{{end}}
      {{if .Worker}}<div class="card-worker">{{.Worker}}</div>{{end}}
    </div>
    {{else}}
    <div class="empty-state">No tickets in progress</div>
    {{end}}
  </div>
```

With two separate sections:

```html
  <div class="column col-plan">
    <div class="column-title">Plan <span class="count">{{len .Plan}}</span></div>
    {{range .Plan}}
    <div class="card">
      <div class="card-id"><a href="/task/{{.ID}}">#{{.ID}}</a></div>
      <div class="card-title">{{.Title}}</div>
      {{if .Labels}}<div class="card-meta"><div class="card-labels">{{range .Labels}}<span class="label">{{.}}</span>{{end}}</div></div>{{end}}
      {{if .Assignee}}<div class="card-meta"><div class="card-assignee">@{{.Assignee}}</div></div>{{end}}
      {{if .Worker}}<div class="card-worker">{{.Worker}}</div>{{end}}
    </div>
    {{else}}
    <div class="empty-state">No tickets in planning</div>
    {{end}}
  </div>

  <div class="column col-code">
    <div class="column-title">Code <span class="count">{{len .Code}}</span></div>
    {{range .Code}}
    <div class="card">
      <div class="card-id"><a href="/task/{{.ID}}">#{{.ID}}</a></div>
      <div class="card-title">{{.Title}}</div>
      {{if .Labels}}<div class="card-meta"><div class="card-labels">{{range .Labels}}<span class="label">{{.}}</span>{{end}}</div></div>{{end}}
      {{if .Assignee}}<div class="card-meta"><div class="card-assignee">@{{.Assignee}}</div></div>{{end}}
      {{if .Worker}}<div class="card-worker">{{.Worker}}</div>{{end}}
    </div>
    {{else}}
    <div class="empty-state">No tickets in coding</div>
    {{end}}
  </div>
```

**Step 2: Verify template syntax**

Run: `go build ./internal/dashboard/...`
Expected: No errors

**Step 3: Commit**

```bash
git add internal/dashboard/templates/board.html
git commit -m "feat: replace In Progress column with Plan and Code columns in template

- Add Plan column section with col-plan class and yellow styling
- Add Code column section with col-code class and blue styling
- Update empty state messages for each column"
```

---

## Task 7: Update Handler Tests

**Files:**
- Modify: `internal/dashboard/handlers_test.go` (test file)

**Step 1: Check if test file exists and read it**

Run: `ls -la internal/dashboard/handlers_test.go`

If file exists, read it to understand current test structure.

**Step 2: Update tests to use new boardData fields**

If tests reference `data.Progress`, update to check `data.Plan` and `data.Code`:

```go
// OLD (if exists):
assert.Equal(t, 1, len(data.Progress))

// NEW:
assert.Equal(t, 1, len(data.Plan))
assert.Equal(t, 1, len(data.Code))
```

**Step 3: Add tests for inferColumnFromIssue with new labels**

Add test cases for the new routing logic:

```go
func TestInferColumnFromIssue_PlanColumn(t *testing.T) {
	tests := []struct {
		name   string
		labels []string
		want   string
	}{
		{"state:analyze label", []string{"state:analyze"}, "Plan"},
		{"state:plan label", []string{"state:plan"}, "Plan"},
		{"analyzing label", []string{"analyzing"}, "Plan"},
		{"planning label", []string{"planning"}, "Plan"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			issue := github.Issue{Labels: tt.labels}
			got := inferColumnFromIssue(issue)
			assert.Equal(t, tt.want, got)
		})
	}
}

func TestInferColumnFromIssue_CodeColumn(t *testing.T) {
	tests := []struct {
		name   string
		labels []string
		want   string
	}{
		{"state:coding label", []string{"state:coding"}, "Code"},
		{"coding label", []string{"coding"}, "Code"},
		{"deprecated in-progress maps to Code", []string{"in-progress"}, "Code"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			issue := github.Issue{Labels: tt.labels}
			got := inferColumnFromIssue(issue)
			assert.Equal(t, tt.want, got)
		})
	}
}
```

**Step 4: Run tests**

Run: `go test ./internal/dashboard/... -v`
Expected: All tests pass

**Step 5: Commit**

```bash
git add internal/dashboard/handlers_test.go
git commit -m "test: update handler tests for Plan and Code columns

- Update boardData assertions to check Plan and Code fields
- Add tests for inferColumnFromIssue with new state labels
- Verify backward compatibility for deprecated in-progress labels"
```

---

## Task 8: Full Build and Integration Test

**Step 1: Build entire project**

Run: `go build ./...`
Expected: No errors

**Step 2: Run all tests**

Run: `go test ./... -v`
Expected: All tests pass

**Step 3: Verify no references to old Progress field**

Run: `grep -r "\.Progress" --include="*.go" .`
Expected: No matches (or only in comments/docs)

Run: `grep -r "In Progress" --include="*.go" internal/`
Expected: Only in comments or backward compatibility code

**Step 4: Commit any final changes**

```bash
git add -A
git commit -m "chore: final verification and cleanup for Plan/Code column split

- Full build passes
- All tests pass
- No remaining references to deprecated Progress field"
```

---

## Testing Checklist

After implementation, verify:

- [ ] Dashboard displays 8 columns in correct order: Blocked, Backlog, Plan, Code, AI Review, Approve, Done, Failed
- [ ] Plan column has yellow/orange styling (#f39c12)
- [ ] Code column has blue styling (#3498db)
- [ ] Issues with `state:analyze` label appear in Plan column
- [ ] Issues with `state:plan` label appear in Plan column
- [ ] Issues with `state:coding` label appear in Code column
- [ ] Issues with deprecated `in-progress` label appear in Code column (backward compatibility)
- [ ] Grid layout accommodates 8 columns without overflow
- [ ] Empty states display appropriate messages for each column
- [ ] All existing functionality (approve, reject, retry) still works

---

## Migration Notes

**For existing issues with old labels:**
- Issues labeled `in-progress` will be routed to Code column
- To move an issue to Plan column, add `state:analyze` or `state:plan` label
- To ensure an issue is in Code column, add `state:coding` label

**GitHub Project columns will be automatically updated** when the application starts via `EnsureProjectColumns()` function.
