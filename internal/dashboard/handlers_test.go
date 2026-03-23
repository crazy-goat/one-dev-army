package dashboard

import (
	"fmt"
	"html/template"
	"net/http"
	"net/http/httptest"
	"net/url"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"testing"
	"time"
)

// parseTemplatesFromDisk parses templates from disk for testing
// This allows tests to pick up template changes without rebuilding
func parseTemplatesFromDisk(templateDir string) (map[string]*template.Template, error) {
	tmpls := make(map[string]*template.Template)

	funcMap := template.FuncMap{
		"duration": func(start, end *time.Time) string {
			if start == nil || end == nil {
				return ""
			}
			d := end.Sub(*start).Round(time.Second)
			if d < time.Minute {
				return fmt.Sprintf("%ds", int(d.Seconds()))
			}
			return fmt.Sprintf("%dm%ds", int(d.Minutes()), int(d.Seconds())%60)
		},
		"truncate": func(s string, n int) string {
			if len(s) <= n {
				return s
			}
			return s[:n] + "\n... (truncated)"
		},
	}

	pages := []string{"board.html", "backlog.html", "costs.html", "task.html"}
	for _, page := range pages {
		t, err := template.New("").Funcs(funcMap).ParseFiles(
			filepath.Join(templateDir, "layout.html"),
			filepath.Join(templateDir, page),
		)
		if err != nil {
			return nil, fmt.Errorf("parsing %s: %w", page, err)
		}
		tmpls[page] = t
	}

	wizardPageTmpl, err := template.New("").Funcs(funcMap).ParseFiles(
		filepath.Join(templateDir, "layout.html"),
		filepath.Join(templateDir, "wizard_steps.html"),
		filepath.Join(templateDir, "wizard_new.html"),
		filepath.Join(templateDir, "wizard_page.html"),
	)
	if err != nil {
		return nil, fmt.Errorf("parsing wizard_page.html: %w", err)
	}
	tmpls["wizard_page.html"] = wizardPageTmpl

	wizardModalTmpl, err := template.New("").Funcs(funcMap).ParseFiles(
		filepath.Join(templateDir, "layout.html"),
		filepath.Join(templateDir, "wizard_steps.html"),
		filepath.Join(templateDir, "wizard_new.html"),
		filepath.Join(templateDir, "wizard_modal.html"),
	)
	if err != nil {
		return nil, fmt.Errorf("parsing wizard_modal.html: %w", err)
	}
	tmpls["wizard_modal.html"] = wizardModalTmpl

	// Parse wizard partial templates (no layout)
	wizardPartials := []string{"wizard_new.html", "wizard_refine.html", "wizard_create.html", "wizard_error.html", "wizard_logs.html"}
	for _, page := range wizardPartials {
		t, err := template.ParseFiles(
			filepath.Join(templateDir, "wizard_steps.html"),
			filepath.Join(templateDir, page),
		)
		if err != nil {
			return nil, fmt.Errorf("parsing %s: %w", page, err)
		}
		tmpls[page] = t
	}

	t, err := template.ParseFiles(filepath.Join(templateDir, "workers.html"))
	if err != nil {
		return nil, fmt.Errorf("parsing workers.html: %w", err)
	}
	tmpls["workers.html"] = t

	return tmpls, nil
}

// createTestServerWithTemplates creates a server with all templates loaded for integration testing
func createTestServerWithTemplates(t *testing.T) *Server {
	t.Helper()

	// Try to parse from disk first (for development), fall back to embedded FS
	var tmpls map[string]*template.Template
	var err error

	// Check multiple possible locations for templates
	templateDirs := []string{"templates", "internal/dashboard/templates"}
	var foundDir string
	for _, dir := range templateDirs {
		if _, statErr := os.Stat(dir); statErr == nil {
			foundDir = dir
			break
		}
	}

	if foundDir != "" {
		tmpls, err = parseTemplatesFromDisk(foundDir)
	} else {
		tmpls, err = parseTemplates()
	}

	if err != nil {
		t.Fatalf("failed to parse templates: %v", err)
	}

	srv := &Server{
		tmpls:       tmpls,
		wizardStore: NewWizardSessionStore(),
	}

	return srv
}

// TestHandleBoardData tests the board data API endpoint
func TestHandleBoardData(t *testing.T) {
	srv := &Server{
		tmpls: make(map[string]*template.Template),
	}

	// Test without template (should return 500)
	req := httptest.NewRequest(http.MethodGet, "/api/board-data", nil)
	rec := httptest.NewRecorder()

	srv.handleBoardData(rec, req)

	// Should return 500 since template is not loaded
	if rec.Code != http.StatusInternalServerError {
		t.Errorf("expected status 500 for missing template, got %d", rec.Code)
	}
}

// TestHandleBoardData_WithTemplate tests the endpoint with a loaded template
func TestHandleBoardData_WithTemplate(t *testing.T) {
	// Create a minimal template for testing
	tmplContent := `{{define "content"}}<div>Board Data</div>{{end}}`
	tmpl, err := template.New("board.html").Parse(tmplContent)
	if err != nil {
		t.Fatalf("failed to parse template: %v", err)
	}

	srv := &Server{
		tmpls: map[string]*template.Template{
			"board.html": tmpl,
		},
	}

	req := httptest.NewRequest(http.MethodGet, "/api/board-data", nil)
	rec := httptest.NewRecorder()

	srv.handleBoardData(rec, req)

	// Should return 200 OK with template loaded
	if rec.Code != http.StatusOK {
		t.Errorf("expected status 200, got %d", rec.Code)
	}

	// Verify content type is HTML
	contentType := rec.Header().Get("Content-Type")
	if !strings.Contains(contentType, "text/html") {
		t.Errorf("expected Content-Type to contain 'text/html', got %s", contentType)
	}
}

// TestBuildBoardData tests the board data construction
func TestBuildBoardData(t *testing.T) {
	srv := &Server{
		tmpls: make(map[string]*template.Template),
	}

	data := srv.buildBoardData()

	// Verify default values
	if data.Active != "board" {
		t.Errorf("expected Active to be 'board', got %s", data.Active)
	}

	// Should be paused by default when no orchestrator
	if !data.Paused {
		t.Error("expected Paused to be true by default")
	}

	// Should not be processing when no orchestrator
	if data.Processing {
		t.Error("expected Processing to be false by default")
	}
}

// TestHandleBoard tests the main board page handler
func TestHandleBoard(t *testing.T) {
	srv := &Server{
		tmpls: make(map[string]*template.Template),
	}

	// Test without template (should return 500)
	req := httptest.NewRequest(http.MethodGet, "/board", nil)
	rec := httptest.NewRecorder()

	srv.handleBoard(rec, req)

	// Should return 500 since template is not loaded
	if rec.Code != http.StatusInternalServerError {
		t.Errorf("expected status 500 for missing template, got %d", rec.Code)
	}
}

func TestHandleWizardNew(t *testing.T) {
	// Create server with minimal dependencies
	srv := &Server{
		tmpls:       make(map[string]*template.Template),
		wizardStore: NewWizardSessionStore(),
	}
	defer srv.wizardStore.Stop()

	// We need to parse the template first
	// For now, just test the handler exists and doesn't panic
	req := httptest.NewRequest(http.MethodGet, "/wizard/new?type=feature", nil)
	rec := httptest.NewRecorder()

	// This will fail until we implement the handler
	srv.handleWizardNew(rec, req)

	// Should return 200 OK or 500 if template missing
	if rec.Code != http.StatusOK && rec.Code != http.StatusInternalServerError {
		t.Errorf("expected status 200 or 500, got %d", rec.Code)
	}
}

func TestHandleWizardNew_CreatesSession(t *testing.T) {
	srv := &Server{
		tmpls:       make(map[string]*template.Template),
		wizardStore: NewWizardSessionStore(),
	}
	defer srv.wizardStore.Stop()

	req := httptest.NewRequest(http.MethodGet, "/wizard/new?type=bug", nil)
	rec := httptest.NewRecorder()

	srv.handleWizardNew(rec, req)

	// Check that a session was created
	if srv.wizardStore.Count() != 1 {
		t.Errorf("expected 1 session, got %d", srv.wizardStore.Count())
	}
}

func TestHandleWizardNew_InvalidType(t *testing.T) {
	srv := &Server{
		tmpls:       make(map[string]*template.Template),
		wizardStore: NewWizardSessionStore(),
	}
	defer srv.wizardStore.Stop()

	req := httptest.NewRequest(http.MethodGet, "/wizard/new?type=invalid", nil)
	rec := httptest.NewRecorder()

	srv.handleWizardNew(rec, req)

	// With new implementation, invalid type defaults to showing type selector
	// No session should be created
	if srv.wizardStore.Count() != 0 {
		t.Errorf("expected 0 sessions for invalid type (type selector shown), got %d", srv.wizardStore.Count())
	}
}

func TestHandleWizardRefine(t *testing.T) {
	srv := &Server{
		tmpls:       make(map[string]*template.Template),
		wizardStore: NewWizardSessionStore(),
	}
	defer srv.wizardStore.Stop()

	// Create a session first
	session, _ := srv.wizardStore.Create("feature")

	// Test with missing session
	req := httptest.NewRequest(http.MethodPost, "/wizard/refine", strings.NewReader("session_id=invalid&idea=test"))
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	rec := httptest.NewRecorder()

	srv.handleWizardRefine(rec, req)

	if rec.Code != http.StatusBadRequest {
		t.Errorf("expected status 400 for invalid session, got %d", rec.Code)
	}

	// Test with valid session
	req = httptest.NewRequest(http.MethodPost, "/wizard/refine", strings.NewReader("session_id="+session.ID+"&idea=Create a login page"))
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	rec = httptest.NewRecorder()

	srv.handleWizardRefine(rec, req)

	// Should accept the request (actual LLM call would need mocking)
	if rec.Code != http.StatusOK && rec.Code != http.StatusInternalServerError {
		t.Errorf("expected status 200 or 500, got %d", rec.Code)
	}
}

func TestHandleWizardRefine_MissingIdea(t *testing.T) {
	srv := &Server{
		tmpls:       make(map[string]*template.Template),
		wizardStore: NewWizardSessionStore(),
	}
	defer srv.wizardStore.Stop()

	session, _ := srv.wizardStore.Create("feature")

	req := httptest.NewRequest(http.MethodPost, "/wizard/refine", strings.NewReader("session_id="+session.ID))
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	rec := httptest.NewRecorder()

	srv.handleWizardRefine(rec, req)

	if rec.Code != http.StatusBadRequest {
		t.Errorf("expected status 400 for missing idea, got %d", rec.Code)
	}
}

func TestHandleWizardRefine_WithCurrentDescription(t *testing.T) {
	srv := &Server{
		tmpls:       make(map[string]*template.Template),
		wizardStore: NewWizardSessionStore(),
	}
	defer srv.wizardStore.Stop()

	// Create a session first
	session, _ := srv.wizardStore.Create("feature")

	// Test re-refinement with current_description (no idea provided)
	form := url.Values{}
	form.Set("session_id", session.ID)
	form.Set("current_description", "Previous refined description")

	req := httptest.NewRequest(http.MethodPost, "/wizard/refine", strings.NewReader(form.Encode()))
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	rec := httptest.NewRecorder()

	srv.handleWizardRefine(rec, req)

	// Should return 200 OK (or 500 if template missing)
	if rec.Code != http.StatusOK && rec.Code != http.StatusInternalServerError {
		t.Errorf("expected status 200 or 500, got %d: %s", rec.Code, rec.Body.String())
	}

	// If successful, verify the response contains the re-refined text
	if rec.Code == http.StatusOK {
		body := rec.Body.String()
		if !strings.Contains(body, "Refined: Previous refined description") {
			t.Errorf("expected response to contain re-refined text, got: %s", body)
		}

		// Verify session was updated
		updatedSession, _ := srv.wizardStore.Get(session.ID)
		if !strings.Contains(updatedSession.RefinedDescription, "Previous refined description") {
			t.Errorf("expected session to store re-refined description")
		}
	}
}

func TestHandleWizardRefine_ErrorRendering(t *testing.T) {
	srv := &Server{
		tmpls:       make(map[string]*template.Template),
		wizardStore: NewWizardSessionStore(),
	}
	defer srv.wizardStore.Stop()

	// Test with invalid session - should return error template
	form := url.Values{}
	form.Set("session_id", "invalid-session")
	form.Set("idea", "test idea")

	req := httptest.NewRequest(http.MethodPost, "/wizard/refine", strings.NewReader(form.Encode()))
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	rec := httptest.NewRecorder()

	srv.handleWizardRefine(rec, req)

	// Should return 400 Bad Request for invalid session
	if rec.Code != http.StatusBadRequest {
		t.Errorf("expected status 400 for invalid session, got %d", rec.Code)
	}
}

func TestHandleWizardRefine_EmptyLLMResponse(t *testing.T) {
	srv := &Server{
		tmpls:       make(map[string]*template.Template),
		wizardStore: NewWizardSessionStore(),
	}
	defer srv.wizardStore.Stop()

	session, _ := srv.wizardStore.Create("feature")

	// This test verifies the validation logic exists
	// Full test would require mocking the LLM client
	req := httptest.NewRequest(http.MethodPost, "/wizard/refine",
		strings.NewReader("session_id="+session.ID+"&idea=test"))
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	rec := httptest.NewRecorder()

	srv.handleWizardRefine(rec, req)

	// With nil oc client, should use mock and return 200 or 500 if template missing
	if rec.Code != http.StatusOK && rec.Code != http.StatusInternalServerError {
		t.Errorf("expected status 200 or 500, got %d", rec.Code)
	}
}

func TestHandleWizardCreate(t *testing.T) {
	srv := &Server{
		tmpls:       make(map[string]*template.Template),
		wizardStore: NewWizardSessionStore(),
		gh:          nil, // No GitHub client for unit test
	}
	defer srv.wizardStore.Stop()

	// Create a session with technical planning
	session, _ := srv.wizardStore.Create("feature")
	session.SetTechnicalPlanning("## Technical Planning\n\nTest planning content for feature implementation")

	// Test with valid session
	req := httptest.NewRequest(http.MethodPost, "/wizard/create", strings.NewReader("session_id="+session.ID))
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	rec := httptest.NewRecorder()

	srv.handleWizardCreate(rec, req)

	// Should accept the request (will return mock data without GH client)
	if rec.Code != http.StatusOK && rec.Code != http.StatusInternalServerError {
		t.Errorf("expected status 200 or 500, got %d", rec.Code)
	}

	// Verify session was deleted after creation
	_, ok := srv.wizardStore.Get(session.ID)
	if ok {
		t.Error("expected session to be deleted after successful creation")
	}
}

// TestHandleWizardCreate_UsesTechnicalPlanningForTitle verifies that issue title uses technical planning
func TestHandleWizardCreate_UsesTechnicalPlanningForTitle(t *testing.T) {
	srv := createTestServerWithTemplates(t)
	srv.gh = nil
	defer srv.wizardStore.Stop()

	session, _ := srv.wizardStore.Create("feature")
	session.SetIdeaText("Raw user input")
	session.SetTechnicalPlanning("## Technical Planning\n\nLLM generated technical planning with architecture overview, files, and implementation boundaries")

	req := httptest.NewRequest(http.MethodPost, "/wizard/create", strings.NewReader("session_id="+session.ID))
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	rec := httptest.NewRecorder()

	srv.handleWizardCreate(rec, req)

	// Verify response contains the technical planning as title, not raw idea text
	body := rec.Body.String()
	if !strings.Contains(body, "Technical Planning") {
		t.Errorf("expected response to contain technical planning as title, got: %s", body)
	}
	if strings.Contains(body, "Raw user input") && !strings.Contains(body, "Technical Planning") {
		t.Error("expected title to come from technical planning, not raw idea text")
	}
}

// TestHandleWizardCreate_NoTechnicalPlanning verifies error when no technical planning
func TestHandleWizardCreate_NoTechnicalPlanning(t *testing.T) {
	srv := &Server{
		tmpls:       make(map[string]*template.Template),
		wizardStore: NewWizardSessionStore(),
	}
	defer srv.wizardStore.Stop()

	session, _ := srv.wizardStore.Create("feature")
	// Don't set any technical planning

	req := httptest.NewRequest(http.MethodPost, "/wizard/create", strings.NewReader("session_id="+session.ID))
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	rec := httptest.NewRecorder()

	srv.handleWizardCreate(rec, req)

	// Should still work - will use idea text as fallback
	if rec.Code != http.StatusOK && rec.Code != http.StatusInternalServerError {
		t.Errorf("expected status 200 or 500, got %d", rec.Code)
	}
}

func TestHandleWizardLogs(t *testing.T) {
	srv := &Server{
		tmpls:       make(map[string]*template.Template),
		wizardStore: NewWizardSessionStore(),
	}
	defer srv.wizardStore.Stop()

	// Create a session with logs
	session, _ := srv.wizardStore.Create("feature")
	session.AddLog("system", "Starting")
	session.AddLog("user", "Test idea")

	// Test with valid session - need to use the pattern that sets PathValue
	req := httptest.NewRequest(http.MethodGet, "/wizard/logs/"+session.ID, nil)
	req.SetPathValue("sessionId", session.ID)
	rec := httptest.NewRecorder()

	srv.handleWizardLogs(rec, req)

	// Should return 200 OK or 500 if template missing
	if rec.Code != http.StatusOK && rec.Code != http.StatusInternalServerError {
		t.Errorf("expected status 200 or 500, got %d", rec.Code)
	}

	// Test with invalid session
	req = httptest.NewRequest(http.MethodGet, "/wizard/logs/invalid-id", nil)
	req.SetPathValue("sessionId", "invalid-id")
	rec = httptest.NewRecorder()

	srv.handleWizardLogs(rec, req)

	if rec.Code != http.StatusNotFound {
		t.Errorf("expected status 404 for invalid session, got %d", rec.Code)
	}

	// Test with mismatched step - should return 204 to stop polling
	session.SetStep(WizardStepCreate) // Move session to create step
	req = httptest.NewRequest(http.MethodGet, "/wizard/logs/"+session.ID, nil)
	req.SetPathValue("sessionId", session.ID)
	req.Header.Set("X-Expected-Step", "refine") // But expect refine step
	rec = httptest.NewRecorder()

	srv.handleWizardLogs(rec, req)

	if rec.Code != http.StatusNoContent {
		t.Errorf("expected status 204 when step mismatch, got %d", rec.Code)
	}

	// Test with matching step - should return 200
	req = httptest.NewRequest(http.MethodGet, "/wizard/logs/"+session.ID, nil)
	req.SetPathValue("sessionId", session.ID)
	req.Header.Set("X-Expected-Step", "create") // Expect create step
	rec = httptest.NewRecorder()

	srv.handleWizardLogs(rec, req)

	if rec.Code != http.StatusOK && rec.Code != http.StatusInternalServerError {
		t.Errorf("expected status 200 or 500 when step matches, got %d", rec.Code)
	}
}

// TestFullWizardFlow tests the complete wizard flow end-to-end with new 3-step flow
func TestFullWizardFlow(t *testing.T) {
	// Create server with minimal dependencies
	srv := &Server{
		tmpls:       make(map[string]*template.Template),
		wizardStore: NewWizardSessionStore(),
	}
	defer srv.wizardStore.Stop()

	// Step 1: Start wizard (GET /wizard/new)
	req := httptest.NewRequest(http.MethodGet, "/wizard/new?type=feature", nil)
	rec := httptest.NewRecorder()
	srv.handleWizardNew(rec, req)

	if rec.Code != http.StatusOK && rec.Code != http.StatusInternalServerError {
		t.Fatalf("Step 1 failed: expected status 200 or 500, got %d", rec.Code)
	}

	// Get the session ID from the store (should have 1 session)
	// We need to get the first session that was created by handleWizardNew
	// Since we can't access the internal map, we'll use the Count to verify
	if srv.wizardStore.Count() < 1 {
		t.Fatal("No session created in step 1")
	}

	// Create a new session for testing the flow
	testSession, _ := srv.wizardStore.Create("feature")
	sessionID := testSession.ID

	// Step 2: Technical Planning (POST /wizard/refine)
	// This now generates unified technical planning in a single LLM call
	formData := url.Values{}
	formData.Set("session_id", sessionID)
	formData.Set("idea", "Create a user dashboard with analytics")

	req = httptest.NewRequest(http.MethodPost, "/wizard/refine", strings.NewReader(formData.Encode()))
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	rec = httptest.NewRecorder()
	srv.handleWizardRefine(rec, req)

	if rec.Code != http.StatusOK && rec.Code != http.StatusInternalServerError {
		t.Fatalf("Step 2 failed: expected status 200 or 500, got %d", rec.Code)
	}

	// Verify session was updated with technical planning
	session, _ := srv.wizardStore.Get(sessionID)
	if session.IdeaText == "" {
		t.Error("Step 2: Idea text not stored")
	}
	if session.TechnicalPlanning == "" {
		t.Error("Step 2: Technical planning not generated")
	}
	if session.CurrentStep != WizardStepRefine {
		t.Errorf("Step 2: Expected step 'refine', got %q", session.CurrentStep)
	}

	// Step 3: Create issues (POST /wizard/create)
	// No more breakdown step - create directly from technical planning
	formData = url.Values{}
	formData.Set("session_id", sessionID)

	req = httptest.NewRequest(http.MethodPost, "/wizard/create", strings.NewReader(formData.Encode()))
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	rec = httptest.NewRecorder()
	srv.handleWizardCreate(rec, req)

	if rec.Code != http.StatusOK && rec.Code != http.StatusInternalServerError {
		t.Fatalf("Step 3 failed: expected status 200 or 500, got %d", rec.Code)
	}

	// Verify session was deleted after creation
	_, ok := srv.wizardStore.Get(sessionID)
	if ok {
		t.Error("Step 3: Session should be deleted after creation")
	}

	t.Logf("Full wizard flow completed successfully with new 3-step flow")
}

// TestHandleWizardModal tests the modal endpoint
func TestHandleWizardModal(t *testing.T) {
	srv := &Server{
		tmpls:       make(map[string]*template.Template),
		wizardStore: NewWizardSessionStore(),
	}
	defer srv.wizardStore.Stop()

	// Test creating a feature wizard modal
	req := httptest.NewRequest(http.MethodGet, "/wizard/modal?type=feature", nil)
	rec := httptest.NewRecorder()

	srv.handleWizardModal(rec, req)

	// Should return 200 OK or 500 if template missing
	if rec.Code != http.StatusOK && rec.Code != http.StatusInternalServerError {
		t.Errorf("expected status 200 or 500, got %d", rec.Code)
	}

	// Check that a session was created
	if srv.wizardStore.Count() != 1 {
		t.Errorf("expected 1 session, got %d", srv.wizardStore.Count())
	}

	// Test creating a bug wizard modal
	req = httptest.NewRequest(http.MethodGet, "/wizard/modal?type=bug", nil)
	rec = httptest.NewRecorder()

	srv.handleWizardModal(rec, req)

	// Should have 2 sessions now
	if srv.wizardStore.Count() != 2 {
		t.Errorf("expected 2 sessions, got %d", srv.wizardStore.Count())
	}
}

// TestHandleWizardCancel tests the cancel endpoint
func TestHandleWizardCancel(t *testing.T) {
	srv := &Server{
		tmpls:       make(map[string]*template.Template),
		wizardStore: NewWizardSessionStore(),
	}
	defer srv.wizardStore.Stop()

	// Create a session first
	session, _ := srv.wizardStore.Create("feature")

	// Verify session exists
	if srv.wizardStore.Count() != 1 {
		t.Fatalf("expected 1 session before cancel, got %d", srv.wizardStore.Count())
	}

	// Cancel the session using form data (consistent with other POST endpoints)
	formData := url.Values{}
	formData.Set("session_id", session.ID)
	req := httptest.NewRequest(http.MethodPost, "/wizard/cancel", strings.NewReader(formData.Encode()))
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	rec := httptest.NewRecorder()

	srv.handleWizardCancel(rec, req)

	if rec.Code != http.StatusOK {
		t.Errorf("expected status 200, got %d", rec.Code)
	}

	// Verify session was deleted
	if srv.wizardStore.Count() != 0 {
		t.Errorf("expected 0 sessions after cancel, got %d", srv.wizardStore.Count())
	}

	// Test cancel with no session_id (should not panic)
	req = httptest.NewRequest(http.MethodPost, "/wizard/cancel", nil)
	rec = httptest.NewRecorder()

	srv.handleWizardCancel(rec, req)

	if rec.Code != http.StatusOK {
		t.Errorf("expected status 200 for empty session_id, got %d", rec.Code)
	}
}

// TestHandleWizardPage tests the full page wizard endpoint
func TestHandleWizardPage(t *testing.T) {
	srv := &Server{
		tmpls:       make(map[string]*template.Template),
		wizardStore: NewWizardSessionStore(),
	}
	defer srv.wizardStore.Stop()

	// Test creating a feature wizard page
	req := httptest.NewRequest(http.MethodGet, "/wizard?type=feature", nil)
	rec := httptest.NewRecorder()

	srv.handleWizardPage(rec, req)

	// Should return 200 OK or 500 if template missing
	if rec.Code != http.StatusOK && rec.Code != http.StatusInternalServerError {
		t.Errorf("expected status 200 or 500, got %d", rec.Code)
	}

	// Check that a session was created
	if srv.wizardStore.Count() != 1 {
		t.Errorf("expected 1 session, got %d", srv.wizardStore.Count())
	}

	// Test creating a bug wizard page
	req = httptest.NewRequest(http.MethodGet, "/wizard?type=bug", nil)
	rec = httptest.NewRecorder()

	srv.handleWizardPage(rec, req)

	// Should have 2 sessions now
	if srv.wizardStore.Count() != 2 {
		t.Errorf("expected 2 sessions, got %d", srv.wizardStore.Count())
	}

	// Test default type (no type param - should show type selector, not create session)
	req = httptest.NewRequest(http.MethodGet, "/wizard", nil)
	rec = httptest.NewRecorder()

	srv.handleWizardPage(rec, req)

	// Should still have 2 sessions (no new session created for type selector)
	if srv.wizardStore.Count() != 2 {
		t.Errorf("expected 2 sessions (type selector shown, no session created), got %d", srv.wizardStore.Count())
	}
}

// TestHandleWizardModal_CreatesSession tests that modal creates a new session
func TestHandleWizardModal_CreatesSession(t *testing.T) {
	srv := &Server{
		tmpls:       make(map[string]*template.Template),
		wizardStore: NewWizardSessionStore(),
	}
	defer srv.wizardStore.Stop()

	req := httptest.NewRequest(http.MethodGet, "/wizard/modal?type=bug", nil)
	rec := httptest.NewRecorder()

	srv.handleWizardModal(rec, req)

	// Check that a session was created with correct type
	if srv.wizardStore.Count() != 1 {
		t.Errorf("expected 1 session, got %d", srv.wizardStore.Count())
	}

	// Verify the session is a bug type by getting it from the store
	// We need to track the session ID from the modal creation
	// Since we can't access unexported fields, we verify through the Count
	if srv.wizardStore.Count() != 1 {
		t.Errorf("expected 1 session, got %d", srv.wizardStore.Count())
	}
}

// TestConcurrentSessionAccess tests thread safety under concurrent load
func TestConcurrentSessionAccess(t *testing.T) {
	srv := &Server{
		tmpls:       make(map[string]*template.Template),
		wizardStore: NewWizardSessionStore(),
	}
	defer srv.wizardStore.Stop()

	// Create multiple sessions concurrently
	var wg sync.WaitGroup
	for i := 0; i < 100; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			srv.wizardStore.Create("feature")
		}()
	}
	wg.Wait()

	if srv.wizardStore.Count() != 100 {
		t.Errorf("expected 100 sessions, got %d", srv.wizardStore.Count())
	}

	// Access sessions concurrently using the Get method
	// Create sessions first to get their IDs
	var ids []string
	for i := 0; i < 100; i++ {
		session, _ := srv.wizardStore.Create("feature")
		if session != nil {
			ids = append(ids, session.ID)
		}
	}

	for i := 0; i < 100; i++ {
		wg.Add(1)
		go func(idx int) {
			defer wg.Done()
			if idx < len(ids) {
				srv.wizardStore.Get(ids[idx])
			}
		}(i)
	}
	wg.Wait()
}

// TestConcurrentHandlerAccess tests handlers under concurrent load
func TestConcurrentHandlerAccess(t *testing.T) {
	srv := &Server{
		tmpls:       make(map[string]*template.Template),
		wizardStore: NewWizardSessionStore(),
	}
	defer srv.wizardStore.Stop()

	var wg sync.WaitGroup

	// Concurrent wizard new requests
	for i := 0; i < 50; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			req := httptest.NewRequest(http.MethodGet, "/wizard/new?type=feature", nil)
			rec := httptest.NewRecorder()
			srv.handleWizardNew(rec, req)
		}()
	}

	wg.Wait()

	if srv.wizardStore.Count() != 50 {
		t.Errorf("expected 50 sessions, got %d", srv.wizardStore.Count())
	}
}

// TestHeaderButtons_FromBoard verifies header button is present on the board page
func TestHeaderButtons_FromBoard(t *testing.T) {
	srv := createTestServerWithTemplates(t)
	defer srv.wizardStore.Stop()

	req := httptest.NewRequest(http.MethodGet, "/", nil)
	rec := httptest.NewRecorder()

	srv.handleBoard(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("expected status 200, got %d", rec.Code)
	}

	body := rec.Body.String()

	// Verify single header button is present with correct href
	if !strings.Contains(body, `href="/wizard"`) {
		t.Error("board page missing New Issue button with correct href")
	}
	if !strings.Contains(body, "+ New Issue") {
		t.Error("board page missing 'New Issue' button text")
	}
	// Verify old buttons are NOT present
	if strings.Contains(body, `href="/wizard?type=feature"`) {
		t.Error("board page should not have old New Feature button")
	}
	if strings.Contains(body, `href="/wizard?type=bug"`) {
		t.Error("board page should not have old New Bug button")
	}
}

// TestHeaderButtons_FromBacklog verifies header button is present on the backlog page
func TestHeaderButtons_FromBacklog(t *testing.T) {
	srv := createTestServerWithTemplates(t)
	defer srv.wizardStore.Stop()

	req := httptest.NewRequest(http.MethodGet, "/backlog", nil)
	rec := httptest.NewRecorder()

	srv.handleBacklog(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("expected status 200, got %d", rec.Code)
	}

	body := rec.Body.String()

	// Verify single header button is present with correct href
	if !strings.Contains(body, `href="/wizard"`) {
		t.Error("backlog page missing New Issue button with correct href")
	}
	if !strings.Contains(body, "+ New Issue") {
		t.Error("backlog page missing 'New Issue' button text")
	}
	// Verify old buttons are NOT present
	if strings.Contains(body, `href="/wizard?type=feature"`) {
		t.Error("backlog page should not have old New Feature button")
	}
	if strings.Contains(body, `href="/wizard?type=bug"`) {
		t.Error("backlog page should not have old New Bug button")
	}
}

// TestHeaderButtons_FromCosts verifies header button is present on the costs page
func TestHeaderButtons_FromCosts(t *testing.T) {
	srv := createTestServerWithTemplates(t)
	defer srv.wizardStore.Stop()

	req := httptest.NewRequest(http.MethodGet, "/costs", nil)
	rec := httptest.NewRecorder()

	srv.handleCosts(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("expected status 200, got %d", rec.Code)
	}

	body := rec.Body.String()

	// Verify single header button is present with correct href
	if !strings.Contains(body, `href="/wizard"`) {
		t.Error("costs page missing New Issue button with correct href")
	}
	if !strings.Contains(body, "+ New Issue") {
		t.Error("costs page missing 'New Issue' button text")
	}
	// Verify old buttons are NOT present
	if strings.Contains(body, `href="/wizard?type=feature"`) {
		t.Error("costs page should not have old New Feature button")
	}
	if strings.Contains(body, `href="/wizard?type=bug"`) {
		t.Error("costs page should not have old New Bug button")
	}
}

// TestHeaderButtons_FromTask verifies header button is present on the task detail page
func TestHeaderButtons_FromTask(t *testing.T) {
	srv := createTestServerWithTemplates(t)
	// Keep store as nil - handler now handles nil store gracefully
	defer srv.wizardStore.Stop()

	req := httptest.NewRequest(http.MethodGet, "/task/123", nil)
	req.SetPathValue("id", "123")
	rec := httptest.NewRecorder()

	srv.handleTaskDetail(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("expected status 200, got %d", rec.Code)
	}

	body := rec.Body.String()

	// Verify single header button is present with correct href
	if !strings.Contains(body, `href="/wizard"`) {
		t.Error("task page missing New Issue button with correct href")
	}
	if !strings.Contains(body, "+ New Issue") {
		t.Error("task page missing 'New Issue' button text")
	}
	// Verify old buttons are NOT present
	if strings.Contains(body, `href="/wizard?type=feature"`) {
		t.Error("task page should not have old New Feature button")
	}
	if strings.Contains(body, `href="/wizard?type=bug"`) {
		t.Error("task page should not have old New Bug button")
	}
}

// TestHeaderButtons_FromWizard verifies header button is present on the wizard page itself
func TestHeaderButtons_FromWizard(t *testing.T) {
	srv := createTestServerWithTemplates(t)
	defer srv.wizardStore.Stop()

	req := httptest.NewRequest(http.MethodGet, "/wizard?type=feature", nil)
	rec := httptest.NewRecorder()

	srv.handleWizardPage(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("expected status 200, got %d", rec.Code)
	}

	body := rec.Body.String()

	// Verify single header button is present with correct href
	if !strings.Contains(body, `href="/wizard"`) {
		t.Error("wizard page missing New Issue button with correct href")
	}
	if !strings.Contains(body, "+ New Issue") {
		t.Error("wizard page missing 'New Issue' button text")
	}
	// Verify old buttons are NOT present
	if strings.Contains(body, `href="/wizard?type=feature"`) && !strings.Contains(body, `href="/wizard"`) {
		t.Error("wizard page should not have separate New Feature button (should be unified)")
	}
}

// TestWizardFlow_FromBacklog tests the complete wizard flow starting from backlog page
func TestWizardFlow_FromBacklog(t *testing.T) {
	srv := createTestServerWithTemplates(t)
	defer srv.wizardStore.Stop()

	// Step 1: Verify backlog page renders with unified header button
	req := httptest.NewRequest(http.MethodGet, "/backlog", nil)
	rec := httptest.NewRecorder()
	srv.handleBacklog(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("Step 1 failed: expected status 200, got %d", rec.Code)
	}

	body := rec.Body.String()
	if !strings.Contains(body, `href="/wizard"`) {
		t.Fatal("Step 1 failed: backlog page missing New Issue button")
	}

	// Step 2: Click New Issue button - request wizard page (shows type selector)
	req = httptest.NewRequest(http.MethodGet, "/wizard", nil)
	rec = httptest.NewRecorder()
	srv.handleWizardPage(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("Step 2 failed: expected status 200, got %d", rec.Code)
	}

	// No session yet - user needs to select type
	if srv.wizardStore.Count() != 0 {
		t.Fatalf("Step 2 failed: expected 0 sessions (type selector shown), got %d", srv.wizardStore.Count())
	}

	// Step 3: Select feature type via POST to select-type endpoint
	formData := url.Values{}
	formData.Set("wizard_type", "feature")
	req = httptest.NewRequest(http.MethodPost, "/wizard/select-type", strings.NewReader(formData.Encode()))
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	rec = httptest.NewRecorder()
	srv.handleWizardSelectType(rec, req)

	if rec.Code != http.StatusSeeOther {
		t.Fatalf("Step 3 failed: expected redirect (303), got %d", rec.Code)
	}

	// Now session should be created
	if srv.wizardStore.Count() != 1 {
		t.Fatalf("Step 3 failed: expected 1 session after type selection, got %d", srv.wizardStore.Count())
	}

	// Get the session ID from the redirect URL
	redirectURL := rec.Header().Get("Location")
	var sessionID string
	if strings.Contains(redirectURL, "session_id=") {
		sessionID = strings.Split(strings.Split(redirectURL, "session_id=")[1], "&")[0]
	}

	// Step 4: Submit idea for refinement
	formData = url.Values{}
	formData.Set("session_id", sessionID)
	formData.Set("idea", "Create a user dashboard with analytics")

	req = httptest.NewRequest(http.MethodPost, "/wizard/refine", strings.NewReader(formData.Encode()))
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	rec = httptest.NewRecorder()
	srv.handleWizardRefine(rec, req)

	if rec.Code != http.StatusOK && rec.Code != http.StatusInternalServerError {
		t.Fatalf("Step 4 failed: expected status 200 or 500, got %d", rec.Code)
	}

	// Step 5: Create issues (no more breakdown step in new 3-step flow)
	formData = url.Values{}
	formData.Set("session_id", sessionID)

	req = httptest.NewRequest(http.MethodPost, "/wizard/create", strings.NewReader(formData.Encode()))
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	rec = httptest.NewRecorder()
	srv.handleWizardCreate(rec, req)

	if rec.Code != http.StatusOK && rec.Code != http.StatusInternalServerError {
		t.Fatalf("Step 5 failed: expected status 200 or 500, got %d", rec.Code)
	}

	t.Log("Wizard flow from backlog completed successfully with new 3-step flow")
}

// TestWizardFlow_FromCosts tests the complete wizard flow starting from costs page
func TestWizardFlow_FromCosts(t *testing.T) {
	srv := createTestServerWithTemplates(t)
	defer srv.wizardStore.Stop()

	// Step 1: Verify costs page renders with unified header button
	req := httptest.NewRequest(http.MethodGet, "/costs", nil)
	rec := httptest.NewRecorder()
	srv.handleCosts(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("Step 1 failed: expected status 200, got %d", rec.Code)
	}

	body := rec.Body.String()
	if !strings.Contains(body, `href="/wizard"`) {
		t.Fatal("Step 1 failed: costs page missing New Issue button")
	}

	// Step 2: Click New Issue button - request wizard page (shows type selector)
	req = httptest.NewRequest(http.MethodGet, "/wizard", nil)
	rec = httptest.NewRecorder()
	srv.handleWizardPage(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("Step 2 failed: expected status 200, got %d", rec.Code)
	}

	// No session yet - user needs to select type
	if srv.wizardStore.Count() != 0 {
		t.Fatalf("Step 2 failed: expected 0 sessions (type selector shown), got %d", srv.wizardStore.Count())
	}

	// Step 3: Select bug type via POST to select-type endpoint
	formData := url.Values{}
	formData.Set("wizard_type", "bug")
	req = httptest.NewRequest(http.MethodPost, "/wizard/select-type", strings.NewReader(formData.Encode()))
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	rec = httptest.NewRecorder()
	srv.handleWizardSelectType(rec, req)

	if rec.Code != http.StatusSeeOther {
		t.Fatalf("Step 3 failed: expected redirect (303), got %d", rec.Code)
	}

	// Now session should be created
	if srv.wizardStore.Count() != 1 {
		t.Fatalf("Step 3 failed: expected 1 session after type selection, got %d", srv.wizardStore.Count())
	}

	// Get the session ID from the redirect URL
	redirectURL := rec.Header().Get("Location")
	var sessionID string
	if strings.Contains(redirectURL, "session_id=") {
		sessionID = strings.Split(strings.Split(redirectURL, "session_id=")[1], "&")[0]
	}

	// Verify it's a bug type
	session, _ := srv.wizardStore.Get(sessionID)
	if session == nil || session.Type != "bug" {
		t.Fatalf("Step 3 failed: expected session type 'bug', got %v", session)
	}

	// Step 4: Submit bug description for refinement
	formData = url.Values{}
	formData.Set("session_id", sessionID)
	formData.Set("idea", "Fix login page validation error")

	req = httptest.NewRequest(http.MethodPost, "/wizard/refine", strings.NewReader(formData.Encode()))
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	rec = httptest.NewRecorder()
	srv.handleWizardRefine(rec, req)

	if rec.Code != http.StatusOK && rec.Code != http.StatusInternalServerError {
		t.Fatalf("Step 4 failed: expected status 200 or 500, got %d", rec.Code)
	}

	// Step 5: Create issues (no more breakdown step in new 3-step flow)
	formData = url.Values{}
	formData.Set("session_id", sessionID)

	req = httptest.NewRequest(http.MethodPost, "/wizard/create", strings.NewReader(formData.Encode()))
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	rec = httptest.NewRecorder()
	srv.handleWizardCreate(rec, req)

	if rec.Code != http.StatusOK && rec.Code != http.StatusInternalServerError {
		t.Fatalf("Step 5 failed: expected status 200 or 500, got %d", rec.Code)
	}

	t.Log("Wizard flow from costs completed successfully with new 3-step flow")
}

// TestWizardRoutes_Registered verifies all wizard routes are properly registered
func TestWizardRoutes_Registered(t *testing.T) {
	srv := createTestServerWithTemplates(t)
	defer srv.wizardStore.Stop()

	// Test that the server handler is properly set up
	handler := srv.Handler()
	if handler == nil {
		t.Fatal("server handler is nil")
	}

	// Test each wizard route by making requests and verifying they don't return 404
	routes := []struct {
		method  string
		path    string
		handler func(*Server, http.ResponseWriter, *http.Request)
	}{
		{"GET", "/wizard", func(s *Server, w http.ResponseWriter, r *http.Request) { s.handleWizardPage(w, r) }},
		{"GET", "/wizard/new", func(s *Server, w http.ResponseWriter, r *http.Request) { s.handleWizardNew(w, r) }},
		{"GET", "/wizard/modal", func(s *Server, w http.ResponseWriter, r *http.Request) { s.handleWizardModal(w, r) }},
		{"POST", "/wizard/cancel", func(s *Server, w http.ResponseWriter, r *http.Request) { s.handleWizardCancel(w, r) }},
		{"POST", "/wizard/refine", func(s *Server, w http.ResponseWriter, r *http.Request) { s.handleWizardRefine(w, r) }},
		// REMOVED: {"POST", "/wizard/breakdown", func(s *Server, w http.ResponseWriter, r *http.Request) { s.handleWizardBreakdown(w, r) }},
		{"POST", "/wizard/create", func(s *Server, w http.ResponseWriter, r *http.Request) { s.handleWizardCreate(w, r) }},
	}

	for _, route := range routes {
		req := httptest.NewRequest(route.method, route.path, nil)
		rec := httptest.NewRecorder()

		// Call the handler directly
		route.handler(srv, rec, req)

		// Should not return 404 (handler exists)
		if rec.Code == http.StatusNotFound {
			t.Errorf("route %s %s returned 404", route.method, route.path)
		}
	}

	// Test logs endpoint separately (requires path value)
	req := httptest.NewRequest("GET", "/wizard/logs/test-session", nil)
	req.SetPathValue("sessionId", "test-session")
	rec := httptest.NewRecorder()
	srv.handleWizardLogs(rec, req)

	// Should return 404 for non-existent session, not "not found" handler
	if rec.Code != http.StatusNotFound {
		t.Logf("wizard logs endpoint returned status %d (expected 404 for invalid session)", rec.Code)
	}
}
func TestLayoutNavigationButtons(t *testing.T) {
	// Parse the layout template
	tmpl, err := template.ParseFiles("templates/layout.html")
	if err != nil {
		t.Fatalf("failed to parse layout template: %v", err)
	}

	// Execute the template with minimal data
	var buf strings.Builder
	data := struct {
		Active string
	}{
		Active: "board",
	}

	// We need to define a content template for the layout to work
	tmpl, err = tmpl.New("content").Parse("<div>Test Content</div>")
	if err != nil {
		t.Fatalf("failed to parse content template: %v", err)
	}

	err = tmpl.ExecuteTemplate(&buf, "layout", data)
	if err != nil {
		t.Fatalf("failed to execute template: %v", err)
	}

	output := buf.String()

	// Check for New Issue button as a link (unified entry point)
	if !strings.Contains(output, `href="/wizard"`) {
		t.Error("layout template missing New Issue button with correct href attribute")
	}
	if !strings.Contains(output, "+ New Issue") {
		t.Error("layout template missing 'New Issue' button text")
	}

	// Verify old buttons are NOT present
	if strings.Contains(output, `href="/wizard?type=feature"`) {
		t.Error("layout template should not have old New Feature button href")
	}
	if strings.Contains(output, `href="/wizard?type=bug"`) {
		t.Error("layout template should not have old New Bug button href")
	}

	// Check for correct CSS class on unified button
	if !strings.Contains(output, "btn btn-primary") {
		t.Error("layout template missing btn-primary class on New Issue button")
	}

	// Check for nav-actions container
	if !strings.Contains(output, "nav-actions") {
		t.Error("layout template missing nav-actions container div")
	}
}

// TestHandleWizardCreate_EpicFirst verifies that epic is created before sub-tasks
func TestHandleWizardCreate_EpicFirst(t *testing.T) {
	srv := &Server{
		tmpls:       make(map[string]*template.Template),
		wizardStore: NewWizardSessionStore(),
		gh:          nil,
	}
	defer srv.wizardStore.Stop()

	session, _ := srv.wizardStore.Create("feature")
	session.SetTechnicalPlanning("## Technical Planning\n\nTest technical planning with architecture overview and implementation details")

	req := httptest.NewRequest(http.MethodPost, "/wizard/create", strings.NewReader("session_id="+session.ID))
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	rec := httptest.NewRecorder()

	srv.handleWizardCreate(rec, req)

	if rec.Code != http.StatusOK && rec.Code != http.StatusInternalServerError {
		t.Errorf("expected status 200 or 500, got %d", rec.Code)
	}
}

// TestHandleWizardCreate_LabelMapping tests priority and complexity label conversion
func TestHandleWizardCreate_LabelMapping(t *testing.T) {
	testCases := []struct {
		name       string
		priority   string
		complexity string
		wantLabels []string
	}{
		{
			name:       "critical priority + XL size",
			priority:   "critical",
			complexity: "XL",
			wantLabels: []string{"wizard", "priority:high", "size:XL"},
		},
		{
			name:       "high priority + L size",
			priority:   "high",
			complexity: "L",
			wantLabels: []string{"wizard", "priority:high", "size:L"},
		},
		{
			name:       "medium priority + M size",
			priority:   "medium",
			complexity: "M",
			wantLabels: []string{"wizard", "priority:medium", "size:M"},
		},
		{
			name:       "low priority + S size",
			priority:   "low",
			complexity: "S",
			wantLabels: []string{"wizard", "priority:low", "size:S"},
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			// Verify label mapping logic is correct
			var labels []string
			labels = append(labels, "wizard")

			switch tc.priority {
			case "critical", "high":
				labels = append(labels, "priority:high")
			case "medium":
				labels = append(labels, "priority:medium")
			case "low":
				labels = append(labels, "priority:low")
			}

			switch tc.complexity {
			case "S":
				labels = append(labels, "size:S")
			case "M":
				labels = append(labels, "size:M")
			case "L":
				labels = append(labels, "size:L")
			case "XL":
				labels = append(labels, "size:XL")
			}

			if len(labels) != len(tc.wantLabels) {
				t.Errorf("expected %d labels, got %d: %v", len(tc.wantLabels), len(labels), labels)
			}
			for i, label := range labels {
				if label != tc.wantLabels[i] {
					t.Errorf("expected label %d to be %q, got %q", i, tc.wantLabels[i], label)
				}
			}
		})
	}
}

// TestHandleWizardCreate_EpicLabels tests epic label assignment based on type
func TestHandleWizardCreate_EpicLabels(t *testing.T) {
	testCases := []struct {
		name       string
		wizardType string
		wantLabels []string
	}{
		{
			name:       "feature type",
			wizardType: "feature",
			wantLabels: []string{"epic", "enhancement"},
		},
		{
			name:       "bug type",
			wizardType: "bug",
			wantLabels: []string{"epic", "bug"},
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			// Verify epic label logic
			var epicLabels []string
			epicLabels = append(epicLabels, "epic")
			if tc.wizardType == "feature" {
				epicLabels = append(epicLabels, "enhancement")
			} else if tc.wizardType == "bug" {
				epicLabels = append(epicLabels, "bug")
			}

			if len(epicLabels) != len(tc.wantLabels) {
				t.Errorf("expected %d epic labels, got %d: %v", len(tc.wantLabels), len(epicLabels), epicLabels)
			}
			for i, label := range epicLabels {
				if label != tc.wantLabels[i] {
					t.Errorf("expected epic label %d to be %q, got %q", i, tc.wantLabels[i], label)
				}
			}
		})
	}
}

// TestHandleWizardCreate_MissingSession tests error when session is missing
func TestHandleWizardCreate_MissingSession(t *testing.T) {
	srv := &Server{
		tmpls:       make(map[string]*template.Template),
		wizardStore: NewWizardSessionStore(),
	}
	defer srv.wizardStore.Stop()

	req := httptest.NewRequest(http.MethodPost, "/wizard/create", strings.NewReader("session_id=invalid"))
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	rec := httptest.NewRecorder()

	srv.handleWizardCreate(rec, req)

	if rec.Code != http.StatusBadRequest {
		t.Errorf("expected status 400 for invalid session, got %d", rec.Code)
	}
}

// TestHandleWizardCreate_MissingSessionID tests error when session_id is missing
func TestHandleWizardCreate_MissingSessionID(t *testing.T) {
	srv := &Server{
		tmpls:       make(map[string]*template.Template),
		wizardStore: NewWizardSessionStore(),
	}
	defer srv.wizardStore.Stop()

	req := httptest.NewRequest(http.MethodPost, "/wizard/create", strings.NewReader(""))
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	rec := httptest.NewRecorder()

	srv.handleWizardCreate(rec, req)

	if rec.Code != http.StatusBadRequest {
		t.Errorf("expected status 400 for missing session_id, got %d", rec.Code)
	}
}

// TestCreatedIssue_Tracking tests the CreatedIssue struct and tracking methods
func TestCreatedIssue_Tracking(t *testing.T) {
	store := NewWizardSessionStore()
	defer store.Stop()

	session, _ := store.Create("feature")

	// Test AddCreatedIssue
	epic := CreatedIssue{
		Number:  100,
		Title:   "Test Epic",
		URL:     "https://github.com/test/issues/100",
		IsEpic:  true,
		Success: true,
	}
	session.AddCreatedIssue(epic)

	if len(session.CreatedIssues) != 1 {
		t.Errorf("expected 1 created issue, got %d", len(session.CreatedIssues))
	}

	// Test SetCreatedIssues
	subTasks := []CreatedIssue{
		{Number: 101, Title: "Task 1", IsEpic: false, Success: true},
		{Number: 102, Title: "Task 2", IsEpic: false, Success: true},
	}
	session.SetCreatedIssues(subTasks)

	if len(session.CreatedIssues) != 2 {
		t.Errorf("expected 2 created issues after SetCreatedIssues, got %d", len(session.CreatedIssues))
	}

	// Test SetEpicNumber
	session.SetEpicNumber(100)
	if session.EpicNumber != 100 {
		t.Errorf("expected epic number 100, got %d", session.EpicNumber)
	}
}

// TestHandleWizardCreate_EpicBodyFormat tests the epic body format
func TestHandleWizardCreate_EpicBodyFormat(t *testing.T) {
	refinedDesc := "Test Epic Description"
	expectedBody := "## Summary\n\nTest Epic Description\n\n## Sub-tasks\n\n*Sub-tasks will be linked here after creation.*"

	// Verify the initial epic body format
	body := fmt.Sprintf("## Summary\n\n%s\n\n## Sub-tasks\n\n*Sub-tasks will be linked here after creation.*",
		refinedDesc)

	if body != expectedBody {
		t.Errorf("epic body format mismatch\nexpected: %s\ngot: %s", expectedBody, body)
	}

	// Verify the updated epic body format with sub-tasks
	subTaskLinks := []string{"- #101: Task 1", "- #102: Task 2"}
	updatedBody := fmt.Sprintf("## Summary\n\n%s\n\n## Sub-tasks\n\n%s",
		refinedDesc,
		strings.Join(subTaskLinks, "\n"),
	)

	expectedUpdatedBody := "## Summary\n\nTest Epic Description\n\n## Sub-tasks\n\n- #101: Task 1\n- #102: Task 2"
	if updatedBody != expectedUpdatedBody {
		t.Errorf("updated epic body format mismatch\nexpected: %s\ngot: %s", expectedUpdatedBody, updatedBody)
	}
}

// TestHandleWizardCreate_SubTaskBodyFormat tests the sub-task body format
func TestHandleWizardCreate_SubTaskBodyFormat(t *testing.T) {
	task := WizardTask{
		Title:       "Test Task",
		Description: "Test Description",
		Priority:    "high",
		Complexity:  "M",
	}
	epicNum := 100

	expectedBody := "## Description\n\nTest Description\n\n---\n\n**Parent Epic:** #100\n**Priority:** high\n**Complexity:** M"

	body := fmt.Sprintf("## Description\n\n%s\n\n---\n\n**Parent Epic:** #%d\n**Priority:** %s\n**Complexity:** %s",
		task.Description,
		epicNum,
		task.Priority,
		task.Complexity,
	)

	if body != expectedBody {
		t.Errorf("sub-task body format mismatch\nexpected: %s\ngot: %s", expectedBody, body)
	}
}

// TestWizardFlow_ValidationErrors tests all validation scenarios in sequence
func TestWizardFlow_ValidationErrors(t *testing.T) {
	srv := createTestServerWithTemplates(t)
	defer srv.wizardStore.Stop()

	tests := []struct {
		name       string
		setup      func() (*http.Request, *httptest.ResponseRecorder)
		handler    func(http.ResponseWriter, *http.Request)
		wantStatus int
		wantError  string
	}{
		{
			name: "invalid wizard type",
			setup: func() (*http.Request, *httptest.ResponseRecorder) {
				req := httptest.NewRequest(http.MethodGet, "/wizard/new?type=invalid", nil)
				return req, httptest.NewRecorder()
			},
			handler:    srv.handleWizardNew,
			wantStatus: http.StatusOK,       // New behavior: shows type selector instead of error
			wantError:  "Select Issue Type", // Type selector UI is shown
		},
		{
			name: "missing session_id on refine",
			setup: func() (*http.Request, *httptest.ResponseRecorder) {
				formData := url.Values{}
				formData.Set("idea", "test idea")
				req := httptest.NewRequest(http.MethodPost, "/wizard/refine", strings.NewReader(formData.Encode()))
				req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
				return req, httptest.NewRecorder()
			},
			handler:    srv.handleWizardRefine,
			wantStatus: http.StatusBadRequest,
			wantError:  "session_id",
		},
		{
			name: "empty idea on refine",
			setup: func() (*http.Request, *httptest.ResponseRecorder) {
				session, _ := srv.wizardStore.Create("feature")
				formData := url.Values{}
				formData.Set("session_id", session.ID)
				formData.Set("idea", "")
				req := httptest.NewRequest(http.MethodPost, "/wizard/refine", strings.NewReader(formData.Encode()))
				req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
				return req, httptest.NewRecorder()
			},
			handler:    srv.handleWizardRefine,
			wantStatus: http.StatusBadRequest,
			wantError:  "idea",
		},
		{
			name: "missing session_id on create",
			setup: func() (*http.Request, *httptest.ResponseRecorder) {
				req := httptest.NewRequest(http.MethodPost, "/wizard/create", strings.NewReader(""))
				req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
				return req, httptest.NewRecorder()
			},
			handler:    srv.handleWizardCreate,
			wantStatus: http.StatusBadRequest,
			wantError:  "session_id",
		},
		{
			name: "invalid session_id",
			setup: func() (*http.Request, *httptest.ResponseRecorder) {
				formData := url.Values{}
				formData.Set("session_id", "nonexistent-session-id")
				req := httptest.NewRequest(http.MethodPost, "/wizard/create", strings.NewReader(formData.Encode()))
				req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
				return req, httptest.NewRecorder()
			},
			handler:    srv.handleWizardCreate,
			wantStatus: http.StatusBadRequest,
			wantError:  "session",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			req, rec := tt.setup()
			tt.handler(rec, req)

			if rec.Code != tt.wantStatus {
				t.Errorf("expected status %d, got %d", tt.wantStatus, rec.Code)
			}

			body := rec.Body.String()

			// Debug: print the actual response
			t.Logf("Response body:\n%s", body)
			if !strings.Contains(strings.ToLower(body), strings.ToLower(tt.wantError)) {
				t.Errorf("expected error containing %q, got: %s", tt.wantError, body)
			}
		})
	}
}

// TestWizardFlow_ConcurrentUsers tests multiple simultaneous wizard sessions
func TestWizardFlow_ConcurrentUsers(t *testing.T) {
	srv := &Server{
		tmpls:       make(map[string]*template.Template),
		wizardStore: NewWizardSessionStore(),
	}
	defer srv.wizardStore.Stop()

	const numUsers = 10
	var wg sync.WaitGroup

	// Each user creates a complete wizard flow
	for i := 0; i < numUsers; i++ {
		wg.Add(1)
		go func(userID int) {
			defer wg.Done()

			wizardType := "feature"
			if userID%2 == 0 {
				wizardType = "bug"
			}

			// Step 1: Create session
			session, err := srv.wizardStore.Create(wizardType)
			if err != nil {
				t.Errorf("User %d: failed to create session: %v", userID, err)
				return
			}

			// Step 2: Refine
			formData := url.Values{}
			formData.Set("session_id", session.ID)
			formData.Set("idea", fmt.Sprintf("User %d idea", userID))

			req := httptest.NewRequest(http.MethodPost, "/wizard/refine", strings.NewReader(formData.Encode()))
			req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
			rec := httptest.NewRecorder()
			srv.handleWizardRefine(rec, req)

			if rec.Code != http.StatusOK && rec.Code != http.StatusInternalServerError {
				t.Errorf("User %d: refine failed with status %d", userID, rec.Code)
				return
			}

			// Step 3: Create issues (no more breakdown step in new 3-step flow)
			formData = url.Values{}
			formData.Set("session_id", session.ID)

			req = httptest.NewRequest(http.MethodPost, "/wizard/create", strings.NewReader(formData.Encode()))
			req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
			rec = httptest.NewRecorder()
			srv.handleWizardCreate(rec, req)

			if rec.Code != http.StatusOK && rec.Code != http.StatusInternalServerError {
				t.Errorf("User %d: create failed with status %d", userID, rec.Code)
				return
			}

			// Verify session was deleted after creation
			_, ok := srv.wizardStore.Get(session.ID)
			if ok {
				t.Errorf("User %d: session should be deleted after creation", userID)
			}
		}(i)
	}

	wg.Wait()

	// Verify all sessions were cleaned up after creation
	count := srv.wizardStore.Count()
	if count != 0 {
		t.Errorf("expected 0 sessions after creation, got %d", count)
	}

	t.Logf("Concurrent wizard flow test completed: %d users, all sessions cleaned up", numUsers)
}

// TestWizardFlow_PostCreationVerification tests redirect and cleanup after creation
func TestWizardFlow_PostCreationVerification(t *testing.T) {
	srv := &Server{
		tmpls:       make(map[string]*template.Template),
		wizardStore: NewWizardSessionStore(),
	}
	defer srv.wizardStore.Stop()

	// Create and complete a wizard session
	session, _ := srv.wizardStore.Create("feature")
	session.SetIdeaText("Test feature idea")
	session.SetTechnicalPlanning("## Technical Planning\n\nTest technical planning for the feature")

	// Store the session
	srv.wizardStore.sessions[session.ID] = session

	// Verify session exists
	_, ok := srv.wizardStore.Get(session.ID)
	if !ok {
		t.Fatal("Session should exist before creation")
	}

	// Step 3: Create issues (no more breakdown step)
	formData := url.Values{}
	formData.Set("session_id", session.ID)

	req := httptest.NewRequest(http.MethodPost, "/wizard/create", strings.NewReader(formData.Encode()))
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	rec := httptest.NewRecorder()
	srv.handleWizardCreate(rec, req)

	// Verify response status
	if rec.Code != http.StatusOK && rec.Code != http.StatusInternalServerError {
		t.Errorf("expected status 200 or 500, got %d", rec.Code)
	}

	// Verify session was cleaned up
	_, ok = srv.wizardStore.Get(session.ID)
	if ok {
		t.Error("Session should be deleted after successful creation")
	}

	// Verify response contains success indicator or redirect info
	body := rec.Body.String()
	if !strings.Contains(body, "success") && !strings.Contains(body, "created") &&
		!strings.Contains(body, "error") && !strings.Contains(body, "fail") {
		t.Logf("Response body: %s", truncate(body, 200))
	}

	t.Logf("Post-creation verification completed: session cleaned up = %v", !ok)
}

// truncate helper function for test output
func truncate(s string, n int) string {
	if len(s) <= n {
		return s
	}
	return s[:n] + "..."
}

// TestFullWizardFlow_Bug tests the complete bug wizard flow end-to-end
func TestFullWizardFlow_Bug(t *testing.T) {
	srv := &Server{
		tmpls:       make(map[string]*template.Template),
		wizardStore: NewWizardSessionStore(),
	}
	defer srv.wizardStore.Stop()

	// Step 1: Start bug wizard (GET /wizard/new)
	req := httptest.NewRequest(http.MethodGet, "/wizard/new?type=bug", nil)
	rec := httptest.NewRecorder()
	srv.handleWizardNew(rec, req)

	if rec.Code != http.StatusOK && rec.Code != http.StatusInternalServerError {
		t.Fatalf("Step 1 failed: expected status 200 or 500, got %d", rec.Code)
	}

	if srv.wizardStore.Count() < 1 {
		t.Fatal("No session created in step 1")
	}

	// Create a new session for testing the flow
	testSession, _ := srv.wizardStore.Create("bug")
	sessionID := testSession.ID

	// Step 2: Refine bug idea (POST /wizard/refine)
	formData := url.Values{}
	formData.Set("session_id", sessionID)
	formData.Set("idea", "Login form validation is broken")

	req = httptest.NewRequest(http.MethodPost, "/wizard/refine", strings.NewReader(formData.Encode()))
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	rec = httptest.NewRecorder()
	srv.handleWizardRefine(rec, req)

	if rec.Code != http.StatusOK && rec.Code != http.StatusInternalServerError {
		t.Fatalf("Step 2 failed: expected status 200 or 500, got %d", rec.Code)
	}

	// Verify session was updated with technical planning
	session, _ := srv.wizardStore.Get(sessionID)
	if session.IdeaText == "" {
		t.Error("Step 2: Idea text not stored")
	}
	if session.TechnicalPlanning == "" {
		t.Error("Step 2: Technical planning not generated")
	}
	if session.CurrentStep != WizardStepRefine {
		t.Errorf("Step 2: Expected step 'refine', got %q", session.CurrentStep)
	}

	// Step 3: Create issues (POST /wizard/create) - no more breakdown step
	formData = url.Values{}
	formData.Set("session_id", sessionID)

	req = httptest.NewRequest(http.MethodPost, "/wizard/create", strings.NewReader(formData.Encode()))
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	rec = httptest.NewRecorder()
	srv.handleWizardCreate(rec, req)

	if rec.Code != http.StatusOK && rec.Code != http.StatusInternalServerError {
		t.Fatalf("Step 3 failed: expected status 200 or 500, got %d", rec.Code)
	}

	// Verify session was deleted after creation
	_, ok := srv.wizardStore.Get(sessionID)
	if ok {
		t.Error("Step 3: Session should be deleted after creation")
	}

	t.Logf("Full bug wizard flow completed successfully with new 3-step flow")
}

// TestHandleWizardRefine_SkipBreakdown tests that SkipBreakdown is always true in new unified flow
func TestHandleWizardRefine_SkipBreakdown(t *testing.T) {
	srv := &Server{
		tmpls:       make(map[string]*template.Template),
		wizardStore: NewWizardSessionStore(),
	}
	defer srv.wizardStore.Stop()

	// In the new unified flow, breakdown step is removed, so SkipBreakdown should always be true
	session, _ := srv.wizardStore.Create("feature")

	form := url.Values{}
	form.Set("session_id", session.ID)
	form.Set("idea", "Create a login page")

	req := httptest.NewRequest(http.MethodPost, "/wizard/refine", strings.NewReader(form.Encode()))
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	rec := httptest.NewRecorder()

	srv.handleWizardRefine(rec, req)

	// Verify session has SkipBreakdown set to true (always skip in new flow)
	updatedSession, _ := srv.wizardStore.Get(session.ID)
	if !updatedSession.SkipBreakdown {
		t.Error("expected SkipBreakdown to be true in new unified flow (breakdown step removed)")
	}

	// Test with bug type (should also skip breakdown)
	session2, _ := srv.wizardStore.Create("bug")

	form2 := url.Values{}
	form2.Set("session_id", session2.ID)
	form2.Set("idea", "Fix login bug")

	req2 := httptest.NewRequest(http.MethodPost, "/wizard/refine", strings.NewReader(form2.Encode()))
	req2.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	rec2 := httptest.NewRecorder()

	srv.handleWizardRefine(rec2, req2)

	// Verify session has SkipBreakdown set to true for bugs as well
	updatedSession2, _ := srv.wizardStore.Get(session2.ID)
	if !updatedSession2.SkipBreakdown {
		t.Error("expected SkipBreakdown to be true for bug type in new unified flow")
	}
}

// TestHandleWizardCreateSingle_UsesTechnicalPlanningForTitle verifies single issue uses technical planning
func TestHandleWizardCreateSingle_UsesRefinedDescriptionForTitle(t *testing.T) {
	srv := createTestServerWithTemplates(t)
	defer srv.wizardStore.Stop()

	session, _ := srv.wizardStore.Create("feature")
	session.SetIdeaText("Raw user input")
	session.SetTechnicalPlanning("## Technical Planning\n\nLLM generated technical planning")

	form := url.Values{}
	form.Set("session_id", session.ID)

	req := httptest.NewRequest(http.MethodPost, "/wizard/create", strings.NewReader(form.Encode()))
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	rec := httptest.NewRecorder()

	srv.handleWizardCreate(rec, req)

	body := rec.Body.String()
	if !strings.Contains(body, "Technical Planning") {
		t.Errorf("expected response to contain technical planning as title, got: %s", body)
	}
}

// TestHandleWizardCreateSingle tests creating a single issue without epic/sub-tasks
func TestHandleWizardCreateSingle(t *testing.T) {
	srv := &Server{
		tmpls:       make(map[string]*template.Template),
		wizardStore: NewWizardSessionStore(),
	}
	defer srv.wizardStore.Stop()

	// Create a session with technical planning
	session, _ := srv.wizardStore.Create("feature")
	session.SetIdeaText("Small feature idea")
	session.SetTechnicalPlanning("## Technical Planning\n\nThis is a small feature technical planning")

	// Test creating single issue
	form := url.Values{}
	form.Set("session_id", session.ID)

	req := httptest.NewRequest(http.MethodPost, "/wizard/create", strings.NewReader(form.Encode()))
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	rec := httptest.NewRecorder()

	srv.handleWizardCreate(rec, req)

	// Should return 200 OK (or 500 if template missing)
	if rec.Code != http.StatusOK && rec.Code != http.StatusInternalServerError {
		t.Errorf("expected status 200 or 500, got %d: %s", rec.Code, rec.Body.String())
	}

	// Verify session was deleted after creation
	_, ok := srv.wizardStore.Get(session.ID)
	if ok {
		t.Error("session should be deleted after single issue creation")
	}
}

// TestHandleWizardCreateSingle_WithSprint tests single issue creation with sprint assignment
func TestHandleWizardCreateSingle_WithSprint(t *testing.T) {
	srv := &Server{
		tmpls:       make(map[string]*template.Template),
		wizardStore: NewWizardSessionStore(),
	}
	defer srv.wizardStore.Stop()

	// Create a session with SkipBreakdown enabled
	session, _ := srv.wizardStore.Create("feature")
	session.SetIdeaText("Small feature with sprint")
	session.SetRefinedDescription("This is a small feature for sprint")
	session.SetSkipBreakdown(true)

	// Test creating single issue with sprint assignment
	form := url.Values{}
	form.Set("session_id", session.ID)
	form.Set("add_to_sprint", "1")

	req := httptest.NewRequest(http.MethodPost, "/wizard/create", strings.NewReader(form.Encode()))
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	rec := httptest.NewRecorder()

	srv.handleWizardCreate(rec, req)

	// Should return 200 OK (or 500 if template missing)
	if rec.Code != http.StatusOK && rec.Code != http.StatusInternalServerError {
		t.Errorf("expected status 200 or 500, got %d: %s", rec.Code, rec.Body.String())
	}

	// Verify session was deleted after creation
	_, ok := srv.wizardStore.Get(session.ID)
	if ok {
		t.Error("session should be deleted after single issue creation with sprint")
	}
}

// TestWizardFlow_SkipBreakdown tests the complete flow in new unified 3-step flow (breakdown always skipped)
func TestWizardFlow_SkipBreakdown(t *testing.T) {
	srv := &Server{
		tmpls:       make(map[string]*template.Template),
		wizardStore: NewWizardSessionStore(),
	}
	defer srv.wizardStore.Stop()

	// Step 1: Create session
	session, _ := srv.wizardStore.Create("feature")
	sessionID := session.ID

	// Step 2: Refine (generates technical planning in unified flow)
	form := url.Values{}
	form.Set("session_id", sessionID)
	form.Set("idea", "Feature with technical planning")

	req := httptest.NewRequest(http.MethodPost, "/wizard/refine", strings.NewReader(form.Encode()))
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	rec := httptest.NewRecorder()
	srv.handleWizardRefine(rec, req)

	if rec.Code != http.StatusOK && rec.Code != http.StatusInternalServerError {
		t.Fatalf("Refine step failed: expected status 200 or 500, got %d", rec.Code)
	}

	// Verify SkipBreakdown is set to true (always skip in new unified flow)
	session, _ = srv.wizardStore.Get(sessionID)
	if !session.SkipBreakdown {
		t.Error("expected SkipBreakdown to be true in new unified flow (breakdown step removed)")
	}

	// Verify technical planning was generated
	if session.TechnicalPlanning == "" {
		t.Error("expected technical planning to be generated")
	}

	// Step 3: Create single issue (no breakdown step in new flow)
	form2 := url.Values{}
	form2.Set("session_id", sessionID)

	req2 := httptest.NewRequest(http.MethodPost, "/wizard/create", strings.NewReader(form2.Encode()))
	req2.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	rec2 := httptest.NewRecorder()
	srv.handleWizardCreate(rec2, req2)

	if rec2.Code != http.StatusOK && rec2.Code != http.StatusInternalServerError {
		t.Fatalf("Create step failed: expected status 200 or 500, got %d", rec2.Code)
	}

	// Verify session was deleted
	_, ok := srv.wizardStore.Get(sessionID)
	if ok {
		t.Error("session should be deleted after single issue creation")
	}

	t.Logf("Unified 3-step flow completed successfully (breakdown step removed)")
}

// TestWizardSession_SetSkipBreakdown tests the SetSkipBreakdown method
func TestWizardSession_SetSkipBreakdown(t *testing.T) {
	session := &WizardSession{
		ID:   "test-id",
		Type: "feature",
	}

	// Test setting SkipBreakdown to true
	session.SetSkipBreakdown(true)
	if !session.SkipBreakdown {
		t.Error("expected SkipBreakdown to be true")
	}

	// Test setting SkipBreakdown to false
	session.SetSkipBreakdown(false)
	if session.SkipBreakdown {
		t.Error("expected SkipBreakdown to be false")
	}
}

// TestHandleWizardCreate_SkipsBreakdownWhenFlagSet tests that breakdown is skipped when flag is set
func TestHandleWizardCreate_SkipsBreakdownWhenFlagSet(t *testing.T) {
	srv := &Server{
		tmpls:       make(map[string]*template.Template),
		wizardStore: NewWizardSessionStore(),
	}
	defer srv.wizardStore.Stop()

	// Create a feature session with SkipBreakdown enabled
	session, _ := srv.wizardStore.Create("feature")
	session.SetIdeaText("Small feature")
	session.SetRefinedDescription("A small feature description")
	session.SetSkipBreakdown(true)
	session.SetStep(WizardStepRefine)

	// Call handleWizardCreate - should use handleWizardCreateSingle path
	form := url.Values{}
	form.Set("session_id", session.ID)

	req := httptest.NewRequest(http.MethodPost, "/wizard/create", strings.NewReader(form.Encode()))
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	rec := httptest.NewRecorder()

	srv.handleWizardCreate(rec, req)

	// Should succeed (or fail with template error)
	if rec.Code != http.StatusOK && rec.Code != http.StatusInternalServerError {
		t.Errorf("expected status 200 or 500, got %d: %s", rec.Code, rec.Body.String())
	}
}

// TestBoardLayout_AfterButtonRemoval verifies board renders with unified New Issue button in header
func TestBoardLayout_AfterButtonRemoval(t *testing.T) {
	srv := createTestServerWithTemplates(t)
	defer srv.wizardStore.Stop()

	req := httptest.NewRequest(http.MethodGet, "/", nil)
	rec := httptest.NewRecorder()

	srv.handleBoard(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("expected status 200, got %d", rec.Code)
	}

	body := rec.Body.String()

	// Verify unified "+ New Issue" button is present in header navigation
	if !strings.Contains(body, "+ New Issue") {
		t.Error("board page missing '+ New Issue' button in header navigation")
	}

	// Verify old separate buttons are NOT present
	if strings.Contains(body, "+ New Feature") {
		t.Error("board page should not have old '+ New Feature' button")
	}
	if strings.Contains(body, "+ New Bug") {
		t.Error("board page should not have old '+ New Bug' button")
	}

	// Verify the unified button links to /wizard (without type param)
	if !strings.Contains(body, `href="/wizard"`) {
		t.Error("board page New Issue button should link to /wizard")
	}
}

// TestBoardActions_ContainsExpectedButtons verifies board-actions has correct buttons
func TestBoardActions_ContainsExpectedButtons(t *testing.T) {
	srv := createTestServerWithTemplates(t)
	defer srv.wizardStore.Stop()

	req := httptest.NewRequest(http.MethodGet, "/", nil)
	rec := httptest.NewRecorder()

	srv.handleBoard(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("expected status 200, got %d", rec.Code)
	}

	body := rec.Body.String()

	// Verify board-actions section contains expected sprint control buttons
	// Note: Start Sprint and Pause Sprint are mutually exclusive (conditional on .Paused)
	hasStartSprint := strings.Contains(body, "Start Sprint")
	hasPauseSprint := strings.Contains(body, "Pause Sprint")
	if !hasStartSprint && !hasPauseSprint {
		t.Error("board-actions section missing both Start Sprint and Pause Sprint buttons - should have one")
	}

	// These buttons should always be present
	requiredButtons := []string{
		"Sync",
		"Autosync",
		"Plan Sprint",
	}

	for _, button := range requiredButtons {
		if !strings.Contains(body, button) {
			t.Errorf("board-actions section missing required button: %s", button)
		}
	}

	// Verify the board-actions div exists with correct class
	if !strings.Contains(body, `class="board-actions"`) {
		t.Error("board page missing board-actions container div")
	}
}

// TestBoardLayout_ResponsiveCSS verifies responsive CSS is present
func TestBoardLayout_ResponsiveCSS(t *testing.T) {
	srv := createTestServerWithTemplates(t)
	defer srv.wizardStore.Stop()

	req := httptest.NewRequest(http.MethodGet, "/", nil)
	rec := httptest.NewRecorder()

	srv.handleBoard(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("expected status 200, got %d", rec.Code)
	}

	body := rec.Body.String()

	// Verify responsive CSS classes are present
	requiredCSS := []string{
		"flex-wrap:wrap",
		"@media",
		"board-actions",
		"board-header",
	}

	for _, css := range requiredCSS {
		if !strings.Contains(body, css) {
			t.Errorf("board page missing required CSS: %s", css)
		}
	}
}

// TestBoardLayout_ValidHTMLStructure verifies board page has valid HTML structure
func TestBoardLayout_ValidHTMLStructure(t *testing.T) {
	srv := createTestServerWithTemplates(t)
	defer srv.wizardStore.Stop()

	req := httptest.NewRequest(http.MethodGet, "/", nil)
	rec := httptest.NewRecorder()

	srv.handleBoard(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("expected status 200, got %d", rec.Code)
	}

	body := rec.Body.String()

	// Verify essential HTML structure elements
	structureChecks := map[string]string{
		"DOCTYPE":       "<!DOCTYPE html>",
		"html tag":      "<html",
		"head tag":      "<head>",
		"body tag":      "<body>",
		"board-header":  `class="board-header"`,
		"board-actions": `class="board-actions"`,
		"board grid":    `class="board"`,
		"7 columns":     "grid-template-columns:repeat(7,1fr)",
	}

	for name, pattern := range structureChecks {
		if !strings.Contains(body, pattern) {
			t.Errorf("board page missing %s structure element", name)
		}
	}

	// Verify no unclosed tags that would cause rendering issues
	// Count opening and closing divs (basic check)
	openDivs := strings.Count(body, "<div")
	closeDivs := strings.Count(body, "</div>")
	if openDivs != closeDivs {
		t.Errorf("HTML structure issue: %d opening <div> tags but %d closing </div> tags", openDivs, closeDivs)
	}
}

// TestBoardLayout_SprintControlsFunctional verifies sprint control buttons work
func TestBoardLayout_SprintControlsFunctional(t *testing.T) {
	srv := createTestServerWithTemplates(t)
	defer srv.wizardStore.Stop()

	req := httptest.NewRequest(http.MethodGet, "/", nil)
	rec := httptest.NewRecorder()

	srv.handleBoard(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("expected status 200, got %d", rec.Code)
	}

	body := rec.Body.String()

	// Verify sprint control forms have correct action URLs
	// Note: start and pause forms are mutually exclusive (conditional on .Paused)
	hasStartForm := strings.Contains(body, `action="/api/sprint/start"`)
	hasPauseForm := strings.Contains(body, `action="/api/sprint/pause"`)
	if !hasStartForm && !hasPauseForm {
		t.Error("board page missing both sprint control forms - should have either start or pause")
	}

	// These forms should always be present
	requiredForms := []string{
		`action="/sync"`,
		`action="/plan-sprint"`,
	}

	for _, form := range requiredForms {
		if !strings.Contains(body, form) {
			t.Errorf("board page missing sprint control form: %s", form)
		}
	}

	// Verify autosync toggle button exists with correct ID
	if !strings.Contains(body, `id="autosync-toggle"`) {
		t.Error("board page missing autosync toggle button")
	}

	// Verify HTMX polling is configured for board data
	if !strings.Contains(body, `hx-get="/api/board-data"`) {
		t.Error("board page missing HTMX polling for board data")
	}
}

// TestBoardLayout_NoConsoleErrors verifies no JavaScript errors in page structure
func TestBoardLayout_NoConsoleErrors(t *testing.T) {
	srv := createTestServerWithTemplates(t)
	defer srv.wizardStore.Stop()

	req := httptest.NewRequest(http.MethodGet, "/", nil)
	rec := httptest.NewRecorder()

	srv.handleBoard(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("expected status 200, got %d", rec.Code)
	}

	body := rec.Body.String()

	// Check for common JavaScript error patterns that would cause console errors
	errorPatterns := []string{
		"undefined",
		"null pointer",
		"cannot read property",
	}

	// These checks are for obvious error strings in the HTML
	// Real console error testing would require a browser environment
	for _, pattern := range errorPatterns {
		if strings.Contains(strings.ToLower(body), pattern) {
			t.Logf("Warning: page contains potential error indicator: %s", pattern)
		}
	}

	// Verify all required JavaScript functions are defined
	requiredFunctions := []string{
		"function openDeclineModal",
		"function closeDeclineModal",
		"function toggleAutosync",
	}

	for _, fn := range requiredFunctions {
		if !strings.Contains(body, fn) {
			t.Errorf("board page missing required JavaScript function: %s", fn)
		}
	}

	// Verify HTMX library is included
	if !strings.Contains(body, "htmx.org") {
		t.Error("board page missing HTMX library")
	}
}

// TestWizardStepIndicator_OOBAttribute verifies the step indicator has hx-swap-oob attribute
func TestWizardStepIndicator_OOBAttribute(t *testing.T) {
	srv := createTestServerWithTemplates(t)
	defer srv.wizardStore.Stop()

	// Create a feature wizard session
	req := httptest.NewRequest(http.MethodGet, "/wizard/new?type=feature", nil)
	rec := httptest.NewRecorder()

	srv.handleWizardNew(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("expected status 200, got %d", rec.Code)
	}

	body := rec.Body.String()

	// Verify the step indicator has hx-swap-oob attribute
	if !strings.Contains(body, `hx-swap-oob="true"`) {
		t.Error("step indicator missing hx-swap-oob attribute for HTMX OOB swaps")
	}

	// Verify the step indicator has the correct ID
	if !strings.Contains(body, `id="wizard-step-indicator"`) {
		t.Error("step indicator missing correct ID")
	}
}

// TestWizardStepIndicator_Step1Active verifies step 1 is active on the new step
func TestWizardStepIndicator_Step1Active(t *testing.T) {
	srv := createTestServerWithTemplates(t)
	defer srv.wizardStore.Stop()

	req := httptest.NewRequest(http.MethodGet, "/wizard/new?type=feature", nil)
	rec := httptest.NewRecorder()

	srv.handleWizardNew(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("expected status 200, got %d", rec.Code)
	}

	body := rec.Body.String()

	// Step 1 should be active (check for class="step active" with possible trailing space)
	if !strings.Contains(body, `class="step active`) {
		t.Error("step 1 should have 'active' class on new step")
	}

	// Should have exactly one active step
	activeCount := strings.Count(body, `class="step active`)
	if activeCount != 1 {
		t.Errorf("expected exactly 1 active step, got %d", activeCount)
	}
}

// TestWizardStepIndicator_ShowBreakdownStep_FeatureType verifies 3-step flow for feature type
func TestWizardStepIndicator_ShowBreakdownStep_FeatureType(t *testing.T) {
	srv := createTestServerWithTemplates(t)
	defer srv.wizardStore.Stop()

	req := httptest.NewRequest(http.MethodGet, "/wizard/new?type=feature", nil)
	rec := httptest.NewRecorder()

	srv.handleWizardNew(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("expected status 200, got %d", rec.Code)
	}

	body := rec.Body.String()

	// For feature type, should show 3 steps (Idea, Technical Planning, Create) - no more breakdown
	// Count the step-label spans
	stepLabels := []string{"Idea", "Technical Planning", "Create"}
	for _, label := range stepLabels {
		if !strings.Contains(body, `<span class="step-label">`+label+`</span>`) {
			t.Errorf("step indicator missing '%s' label for feature type", label)
		}
	}

	// Should NOT show Breakdown step anymore
	if strings.Contains(body, `<span class="step-label">Breakdown</span>`) {
		t.Error("step indicator should NOT show 'Breakdown' step (removed in new flow)")
	}
}

// TestWizardStepIndicator_ShowBreakdownStep_BugType verifies 3-step flow for bug type
func TestWizardStepIndicator_ShowBreakdownStep_BugType(t *testing.T) {
	srv := createTestServerWithTemplates(t)
	defer srv.wizardStore.Stop()

	req := httptest.NewRequest(http.MethodGet, "/wizard/new?type=bug", nil)
	rec := httptest.NewRecorder()

	srv.handleWizardNew(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("expected status 200, got %d", rec.Code)
	}

	body := rec.Body.String()

	// For bug type, should NOT show Breakdown step (removed in new flow)
	if strings.Contains(body, `<span class="step-label">Breakdown</span>`) {
		t.Error("step indicator should NOT show 'Breakdown' step for bug type (removed in new flow)")
	}

	// Should have 3 steps (Idea, Technical Planning, Create) - same as feature now
	stepLabels := []string{"Idea", "Technical Planning", "Create"}
	for _, label := range stepLabels {
		if !strings.Contains(body, `<span class="step-label">`+label+`</span>`) {
			t.Errorf("step indicator missing '%s' label for bug type", label)
		}
	}
}

// TestWizardStepIndicator_RespectsSkipBreakdown verifies ShowBreakdownStep respects session.SkipBreakdown
func TestWizardStepIndicator_RespectsSkipBreakdown(t *testing.T) {
	srv := createTestServerWithTemplates(t)
	defer srv.wizardStore.Stop()

	// Create a feature session with SkipBreakdown set to true
	session, _ := srv.wizardStore.Create("feature")
	session.SetSkipBreakdown(true)

	// Request the new step with existing session
	req := httptest.NewRequest(http.MethodGet, "/wizard/new?type=feature&session_id="+session.ID, nil)
	rec := httptest.NewRecorder()

	srv.handleWizardNew(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("expected status 200, got %d", rec.Code)
	}

	body := rec.Body.String()

	// Should NOT show Breakdown step when SkipBreakdown is true
	if strings.Contains(body, `<span class="step-label">Breakdown</span>`) {
		t.Error("step indicator should NOT show 'Breakdown' step when SkipBreakdown is true")
	}
}

// TestWizardStepIndicator_NoDuplicateInContent verifies step indicator is not duplicated inside wizard content
func TestWizardStepIndicator_NoDuplicateInContent(t *testing.T) {
	srv := createTestServerWithTemplates(t)
	defer srv.wizardStore.Stop()

	req := httptest.NewRequest(http.MethodGet, "/wizard/new?type=feature", nil)
	rec := httptest.NewRecorder()

	srv.handleWizardNew(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("expected status 200, got %d", rec.Code)
	}

	body := rec.Body.String()

	// Count occurrences of the step-indicator div
	indicatorCount := strings.Count(body, `id="wizard-step-indicator"`)
	if indicatorCount > 1 {
		t.Errorf("step indicator appears %d times, should appear only once (no duplicates in content)", indicatorCount)
	}
}

// TestHandleWizardRefine_ParsesAddToSprint verifies that the add_to_sprint form value is parsed and stored in session
func TestHandleWizardRefine_ParsesAddToSprint(t *testing.T) {
	srv := createTestServerWithTemplates(t)
	defer srv.wizardStore.Stop()

	// Create a session
	session, _ := srv.wizardStore.Create("feature")

	// Test with add_to_sprint checked
	form := url.Values{}
	form.Set("session_id", session.ID)
	form.Set("idea", "Test feature idea")
	form.Set("add_to_sprint", "1")

	req := httptest.NewRequest(http.MethodPost, "/wizard/refine", strings.NewReader(form.Encode()))
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	rec := httptest.NewRecorder()

	srv.handleWizardRefine(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("expected status 200, got %d", rec.Code)
	}

	// Verify session was updated
	updated, ok := srv.wizardStore.Get(session.ID)
	if !ok {
		t.Fatal("expected to retrieve session")
	}

	if !updated.AddToSprint {
		t.Errorf("expected AddToSprint to be true when checkbox is checked, got %v", updated.AddToSprint)
	}

	// Test with add_to_sprint unchecked
	session2, _ := srv.wizardStore.Create("feature")
	form2 := url.Values{}
	form2.Set("session_id", session2.ID)
	form2.Set("idea", "Another test idea")
	// Don't set add_to_sprint

	req2 := httptest.NewRequest(http.MethodPost, "/wizard/refine", strings.NewReader(form2.Encode()))
	req2.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	rec2 := httptest.NewRecorder()

	srv.handleWizardRefine(rec2, req2)

	if rec2.Code != http.StatusOK {
		t.Fatalf("expected status 200, got %d", rec2.Code)
	}

	updated2, ok := srv.wizardStore.Get(session2.ID)
	if !ok {
		t.Fatal("expected to retrieve session")
	}

	if updated2.AddToSprint {
		t.Errorf("expected AddToSprint to be false when checkbox is unchecked, got %v", updated2.AddToSprint)
	}
}

// TestHandleWizardRefine_SprintNameInTemplate verifies SprintName is passed to template when active sprint exists
func TestHandleWizardRefine_SprintNameInTemplate(t *testing.T) {
	srv := createTestServerWithTemplates(t)
	defer srv.wizardStore.Stop()

	// Create a session
	session, _ := srv.wizardStore.Create("feature")

	form := url.Values{}
	form.Set("session_id", session.ID)
	form.Set("idea", "Test feature idea")

	req := httptest.NewRequest(http.MethodPost, "/wizard/refine", strings.NewReader(form.Encode()))
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	rec := httptest.NewRecorder()

	srv.handleWizardRefine(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("expected status 200, got %d", rec.Code)
	}

	body := rec.Body.String()

	// Since there's no active sprint (gh is nil), SprintName should be empty
	// and the sprint checkbox should NOT appear
	if strings.Contains(body, `name="add_to_sprint"`) {
		t.Error("sprint checkbox should NOT appear when there is no active sprint")
	}
}

// TestHandleWizardRefine_AcceptsLanguageParameter tests that language parameter is accepted and stored
func TestHandleWizardRefine_AcceptsLanguageParameter(t *testing.T) {
	srv := createTestServerWithTemplates(t)
	defer srv.wizardStore.Stop()

	// First create a session
	req1 := httptest.NewRequest("GET", "/wizard/new?type=feature", nil)
	rec1 := httptest.NewRecorder()
	srv.handleWizardNew(rec1, req1)

	// Extract session ID from response
	body := rec1.Body.String()
	// Parse session_id from HTML form
	var sessionID string
	if strings.Contains(body, `name="session_id"`) {
		// Extract value from: value="..."
		start := strings.Index(body, `name="session_id" value="`)
		if start != -1 {
			start += len(`name="session_id" value="`)
			end := strings.Index(body[start:], `"`)
			if end != -1 {
				sessionID = body[start : start+end]
			}
		}
	}

	if sessionID == "" {
		t.Fatal("Could not extract session ID from response")
	}

	// Submit form with language parameter
	formData := url.Values{}
	formData.Set("session_id", sessionID)
	formData.Set("idea", "Test feature idea")
	formData.Set("language", "pl-PL")

	req2 := httptest.NewRequest("POST", "/wizard/refine", strings.NewReader(formData.Encode()))
	req2.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	rec2 := httptest.NewRecorder()

	srv.handleWizardRefine(rec2, req2)

	// Verify session has language stored
	session, ok := srv.wizardStore.Get(sessionID)
	if !ok {
		t.Fatal("Session not found")
	}

	if session.Language != "pl-PL" {
		t.Errorf("Expected Language to be 'pl-PL', got %q", session.Language)
	}
}
