# GitHub Issue #89: Move Wizard from Modal to Page - Implementation Plan

> **For Claude:** REQUIRED SUB-SKILL: Use superpowers:executing-plans to implement this plan task-by-task.

**Goal:** Move the 4-step wizard (Idea → Refine → Breakdown → Create) for creating new features/bugs from a modal overlay to a dedicated full-page experience.

**Architecture:** Create a dual-mode system where the wizard can render as either a modal fragment (for backward compatibility) or a full page (new default). Add a `?page=1` query parameter to detect page mode, update all HTMX targets to use a shared content container, and modify navigation to redirect instead of closing modals.

**Tech Stack:** Go 1.24, HTMX, standard Go templates

---

## Analysis Summary

**Current State:**
- Modal triggered via HTMX: `hx-get="/wizard/modal?type=feature"`
- Steps swap content in `#wizard-modal-content` container
- Close button calls `closeWizardModal()` JavaScript function
- Modal backdrop has z-index: 1000

**Problems with Modal:**
- Z-index conflicts with other UI elements
- HTMX swapping issues within modal context
- Mobile responsiveness problems
- Session management complications
- "Almost nothing works because of it"

**Solution:**
- New route `GET /wizard` renders wizard as full page with layout
- Add `?page=1` query param to detect page vs modal mode
- Update all step handlers to support both modes
- Change cancel/close to redirect to `/` instead of closing modal
- Keep modal routes for backward compatibility (optional)

---

## Task 1: Add New Route for Full-Page Wizard

**Files:**
- Modify: `internal/dashboard/server.go:210-218`

**Step 1: Add new route handler registration**

Add route registration after existing wizard routes:

```go
// Wizard routes - with CSRF protection
s.mux.HandleFunc("GET /wizard/new", s.handleWizardNew)
s.mux.HandleFunc("GET /wizard/modal", s.handleWizardModal)
s.mux.HandleFunc("POST /wizard/cancel", s.csrfMiddleware(s.handleWizardCancel))
s.mux.HandleFunc("POST /wizard/refine", s.csrfMiddleware(s.handleWizardRefine))
s.mux.HandleFunc("POST /wizard/breakdown", s.csrfMiddleware(s.handleWizardBreakdown))
s.mux.HandleFunc("POST /wizard/create", s.csrfMiddleware(s.handleWizardCreate))
s.mux.HandleFunc("GET /wizard/logs/{sessionId}", s.handleWizardLogs)

// NEW: Full-page wizard route
s.mux.HandleFunc("GET /wizard", s.handleWizardPage)
```

**Step 2: Verify route order**

Ensure `/wizard` route comes after `/wizard/*` specific routes to avoid conflicts.

**Step 3: Commit**

```bash
git add internal/dashboard/server.go
git commit -m "feat: add /wizard route for full-page wizard"
```

---

## Task 2: Create handleWizardPage Handler

**Files:**
- Create: `internal/dashboard/handlers.go` (add new handler function)
- Location: After `handleWizardModal` (around line 1376)

**Step 1: Write the handler function**

```go
// handleWizardPage returns the wizard as a full page (not modal)
func (s *Server) handleWizardPage(w http.ResponseWriter, r *http.Request) {
	// Get wizard type from query param (default to feature)
	wizardType := r.URL.Query().Get("type")
	if wizardType != "bug" {
		wizardType = "feature"
	}

	// Create new session
	session, err := s.wizardStore.Create(wizardType)
	if err != nil {
		http.Error(w, "invalid wizard type", http.StatusBadRequest)
		return
	}

	data := struct {
		Type        string
		SessionID   string
		CurrentStep int
		IsPage      bool // Flag to indicate page mode
	}{
		Type:        wizardType,
		SessionID:   session.ID,
		CurrentStep: 1,
		IsPage:      true,
	}

	// Render full page with layout
	s.renderTemplate(w, "wizard_page.html", data)
}
```

**Step 2: Verify handler compiles**

Run: `go build ./internal/dashboard/...`
Expected: No compilation errors

**Step 3: Commit**

```bash
git add internal/dashboard/handlers.go
git commit -m "feat: add handleWizardPage handler for full-page wizard"
```

---

## Task 3: Create wizard_page.html Template

**Files:**
- Create: `internal/dashboard/templates/wizard_page.html`

**Step 1: Write the full-page template**

```html
{{define "content"}}
<div class="wizard-page-container">
  <div class="wizard-page-header">
    <div class="step-indicator">
      <div class="step {{if eq .CurrentStep 1}}active{{end}} {{if gt .CurrentStep 1}}completed{{end}}">
        <span class="step-number">1</span>
        <span class="step-label">Idea</span>
      </div>
      <div class="step-connector"></div>
      <div class="step {{if eq .CurrentStep 2}}active{{end}} {{if gt .CurrentStep 2}}completed{{end}}">
        <span class="step-number">2</span>
        <span class="step-label">Refine</span>
      </div>
      <div class="step-connector"></div>
      <div class="step {{if eq .CurrentStep 3}}active{{end}} {{if gt .CurrentStep 3}}completed{{end}}">
        <span class="step-number">3</span>
        <span class="step-label">Breakdown</span>
      </div>
      <div class="step-connector"></div>
      <div class="step {{if eq .CurrentStep 4}}active{{end}}">
        <span class="step-number">4</span>
        <span class="step-label">Create</span>
      </div>
    </div>
    <a href="/" class="wizard-close-btn" aria-label="Close wizard">✕</a>
  </div>
  
  <div id="wizard-content" class="wizard-page-content">
    {{template "wizard-step-content" .}}
  </div>
</div>

<style>
.wizard-page-container {
  max-width: 900px;
  margin: 0 auto;
  padding: 2rem 1rem;
}

.wizard-page-header {
  display: flex;
  align-items: center;
  justify-content: space-between;
  margin-bottom: 2rem;
  padding-bottom: 1rem;
  border-bottom: 1px solid var(--border);
}

.step-indicator {
  display: flex;
  align-items: center;
  gap: 0.5rem;
  flex: 1;
}

.step {
  display: flex;
  align-items: center;
  gap: 0.5rem;
  opacity: 0.5;
  transition: opacity 0.2s;
}

.step.active {
  opacity: 1;
}

.step.completed {
  opacity: 0.8;
}

.step-number {
  width: 28px;
  height: 28px;
  border-radius: 50%;
  background: var(--border);
  color: var(--text);
  display: flex;
  align-items: center;
  justify-content: center;
  font-size: 0.8rem;
  font-weight: 600;
  transition: background 0.2s, color 0.2s;
}

.step.active .step-number {
  background: var(--accent);
  color: #fff;
}

.step.completed .step-number {
  background: var(--green);
  color: #fff;
}

.step-label {
  font-size: 0.85rem;
  font-weight: 500;
  color: var(--text);
}

.step-connector {
  width: 24px;
  height: 2px;
  background: var(--border);
  margin: 0 0.25rem;
}

.wizard-close-btn {
  background: none;
  border: none;
  color: var(--muted);
  cursor: pointer;
  padding: 0.5rem;
  border-radius: 6px;
  display: flex;
  align-items: center;
  justify-content: center;
  transition: color 0.2s, background 0.2s;
  text-decoration: none;
  font-size: 1.25rem;
}

.wizard-close-btn:hover {
  color: var(--text);
  background: var(--border);
}

.wizard-page-content {
  min-height: 400px;
}

/* Responsive adjustments */
@media (max-width: 640px) {
  .wizard-page-container {
    padding: 1rem;
  }
  
  .step-label {
    display: none;
  }
  
  .step-connector {
    width: 16px;
  }
}
</style>
{{end}}
```

**Step 2: Verify template syntax**

Run: `go test ./internal/dashboard/... -run TestNothing`
Expected: Templates compile without errors

**Step 3: Commit**

```bash
git add internal/dashboard/templates/wizard_page.html
git commit -m "feat: create wizard_page.html template for full-page wizard"
```

---

## Task 4: Update wizard_new.html for Page Mode

**Files:**
- Modify: `internal/dashboard/templates/wizard_new.html:1-53`

**Step 1: Update form to support both modal and page modes**

Change line 5 from:
```html
<form hx-post="/wizard/refine" hx-target="#wizard-modal-content" hx-swap="innerHTML" hx-disabled-elt="button[type='submit']">
```

To:
```html
<form hx-post="/wizard/refine{{if .IsPage}}?page=1{{end}}" hx-target="#wizard-content" hx-swap="innerHTML" hx-disabled-elt="button[type='submit']">
```

**Step 2: Update cancel button for page mode**

Change line 15 from:
```html
<button type="button" class="btn" onclick="closeWizardModal()">Cancel</button>
```

To:
```html
{{if .IsPage}}
<a href="/" class="btn">Cancel</a>
{{else}}
<button type="button" class="btn" onclick="closeWizardModal()">Cancel</button>
{{end}}
```

**Step 3: Update logs polling for page mode**

Change line 23 from:
```html
<div id="wizard-logs" hx-get="/wizard/logs/{{.SessionID}}" hx-trigger="every 3s" hx-swap="innerHTML" style="...">
```

To:
```html
<div id="wizard-logs" hx-get="/wizard/logs/{{.SessionID}}{{if .IsPage}}?page=1{{end}}" hx-trigger="every 3s" hx-swap="innerHTML" style="...">
```

**Step 4: Commit**

```bash
git add internal/dashboard/templates/wizard_new.html
git commit -m "feat: update wizard_new.html to support page mode"
```

---

## Task 5: Update wizard_refine.html for Page Mode

**Files:**
- Modify: `internal/dashboard/templates/wizard_refine.html:1-61`

**Step 1: Update form target and add page param**

Change line 4 from:
```html
<form hx-post="/wizard/breakdown" hx-target="#wizard-modal-content" hx-swap="innerHTML" hx-disabled-elt="button[type='submit']">
```

To:
```html
<form hx-post="/wizard/breakdown{{if .IsPage}}?page=1{{end}}" hx-target="#wizard-content" hx-swap="innerHTML" hx-disabled-elt="button[type='submit']">
```

**Step 2: Update cancel button for page mode**

Change line 15 from:
```html
<button type="button" class="btn" onclick="closeWizardModal()">Cancel</button>
```

To:
```html
{{if .IsPage}}
<a href="/" class="btn">Cancel</a>
{{else}}
<button type="button" class="btn" onclick="closeWizardModal()">Cancel</button>
{{end}}
```

**Step 3: Update back button for page mode**

Change line 17 from:
```html
<button type="button" class="btn" hx-get="/wizard/new?type={{.Type}}&amp;session_id={{.SessionID}}" hx-target="#wizard-modal-content">
```

To:
```html
<button type="button" class="btn" hx-get="/wizard/new?type={{.Type}}&amp;session_id={{.SessionID}}{{if .IsPage}}&amp;page=1{{end}}" hx-target="#wizard-content">
```

**Step 4: Update refine again button for page mode**

Change line 20 from:
```html
<button type="button" class="btn btn-secondary" hx-post="/wizard/refine" hx-target="#wizard-modal-content" hx-disabled-elt="button[type='submit']" hx-vals='{"current_description": "{{.RefinedDescription}}"}'>
```

To:
```html
<button type="button" class="btn btn-secondary" hx-post="/wizard/refine{{if .IsPage}}?page=1{{end}}" hx-target="#wizard-content" hx-disabled-elt="button[type='submit']" hx-vals='{"current_description": "{{.RefinedDescription}}"}'>
```

**Step 5: Update logs polling for page mode**

Change line 32 from:
```html
<div id="wizard-logs" hx-get="/wizard/logs/{{.SessionID}}" hx-trigger="every 1s" hx-swap="innerHTML" hx-headers='{"X-Expected-Step": "refine"}' style="...">
```

To:
```html
<div id="wizard-logs" hx-get="/wizard/logs/{{.SessionID}}{{if .IsPage}}?page=1{{end}}" hx-trigger="every 1s" hx-swap="innerHTML" hx-headers='{"X-Expected-Step": "refine"}' style="...">
```

**Step 6: Commit**

```bash
git add internal/dashboard/templates/wizard_refine.html
git commit -m "feat: update wizard_refine.html to support page mode"
```

---

## Task 6: Update wizard_breakdown.html for Page Mode

**Files:**
- Modify: `internal/dashboard/templates/wizard_breakdown.html:1-82`

**Step 1: Update form target and add page param**

Change line 21 from:
```html
<form hx-post="/wizard/create" hx-target="#wizard-modal-content" hx-swap="innerHTML">
```

To:
```html
<form hx-post="/wizard/create{{if .IsPage}}?page=1{{end}}" hx-target="#wizard-content" hx-swap="innerHTML">
```

**Step 2: Update cancel button for page mode**

Change line 25 from:
```html
<button type="button" class="btn" onclick="closeWizardModal()">Cancel</button>
```

To:
```html
{{if .IsPage}}
<a href="/" class="btn">Cancel</a>
{{else}}
<button type="button" class="btn" onclick="closeWizardModal()">Cancel</button>
{{end}}
```

**Step 3: Update back button for page mode**

Change line 27 from:
```html
<button type="button" class="btn" hx-get="/wizard/refine?session_id={{.SessionID}}" hx-target="#wizard-modal-content">
```

To:
```html
<button type="button" class="btn" hx-get="/wizard/refine?session_id={{.SessionID}}{{if .IsPage}}&amp;page=1{{end}}" hx-target="#wizard-content">
```

**Step 4: Update logs polling for page mode**

Change line 38 from:
```html
<div id="wizard-logs" hx-get="/wizard/logs/{{.SessionID}}" hx-trigger="every 1s" hx-swap="innerHTML" hx-headers='{"X-Expected-Step": "breakdown"}' style="...">
```

To:
```html
<div id="wizard-logs" hx-get="/wizard/logs/{{.SessionID}}{{if .IsPage}}?page=1{{end}}" hx-trigger="every 1s" hx-swap="innerHTML" hx-headers='{"X-Expected-Step": "breakdown"}' style="...">
```

**Step 5: Commit**

```bash
git add internal/dashboard/templates/wizard_breakdown.html
git commit -m "feat: update wizard_breakdown.html to support page mode"
```

---

## Task 7: Update wizard_create.html for Page Mode

**Files:**
- Modify: `internal/dashboard/templates/wizard_create.html:1-130`

**Step 1: Update close button for page mode**

Change lines 43-48 from:
```html
<div class="form-actions">
  <button type="button" class="btn btn-primary" onclick="closeWizardModal()">
    Close Wizard
  </button>
  <a href="/backlog" class="btn" onclick="closeWizardModal()">View Backlog →</a>
</div>
```

To:
```html
<div class="form-actions">
  {{if .IsPage}}
  <a href="/" class="btn btn-primary">Close Wizard</a>
  {{else}}
  <button type="button" class="btn btn-primary" onclick="closeWizardModal()">
    Close Wizard
  </button>
  {{end}}
  <a href="/backlog" class="btn">View Backlog →</a>
</div>
```

**Step 2: Commit**

```bash
git add internal/dashboard/templates/wizard_create.html
git commit -m "feat: update wizard_create.html to support page mode"
```

---

## Task 8: Update Step Handlers to Support Page Mode

**Files:**
- Modify: `internal/dashboard/handlers.go:781-824` (handleWizardNew)
- Modify: `internal/dashboard/handlers.go:826-977` (handleWizardRefine)
- Modify: `internal/dashboard/handlers.go:980-1087` (handleWizardBreakdown)
- Modify: `internal/dashboard/handlers.go:1115-1300` (handleWizardCreate)

**Step 1: Update handleWizardNew to detect page mode**

Add at the beginning of the handler (after line 783):
```go
// Check if this is a page mode request (vs modal)
isPage := r.URL.Query().Get("page") == "1"
```

Update the data struct (around line 815) to include IsPage:
```go
data := struct {
	Type      string
	SessionID string
	IsPage    bool
}{
	Type:      wizardType,
	SessionID: session.ID,
	IsPage:    isPage,
}
```

**Step 2: Update handleWizardRefine to detect page mode**

Add at the beginning of the handler (after line 827):
```go
// Check if this is a page mode request
isPage := r.URL.Query().Get("page") == "1"
```

Update the data struct for rendering wizard_refine.html (around line 879) to include IsPage and Type:
```go
data := struct {
	SessionID          string
	Type               string
	RefinedDescription string
	IsPage             bool
}{
	SessionID:          session.ID,
	Type:               string(session.Type),
	RefinedDescription: mockRefined,
	IsPage:             isPage,
}
```

Also update the real LLM response data struct (around line 920) similarly.

**Step 3: Update handleWizardBreakdown to detect page mode**

Add at the beginning of the handler (after line 981):
```go
// Check if this is a page mode request
isPage := r.URL.Query().Get("page") == "1"
```

Update the data struct (around line 1078) to include IsPage:
```go
data := struct {
	SessionID string
	Tasks     []WizardTask
	IsPage    bool
}{
	SessionID: session.ID,
	Tasks:     tasks,
	IsPage:    isPage,
}
```

**Step 4: Update handleWizardCreate to detect page mode**

Add at the beginning of the handler (after line 1116):
```go
// Check if this is a page mode request
isPage := r.URL.Query().Get("page") == "1"
```

Update the data struct for rendering wizard_create.html (around line 1289) to include IsPage:
```go
data := struct {
	Epic      CreatedIssue
	SubTasks  []CreatedIssue
	HasErrors bool
	IsPage    bool
}{
	Epic:      epic,
	SubTasks:  subTasks,
	HasErrors: hasErrors,
	IsPage:    isPage,
}
```

**Step 5: Verify handlers compile**

Run: `go build ./internal/dashboard/...`
Expected: No compilation errors

**Step 6: Commit**

```bash
git add internal/dashboard/handlers.go
git commit -m "feat: update step handlers to support page mode with IsPage flag"
```

---

## Task 9: Update layout.html Navigation Buttons

**Files:**
- Modify: `internal/dashboard/templates/layout.html:43-44`

**Step 1: Change HTMX modal triggers to page links**

Change from:
```html
<div class="nav-actions">
  <button class="btn btn-success" hx-get="/wizard/modal?type=feature" hx-target="body" hx-swap="beforeend">+ New Feature</button>
  <button class="btn btn-danger" hx-get="/wizard/modal?type=bug" hx-target="body" hx-swap="beforeend">+ New Bug</button>
</div>
```

To:
```html
<div class="nav-actions">
  <a href="/wizard?type=feature" class="btn btn-success">+ New Feature</a>
  <a href="/wizard?type=bug" class="btn btn-danger">+ New Bug</a>
</div>
```

**Step 2: Commit**

```bash
git add internal/dashboard/templates/layout.html
git commit -m "feat: update nav buttons to use full-page wizard instead of modal"
```

---

## Task 10: Update board.html Action Buttons

**Files:**
- Modify: `internal/dashboard/templates/board.html:41-46`

**Step 1: Change HTMX modal triggers to page links**

Change from:
```html
<div class="board-actions">
  <button class="btn btn-primary" hx-get="/wizard/modal?type=feature" hx-target="#modal-container" hx-swap="innerHTML">
    + Feature
  </button>
  <button class="btn" hx-get="/wizard/modal?type=bug" hx-target="#modal-container" hx-swap="innerHTML">
    + Bug
  </button>
```

To:
```html
<div class="board-actions">
  <a href="/wizard?type=feature" class="btn btn-primary">+ Feature</a>
  <a href="/wizard?type=bug" class="btn">+ Bug</a>
```

**Step 2: Commit**

```bash
git add internal/dashboard/templates/board.html
git commit -m "feat: update board buttons to use full-page wizard instead of modal"
```

---

## Task 11: Add Tests for Page Mode

**Files:**
- Modify: `internal/dashboard/handlers_test.go`

**Step 1: Add test for handleWizardPage handler**

Add new test function after existing wizard tests:

```go
func TestHandleWizardPage(t *testing.T) {
	srv := NewTestServer(t)

	// Test GET /wizard with type=feature
	req := httptest.NewRequest("GET", "/wizard?type=feature", nil)
	rec := httptest.NewRecorder()
	srv.handleWizardPage(rec, req)

	assert.Equal(t, http.StatusOK, rec.Code)
	assert.Contains(t, rec.Body.String(), "Create New Feature")
	assert.Contains(t, rec.Body.String(), "wizard-page-container")

	// Test GET /wizard with type=bug
	req = httptest.NewRequest("GET", "/wizard?type=bug", nil)
	rec = httptest.NewRecorder()
	srv.handleWizardPage(rec, req)

	assert.Equal(t, http.StatusOK, rec.Code)
	assert.Contains(t, rec.Body.String(), "Create New Bug Report")
	assert.Contains(t, rec.Body.String(), "wizard-page-container")

	// Test GET /wizard without type (defaults to feature)
	req = httptest.NewRequest("GET", "/wizard", nil)
	rec = httptest.NewRecorder()
	srv.handleWizardPage(rec, req)

	assert.Equal(t, http.StatusOK, rec.Code)
	assert.Contains(t, rec.Body.String(), "Create New Feature")
}
```

**Step 2: Add test for page mode in step handlers**

Add test to verify step handlers work with `?page=1` parameter:

```go
func TestWizardStepHandlers_PageMode(t *testing.T) {
	srv := NewTestServer(t)

	// Create a session first
	req := httptest.NewRequest("GET", "/wizard?type=feature", nil)
	rec := httptest.NewRecorder()
	srv.handleWizardPage(rec, req)
	assert.Equal(t, http.StatusOK, rec.Code)

	// Extract session ID from response
	body := rec.Body.String()
	// Find session_id in the form
	// This is a simplified check - in real test you'd parse the HTML
	assert.Contains(t, body, "session_id")

	// Test that refine handler accepts page=1 parameter
	// (Full integration test would require parsing session ID and submitting form)
}
```

**Step 3: Run tests**

Run: `go test ./internal/dashboard/... -run TestWizard -v`
Expected: All tests pass

**Step 4: Commit**

```bash
git add internal/dashboard/handlers_test.go
git commit -m "test: add tests for full-page wizard mode"
```

---

## Task 12: Integration Testing

**Files:**
- Manual testing via browser

**Step 1: Start the server**

Run: `go run ./cmd/dashboard`
Expected: Server starts on configured port

**Step 2: Test full feature creation flow**

1. Navigate to `/wizard?type=feature`
2. Verify page renders with step indicator
3. Enter feature description
4. Click "Refine with AI"
5. Verify refine step loads with refined description
6. Click "Accept & Continue"
7. Verify breakdown step loads with task list
8. Click "Create GitHub Issues"
9. Verify success page loads with epic and sub-tasks
10. Click "Close Wizard" - should redirect to `/`

**Step 3: Test full bug creation flow**

Repeat Step 2 with `/wizard?type=bug`

**Step 4: Test cancel at each step**

At each step, click Cancel and verify redirect to `/`

**Step 5: Test back navigation**

Use browser back button between steps and verify state is maintained

**Step 6: Test mobile responsiveness**

Use browser dev tools to test at 375px width (iPhone SE)
Verify step labels are hidden and layout is usable

**Step 7: Commit test results**

```bash
git commit -m "test: verify full-page wizard works end-to-end"
```

---

## Task 13: Cleanup (Optional)

**Files:**
- Consider removing: `internal/dashboard/templates/wizard_modal.html`
- Consider removing: `internal/dashboard/handlers.go:handleWizardModal` (lines 1350-1376)
- Consider removing: `internal/dashboard/server.go:212` (GET /wizard/modal route)

**Note:** Only perform this cleanup if backward compatibility is not needed. Otherwise, keep modal support for potential future use.

**Step 1: Document decision**

Add comment in code explaining modal routes are deprecated but kept for compatibility.

**Step 2: Commit**

```bash
git commit -m "docs: mark modal wizard as deprecated in favor of page mode"
```

---

## Summary

This implementation plan moves the wizard from a modal to a full-page experience by:

1. **Adding a new route** (`/wizard`) that renders the wizard as a full page
2. **Creating a new template** (`wizard_page.html`) that wraps the wizard in a page layout
3. **Updating all step templates** to support both modal and page modes via `IsPage` flag
4. **Modifying step handlers** to detect page mode via `?page=1` query parameter
5. **Updating navigation** in `layout.html` and `board.html` to use page links instead of HTMX modal triggers
6. **Adding tests** to verify the new page mode works correctly
7. **Testing end-to-end** to ensure the full flow works as expected

The solution maintains backward compatibility with the modal system while making the page mode the new default for all entry points.
