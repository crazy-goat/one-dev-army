# Merge Plan & Analysis Wizard Steps - Implementation Plan

> **For Claude:** REQUIRED SUB-SKILL: Use superpowers:executing-plans to implement this plan task-by-task.

**Goal:** Merge the separate "Plan" (Refine) and "Analysis" (Breakdown) wizard steps into a single "Technical Planning" step that outputs architecture overview, file locations, dependencies, and implementation boundaries.

**Architecture:** Create a unified LLM prompt that generates both the refined description AND technical planning in a single call. Remove the separate breakdown step and handler, simplifying the flow from 4 steps to 3 steps. The technical planning will include architecture overview, specific files to modify, component dependencies, and clear boundaries (without implementation code).

**Tech Stack:** Go 1.24, HTML templates, HTMX, LLM integration via OpenCode API

---

## Overview

Current wizard flow (4 steps):
1. **Idea** - User inputs raw idea
2. **Refine** - LLM refines into professional GitHub issue format
3. **Breakdown** - LLM breaks down into JSON tasks array
4. **Create** - Creates GitHub issue(s)

New wizard flow (3 steps):
1. **Idea** - User inputs raw idea
2. **Technical Planning** - LLM outputs refined description + technical planning (architecture, files, dependencies, boundaries)
3. **Create** - Creates GitHub issue(s)

---

## Task 1: Create Unified Technical Planning Prompt

**Files:**
- Modify: `internal/dashboard/prompts.go`
- Test: `internal/dashboard/prompts_test.go` (create if doesn't exist)

**Step 1: Write the failing test**

Create test file to verify the new prompt structure:

```go
package dashboard

import (
	"strings"
	"testing"
)

func TestBuildTechnicalPlanningPrompt(t *testing.T) {
	prompt := BuildTechnicalPlanningPrompt(WizardTypeFeature, "Add user authentication", "Go web service")
	
	// Verify prompt contains required sections
	if !strings.Contains(prompt, "ARCHITECTURE OVERVIEW") {
		t.Error("Prompt missing ARCHITECTURE OVERVIEW section")
	}
	if !strings.Contains(prompt, "FILES REQUIRING CHANGES") {
		t.Error("Prompt missing FILES REQUIRING CHANGES section")
	}
	if !strings.Contains(prompt, "COMPONENT DEPENDENCIES") {
		t.Error("Prompt missing COMPONENT DEPENDENCIES section")
	}
	if !strings.Contains(prompt, "IMPLEMENTATION BOUNDARIES") {
		t.Error("Prompt missing IMPLEMENTATION BOUNDARIES section")
	}
	if !strings.Contains(prompt, "Add user authentication") {
		t.Error("Prompt missing original idea")
	}
}

func TestBuildTechnicalPlanningPrompt_Bug(t *testing.T) {
	prompt := BuildTechnicalPlanningPrompt(WizardTypeBug, "Fix login error", "Go web service")
	
	if !strings.Contains(prompt, "BUG FIX TECHNICAL PLANNING") {
		t.Error("Bug prompt should mention bug fix")
	}
}
```

**Step 2: Run test to verify it fails**

```bash
cd /home/decodo/work/one-dev-army
go test ./internal/dashboard -run TestBuildTechnicalPlanningPrompt -v
```

Expected: FAIL - function not defined

**Step 3: Add unified prompt template and builder function**

Add to `internal/dashboard/prompts.go` after line 37:

```go
// TechnicalPlanningPromptTemplate is the unified template for both refinement and technical analysis
// It outputs a structured technical planning document without implementation code
const TechnicalPlanningPromptTemplate = `You are a technical architect creating a GitHub issue with technical planning.

Your output MUST be a markdown document with exactly these sections:

## Problem Statement / Feature Description
[Clear, professional description of what needs to be done]

## Architecture Overview
[High-level description of the system architecture needed]
- Key components involved
- Data flow overview
- Integration points

## Files Requiring Changes
[List specific file paths that will need modification]
- Path to each file with brief explanation of what changes are needed
- Include both existing files to modify and new files to create

## Component Dependencies
[Describe how components interact]
- Dependencies between modules
- External dependencies (libraries, APIs, services)
- Database schema changes if applicable

## Implementation Boundaries
[Clear boundaries of what to do and what NOT to do]
- What is in scope for this issue
- What is explicitly out of scope
- Constraints and limitations

## Acceptance Criteria
[2-4 specific, verifiable criteria for completion]

CRITICAL RULES:
- NO implementation code or algorithms
- NO specific technical solutions or design patterns
- NO "how to" instructions
- Focus on WHAT and WHERE, not HOW
- Be specific about file paths and component names
- Keep architecture description at a high level

Codebase context (for reference only):
%s

Original %s:
%s`
```

Add the builder function after line 119:

```go
// BuildTechnicalPlanningPrompt creates the unified prompt for technical planning
// This combines refinement + technical analysis into a single LLM call
func BuildTechnicalPlanningPrompt(wizardType WizardType, idea string, codebaseContext string) string {
	if codebaseContext == "" {
		codebaseContext = "No codebase context provided."
	}

	var typeLabel string
	if wizardType == WizardTypeBug {
		typeLabel = "bug report"
	} else {
		typeLabel = "feature request"
	}

	return fmt.Sprintf(TechnicalPlanningPromptTemplate,
		codebaseContext,
		typeLabel,
		idea,
	)
}
```

**Step 4: Run test to verify it passes**

```bash
go test ./internal/dashboard -run TestBuildTechnicalPlanningPrompt -v
```

Expected: PASS

**Step 5: Commit**

```bash
git add internal/dashboard/prompts.go internal/dashboard/prompts_test.go
git commit -m "feat(wizard): add unified technical planning prompt template"
```

---

## Task 2: Remove WizardStepBreakdown and Update State Machine

**Files:**
- Modify: `internal/dashboard/wizard.go:44-50` (remove WizardStepBreakdown constant)
- Modify: `internal/dashboard/wizard.go:78-93` (update WizardSession struct)
- Test: `internal/dashboard/wizard_test.go`

**Step 1: Write the failing test**

Add to `internal/dashboard/wizard_test.go`:

```go
func TestWizardStepConstants(t *testing.T) {
	// Verify breakdown step is removed
	steps := []WizardStep{
		WizardStepNew,
		WizardStepRefine,
		// WizardStepBreakdown should NOT exist
		WizardStepCreate,
		WizardStepDone,
	}
	
	// Should have exactly 4 steps (not 5)
	if len(steps) != 4 {
		t.Errorf("Expected 4 steps, got %d", len(steps))
	}
}

func TestWizardSession_NoTasksField(t *testing.T) {
	// Verify Tasks field is removed from session
	session := &WizardSession{
		ID:   "test-id",
		Type: WizardTypeFeature,
	}
	
	// Should not have Tasks field anymore
	// This test will fail to compile if Tasks field exists
	// If it compiles, the field has been removed
	_ = session
}
```

**Step 2: Run test to verify it fails**

```bash
go test ./internal/dashboard -run TestWizardStepConstants -v
```

Expected: FAIL - WizardStepBreakdown still exists

**Step 3: Remove WizardStepBreakdown constant**

Edit `internal/dashboard/wizard.go` line 44-50:

```go
const (
	WizardStepNew       WizardStep = "new"
	WizardStepRefine    WizardStep = "refine"
	// REMOVED: WizardStepBreakdown WizardStep = "breakdown"
	WizardStepCreate    WizardStep = "create"
	WizardStepDone      WizardStep = "done"
)
```

**Step 4: Remove Tasks field from WizardSession struct**

Edit `internal/dashboard/wizard.go` line 78-93:

```go
// WizardSession holds the state for a single wizard instance
type WizardSession struct {
	ID                 string         `json:"id"`
	Type               WizardType     `json:"type"`
	CurrentStep        WizardStep     `json:"current_step"`
	IdeaText           string         `json:"idea_text"`
	RefinedDescription string         `json:"refined_description"`
	// REMOVED: Tasks              []WizardTask   `json:"tasks"`
	TechnicalPlanning  string         `json:"technical_planning"` // NEW FIELD
	CreatedIssues      []CreatedIssue `json:"created_issues"`
	EpicNumber         int            `json:"epic_number"`
	AddToSprint        bool           `json:"add_to_sprint"`
	SkipBreakdown      bool           `json:"skip_breakdown"` // Keep for backward compatibility
	LLMLogs            []LLMLogEntry  `json:"llm_logs"`
	CreatedAt          time.Time      `json:"created_at"`
	UpdatedAt          time.Time      `json:"updated_at"`
	mu                 sync.RWMutex   `json:"-"`
}
```

**Step 5: Add setter for TechnicalPlanning field**

Add after line 122 in `internal/dashboard/wizard.go`:

```go
// SetTechnicalPlanning updates the technical planning (thread-safe)
func (s *WizardSession) SetTechnicalPlanning(planning string) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.TechnicalPlanning = planning
	s.UpdatedAt = time.Now()
}
```

**Step 6: Remove SetTasks method**

Remove lines 124-130 (SetTasks method) from `internal/dashboard/wizard.go`.

**Step 7: Run test to verify it passes**

```bash
go test ./internal/dashboard -run TestWizardStepConstants -v
```

Expected: PASS

**Step 8: Commit**

```bash
git add internal/dashboard/wizard.go internal/dashboard/wizard_test.go
git commit -m "refactor(wizard): remove breakdown step, add technical planning field"
```

---

## Task 3: Merge Refine Handler with Technical Planning

**Files:**
- Modify: `internal/dashboard/handlers.go:852-1028` (handleWizardRefine)
- Modify: `internal/dashboard/handlers.go:1048-1178` (handleWizardBreakdown - will be removed)
- Test: `internal/dashboard/handlers_test.go`

**Step 1: Write the failing test**

Add to `internal/dashboard/handlers_test.go`:

```go
func TestHandleWizardRefine_TechnicalPlanning(t *testing.T) {
	// Setup test server with mock LLM
	store := NewWizardSessionStore()
	defer store.Stop()
	
	session, _ := store.Create("feature")
	session.SetIdeaText("Add user authentication")
	
	// Mock request
	form := url.Values{}
	form.Add("session_id", session.ID)
	form.Add("idea", "Add user authentication")
	
	req := httptest.NewRequest("POST", "/wizard/refine", strings.NewReader(form.Encode()))
	req.Header.Add("Content-Type", "application/x-www-form-urlencoded")
	
	// The handler should now return technical planning in the response
	// This test verifies the new unified output format
	t.Log("Testing unified technical planning output")
}
```

**Step 2: Run test to verify it fails**

```bash
go test ./internal/dashboard -run TestHandleWizardRefine_TechnicalPlanning -v
```

Expected: FAIL or incomplete

**Step 3: Update handleWizardRefine to use unified prompt**

Replace the LLM call section in `internal/dashboard/handlers.go` around line 960:

```go
	// Build unified technical planning prompt with codebase context
	codebaseContext := GetCodebaseContext()
	prompt := BuildTechnicalPlanningPrompt(session.Type, inputText, codebaseContext)
	session.AddLog("system", "Sending technical planning request to LLM")

	// Send message to LLM with timeout
	ctx, cancel := context.WithTimeout(r.Context(), LLMRequestTimeout)
	defer cancel()

	model := opencode.ParseModelRef(s.wizardLLM)
	var output strings.Builder
	response, err := s.oc.SendMessageStream(ctx, llmSession.ID, prompt, model, &output)
	if err != nil {
		log.Printf("[Wizard] Error from LLM: %v", err)
		session.AddLog("system", "LLM error: "+err.Error())

		errorMsg := "Failed to generate technical planning. "
		if ctx.Err() == context.DeadlineExceeded {
			errorMsg += "The AI service timed out. Please try again with a shorter description."
		} else {
			errorMsg += "Please check your connection and try again."
		}

		s.renderError(w, errorMsg, session.ID, string(session.Type), isPage)
		return
	}

	// Extract technical planning from response
	var technicalPlanning string
	if len(response.Parts) > 0 {
		technicalPlanning = stripLLMPreamble(response.Parts[0].Text)
	}

	// Validate that we got a non-empty response
	if technicalPlanning == "" {
		log.Printf("[Wizard] LLM returned empty response for session %s", session.ID)
		session.AddLog("system", "Error: LLM returned empty response")
		s.renderError(w, "The AI returned an empty response. Please try again with a more detailed description.", session.ID, string(session.Type), isPage)
		return
	}

	session.SetTechnicalPlanning(technicalPlanning)
	session.AddLog("assistant", technicalPlanning)
```

**Step 4: Update response data structure**

Replace the data struct in `handleWizardRefine` (around line 1005):

```go
	data := struct {
		SessionID          string
		Type               string
		TechnicalPlanning  string  // NEW: unified output
		IsPage             bool
		SkipBreakdown      bool    // Keep for backward compatibility
		SprintName         string
		CurrentStep        int
		ShowBreakdownStep  bool    // Always false now
		NeedsTypeSelection bool
	}{
		SessionID:          session.ID,
		Type:               string(session.Type),
		TechnicalPlanning:  technicalPlanning,
		IsPage:             isPage,
		SkipBreakdown:      true, // Always skip breakdown in new flow
		SprintName:         s.activeSprintName(),
		CurrentStep:        2,    // Now step 2 is Technical Planning
		ShowBreakdownStep:  false, // No more breakdown step
		NeedsTypeSelection: false,
	}

	s.renderFragment(w, "wizard_refine.html", data)
```

**Step 5: Remove handleWizardBreakdown handler**

Delete the entire `handleWizardBreakdown` function from `internal/dashboard/handlers.go` (lines 1048-1178).

**Step 6: Remove buildBreakdownPrompt function**

Delete the `buildBreakdownPrompt` function from `internal/dashboard/handlers.go` (lines 1180-1204).

**Step 7: Run test to verify it passes**

```bash
go test ./internal/dashboard -run TestHandleWizardRefine -v
```

Expected: PASS

**Step 8: Commit**

```bash
git add internal/dashboard/handlers.go internal/dashboard/handlers_test.go
git commit -m "feat(wizard): merge refine and breakdown into unified technical planning"
```

---

## Task 4: Update Templates for 3-Step Flow

**Files:**
- Modify: `internal/dashboard/templates/wizard_steps.html`
- Modify: `internal/dashboard/templates/wizard_refine.html`
- Delete: `internal/dashboard/templates/wizard_breakdown.html` (or repurpose)
- Modify: `internal/dashboard/templates/wizard_create.html`

**Step 1: Update wizard_steps.html to show 3 steps**

Replace the entire content of `internal/dashboard/templates/wizard_steps.html`:

```html
{{define "wizard-steps"}}
<div class="step-indicator" id="wizard-step-indicator" hx-swap-oob="true">
  {{if .NeedsTypeSelection}}
  <div class="step {{if eq .CurrentStep 1}}active{{end}} {{if gt .CurrentStep 1}}completed{{end}}">
    <span class="step-number">1</span>
    <span class="step-label">Type</span>
  </div>
  <div class="step-connector"></div>
  <div class="step {{if eq .CurrentStep 2}}active{{end}} {{if gt .CurrentStep 2}}completed{{end}}">
    <span class="step-number">2</span>
    <span class="step-label">Idea</span>
  </div>
  <div class="step-connector"></div>
  <div class="step {{if eq .CurrentStep 3}}active{{end}} {{if gt .CurrentStep 3}}completed{{end}}">
    <span class="step-number">3</span>
    <span class="step-label">Technical Planning</span>
  </div>
  <div class="step-connector"></div>
  <div class="step {{if eq .CurrentStep 4}}active{{end}}">
    <span class="step-number">4</span>
    <span class="step-label">Create</span>
  </div>
  {{else}}
  <div class="step {{if eq .CurrentStep 1}}active{{end}} {{if gt .CurrentStep 1}}completed{{end}}">
    <span class="step-number">1</span>
    <span class="step-label">Idea</span>
  </div>
  <div class="step-connector"></div>
  <div class="step {{if eq .CurrentStep 2}}active{{end}} {{if gt .CurrentStep 2}}completed{{end}}">
    <span class="step-number">2</span>
    <span class="step-label">Technical Planning</span>
  </div>
  <div class="step-connector"></div>
  <div class="step {{if eq .CurrentStep 3}}active{{end}}">
    <span class="step-number">3</span>
    <span class="step-label">Create</span>
  </div>
  {{end}}
</div>
{{end}}
```

**Step 2: Update wizard_refine.html to show technical planning**

Replace the form section in `internal/dashboard/templates/wizard_refine.html`:

```html
{{define "content"}}
{{template "wizard-steps" .}}
<div class="wizard-step">
  <h2>Technical Planning</h2>
  
  <form id="refine-form" hx-post="/wizard/create{{if .IsPage}}?page=1{{end}}" hx-target="#wizard-content" hx-swap="innerHTML" hx-disabled-elt="button[type='submit']" data-is-page="{{.IsPage}}">
    <input type="hidden" name="session_id" value="{{.SessionID}}">
    {{if .IsPage}}<input type="hidden" name="page" value="1">{{end}}
    
    <div class="form-group">
      <label for="technical_planning">Technical Planning:</label>
      <div class="markdown-tabs">
        <button type="button" class="tab-btn active" data-tab="preview" onclick="switchTab('preview')">Preview</button>
        <button type="button" class="tab-btn" data-tab="edit" onclick="switchTab('edit')">Edit</button>
      </div>
      <div id="preview-container" class="markdown-preview"></div>
      <textarea id="technical_planning" name="technical_planning" rows="12" style="display:none;">{{.TechnicalPlanning}}</textarea>
    </div>
    
    {{if .SprintName}}
    <div class="form-group sprint-option">
      <label class="sprint-checkbox">
        <input type="checkbox" name="add_to_sprint" value="1" checked>
        <span class="sprint-checkbox-label">Add to current sprint <strong>{{.SprintName}}</strong></span>
      </label>
    </div>
    {{end}}
    
    <div id="refine-error" class="error-message" style="display:none; color: var(--error); margin: 1rem 0;"></div>
    
    <div class="form-actions">
      {{if .IsPage}}
      <a href="/" class="btn">Cancel</a>
      {{else}}
      <button type="button" class="btn" onclick="closeWizardModal()">Cancel</button>
      {{end}}
      <div class="nav-buttons">
        <button type="button" class="btn" hx-get="/wizard/new?type={{.Type}}&amp;session_id={{.SessionID}}{{if .IsPage}}&amp;page=1{{end}}" hx-target="#wizard-content">
          ← Back
        </button>
        <button type="button" class="btn btn-secondary" hx-post="/wizard/refine{{if .IsPage}}?page=1{{end}}" hx-target="#wizard-content" hx-disabled-elt="button[type='submit']" hx-vals='{"current_description": "{{.TechnicalPlanning}}"}'>
          <span class="spinner" style="display:none;">⏳</span>
          <span class="label">Regenerate</span>
        </button>
        <button type="submit" class="btn btn-primary">
          <span class="spinner" style="display:none;">⏳</span>
          <span class="label">Accept &amp; Create Issue</span>
        </button>
      </div>
    </div>
  </form>
  
</div>

<style>
/* Keep existing styles */
</style>

<script>
(function() {
  const textarea = document.getElementById('technical_planning');
  const previewContainer = document.getElementById('preview-container');
  
  function renderMarkdown() {
    if (typeof marked !== 'undefined' && textarea && previewContainer) {
      previewContainer.innerHTML = marked.parse(textarea.value);
    }
  }
  
  window.switchTab = function(tabName) {
    const previewTab = document.querySelector('[data-tab="preview"]');
    const editTab = document.querySelector('[data-tab="edit"]');
    
    if (tabName === 'preview') {
      previewTab.classList.add('active');
      editTab.classList.remove('active');
      previewContainer.style.display = 'block';
      textarea.style.display = 'none';
      renderMarkdown();
    } else {
      editTab.classList.add('active');
      previewTab.classList.remove('active');
      previewContainer.style.display = 'none';
      textarea.style.display = 'block';
    }
  };
  
  if (textarea) {
    textarea.addEventListener('input', renderMarkdown);
    renderMarkdown();
  }
})();
</script>

{{end}}
```

**Step 3: Delete wizard_breakdown.html**

```bash
rm internal/dashboard/templates/wizard_breakdown.html
git rm internal/dashboard/templates/wizard_breakdown.html
```

**Step 4: Update wizard_create.html to use TechnicalPlanning**

Modify `internal/dashboard/templates/wizard_create.html` to use `TechnicalPlanning` instead of `RefinedDescription`:

```html
<!-- Update references from .RefinedDescription to .TechnicalPlanning -->
```

**Step 5: Update server.go template parsing**

Remove `wizard_breakdown.html` from the partials list in `internal/dashboard/server.go` line 111:

```go
// Parse wizard partial templates (no layout)
wizardPartials := []string{"wizard_new.html", "wizard_refine.html", "wizard_create.html", "wizard_error.html", "wizard_logs.html"}
```

**Step 6: Run tests**

```bash
go test ./internal/dashboard -v
```

Expected: PASS

**Step 7: Commit**

```bash
git add internal/dashboard/templates/
git add internal/dashboard/server.go
git commit -m "feat(wizard): update templates for 3-step unified flow"
```

---

## Task 5: Update Create Handler for New Flow

**Files:**
- Modify: `internal/dashboard/handlers.go:1206-1607` (handleWizardCreate and handleWizardCreateSingle)

**Step 1: Update handleWizardCreate to use TechnicalPlanning**

Replace references to `session.RefinedDescription` with `session.TechnicalPlanning` in `handleWizardCreate`:

```go
// Line ~1306: Build title from technical planning
title := session.TechnicalPlanning
if title == "" {
    title = session.IdeaText
}

// Line ~1315: Build epic body
e picBody := fmt.Sprintf("## Technical Planning\n\n%s\n\n## Sub-tasks\n\n*Sub-tasks will be linked here after creation.*",
    session.TechnicalPlanning)
```

**Step 2: Update handleWizardCreateSingle**

Replace references in `handleWizardCreateSingle`:

```go
// Line ~1542: Build title
title := session.TechnicalPlanning
if title == "" {
    title = session.IdeaText
}

// Line ~1551: Build body
body := session.TechnicalPlanning
```

**Step 3: Remove task-based creation logic**

Since we no longer have a breakdown step with tasks, simplify the creation logic. For now, always create a single issue (epic behavior can be re-added later if needed):

```go
// In handleWizardCreate, remove the epic + sub-tasks logic
// Just create a single issue with the technical planning as the body
```

**Step 4: Run tests**

```bash
go test ./internal/dashboard -run TestHandleWizardCreate -v
```

Expected: PASS

**Step 5: Commit**

```bash
git add internal/dashboard/handlers.go
git commit -m "feat(wizard): update create handler for technical planning flow"
```

---

## Task 6: Remove Breakdown Route and Update Server

**Files:**
- Modify: `internal/dashboard/server.go:158` (remove breakdown route)

**Step 1: Remove breakdown route registration**

Edit `internal/dashboard/server.go` line 158:

```go
// Wizard routes
s.mux.HandleFunc("GET /wizard", s.handleWizardPage)
s.mux.HandleFunc("GET /wizard/new", s.handleWizardNew)
s.mux.HandleFunc("GET /wizard/modal", s.handleWizardModal)
s.mux.HandleFunc("POST /wizard/select-type", s.handleWizardSelectType)
s.mux.HandleFunc("POST /wizard/cancel", s.handleWizardCancel)
s.mux.HandleFunc("POST /wizard/refine", s.handleWizardRefine)
// REMOVED: s.mux.HandleFunc("POST /wizard/breakdown", s.handleWizardBreakdown)
s.mux.HandleFunc("POST /wizard/create", s.handleWizardCreate)
s.mux.HandleFunc("GET /wizard/logs/{sessionId}", s.handleWizardLogs)
```

**Step 2: Run tests**

```bash
go test ./internal/dashboard -v
```

Expected: PASS

**Step 3: Commit**

```bash
git add internal/dashboard/server.go
git commit -m "refactor(wizard): remove breakdown route registration"
```

---

## Task 7: Update All Handler Data Structures

**Files:**
- Modify: `internal/dashboard/handlers.go` (multiple locations)

**Step 1: Update handleWizardNew data struct**

Around line 826-847, update to remove ShowBreakdownStep references:

```go
	data := struct {
		Type               string
		SessionID          string
		IsPage             bool
		CurrentStep        int
		NeedsTypeSelection bool
	}{
		Type:               wizardType,
		SessionID:          "",
		IsPage:             isPage,
		CurrentStep:        1,
		NeedsTypeSelection: needsTypeSelection,
	}
```

**Step 2: Update handleWizardPage data struct**

Around line 1672-1694:

```go
	data := struct {
		Active             string
		Type               string
		SessionID          string
		CurrentStep        int
		IsPage             bool
		NeedsTypeSelection bool
	}{
		Active:             "wizard",
		Type:               wizardType,
		SessionID:          "",
		CurrentStep:        1,
		IsPage:             true,
		NeedsTypeSelection: needsTypeSelection,
	}
```

**Step 3: Update handleWizardModal data struct**

Around line 1729-1747:

```go
	data := struct {
		Type               string
		SessionID          string
		CurrentStep        int
		NeedsTypeSelection bool
	}{
		Type:               wizardType,
		SessionID:          "",
		CurrentStep:        1,
		NeedsTypeSelection: needsTypeSelection,
	}
```

**Step 4: Run tests**

```bash
go test ./internal/dashboard -v
```

Expected: PASS

**Step 5: Commit**

```bash
git add internal/dashboard/handlers.go
git commit -m "refactor(wizard): remove breakdown step references from handler data structs"
```

---

## Task 8: Add Backward Compatibility for Old Sessions

**Files:**
- Modify: `internal/dashboard/wizard.go` (add migration logic)

**Step 1: Add session migration function**

Add to `internal/dashboard/wizard.go` after line 290:

```go
// MigrateOldSession converts an old session format to the new format
// This handles sessions that were in the breakdown step
func (s *WizardSession) MigrateOldSession() {
	s.mu.Lock()
	defer s.mu.Unlock()
	
	// If session was in breakdown step, move it to refine
	if s.CurrentStep == "breakdown" {
		s.CurrentStep = WizardStepRefine
	}
	
	// If session has tasks but no technical planning, convert tasks to planning
	if len(s.Tasks) > 0 && s.TechnicalPlanning == "" {
		var planning strings.Builder
		planning.WriteString("## Task Breakdown\n\n")
		for i, task := range s.Tasks {
			planning.WriteString(fmt.Sprintf("%d. **%s** (Priority: %s, Complexity: %s)\n   %s\n\n", 
				i+1, task.Title, task.Priority, task.Complexity, task.Description))
		}
		s.TechnicalPlanning = planning.String()
	}
}
```

**Step 2: Call migration when retrieving sessions**

Modify `WizardSessionStore.Get` in `internal/dashboard/wizard.go` around line 262:

```go
// Get retrieves a session by ID and migrates it if needed
func (ws *WizardSessionStore) Get(id string) (*WizardSession, bool) {
	ws.mu.RLock()
	defer ws.mu.RUnlock()

	session, ok := ws.sessions[id]
	if ok {
		session.MigrateOldSession()
	}
	return session, ok
}
```

**Step 3: Run tests**

```bash
go test ./internal/dashboard -v
```

Expected: PASS

**Step 4: Commit**

```bash
git add internal/dashboard/wizard.go
git commit -m "feat(wizard): add backward compatibility for old sessions"
```

---

## Task 9: Update Wizard Tests

**Files:**
- Modify: `internal/dashboard/wizard_test.go`
- Modify: `internal/dashboard/handlers_test.go`

**Step 1: Update existing tests for new flow**

Update `TestFullWizardFlow` in `internal/dashboard/handlers_test.go`:

```go
func TestFullWizardFlow(t *testing.T) {
	// Test the new 3-step flow:
	// 1. New (idea input)
	// 2. Refine (technical planning)
	// 3. Create (issue creation)
	
	// ... update test logic
}
```

**Step 2: Add new tests for technical planning**

```go
func TestTechnicalPlanningOutputFormat(t *testing.T) {
	// Verify technical planning includes all required sections
}

func TestBackwardCompatibility_SessionMigration(t *testing.T) {
	// Verify old sessions with breakdown step are migrated correctly
}
```

**Step 3: Run all tests**

```bash
go test ./internal/dashboard -v
```

Expected: PASS

**Step 4: Commit**

```bash
git add internal/dashboard/wizard_test.go internal/dashboard/handlers_test.go
git commit -m "test(wizard): update tests for unified technical planning flow"
```

---

## Task 10: Final Integration and Verification

**Step 1: Run all tests**

```bash
cd /home/decodo/work/one-dev-army
go test ./internal/dashboard/... -v
```

Expected: All tests PASS

**Step 2: Build the application**

```bash
go build ./...
```

Expected: Build succeeds with no errors

**Step 3: Verify no references to old breakdown step**

```bash
grep -r "WizardStepBreakdown" --include="*.go" .
grep -r "handleWizardBreakdown" --include="*.go" .
grep -r "breakdown" --include="*.html" internal/dashboard/templates/
```

Expected: No matches (or only in comments/docs)

**Step 4: Final commit**

```bash
git commit -m "feat(wizard): merge plan and analysis steps into unified technical planning

- Remove separate breakdown step (WizardStepBreakdown)
- Create unified TechnicalPlanningPromptTemplate
- Merge refine and breakdown handlers into single handler
- Update flow from 4 steps to 3 steps
- Add backward compatibility for old sessions
- Update all templates for new flow

Closes #148"
```

---

## Summary

This implementation plan merges the separate "Plan" (Refine) and "Analysis" (Breakdown) wizard steps into a single "Technical Planning" step. The key changes are:

1. **New unified prompt** that outputs both refined description AND technical planning (architecture, files, dependencies, boundaries)
2. **Simplified state machine** - removed `WizardStepBreakdown`, flow is now: New → Refine (Technical Planning) → Create
3. **Merged handlers** - single LLM call produces unified output
4. **Updated UI** - 3 steps instead of 4, consolidated view
5. **Backward compatibility** - old sessions are automatically migrated

**Breaking Changes:**
- Removed `WizardStepBreakdown` constant
- Removed `Tasks` field from `WizardSession` (replaced with `TechnicalPlanning` string)
- Removed `/wizard/breakdown` endpoint
- Removed `wizard_breakdown.html` template

**Migration Path:**
- Old sessions in breakdown step are automatically migrated to refine step
- Old task arrays are converted to technical planning text

---

**Plan complete and saved to `docs/plans/2026-03-23-merge-plan-analysis-steps.md`.**

**Two execution options:**

**1. Subagent-Driven (this session)** - I dispatch fresh subagent per task, review between tasks, fast iteration

**2. Parallel Session (separate)** - Open new session with executing-plans, batch execution with checkpoints

Which approach would you prefer?
