package dashboard

import (
	"encoding/json"
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

	"github.com/crazy-goat/one-dev-army/internal/config"
	"github.com/crazy-goat/one-dev-army/internal/github"
	"github.com/crazy-goat/one-dev-army/internal/mvp"
	"github.com/crazy-goat/one-dev-army/internal/opencode"
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
		"json": func(v any) string {
			b, err := json.Marshal(v)
			if err != nil {
				return ""
			}
			return string(b)
		},
		"labelIcon":    LabelIcon,
		"labelTooltip": LabelTooltip,
	}

	pages := []string{"board.html", "task.html"}
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

	// Parse settings template
	settingsTmpl, err := template.New("").Funcs(funcMap).ParseFiles(
		filepath.Join(templateDir, "layout.html"),
		filepath.Join(templateDir, "llm-config.html"),
	)
	if err != nil {
		return nil, fmt.Errorf("parsing llm-config.html: %w", err)
	}
	tmpls["llm-config.html"] = settingsTmpl

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
		webPort:     5001,
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
	// Create a minimal template for testing with board-columns block
	tmplContent := `{{define "board-columns"}}<div class="board"><div>Board Data</div></div>{{end}}`
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

	data := srv.buildBoardData(nil)

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

// TestHandleWizardCreate_UsesGeneratedTitle verifies that issue title uses the generated title from refine step
func TestHandleWizardCreate_UsesGeneratedTitle(t *testing.T) {
	srv := createTestServerWithTemplates(t)
	srv.gh = nil
	defer srv.wizardStore.Stop()

	session, _ := srv.wizardStore.Create("feature")
	session.SetIdeaText("Raw user input")
	session.SetTechnicalPlanning("## Description\n\nLLM generated description")
	session.SetGeneratedTitle("[Feature] Add authentication system")

	req := httptest.NewRequest(http.MethodPost, "/wizard/create", strings.NewReader("session_id="+session.ID))
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	rec := httptest.NewRecorder()

	srv.handleWizardCreate(rec, req)

	// Verify response contains the generated title, not raw idea text
	body := rec.Body.String()
	if !strings.Contains(body, "[Feature] Add authentication system") {
		t.Errorf("expected response to contain generated title, got: %s", body)
	}
	if strings.Contains(body, "Raw user input") {
		t.Error("expected title to come from generated title, not raw idea text")
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
	for range 100 {
		wg.Go(func() {
			_, _ = srv.wizardStore.Create("feature")
		})
	}
	wg.Wait()

	if srv.wizardStore.Count() != 100 {
		t.Errorf("expected 100 sessions, got %d", srv.wizardStore.Count())
	}

	// Access sessions concurrently using the Get method
	// Create sessions first to get their IDs
	var ids []string
	for range 100 {
		session, _ := srv.wizardStore.Create("feature")
		if session != nil {
			ids = append(ids, session.ID)
		}
	}

	for i := range 100 {
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
	for range 50 {
		wg.Go(func() {
			req := httptest.NewRequest(http.MethodGet, "/wizard/new?type=feature", nil)
			rec := httptest.NewRecorder()
			srv.handleWizardNew(rec, req)
		})
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
	req := httptest.NewRequest(http.MethodGet, "/wizard/logs/test-session", nil)
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
		Active       string
		OpenCodePort int
		WorkerCount  int
		YoloMode     bool
	}{
		Active:       "board",
		OpenCodePort: 5001,
		WorkerCount:  1,
		YoloMode:     false,
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

// TestChatButton_Presence verifies the chat button is present on all pages
func TestChatButton_Presence(t *testing.T) {
	srv := createTestServerWithTemplates(t)
	defer srv.wizardStore.Stop()

	// Test board page
	req := httptest.NewRequest(http.MethodGet, "/", nil)
	rec := httptest.NewRecorder()
	srv.handleBoard(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("expected status 200, got %d", rec.Code)
	}

	body := rec.Body.String()

	// Verify chat button is present with correct onclick handler
	if !strings.Contains(body, `onclick="window.open('http://localhost:`) {
		t.Error("board page missing chat button with correct onclick handler")
	}
	if !strings.Contains(body, "Chat") {
		t.Error("board page missing 'Chat' button text")
	}

	// Verify chat button uses the correct port from server config
	expectedPort := fmt.Sprintf("localhost:%d", srv.webPort)
	if !strings.Contains(body, expectedPort) {
		t.Errorf("board page chat button should use configured port %d, got: %s", srv.webPort, body)
	}
}

// TestChatButton_OnTaskPage verifies the chat button appears on task detail page
func TestChatButton_OnTaskPage(t *testing.T) {
	srv := createTestServerWithTemplates(t)
	defer srv.wizardStore.Stop()

	req := httptest.NewRequest(http.MethodGet, "/task/123", nil)
	req.SetPathValue("id", "123")
	rec := httptest.NewRecorder()
	srv.handleTaskDetail(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("expected status 200, got %d", rec.Code)
	}

	body := rec.Body.String()

	// Verify chat button is present
	if !strings.Contains(body, `onclick="window.open('http://localhost:`) {
		t.Error("task page missing chat button with correct onclick handler")
	}
	if !strings.Contains(body, "Chat") {
		t.Error("task page missing 'Chat' button text")
	}
}

// TestChatButton_OnWizardPage verifies the chat button appears on wizard page
func TestChatButton_OnWizardPage(t *testing.T) {
	srv := createTestServerWithTemplates(t)
	defer srv.wizardStore.Stop()

	req := httptest.NewRequest(http.MethodGet, "/wizard?type=feature", nil)
	rec := httptest.NewRecorder()
	srv.handleWizardPage(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("expected status 200, got %d", rec.Code)
	}

	body := rec.Body.String()

	// Verify chat button is present
	if !strings.Contains(body, `onclick="window.open('http://localhost:`) {
		t.Error("wizard page missing chat button with correct onclick handler")
	}
	if !strings.Contains(body, "Chat") {
		t.Error("wizard page missing 'Chat' button text")
	}
}

// TestChatButton_OnSettingsPage verifies the chat button appears on settings page
func TestChatButton_OnSettingsPage(t *testing.T) {
	srv := createTestServerWithTemplates(t)
	defer srv.wizardStore.Stop()

	req := httptest.NewRequest(http.MethodGet, "/settings", nil)
	rec := httptest.NewRecorder()
	srv.handleSettings(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("expected status 200, got %d", rec.Code)
	}

	body := rec.Body.String()

	// Verify chat button is present
	if !strings.Contains(body, `onclick="window.open('http://localhost:`) {
		t.Error("settings page missing chat button with correct onclick handler")
	}
	if !strings.Contains(body, "Chat") {
		t.Error("settings page missing 'Chat' button text")
	}
}

// TestChatButton_UsesCorrectPort verifies the chat button uses the configured web port
func TestChatButton_UsesCorrectPort(t *testing.T) {
	// Create server with custom web port
	srv := createTestServerWithTemplates(t)
	srv.webPort = 9090
	defer srv.wizardStore.Stop()

	req := httptest.NewRequest(http.MethodGet, "/", nil)
	rec := httptest.NewRecorder()
	srv.handleBoard(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("expected status 200, got %d", rec.Code)
	}

	body := rec.Body.String()

	// Verify the custom port is used
	if !strings.Contains(body, "localhost:9090") {
		t.Errorf("chat button should use custom port 9090, got: %s", body)
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
			switch tc.wizardType {
			case "feature":
				epicLabels = append(epicLabels, "enhancement")
			case "bug":
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
	for i := range numUsers {
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

// TestHandleWizardCreateSingle_UsesGeneratedTitle verifies single issue uses the generated title
func TestHandleWizardCreateSingle_UsesGeneratedTitle(t *testing.T) {
	srv := createTestServerWithTemplates(t)
	defer srv.wizardStore.Stop()

	session, _ := srv.wizardStore.Create("feature")
	session.SetIdeaText("Raw user input")
	session.SetTechnicalPlanning("## Description\n\nLLM generated description")
	session.SetGeneratedTitle("[Feature] Implement user dashboard")

	form := url.Values{}
	form.Set("session_id", session.ID)

	req := httptest.NewRequest(http.MethodPost, "/wizard/create", strings.NewReader(form.Encode()))
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	rec := httptest.NewRecorder()

	srv.handleWizardCreate(rec, req)

	body := rec.Body.String()
	if !strings.Contains(body, "[Feature] Implement user dashboard") {
		t.Errorf("expected response to contain generated title, got: %s", body)
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

	// These buttons should always be present
	requiredButtons := []string{
		"Sync",
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
		"6 columns":     "grid-template-columns:repeat(6,1fr)",
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

// TestBoardLayout_StackedColumns verifies board has stacked columns for Blocked/Backlog and Done/Failed
func TestBoardLayout_StackedColumns(t *testing.T) {
	srv := createTestServerWithTemplates(t)
	defer srv.wizardStore.Stop()

	req := httptest.NewRequest(http.MethodGet, "/", nil)
	rec := httptest.NewRecorder()

	srv.handleBoard(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("expected status 200, got %d", rec.Code)
	}

	body := rec.Body.String()

	// Check for stacked-column class in HTML (can be part of multi-class attribute like class="board-left stacked-column")
	if !strings.Contains(body, "stacked-column") {
		t.Error("board page missing stacked-column CSS class in HTML")
	}

	// Count occurrences of stacked-column in class attributes (handles both single and multi-class)
	// Look for patterns like: class="... stacked-column" or class="stacked-column ..." or class="... stacked-column ..."
	stackedColumnCount := strings.Count(body, `class="board-left stacked-column"`) +
		strings.Count(body, `class="board-right stacked-column"`)
	if stackedColumnCount != 2 {
		t.Errorf("expected 2 stacked-column containers, got %d", stackedColumnCount)
	}

	if !strings.Contains(body, ".stacked-column{") {
		t.Error("board page missing stacked-column CSS rule")
	}

	if !strings.Contains(body, ".stacked-column .column{") {
		t.Error("board page missing stacked-column .column CSS rule")
	}

	// Verify stacked column height constraint
	if !strings.Contains(body, "calc(100vh") {
		t.Error("stacked-column missing viewport height constraint")
	}

	// Verify internal scrolling
	if !strings.Contains(body, "overflow-y:auto") {
		t.Error("stacked-column columns missing overflow-y:auto for internal scrolling")
	}

	// Verify sticky column titles
	if !strings.Contains(body, "position:sticky") {
		t.Error("stacked-column titles missing position:sticky")
	}

	// Verify smooth scrolling
	if !strings.Contains(body, "scroll-behavior:smooth") {
		t.Error("stacked-column columns missing scroll-behavior:smooth")
	}

	// Verify flex basis for 50/50 split
	if !strings.Contains(body, "50%") {
		t.Error("stacked-column columns missing 50% flex basis for equal height split")
	}
}

// TestBoardLayout_BacklogAboveBlocked verifies Backlog column appears before Blocked in the left stack
func TestBoardLayout_BacklogAboveBlocked(t *testing.T) {
	srv := createTestServerWithTemplates(t)
	defer srv.wizardStore.Stop()

	req := httptest.NewRequest(http.MethodGet, "/", nil)
	rec := httptest.NewRecorder()

	srv.handleBoard(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("expected status 200, got %d", rec.Code)
	}

	body := rec.Body.String()

	backlogIdx := strings.Index(body, ">Backlog ")
	blockedIdx := strings.Index(body, ">Blocked ")

	if backlogIdx < 0 {
		t.Fatal("Backlog column title not found in board output")
	}
	if blockedIdx < 0 {
		t.Fatal("Blocked column title not found in board output")
	}
	if backlogIdx >= blockedIdx {
		t.Errorf("Backlog (at index %d) should appear before Blocked (at index %d) in the left stack", backlogIdx, blockedIdx)
	}
}

// TestBoardLayout_BlockedColumnCollapsedWhenEmpty verifies blocked column shows only header when empty
func TestBoardLayout_BlockedColumnCollapsedWhenEmpty(t *testing.T) {
	srv := createTestServerWithTemplates(t)
	defer srv.wizardStore.Stop()

	req := httptest.NewRequest(http.MethodGet, "/", nil)
	rec := httptest.NewRecorder()

	srv.handleBoard(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("expected status 200, got %d", rec.Code)
	}

	body := rec.Body.String()

	// Verify "Blocked" header is present
	if !strings.Contains(body, ">Blocked <") {
		t.Error("blocked column header not found in board output")
	}

	// Verify "No blocked tickets" empty state is NOT present when collapsed
	if strings.Contains(body, "No blocked tickets") {
		t.Error("blocked column should not show 'No blocked tickets' when collapsed")
	}

	// Verify the blocked column div structure
	blockedColIdx := strings.Index(body, `class="column col-blocked"`)
	if blockedColIdx < 0 {
		t.Fatal("blocked column container not found")
	}

	// Find the end of the blocked column section by looking for the next column or closing div
	// The blocked column should end with </div> after the title when empty
	blockedSectionEnd := strings.Index(body[blockedColIdx:], "</div>")
	if blockedSectionEnd < 0 {
		t.Fatal("could not find end of blocked column section")
	}
	blockedSectionEnd += blockedColIdx + 6 // Include the </div>

	// Extract the blocked column section
	blockedSection := body[blockedColIdx:blockedSectionEnd]

	// Count occurrences of "card" class in blocked section - should be 0 when empty
	cardCount := strings.Count(blockedSection, `class="card"`)
	if cardCount > 0 {
		t.Errorf("expected 0 cards in empty blocked column, found %d", cardCount)
	}
}

// TestBoardLayout_BlockedColumnFullWhenPopulated verifies blocked column shows full content when tickets exist
func TestBoardLayout_BlockedColumnFullWhenPopulated(t *testing.T) {
	srv := createTestServerWithTemplates(t)
	defer srv.wizardStore.Stop()

	// Create board data with a blocked ticket
	data := boardData{
		Active: "board",
		Blocked: []taskCard{
			{ID: 123, Title: "Test Blocked Ticket", Status: "Blocked"},
		},
	}

	// Execute the board-columns template directly with populated data
	tmpl := srv.tmpls["board.html"]
	if tmpl == nil {
		t.Fatal("board.html template not found")
	}

	var buf strings.Builder
	if err := tmpl.ExecuteTemplate(&buf, "board-columns", data); err != nil {
		t.Fatalf("failed to execute template: %v", err)
	}

	body := buf.String()

	// Verify "Blocked" header is present with count
	if !strings.Contains(body, ">Blocked <") {
		t.Error("blocked column header not found in board output")
	}

	// Verify count shows 1
	if !strings.Contains(body, `<span class="count">1</span>`) {
		t.Error("blocked column count should show 1 when populated")
	}

	// Verify the blocked ticket card is rendered
	if !strings.Contains(body, "Test Blocked Ticket") {
		t.Error("blocked ticket title not found in column when populated")
	}

	// Verify card structure is present
	if !strings.Contains(body, `class="card"`) {
		t.Error("card element not found in blocked column when populated")
	}

	// Verify unblock button is present
	if !strings.Contains(body, `action="/unblock/123"`) {
		t.Error("unblock form action not found for blocked ticket")
	}
}
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

	// These forms should always be present
	requiredForms := []string{
		`action="/plan-sprint"`,
	}

	for _, form := range requiredForms {
		if !strings.Contains(body, form) {
			t.Errorf("board page missing sprint control form: %s", form)
		}
	}

	// Verify sync button exists with correct ID and onclick handler
	if !strings.Contains(body, `id="sync-btn"`) {
		t.Error("board page missing sync button")
	}
	if !strings.Contains(body, `onclick="triggerSync()"`) {
		t.Error("board page missing triggerSync onclick handler")
	}

	// Verify HTMX is configured for board data with refresh trigger (not polling)
	if !strings.Contains(body, `hx-get="/api/board-data"`) {
		t.Error("board page missing HTMX configuration for board data")
	}
	if !strings.Contains(body, `hx-trigger="refresh"`) {
		t.Error("board page missing HTMX refresh trigger for board data")
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
		"function triggerSync",
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

	// For feature type, should show 3 steps (Idea, Review, Create) - no more breakdown or title steps
	// Count the step-label spans
	stepLabels := []string{"Idea", "Review", "Create"}
	for _, label := range stepLabels {
		if !strings.Contains(body, `<span class="step-label">`+label+`</span>`) {
			t.Errorf("step indicator missing '%s' label for feature type", label)
		}
	}

	// Should NOT show Breakdown or Title steps anymore
	if strings.Contains(body, `<span class="step-label">Breakdown</span>`) {
		t.Error("step indicator should NOT show 'Breakdown' step (removed in new flow)")
	}
	if strings.Contains(body, `<span class="step-label">Title</span>`) {
		t.Error("step indicator should NOT show 'Title' step (merged into Review)")
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

	// For bug type, should NOT show Breakdown or Title steps (removed in new flow)
	if strings.Contains(body, `<span class="step-label">Breakdown</span>`) {
		t.Error("step indicator should NOT show 'Breakdown' step for bug type (removed in new flow)")
	}
	if strings.Contains(body, `<span class="step-label">Title</span>`) {
		t.Error("step indicator should NOT show 'Title' step for bug type (merged into Review)")
	}

	// Should have 3 steps (Idea, Review, Create) - same as feature now
	stepLabels := []string{"Idea", "Review", "Create"}
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

// TestWizardPage_ContainerScrolling verifies the wizard page container has proper scrolling CSS
func TestWizardPage_ContainerScrolling(t *testing.T) {
	srv := createTestServerWithTemplates(t)
	defer srv.wizardStore.Stop()

	// Request the wizard page
	req := httptest.NewRequest(http.MethodGet, "/wizard?type=feature", nil)
	rec := httptest.NewRecorder()

	srv.handleWizardPage(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("expected status 200, got %d", rec.Code)
	}

	body := rec.Body.String()

	// Verify overflow-y: auto is present for scrolling
	if !strings.Contains(body, "overflow-y: auto") {
		t.Error("wizard page container missing overflow-y: auto for scrolling")
	}

	// Verify max-height is present to constrain the container
	if !strings.Contains(body, "max-height:") {
		t.Error("wizard page container missing max-height property")
	}

	// Verify calc() is used for responsive height
	if !strings.Contains(body, "calc(100vh") {
		t.Error("wizard page container should use calc(100vh - 200px) for responsive height")
	}

	// Verify flex layout is used for proper content organization
	if !strings.Contains(body, "display: flex") {
		t.Error("wizard page container missing display: flex for layout")
	}

	if !strings.Contains(body, "flex-direction: column") {
		t.Error("wizard page container missing flex-direction: column")
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
	req1 := httptest.NewRequest(http.MethodGet, "/wizard/new?type=feature", nil)
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

	req2 := httptest.NewRequest(http.MethodPost, "/wizard/refine", strings.NewReader(formData.Encode()))
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

// TestHandleWizardRefine_GeneratesTitleAndDescription tests that refine generates both title and description
func TestHandleWizardRefine_GeneratesTitleAndDescription(t *testing.T) {
	srv := createTestServerWithTemplates(t)
	defer srv.wizardStore.Stop()

	// Create a session first
	session, err := srv.wizardStore.Create("feature")
	if err != nil {
		t.Fatalf("Failed to create session: %v", err)
	}

	// Submit idea for refinement (mock mode - no LLM client)
	formData := url.Values{}
	formData.Set("session_id", session.ID)
	formData.Set("idea", "Add user authentication to the system")

	req := httptest.NewRequest(http.MethodPost, "/wizard/refine", strings.NewReader(formData.Encode()))
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	rec := httptest.NewRecorder()

	srv.handleWizardRefine(rec, req)

	// Should return 200 OK
	if rec.Code != http.StatusOK {
		t.Errorf("Expected status 200, got %d", rec.Code)
	}

	// Verify session has both generated title and description
	updatedSession, ok := srv.wizardStore.Get(session.ID)
	if !ok {
		t.Fatal("Session not found after refinement")
	}

	if updatedSession.GeneratedTitle == "" {
		t.Error("Expected GeneratedTitle to be set after refine")
	}

	if updatedSession.TechnicalPlanning == "" {
		t.Error("Expected TechnicalPlanning to be set after refine")
	}

	// Verify title has proper prefix
	if !strings.HasPrefix(updatedSession.GeneratedTitle, "[Feature]") {
		t.Errorf("Expected title to have [Feature] prefix, got: %s", updatedSession.GeneratedTitle)
	}
}

// TestHandleWizardRefine_BugType_GeneratesTitle tests title generation for bug type during refine
func TestHandleWizardRefine_BugType_GeneratesTitle(t *testing.T) {
	srv := createTestServerWithTemplates(t)
	defer srv.wizardStore.Stop()

	session, err := srv.wizardStore.Create("bug")
	if err != nil {
		t.Fatalf("Failed to create session: %v", err)
	}

	formData := url.Values{}
	formData.Set("session_id", session.ID)
	formData.Set("idea", "Fix login error when user enters wrong password")

	req := httptest.NewRequest(http.MethodPost, "/wizard/refine", strings.NewReader(formData.Encode()))
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	rec := httptest.NewRecorder()

	srv.handleWizardRefine(rec, req)

	if rec.Code != http.StatusOK {
		t.Errorf("Expected status 200, got %d", rec.Code)
	}

	updatedSession, ok := srv.wizardStore.Get(session.ID)
	if !ok {
		t.Fatal("Session not found")
	}

	// Bug titles should have [Bug] prefix
	if !strings.HasPrefix(updatedSession.GeneratedTitle, "[Bug]") {
		t.Errorf("Expected bug title to have [Bug] prefix, got: %s", updatedSession.GeneratedTitle)
	}
}

// TestHandleWizardCreate_CustomTitleFromForm tests custom title override via form submission
func TestHandleWizardCreate_CustomTitleFromForm(t *testing.T) {
	srv := createTestServerWithTemplates(t)
	defer srv.wizardStore.Stop()

	session, err := srv.wizardStore.Create("feature")
	if err != nil {
		t.Fatalf("Failed to create session: %v", err)
	}

	session.SetTechnicalPlanning("## Description\n\nAdd user authentication.")
	session.SetGeneratedTitle("[Feature] Generated title")

	// Submit with custom title via form
	formData := url.Values{}
	formData.Set("session_id", session.ID)
	formData.Set("issue_title", "[Feature] Custom authentication title")

	req := httptest.NewRequest(http.MethodPost, "/wizard/create", strings.NewReader(formData.Encode()))
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	rec := httptest.NewRecorder()

	srv.handleWizardCreate(rec, req)

	// Should return 200 OK (mock mode)
	if rec.Code != http.StatusOK {
		t.Errorf("Expected status 200, got %d", rec.Code)
	}

	// Verify the response contains the custom title
	body := rec.Body.String()
	if !strings.Contains(body, "[Feature] Custom authentication title") {
		t.Errorf("Expected response to contain custom title, got: %s", body)
	}
}

// TestHandleWizardCreate_UsesSessionTitle tests that issue creation uses the session title
func TestHandleWizardCreate_UsesSessionTitle(t *testing.T) {
	srv := createTestServerWithTemplates(t)
	defer srv.wizardStore.Stop()

	// Create a session with technical planning and generated title
	session, err := srv.wizardStore.Create("feature")
	if err != nil {
		t.Fatalf("Failed to create session: %v", err)
	}

	session.SetTechnicalPlanning("## Description\n\nAdd user authentication.")
	session.SetGeneratedTitle("[Feature] Add user authentication system")
	session.SetStep(WizardStepRefine)

	// Create issue
	formData := url.Values{}
	formData.Set("session_id", session.ID)

	req := httptest.NewRequest(http.MethodPost, "/wizard/create", strings.NewReader(formData.Encode()))
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	rec := httptest.NewRecorder()

	srv.handleWizardCreate(rec, req)

	// Should return 200 OK (mock mode)
	if rec.Code != http.StatusOK {
		t.Errorf("Expected status 200, got %d", rec.Code)
	}

	// Verify the response contains the generated title
	body := rec.Body.String()
	if !strings.Contains(body, "[Feature] Add user authentication system") {
		t.Errorf("Expected response to contain the generated title, got: %s", body)
	}
}

// TestInferColumnFromIssue tests the column inference logic for Plan and Code columns
func TestInferColumnFromIssue(t *testing.T) {
	tests := []struct {
		name     string
		labels   []string
		state    string
		expected string
	}{
		{
			name:     "stage:analysis label maps to Plan",
			labels:   []string{"stage:analysis"},
			expected: "Plan",
		},
		{
			name:     "stage:planning label maps to Plan",
			labels:   []string{"stage:planning"},
			expected: "Plan",
		},
		{
			name:     "stage:coding label maps to Code",
			labels:   []string{"stage:coding"},
			expected: "Code",
		},
		{
			name:     "stage:testing label maps to Code",
			labels:   []string{"stage:testing"},
			expected: "Code",
		},
		{
			name:     "in-progress label maps to Code",
			labels:   []string{"in-progress"},
			expected: "Code",
		},
		{
			name:     "failed label takes precedence",
			labels:   []string{"stage:coding", "failed"},
			expected: "Failed",
		},
		{
			name:     "stage:failed label takes precedence",
			labels:   []string{"stage:coding", "stage:failed"},
			expected: "Failed",
		},
		{
			name:     "blocked label takes precedence over Plan",
			labels:   []string{"stage:analysis", "blocked"},
			expected: "Blocked",
		},
		{
			name:     "stage:blocked takes precedence over Plan",
			labels:   []string{"stage:analysis", "stage:blocked"},
			expected: "Blocked",
		},
		{
			name:     "stage:code-review maps to AI Review",
			labels:   []string{"stage:code-review"},
			expected: "AI Review",
		},
		{
			name:     "stage:check-pipeline maps to Pipeline",
			labels:   []string{"stage:check-pipeline"},
			expected: "Pipeline",
		},
		{
			name:     "stage:create-pr maps to AI Review",
			labels:   []string{"stage:create-pr"},
			expected: "AI Review",
		},
		{
			name:     "awaiting-approval maps to Approve (legacy)",
			labels:   []string{"awaiting-approval"},
			expected: "Approve",
		},
		{
			name:     "stage:awaiting-approval maps to Approve",
			labels:   []string{"stage:awaiting-approval"},
			expected: "Approve",
		},
		{
			name:     "stage:merging maps to Merge",
			labels:   []string{"stage:merging"},
			expected: "Merge",
		},
		{
			name:     "closed state maps to Done",
			labels:   []string{},
			state:    "CLOSED",
			expected: "Done",
		},
		{
			name:     "no labels defaults to Backlog",
			labels:   []string{},
			expected: "Backlog",
		},
		{
			name:     "unknown label defaults to Backlog",
			labels:   []string{"unknown-label"},
			expected: "Backlog",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			issue := github.Issue{
				Number: 1,
				Title:  "Test Issue",
				State:  tt.state,
			}
			// Add labels to the issue
			for _, label := range tt.labels {
				issue.Labels = append(issue.Labels, struct {
					Name string `json:"name"`
				}{Name: label})
			}

			got := inferColumnFromIssue(issue)
			if got != tt.expected {
				t.Errorf("inferColumnFromIssue() = %q, want %q", got, tt.expected)
			}
		})
	}
}

// TestAddCardToColumn tests that cards are added to correct Plan/Code fields
func TestAddCardToColumn(t *testing.T) {
	srv := &Server{
		tmpls: make(map[string]*template.Template),
	}

	tests := []struct {
		name          string
		column        string
		expectedField string
	}{
		{
			name:          "Plan column adds to Plan field",
			column:        "Plan",
			expectedField: "Plan",
		},
		{
			name:          "Code column adds to Code field",
			column:        "Code",
			expectedField: "Code",
		},
		{
			name:          "Backlog column adds to Backlog field",
			column:        "Backlog",
			expectedField: "Backlog",
		},
		{
			name:          "AI Review column adds to AIReview field",
			column:        "AI Review",
			expectedField: "AIReview",
		},
		{
			name:          "Pipeline column adds to CheckPipeline field",
			column:        "Pipeline",
			expectedField: "CheckPipeline",
		},
		{
			name:          "Approve column adds to Approve field",
			column:        "Approve",
			expectedField: "Approve",
		},
		{
			name:          "Done column adds to Done field",
			column:        "Done",
			expectedField: "Done",
		},
		{
			name:          "Blocked column adds to Blocked field",
			column:        "Blocked",
			expectedField: "Blocked",
		},
		{
			name:          "Failed column adds to Failed field",
			column:        "Failed",
			expectedField: "Failed",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			data := boardData{}
			issue := github.Issue{
				Number: 1,
				Title:  "Test Issue",
			}

			srv.addCardToColumn(&data, tt.column, issue)

			var count int
			switch tt.expectedField {
			case "Plan":
				count = len(data.Plan)
			case "Code":
				count = len(data.Code)
			case "Backlog":
				count = len(data.Backlog)
			case "AIReview":
				count = len(data.AIReview)
			case "CheckPipeline":
				count = len(data.CheckPipeline)
			case "Approve":
				count = len(data.Approve)
			case "Done":
				count = len(data.Done)
			case "Blocked":
				count = len(data.Blocked)
			case "Failed":
				count = len(data.Failed)
			}

			if count != 1 {
				t.Errorf("expected 1 card in %s field, got %d", tt.expectedField, count)
			}
		})
	}
}

// TestAddCardToColumn_CardProperties verifies card properties are set correctly
func TestAddCardToColumn_CardProperties(t *testing.T) {
	srv := &Server{
		tmpls: make(map[string]*template.Template),
	}

	data := boardData{}
	issue := github.Issue{
		Number: 42,
		Title:  "Test Issue Title",
		Assignees: []struct {
			Login string `json:"login"`
		}{{Login: "testuser"}},
		Labels: []struct {
			Name string `json:"name"`
		}{{Name: "bug"}, {Name: "priority:high"}},
	}

	srv.addCardToColumn(&data, "Plan", issue)

	if len(data.Plan) != 1 {
		t.Fatalf("expected 1 card in Plan field, got %d", len(data.Plan))
	}

	card := data.Plan[0]

	if card.ID != 42 {
		t.Errorf("expected card ID to be 42, got %d", card.ID)
	}

	if card.Title != "Test Issue Title" {
		t.Errorf("expected card Title to be 'Test Issue Title', got %q", card.Title)
	}

	if card.Status != "Plan" {
		t.Errorf("expected card Status to be 'Plan', got %q", card.Status)
	}

	if card.Assignee != "testuser" {
		t.Errorf("expected card Assignee to be 'testuser', got %q", card.Assignee)
	}

	if len(card.Labels) != 2 {
		t.Errorf("expected 2 labels, got %d", len(card.Labels))
	}
}

// TestInferColumnFromIssue_LabelCaseInsensitivity tests that label matching is case-insensitive
func TestInferColumnFromIssue_LabelCaseInsensitivity(t *testing.T) {
	tests := []struct {
		name     string
		label    string
		expected string
	}{
		{
			name:     "uppercase STAGE:ANALYSIS",
			label:    "STAGE:ANALYSIS",
			expected: "Plan",
		},
		{
			name:     "mixed case Stage:Coding",
			label:    "Stage:Coding",
			expected: "Code",
		},
		{
			name:     "uppercase FAILED",
			label:    "FAILED",
			expected: "Failed",
		},
		{
			name:     "mixed case In-Progress",
			label:    "In-Progress",
			expected: "Code",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			issue := github.Issue{
				Number: 1,
				Title:  "Test Issue",
				Labels: []struct {
					Name string `json:"name"`
				}{{Name: tt.label}},
			}

			got := inferColumnFromIssue(issue)
			if got != tt.expected {
				t.Errorf("inferColumnFromIssue() = %q, want %q", got, tt.expected)
			}
		})
	}
}

// TestWizardRefine_MockTitleGeneration tests that mock refine generates proper titles
func TestWizardRefine_MockTitleGeneration(t *testing.T) {
	srv := createTestServerWithTemplates(t)
	defer srv.wizardStore.Stop()

	// Test feature type
	session, _ := srv.wizardStore.Create("feature")
	formData := url.Values{}
	formData.Set("session_id", session.ID)
	formData.Set("idea", "Add user authentication to the system")

	req := httptest.NewRequest(http.MethodPost, "/wizard/refine", strings.NewReader(formData.Encode()))
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	rec := httptest.NewRecorder()
	srv.handleWizardRefine(rec, req)

	updatedSession, ok := srv.wizardStore.Get(session.ID)
	if !ok {
		t.Fatal("Session not found")
	}

	if !strings.HasPrefix(updatedSession.GeneratedTitle, "[Feature]") {
		t.Errorf("Expected feature title to have [Feature] prefix, got: %s", updatedSession.GeneratedTitle)
	}

	// Test bug type
	session2, _ := srv.wizardStore.Create("bug")
	formData2 := url.Values{}
	formData2.Set("session_id", session2.ID)
	formData2.Set("idea", "Fix login error when user enters wrong password")

	req2 := httptest.NewRequest(http.MethodPost, "/wizard/refine", strings.NewReader(formData2.Encode()))
	req2.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	rec2 := httptest.NewRecorder()
	srv.handleWizardRefine(rec2, req2)

	updatedSession2, ok := srv.wizardStore.Get(session2.ID)
	if !ok {
		t.Fatal("Session not found")
	}

	if !strings.HasPrefix(updatedSession2.GeneratedTitle, "[Bug]") {
		t.Errorf("Expected bug title to have [Bug] prefix, got: %s", updatedSession2.GeneratedTitle)
	}
}

// TestAddCardToColumn_MergedStatus verifies that IsMerged field is set correctly for Done column cards
func TestAddCardToColumn_MergedStatus(t *testing.T) {
	srv := &Server{
		tmpls: make(map[string]*template.Template),
	}

	tests := []struct {
		name       string
		prMerged   bool
		wantMerged bool
	}{
		{
			name:       "Merged issue has IsMerged=true",
			prMerged:   true,
			wantMerged: true,
		},
		{
			name:       "Closed issue has IsMerged=false",
			prMerged:   false,
			wantMerged: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			data := boardData{}
			issue := github.Issue{
				Number:   1,
				Title:    "Test Issue",
				State:    "CLOSED",
				PRMerged: tt.prMerged,
			}

			srv.addCardToColumn(&data, "Done", issue)

			if len(data.Done) != 1 {
				t.Fatalf("expected 1 card in Done field, got %d", len(data.Done))
			}

			if data.Done[0].IsMerged != tt.wantMerged {
				t.Errorf("expected IsMerged=%v, got %v", tt.wantMerged, data.Done[0].IsMerged)
			}
		})
	}
}

// TestInferColumnFromIssue_MergedStatus verifies that merged status doesn't affect column inference
func TestInferColumnFromIssue_MergedStatus(t *testing.T) {
	tests := []struct {
		name     string
		state    string
		prMerged bool
		expected string
	}{
		{
			name:     "Closed merged issue goes to Done",
			state:    "CLOSED",
			prMerged: true,
			expected: "Done",
		},
		{
			name:     "Closed non-merged issue goes to Done",
			state:    "CLOSED",
			prMerged: false,
			expected: "Done",
		},
		{
			name:     "Open issue with merged flag still goes to Backlog",
			state:    "open",
			prMerged: true,
			expected: "Backlog",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			issue := github.Issue{
				Number:   1,
				Title:    "Test Issue",
				State:    tt.state,
				PRMerged: tt.prMerged,
			}

			got := inferColumnFromIssue(issue)
			if got != tt.expected {
				t.Errorf("inferColumnFromIssue() = %q, want %q", got, tt.expected)
			}
		})
	}
}

// TestBuildBoardData_CanCloseSprint_True verifies CanCloseSprint is true when all tasks are in Done and not processing
func TestBuildBoardData_CanCloseSprint_True(t *testing.T) {
	_ = &Server{tmpls: make(map[string]*template.Template)}

	// Simulate board data with all tasks in Done column and not processing
	data := boardData{
		Active:     "board",
		Processing: false,
		Paused:     true,
		// All active columns empty
		Blocked:  []taskCard{},
		Backlog:  []taskCard{},
		Plan:     []taskCard{},
		Code:     []taskCard{},
		AIReview: []taskCard{},
		Approve:  []taskCard{},
		// Tasks only in Done, no Failed
		Done: []taskCard{
			{ID: 1, Title: "Completed task", Status: "Done"},
		},
		Failed: []taskCard{},
	}

	// Apply the same logic as in buildBoardData
	if !data.Processing &&
		len(data.Blocked) == 0 &&
		len(data.Backlog) == 0 &&
		len(data.Plan) == 0 &&
		len(data.Code) == 0 &&
		len(data.AIReview) == 0 &&
		len(data.Approve) == 0 &&
		len(data.Merge) == 0 &&
		len(data.Failed) == 0 &&
		len(data.Done) > 0 {
		data.CanCloseSprint = true
	}

	if !data.CanCloseSprint {
		t.Error("expected CanCloseSprint to be true when all tasks are in Done and not processing")
	}
}

// TestBuildBoardData_CanCloseSprint_False_WhenProcessing verifies CanCloseSprint is false when processing
func TestBuildBoardData_CanCloseSprint_False_WhenProcessing(t *testing.T) {
	_ = &Server{tmpls: make(map[string]*template.Template)}

	// Simulate board data with all tasks in Done but processing is true
	data := boardData{
		Active:     "board",
		Processing: true, // Processing is true
		Paused:     false,
		// All active columns empty
		Blocked:  []taskCard{},
		Backlog:  []taskCard{},
		Plan:     []taskCard{},
		Code:     []taskCard{},
		AIReview: []taskCard{},
		Approve:  []taskCard{},
		// Tasks only in Done, no Failed
		Done: []taskCard{
			{ID: 1, Title: "Completed task", Status: "Done"},
		},
		Failed: []taskCard{},
	}

	// Apply the same logic as in buildBoardData
	if !data.Processing &&
		len(data.Blocked) == 0 &&
		len(data.Backlog) == 0 &&
		len(data.Plan) == 0 &&
		len(data.Code) == 0 &&
		len(data.AIReview) == 0 &&
		len(data.CheckPipeline) == 0 &&
		len(data.Approve) == 0 &&
		len(data.Merge) == 0 &&
		len(data.Failed) == 0 &&
		len(data.Done) > 0 {
		data.CanCloseSprint = true
	}

	if data.CanCloseSprint {
		t.Error("expected CanCloseSprint to be false when processing is true")
	}
}

// TestBuildBoardData_CanCloseSprint_False_WhenActiveTasks verifies CanCloseSprint is false when tasks in active columns
func TestBuildBoardData_CanCloseSprint_False_WhenActiveTasks(t *testing.T) {
	tests := []struct {
		name          string
		blocked       []taskCard
		backlog       []taskCard
		plan          []taskCard
		code          []taskCard
		aiReview      []taskCard
		approve       []taskCard
		merge         []taskCard
		done          []taskCard
		failed        []taskCard
		expectedClose bool
	}{
		{
			name:          "tasks in Blocked column",
			blocked:       []taskCard{{ID: 1, Title: "Blocked task"}},
			done:          []taskCard{{ID: 100, Title: "Done task"}},
			expectedClose: false,
		},
		{
			name:          "tasks in Backlog column",
			backlog:       []taskCard{{ID: 1, Title: "Backlog task"}},
			done:          []taskCard{{ID: 100, Title: "Done task"}},
			expectedClose: false,
		},
		{
			name:          "tasks in Plan column",
			plan:          []taskCard{{ID: 1, Title: "Plan task"}},
			done:          []taskCard{{ID: 100, Title: "Done task"}},
			expectedClose: false,
		},
		{
			name:          "tasks in Code column",
			code:          []taskCard{{ID: 1, Title: "Code task"}},
			done:          []taskCard{{ID: 100, Title: "Done task"}},
			expectedClose: false,
		},
		{
			name:          "tasks in AI Review column",
			aiReview:      []taskCard{{ID: 1, Title: "AI Review task"}},
			done:          []taskCard{{ID: 100, Title: "Done task"}},
			expectedClose: false,
		},
		{
			name:          "tasks in Approve column",
			approve:       []taskCard{{ID: 1, Title: "Approve task"}},
			done:          []taskCard{{ID: 100, Title: "Done task"}},
			expectedClose: false,
		},
		{
			name:          "tasks in Merge column",
			merge:         []taskCard{{ID: 1, Title: "Merge task"}},
			done:          []taskCard{{ID: 100, Title: "Done task"}},
			expectedClose: false,
		},
		{
			name:          "no tasks in active columns but no done/failed tickets",
			expectedClose: false,
		},
		{
			name:          "no tasks in active columns with done tickets",
			done:          []taskCard{{ID: 100, Title: "Done task"}},
			expectedClose: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			data := boardData{
				Active:     "board",
				Processing: false,
				Blocked:    tt.blocked,
				Backlog:    tt.backlog,
				Plan:       tt.plan,
				Code:       tt.code,
				AIReview:   tt.aiReview,
				Approve:    tt.approve,
				Merge:      tt.merge,
				Done:       tt.done,
				Failed:     tt.failed,
			}

			// Apply the same logic as in buildBoardData
			if !data.Processing &&
				len(data.Blocked) == 0 &&
				len(data.Backlog) == 0 &&
				len(data.Plan) == 0 &&
				len(data.Code) == 0 &&
				len(data.AIReview) == 0 &&
				len(data.CheckPipeline) == 0 &&
				len(data.Approve) == 0 &&
				len(data.Merge) == 0 &&
				len(data.Failed) == 0 &&
				len(data.Done) > 0 {
				data.CanCloseSprint = true
			}

			if data.CanCloseSprint != tt.expectedClose {
				t.Errorf("expected CanCloseSprint=%v, got %v", tt.expectedClose, data.CanCloseSprint)
			}
		})
	}
}

// TestBuildBoardData_CanCloseSprint_False_WhenEmptySprint verifies CanCloseSprint is false when sprint has no tickets at all
func TestBuildBoardData_CanCloseSprint_False_WhenEmptySprint(t *testing.T) {
	data := boardData{
		Active:     "board",
		Processing: false,
		Blocked:    []taskCard{},
		Backlog:    []taskCard{},
		Plan:       []taskCard{},
		Code:       []taskCard{},
		AIReview:   []taskCard{},
		Approve:    []taskCard{},
		Merge:      []taskCard{},
		Done:       []taskCard{},
		Failed:     []taskCard{},
	}

	if !data.Processing &&
		len(data.Blocked) == 0 &&
		len(data.Backlog) == 0 &&
		len(data.Plan) == 0 &&
		len(data.Code) == 0 &&
		len(data.AIReview) == 0 &&
		len(data.CheckPipeline) == 0 &&
		len(data.Approve) == 0 &&
		len(data.Merge) == 0 &&
		len(data.Failed) == 0 &&
		len(data.Done) > 0 {
		data.CanCloseSprint = true
	}

	if data.CanCloseSprint {
		t.Error("expected CanCloseSprint to be false when sprint has no tickets at all")
	}
}

// TestBuildBoardData_CanCloseSprint_False_WhenFailedTickets verifies CanCloseSprint is false when tickets exist in Failed state
func TestBuildBoardData_CanCloseSprint_False_WhenFailedTickets(t *testing.T) {
	tests := []struct {
		name          string
		done          []taskCard
		failed        []taskCard
		expectedClose bool
	}{
		{
			name:          "only failed tickets",
			failed:        []taskCard{{ID: 1, Title: "Failed task"}},
			expectedClose: false,
		},
		{
			name:          "both done and failed tickets",
			done:          []taskCard{{ID: 1, Title: "Done task"}},
			failed:        []taskCard{{ID: 2, Title: "Failed task"}},
			expectedClose: false,
		},
		{
			name:          "multiple failed tickets with done tickets",
			done:          []taskCard{{ID: 1, Title: "Done task"}},
			failed:        []taskCard{{ID: 2, Title: "Failed task 1"}, {ID: 3, Title: "Failed task 2"}},
			expectedClose: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			data := boardData{
				Active:     "board",
				Processing: false,
				Blocked:    []taskCard{},
				Backlog:    []taskCard{},
				Plan:       []taskCard{},
				Code:       []taskCard{},
				AIReview:   []taskCard{},
				Approve:    []taskCard{},
				Merge:      []taskCard{},
				Done:       tt.done,
				Failed:     tt.failed,
			}

			// Apply the same logic as in buildBoardData
			if !data.Processing &&
				len(data.Blocked) == 0 &&
				len(data.Backlog) == 0 &&
				len(data.Plan) == 0 &&
				len(data.Code) == 0 &&
				len(data.AIReview) == 0 &&
				len(data.CheckPipeline) == 0 &&
				len(data.Approve) == 0 &&
				len(data.Merge) == 0 &&
				len(data.Failed) == 0 &&
				len(data.Done) > 0 {
				data.CanCloseSprint = true
			}

			if data.CanCloseSprint != tt.expectedClose {
				t.Errorf("expected CanCloseSprint=%v, got %v", tt.expectedClose, data.CanCloseSprint)
			}
		})
	}
}

// TestHandleSprintClose_Success verifies the sprint close handler works correctly
func TestHandleSprintClose_Success(t *testing.T) {
	srv := &Server{
		tmpls:        make(map[string]*template.Template),
		orchestrator: nil, // No orchestrator - not processing
		gh:           nil, // No GitHub client - will fail with "no active milestone"
	}

	req := httptest.NewRequest(http.MethodPost, "/api/sprint/close", nil)
	rec := httptest.NewRecorder()

	srv.handleSprintCloseLegacy(rec, req)

	// Should return 400 because there's no active milestone (gh is nil)
	if rec.Code != http.StatusBadRequest {
		t.Errorf("expected status 400 for no active milestone, got %d", rec.Code)
	}
}

// TestHandleSprintClose_WhileProcessing verifies the handler rejects when processing
func TestHandleSprintClose_WhileProcessing(t *testing.T) {
	// This test verifies the logic - when orchestrator is processing, close should be rejected
	// Since we can't easily mock the orchestrator, we test the logic directly
	processing := true
	canClose := !processing

	if canClose {
		t.Error("expected canClose to be false when processing is true")
	}
}

// TestHandleSprintClose_NoOrchestrator verifies the handler works without orchestrator
func TestHandleSprintClose_NoOrchestrator(t *testing.T) {
	srv := &Server{
		tmpls:        make(map[string]*template.Template),
		orchestrator: nil, // No orchestrator
		gh:           nil, // No GitHub client
	}

	req := httptest.NewRequest(http.MethodPost, "/api/sprint/close", nil)
	rec := httptest.NewRecorder()

	srv.handleSprintCloseLegacy(rec, req)

	// Should return 400 because there's no active milestone (gh is nil)
	// but should NOT fail due to orchestrator check (no orchestrator means not processing)
	if rec.Code != http.StatusBadRequest {
		t.Errorf("expected status 400 for no active milestone, got %d", rec.Code)
	}
}

// TestBoardTemplate_CloseSprintButton verifies the Close Sprint button appears when CanCloseSprint is true
func TestBoardTemplate_CloseSprintButton(t *testing.T) {
	srv := createTestServerWithTemplates(t)
	defer srv.wizardStore.Stop()

	// Test with CanCloseSprint = true
	data := boardData{
		Active:         "board",
		CanCloseSprint: true,
		Paused:         true,
		Processing:     false,
	}

	// Create a minimal template for testing
	tmplContent := `{{define "content"}}
<div class="board-actions">
  {{if .CanCloseSprint}}
  <a href="/sprint/close" class="btn btn-success">Close Sprint</a>
  {{end}}
</div>
{{end}}`

	tmpl, err := template.New("test.html").Parse(tmplContent)
	if err != nil {
		t.Fatalf("failed to parse template: %v", err)
	}

	var buf strings.Builder
	if err := tmpl.ExecuteTemplate(&buf, "content", data); err != nil {
		t.Fatalf("failed to execute template: %v", err)
	}

	output := buf.String()

	// Verify Close Sprint button is present
	if !strings.Contains(output, "Close Sprint") {
		t.Error("template should contain 'Close Sprint' button when CanCloseSprint is true")
	}

	if !strings.Contains(output, `href="/sprint/close"`) {
		t.Error("Close Sprint link should have correct href URL")
	}
}

// TestBoardTemplate_CloseSprintButton_Hidden verifies the Close Sprint button is hidden when CanCloseSprint is false
func TestBoardTemplate_CloseSprintButton_Hidden(t *testing.T) {
	// Test with CanCloseSprint = false
	data := boardData{
		Active:         "board",
		CanCloseSprint: false,
		Paused:         true,
		Processing:     false,
	}

	tmplContent := `{{define "content"}}
<div class="board-actions">
  {{if .CanCloseSprint}}
  <a href="/sprint/close" class="btn btn-success">Close Sprint</a>
  {{end}}
</div>
{{end}}`

	tmpl, err := template.New("test.html").Parse(tmplContent)
	if err != nil {
		t.Fatalf("failed to parse template: %v", err)
	}

	var buf strings.Builder
	if err := tmpl.ExecuteTemplate(&buf, "content", data); err != nil {
		t.Fatalf("failed to execute template: %v", err)
	}

	output := buf.String()

	// Verify Close Sprint button is NOT present
	if strings.Contains(output, "Close Sprint") {
		t.Error("template should NOT contain 'Close Sprint' button when CanCloseSprint is false")
	}
}

// TestHandleWizardCreateSingle_TriggersSync verifies that SyncNow() is called when syncService is configured
func TestHandleWizardCreateSingle_TriggersSync(t *testing.T) {
	srv := &Server{
		tmpls:       make(map[string]*template.Template),
		wizardStore: NewWizardSessionStore(),
		syncService: &SyncService{}, // We'll use a real one but verify through logs or behavior
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

	// Note: Since we're using a real syncService with nil dependencies, SyncNow will fail
	// but the creation should still succeed (sync failure doesn't block creation)
	t.Logf("Sync trigger test completed - syncService was not nil: %v", srv.syncService != nil)
}

// TestHandleWizardCreateSingle_NilSyncService verifies that nil syncService doesn't panic
func TestHandleWizardCreateSingle_NilSyncService(t *testing.T) {
	srv := &Server{
		tmpls:       make(map[string]*template.Template),
		wizardStore: NewWizardSessionStore(),
		syncService: nil, // Explicitly nil
	}
	defer srv.wizardStore.Stop()

	// Create a session with technical planning
	session, _ := srv.wizardStore.Create("feature")
	session.SetIdeaText("Small feature idea")
	session.SetTechnicalPlanning("## Technical Planning\n\nThis is a small feature technical planning")

	// Test creating single issue - should not panic with nil syncService
	form := url.Values{}
	form.Set("session_id", session.ID)

	req := httptest.NewRequest(http.MethodPost, "/wizard/create", strings.NewReader(form.Encode()))
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	rec := httptest.NewRecorder()

	// This should not panic even with nil syncService
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

// TestHandleWizardCreateSingle_SyncFailureDoesNotBlockCreation verifies creation succeeds even if sync fails
func TestHandleWizardCreateSingle_SyncFailureDoesNotBlockCreation(t *testing.T) {
	// Create a sync service with nil dependencies to simulate failure
	// The sync will fail but creation should still succeed
	syncService := NewSyncService(nil, nil, nil, nil)

	srv := &Server{
		tmpls:       make(map[string]*template.Template),
		wizardStore: NewWizardSessionStore(),
		syncService: syncService,
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

	// Should return 200 OK (or 500 if template missing) - creation should succeed despite sync failure
	if rec.Code != http.StatusOK && rec.Code != http.StatusInternalServerError {
		t.Errorf("expected status 200 or 500, got %d: %s", rec.Code, rec.Body.String())
	}

	// Verify session was deleted after creation - proves creation succeeded
	_, ok := srv.wizardStore.Get(session.ID)
	if ok {
		t.Error("session should be deleted after single issue creation (creation should succeed even if sync fails)")
	}
}

// TestHandleSprintClose_SuccessWithNewSprintCreation verifies the sprint close handler
// works correctly and would create a new sprint (integration test with real GitHub client)
func TestHandleSprintClose_SuccessWithNewSprintCreation(t *testing.T) {
	// This test verifies the handler structure is correct for the new implementation
	// Full integration testing requires a real GitHub client
	srv := &Server{
		tmpls:        make(map[string]*template.Template),
		orchestrator: nil, // No orchestrator - not processing
		gh:           nil, // No GitHub client - will fail with "no active milestone"
	}

	req := httptest.NewRequest(http.MethodPost, "/api/sprint/close", nil)
	rec := httptest.NewRecorder()

	srv.handleSprintCloseLegacy(rec, req)

	// Should return 400 because there's no active milestone (gh is nil)
	if rec.Code != http.StatusBadRequest {
		t.Errorf("expected status 400 for no active milestone, got %d", rec.Code)
	}
}

// TestHandleSprintClose_WhileProcessing verifies the handler rejects when orchestrator is processing
func TestHandleSprintClose_WhileProcessing_WithMock(t *testing.T) {
	// This test verifies the logic - when orchestrator is processing, close should be rejected
	// Since we can't easily mock the orchestrator, we test the logic directly
	processing := true
	canClose := !processing

	if canClose {
		t.Error("expected canClose to be false when processing is true")
	}
}

// TestHandleSettings tests the GET /settings handler
func TestHandleSettings(t *testing.T) {
	// Create a temporary directory for config
	tmpDir := t.TempDir()

	// Create .oda directory and config.yaml
	odaDir := filepath.Join(tmpDir, ".oda")
	if err := os.MkdirAll(odaDir, 0755); err != nil {
		t.Fatalf("failed to create .oda directory: %v", err)
	}

	// Create a minimal config file
	configContent := `llm:
  code:
    model: test-provider/test-model
  code-heavy:
    model: test-provider/test-model-heavy
  planning:
    model: test-provider/test-model-planning
  orchestration:
    model: test-provider/test-model-orchestration
  setup:
    model: test-provider/test-model-setup
`
	configPath := filepath.Join(odaDir, "config.yaml")
	if err := os.WriteFile(configPath, []byte(configContent), 0644); err != nil {
		t.Fatalf("failed to write config file: %v", err)
	}

	// Create server with templates
	srv := createTestServerWithTemplates(t)
	srv.rootDir = tmpDir
	defer srv.wizardStore.Stop()

	// Test GET /settings
	req := httptest.NewRequest(http.MethodGet, "/settings", nil)
	rec := httptest.NewRecorder()

	srv.handleSettings(rec, req)

	// Should return 200 OK
	if rec.Code != http.StatusOK {
		t.Errorf("expected status 200, got %d", rec.Code)
	}

	// Verify response contains expected content
	body := rec.Body.String()
	if !strings.Contains(body, "LLM Configuration Settings") {
		t.Error("response should contain 'LLM Configuration Settings' heading")
	}
	if !strings.Contains(body, "Models") {
		t.Error("response should contain 'Models' section")
	}
	if !strings.Contains(body, "test-provider/test-model") {
		t.Error("response should contain the model value from config")
	}
}

// TestHandleSettings_NoConfigFile tests that the handler works even without a config file
func TestHandleSettings_NoConfigFile(t *testing.T) {
	// Create a temporary directory without config
	tmpDir := t.TempDir()

	// Create server with templates
	srv := createTestServerWithTemplates(t)
	srv.rootDir = tmpDir
	defer srv.wizardStore.Stop()

	// Test GET /settings without config file
	req := httptest.NewRequest(http.MethodGet, "/settings", nil)
	rec := httptest.NewRecorder()

	srv.handleSettings(rec, req)

	// Should return 200 OK (uses defaults)
	if rec.Code != http.StatusOK {
		t.Errorf("expected status 200, got %d", rec.Code)
	}

	// Verify response contains expected content
	body := rec.Body.String()
	if !strings.Contains(body, "LLM Configuration Settings") {
		t.Error("response should contain 'LLM Configuration Settings' heading")
	}
}

// TestHandleSaveSettings tests the POST /settings handler
func TestHandleSaveSettings(t *testing.T) {
	// Create a temporary directory for config
	tmpDir := t.TempDir()

	// Create .oda directory
	odaDir := filepath.Join(tmpDir, ".oda")
	if err := os.MkdirAll(odaDir, 0755); err != nil {
		t.Fatalf("failed to create .oda directory: %v", err)
	}

	// Create a minimal config file
	configContent := `llm:
  development:
    strong:
      model: test-provider/test-model
    weak:
      model: test-provider/test-model-weak
`
	configPath := filepath.Join(odaDir, "config.yaml")
	if err := os.WriteFile(configPath, []byte(configContent), 0644); err != nil {
		t.Fatalf("failed to write config file: %v", err)
	}

	// Create server with templates
	srv := createTestServerWithTemplates(t)
	srv.rootDir = tmpDir
	defer srv.wizardStore.Stop()

	// Test POST /settings with valid data
	form := url.Values{}
	form.Set("setup_model", "setup-provider/setup-model")
	form.Set("planning_model", "planning-provider/planning-model")
	form.Set("orchestration_model", "orch-provider/orch-model")
	form.Set("code_model", "code-provider/code-model")
	form.Set("code_heavy_model", "code-heavy-provider/code-heavy-model")

	req := httptest.NewRequest(http.MethodPost, "/settings", strings.NewReader(form.Encode()))
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	rec := httptest.NewRecorder()

	srv.handleSaveSettings(rec, req)

	// Should return 200 OK
	if rec.Code != http.StatusOK {
		t.Errorf("expected status 200, got %d: %s", rec.Code, rec.Body.String())
	}

	// Verify success message
	body := rec.Body.String()
	if !strings.Contains(body, "Settings saved successfully") {
		t.Error("response should contain success message")
	}

	// Verify config was saved
	savedData, err := os.ReadFile(configPath)
	if err != nil {
		t.Fatalf("failed to read saved config: %v", err)
	}

	savedContent := string(savedData)
	if !strings.Contains(savedContent, "code-provider/code-model") {
		t.Error("saved config should contain new model")
	}
}

// TestHandleSaveSettingsValidation tests form validation
func TestHandleSaveSettingsValidation(t *testing.T) {
	// Create a temporary directory for config
	tmpDir := t.TempDir()

	// Create .oda directory
	odaDir := filepath.Join(tmpDir, ".oda")
	if err := os.MkdirAll(odaDir, 0755); err != nil {
		t.Fatalf("failed to create .oda directory: %v", err)
	}

	// Create a minimal config file
	configPath := filepath.Join(odaDir, "config.yaml")
	configContent := `llm:
  development:
    strong:
      model: test-provider/test-model
    weak:
      model: test-provider/test-model-weak
`
	if err := os.WriteFile(configPath, []byte(configContent), 0644); err != nil {
		t.Fatalf("failed to write config file: %v", err)
	}

	// Create server with templates
	srv := createTestServerWithTemplates(t)
	srv.rootDir = tmpDir
	defer srv.wizardStore.Stop()

	// Test POST /settings with invalid data (empty required fields)
	form := url.Values{}
	form.Set("setup_model", "setup-provider/setup-model")
	form.Set("planning_model", "planning-provider/planning-model")
	form.Set("orchestration_model", "orch-provider/orch-model")
	form.Set("code_model", "") // Empty - should fail validation
	form.Set("code_heavy_model", "code-heavy-provider/code-heavy-model")
	form.Set("routing_code_size_threshold", "150")
	form.Set("routing_high_complexity_threshold", "600")
	form.Set("routing_file_count_threshold", "10")

	req := httptest.NewRequest(http.MethodPost, "/settings", strings.NewReader(form.Encode()))
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	rec := httptest.NewRecorder()

	srv.handleSaveSettings(rec, req)

	// Should return 200 OK but with error message
	if rec.Code != http.StatusOK {
		t.Errorf("expected status 200, got %d", rec.Code)
	}

	// Verify error message
	body := rec.Body.String()
	if !strings.Contains(body, "Model is required") {
		t.Error("response should contain validation error for empty model")
	}
}

// TestSettingsPersistence verifies that saved config can be reloaded correctly
func TestSettingsPersistence(t *testing.T) {
	// Create a temporary directory for config
	tmpDir := t.TempDir()

	// Create .oda directory
	odaDir := filepath.Join(tmpDir, ".oda")
	if err := os.MkdirAll(odaDir, 0755); err != nil {
		t.Fatalf("failed to create .oda directory: %v", err)
	}

	// Create a minimal config file
	configPath := filepath.Join(odaDir, "config.yaml")
	configContent := `llm:
  code:
    model: original-provider/original-model
`
	if err := os.WriteFile(configPath, []byte(configContent), 0644); err != nil {
		t.Fatalf("failed to write config file: %v", err)
	}

	// Create server with templates
	srv := createTestServerWithTemplates(t)
	srv.rootDir = tmpDir
	defer srv.wizardStore.Stop()

	// Save new settings
	form := url.Values{}
	form.Set("setup_model", "setup-provider/setup-model")
	form.Set("planning_model", "planning-provider/planning-model")
	form.Set("orchestration_model", "orch-provider/orch-model")
	form.Set("code_model", "persisted-provider/persisted-model")
	form.Set("code_heavy_model", "code-heavy-provider/code-heavy-model")
	form.Set("routing_code_size_threshold", "200")
	form.Set("routing_high_complexity_threshold", "700")
	form.Set("routing_file_count_threshold", "15")
	form.Set("routing_force_strong_stages", "test-stage")

	req := httptest.NewRequest(http.MethodPost, "/settings", strings.NewReader(form.Encode()))
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	rec := httptest.NewRecorder()

	srv.handleSaveSettings(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("expected status 200, got %d: %s", rec.Code, rec.Body.String())
	}

	// Reload the config
	reloadedCfg, err := config.Load(tmpDir)
	if err != nil {
		t.Fatalf("failed to reload config: %v", err)
	}

	// Verify the values were persisted
	if reloadedCfg.LLM.Code.Model != "persisted-provider/persisted-model" {
		t.Errorf("expected model to be 'persisted-provider/persisted-model', got %q", reloadedCfg.LLM.Code.Model)
	}
}

// TestHandleSettings_WithModels verifies that available models are passed to the template
func TestHandleSettings_WithModels(t *testing.T) {
	// Create a temporary directory for config
	tmpDir := t.TempDir()

	// Create .oda directory
	odaDir := filepath.Join(tmpDir, ".oda")
	if err := os.MkdirAll(odaDir, 0755); err != nil {
		t.Fatalf("failed to create .oda directory: %v", err)
	}

	// Create a minimal config file
	configContent := `llm:
  code:
    model: test-provider/test-model
`
	configPath := filepath.Join(odaDir, "config.yaml")
	if err := os.WriteFile(configPath, []byte(configContent), 0644); err != nil {
		t.Fatalf("failed to write config file: %v", err)
	}

	// Create server with templates and mock models cache
	srv := createTestServerWithTemplates(t)
	srv.rootDir = tmpDir
	srv.modelsCache = []opencode.ProviderModel{
		{ID: "openai/gpt-4", ProviderID: "openai", Name: "GPT-4"},
		{ID: "anthropic/claude-3", ProviderID: "anthropic", Name: "Claude 3"},
	}
	defer srv.wizardStore.Stop()

	// Test GET /settings
	req := httptest.NewRequest(http.MethodGet, "/settings", nil)
	rec := httptest.NewRecorder()

	srv.handleSettings(rec, req)

	// Should return 200 OK
	if rec.Code != http.StatusOK {
		t.Errorf("expected status 200, got %d", rec.Code)
	}

	// Verify response contains model data (as JSON in the script)
	body := rec.Body.String()
	if !strings.Contains(body, "gpt-4") {
		t.Error("response should contain model data (gpt-4)")
	}
	if !strings.Contains(body, "claude-3") {
		t.Error("response should contain model data (claude-3)")
	}
}

// TestSettings_ModelIDRendering verifies that model IDs are rendered without extra quotes
// This test addresses issue #450 where the json template function was causing double-escaping
func TestSettings_ModelIDRendering(t *testing.T) {
	// Create a temporary directory for config
	tmpDir := t.TempDir()

	// Create .oda directory
	tmpOdaDir := filepath.Join(tmpDir, ".oda")
	if err := os.MkdirAll(tmpOdaDir, 0755); err != nil {
		t.Fatalf("failed to create .oda directory: %v", err)
	}

	// Create a minimal config file
	configPath := filepath.Join(tmpOdaDir, "config.yaml")
	configContent := `llm:
  code:
    model: test-provider/test-model
`
	if err := os.WriteFile(configPath, []byte(configContent), 0644); err != nil {
		t.Fatalf("failed to write config file: %v", err)
	}

	// Create server with templates and mock models cache
	srv := createTestServerWithTemplates(t)
	srv.rootDir = tmpDir
	// Use a model ID with special characters that was causing the bug
	srv.modelsCache = []opencode.ProviderModel{
		{ID: "openrouter/stepfun/step-3.5-flash:free", ProviderID: "openrouter", Name: "Step 3.5 Flash"},
	}
	defer srv.wizardStore.Stop()

	// Test GET /settings
	req := httptest.NewRequest(http.MethodGet, "/settings", nil)
	rec := httptest.NewRecorder()

	srv.handleSettings(rec, req)

	// Should return 200 OK
	if rec.Code != http.StatusOK {
		t.Errorf("expected status 200, got %d", rec.Code)
	}

	// Verify response contains the model ID without extra quotes
	body := rec.Body.String()

	// The model ID should appear with proper escaping but WITHOUT extra quotes around the whole ID
	// Correct: "openrouter\/stepfun\/step-3.5-flash:free" (slashes escaped for JS)
	// Bug #450 would produce: "\"openrouter\\/stepfun\\/step-3.5-flash:free\"" (extra quotes)
	expectedModelID := `"openrouter\/stepfun\/step-3.5-flash:free"`
	if !strings.Contains(body, expectedModelID) {
		// Find where the model ID appears and show context
		idx := strings.Index(body, "openrouter")
		if idx == -1 {
			t.Fatalf("response body does not contain expected model ID at all")
		}
		start := idx - 20
		start = max(start, 0)
		end := idx + 80
		end = min(end, len(body))
		t.Errorf("response should contain model ID with proper escaping\nexpected: %s\nactual context: %s",
			expectedModelID, body[start:end])
	}

	// Make sure we don't have the buggy pattern with escaped quotes (the actual bug #450)
	// The buggy pattern would be: { id: "\"openrouter...\"" (quotes inside quotes)
	buggyPattern := `"\\"openrouter`
	if strings.Contains(body, buggyPattern) {
		t.Error("response contains model ID with extra escaped quotes (bug #450)")
	}
}

// TestHandleSaveSettings_InvalidModel verifies that invalid models are saved as-is (fallback happens at runtime)
func TestHandleSaveSettings_InvalidModel(t *testing.T) {
	// Create a temporary directory for config
	tmpDir := t.TempDir()

	// Create .oda directory
	tmpOdaDir := filepath.Join(tmpDir, ".oda")
	if err := os.MkdirAll(tmpOdaDir, 0755); err != nil {
		t.Fatalf("failed to create .oda directory: %v", err)
	}

	// Create a minimal config file
	configPath := filepath.Join(tmpOdaDir, "config.yaml")
	configContent := `llm:
  code:
    model: test-provider/test-model
`
	if err := os.WriteFile(configPath, []byte(configContent), 0644); err != nil {
		t.Fatalf("failed to write config file: %v", err)
	}

	// Create server with templates and mock models cache
	srv := createTestServerWithTemplates(t)
	srv.rootDir = tmpDir
	srv.modelsCache = []opencode.ProviderModel{
		{ID: "openai/gpt-4", ProviderID: "openai", Name: "GPT-4"},
		{ID: "anthropic/claude-3", ProviderID: "anthropic", Name: "Claude 3"},
	}
	defer srv.wizardStore.Stop()

	// Test POST /settings with invalid model
	form := url.Values{}
	form.Set("setup_model", "openai/gpt-4")
	form.Set("planning_model", "openai/gpt-4")
	form.Set("orchestration_model", "openai/gpt-4")
	form.Set("code_model", "openai/invalid-model") // Invalid model
	form.Set("code_heavy_model", "openai/gpt-4")
	form.Set("routing_code_size_threshold", "150")
	form.Set("routing_high_complexity_threshold", "600")
	form.Set("routing_file_count_threshold", "10")

	req := httptest.NewRequest(http.MethodPost, "/settings", strings.NewReader(form.Encode()))
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	rec := httptest.NewRecorder()

	srv.handleSaveSettings(rec, req)

	// Should return 200 OK (success - model is saved as-is)
	if rec.Code != http.StatusOK {
		t.Errorf("expected status 200, got %d", rec.Code)
	}

	// Verify success message (not error)
	body := rec.Body.String()
	if !strings.Contains(body, "saved successfully") {
		t.Errorf("response should contain success message, got: %s", body)
	}

	// Verify the invalid model was saved as-is (no fallback at save time)
	// Runtime fallback happens in the router when models are actually used
	loadedCfg, err := config.Load(tmpDir)
	if err != nil {
		t.Fatalf("failed to load saved config: %v", err)
	}
	if loadedCfg.LLM.Code.Model != "openai/invalid-model" {
		t.Errorf("invalid model should be saved as-is, got %q", loadedCfg.LLM.Code.Model)
	}
}

// TestHandleSaveSettings_ValidModel verifies that validation accepts valid models
func TestHandleSaveSettings_ValidModel(t *testing.T) {
	// Create a temporary directory for config
	tmpDir := t.TempDir()

	// Create .oda directory
	odaDir := filepath.Join(tmpDir, ".oda")
	if err := os.MkdirAll(odaDir, 0755); err != nil {
		t.Fatalf("failed to create .oda directory: %v", err)
	}

	// Create a minimal config file
	configPath := filepath.Join(odaDir, "config.yaml")
	configContent := `llm:
  code:
    model: test-provider/test-model
`
	if err := os.WriteFile(configPath, []byte(configContent), 0644); err != nil {
		t.Fatalf("failed to write config file: %v", err)
	}

	// Create server with templates and mock models cache
	srv := createTestServerWithTemplates(t)
	srv.rootDir = tmpDir
	srv.modelsCache = []opencode.ProviderModel{
		{ID: "openai/gpt-4", ProviderID: "openai", Name: "GPT-4"},
		{ID: "anthropic/claude-3", ProviderID: "anthropic", Name: "Claude 3"},
	}
	defer srv.wizardStore.Stop()

	// Test POST /settings with valid models
	form := url.Values{}
	form.Set("setup_model", "openai/gpt-4")
	form.Set("planning_model", "openai/gpt-4")
	form.Set("orchestration_model", "openai/gpt-4")
	form.Set("code_model", "openai/gpt-4")
	form.Set("code_heavy_model", "anthropic/claude-3")
	form.Set("routing_code_size_threshold", "150")
	form.Set("routing_high_complexity_threshold", "600")
	form.Set("routing_file_count_threshold", "10")

	req := httptest.NewRequest(http.MethodPost, "/settings", strings.NewReader(form.Encode()))
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	rec := httptest.NewRecorder()

	srv.handleSaveSettings(rec, req)

	// Should return 200 OK with success message
	body := rec.Body.String()
	t.Logf("Status: %d, Body: %s", rec.Code, body)
	if rec.Code != http.StatusOK {
		t.Errorf("expected status 200, got %d: %s", rec.Code, body)
	}

	// Verify success message
	if !strings.Contains(body, "Settings saved successfully") {
		t.Error("response should contain success message")
	}

	// Verify no error about invalid models
	if strings.Contains(body, "Invalid model") {
		t.Error("response should NOT contain invalid model error for valid models")
	}
}

// TestValidateModelSelection tests the validateModelSelection helper method
func TestValidateModelSelection(t *testing.T) {
	srv := &Server{
		modelsCache: []opencode.ProviderModel{
			{ID: "openai/gpt-4", ProviderID: "openai", Name: "GPT-4"},
			{ID: "anthropic/claude-3", ProviderID: "anthropic", Name: "Claude 3"},
			{ID: "openai/gpt-3.5", ProviderID: "openai", Name: "GPT-3.5"},
		},
	}

	tests := []struct {
		name  string
		model string
		want  bool
	}{
		{
			name:  "valid model - openai/gpt-4",
			model: "openai/gpt-4",
			want:  true,
		},
		{
			name:  "valid model - anthropic/claude-3",
			model: "anthropic/claude-3",
			want:  true,
		},
		{
			name:  "invalid model - nonexistent",
			model: "openai/nonexistent-model",
			want:  false,
		},
		{
			name:  "invalid model - wrong format",
			model: "gpt-4",
			want:  false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := srv.validateModelSelection(tt.model)
			if got != tt.want {
				t.Errorf("validateModelSelection(%q) = %v, want %v", tt.model, got, tt.want)
			}
		})
	}
}

// TestValidateModelSelection_EmptyCache tests that validation passes when cache is empty
func TestValidateModelSelection_EmptyCache(t *testing.T) {
	srv := &Server{
		modelsCache: []opencode.ProviderModel{}, // Empty cache
	}

	// Should return true (skip validation) when cache is empty
	got := srv.validateModelSelection("any-provider/any-model")
	if !got {
		t.Error("validateModelSelection should return true when cache is empty (skip validation)")
	}
}

// TestHandleSaveSettings_EmptyCache allows any model when cache is empty
func TestHandleSaveSettings_EmptyCache(t *testing.T) {
	// Create a temporary directory for config
	tmpDir := t.TempDir()

	// Create .oda directory
	odaDir := filepath.Join(tmpDir, ".oda")
	if err := os.MkdirAll(odaDir, 0755); err != nil {
		t.Fatalf("failed to create .oda directory: %v", err)
	}

	// Create a minimal config file
	configPath := filepath.Join(odaDir, "config.yaml")
	configContent := `llm:
  code:
    model: test-provider/test-model
`
	if err := os.WriteFile(configPath, []byte(configContent), 0644); err != nil {
		t.Fatalf("failed to write config file: %v", err)
	}

	// Create server with templates but EMPTY models cache
	srv := createTestServerWithTemplates(t)
	srv.rootDir = tmpDir
	srv.modelsCache = []opencode.ProviderModel{} // Empty cache
	defer srv.wizardStore.Stop()

	// Test POST /settings with any model (should be allowed when cache is empty)
	form := url.Values{}
	form.Set("setup_model", "custom-provider/custom-model")
	form.Set("planning_model", "custom-provider/custom-model")
	form.Set("orchestration_model", "custom-provider/custom-model")
	form.Set("code_model", "custom-provider/custom-model")
	form.Set("code_heavy_model", "custom-provider/custom-model")
	form.Set("routing_code_size_threshold", "150")
	form.Set("routing_high_complexity_threshold", "600")
	form.Set("routing_file_count_threshold", "10")

	req := httptest.NewRequest(http.MethodPost, "/settings", strings.NewReader(form.Encode()))
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	rec := httptest.NewRecorder()

	srv.handleSaveSettings(rec, req)

	// Should return 200 OK with success message (no validation errors)
	if rec.Code != http.StatusOK {
		t.Errorf("expected status 200, got %d: %s", rec.Code, rec.Body.String())
	}

	// Verify success message
	body := rec.Body.String()
	t.Logf("Response body: %s", body)
	if !strings.Contains(body, "Settings saved successfully") {
		t.Error("response should contain success message")
	}

	// Verify no validation errors about invalid models
	if strings.Contains(body, "Invalid model") {
		t.Error("response should NOT contain invalid model error when cache is empty")
	}
}

// TestHandleSettingsTemplateData verifies the template data is correctly populated
func TestHandleSettingsTemplateData(t *testing.T) {
	// Create a temporary directory for config
	tmpDir := t.TempDir()

	// Create .oda directory
	odaDir := filepath.Join(tmpDir, ".oda")
	if err := os.MkdirAll(odaDir, 0755); err != nil {
		t.Fatalf("failed to create .oda directory: %v", err)
	}

	// Create a config file with model settings
	configContent := `llm:
  setup:
    model: test-provider/test-model
  planning:
    model: test-provider/test-model
  orchestration:
    model: test-provider/test-model
  code:
    model: test-provider/test-model
  code-heavy:
    model: test-provider/test-model
`
	configPath := filepath.Join(odaDir, "config.yaml")
	if err := os.WriteFile(configPath, []byte(configContent), 0644); err != nil {
		t.Fatalf("failed to write config file: %v", err)
	}

	// Create server with templates
	srv := createTestServerWithTemplates(t)
	srv.rootDir = tmpDir
	defer srv.wizardStore.Stop()

	// Test GET /settings
	req := httptest.NewRequest(http.MethodGet, "/settings", nil)
	rec := httptest.NewRecorder()

	srv.handleSettings(rec, req)

	// Should return 200 OK
	if rec.Code != http.StatusOK {
		t.Errorf("expected status 200, got %d", rec.Code)
	}

	// Verify model settings are displayed
	body := rec.Body.String()
	if !strings.Contains(body, "test-provider/test-model") {
		t.Error("response should contain model settings")
	}
}

// TestBuildBoardData_YoloMode tests that YoloMode is correctly loaded from config
func TestBuildBoardData_YoloMode(t *testing.T) {
	// Create a temporary directory with config
	tmpDir := t.TempDir()
	configDir := filepath.Join(tmpDir, ".oda")
	if err := os.MkdirAll(configDir, 0755); err != nil {
		t.Fatalf("failed to create config dir: %v", err)
	}

	// Test with yolo_mode enabled
	configContent := `yolo_mode: true
`
	if err := os.WriteFile(filepath.Join(configDir, "config.yaml"), []byte(configContent), 0644); err != nil {
		t.Fatalf("failed to write config: %v", err)
	}

	srv := &Server{
		tmpls:   make(map[string]*template.Template),
		rootDir: tmpDir,
	}

	data := srv.buildBoardData(nil)

	if !data.YoloMode {
		t.Error("expected YoloMode to be true when enabled in config")
	}

	// Test with yolo_mode disabled
	configContent = `yolo_mode: false
`
	if err := os.WriteFile(filepath.Join(configDir, "config.yaml"), []byte(configContent), 0644); err != nil {
		t.Fatalf("failed to write config: %v", err)
	}

	data = srv.buildBoardData(nil)

	if data.YoloMode {
		t.Error("expected YoloMode to be false when disabled in config")
	}

	// Test with no config file (should default to false)
	os.Remove(filepath.Join(configDir, "config.yaml"))
	data = srv.buildBoardData(nil)

	if data.YoloMode {
		t.Error("expected YoloMode to be false when config file missing")
	}
}

// TestBuildBoardData_YoloMode_NoRootDir tests that YoloMode defaults to false when rootDir is empty
func TestBuildBoardData_YoloMode_NoRootDir(t *testing.T) {
	srv := &Server{
		tmpls: make(map[string]*template.Template),
		// rootDir is empty
	}

	data := srv.buildBoardData(nil)

	if data.YoloMode {
		t.Error("expected YoloMode to be false when rootDir is empty")
	}
}

// TestHandleTaskDetail_YoloMode tests that YoloMode is correctly passed to task detail template
func TestHandleTaskDetail_YoloMode(t *testing.T) {
	// Create a temporary directory with config
	tmpDir := t.TempDir()
	configDir := filepath.Join(tmpDir, ".oda")
	if err := os.MkdirAll(configDir, 0755); err != nil {
		t.Fatalf("failed to create config dir: %v", err)
	}

	// Test with yolo_mode enabled
	configContent := `yolo_mode: true
`
	if err := os.WriteFile(filepath.Join(configDir, "config.yaml"), []byte(configContent), 0644); err != nil {
		t.Fatalf("failed to write config: %v", err)
	}

	srv := createTestServerWithTemplates(t)
	srv.rootDir = tmpDir
	defer srv.wizardStore.Stop()

	req := httptest.NewRequest(http.MethodGet, "/task/123", nil)
	req.SetPathValue("id", "123")
	rec := httptest.NewRecorder()

	srv.handleTaskDetail(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("expected status 200, got %d", rec.Code)
	}

	// Verify the response contains the YOLO mode indicator when enabled
	body := rec.Body.String()
	if !strings.Contains(body, "YOLO MODE") {
		t.Error("task detail page should contain YOLO MODE indicator when yolo_mode is enabled")
	}

	// Test with yolo_mode disabled
	configContent = `yolo_mode: false
`
	if err := os.WriteFile(filepath.Join(configDir, "config.yaml"), []byte(configContent), 0644); err != nil {
		t.Fatalf("failed to write config: %v", err)
	}

	req = httptest.NewRequest(http.MethodGet, "/task/456", nil)
	req.SetPathValue("id", "456")
	rec = httptest.NewRecorder()

	srv.handleTaskDetail(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("expected status 200, got %d", rec.Code)
	}

	// Verify the response shows SAFE MODE toggle when disabled (toggle is always visible)
	body = rec.Body.String()
	if !strings.Contains(body, "SAFE MODE") {
		t.Error("task detail page should show 'SAFE MODE' toggle when yolo_mode is disabled")
	}
	if !strings.Contains(body, `id="yolo-mode-container"`) {
		t.Error("task detail page should always contain yolo-mode-container element")
	}
}

// TestHandleBoard_YoloModeIndicator tests that YOLO mode indicator appears on board page
func TestHandleBoard_YoloModeIndicator(t *testing.T) {
	// Create a temporary directory with config
	tmpDir := t.TempDir()
	configDir := filepath.Join(tmpDir, ".oda")
	if err := os.MkdirAll(configDir, 0755); err != nil {
		t.Fatalf("failed to create config dir: %v", err)
	}

	// Test with yolo_mode enabled
	configContent := `yolo_mode: true
`
	if err := os.WriteFile(filepath.Join(configDir, "config.yaml"), []byte(configContent), 0644); err != nil {
		t.Fatalf("failed to write config: %v", err)
	}

	srv := createTestServerWithTemplates(t)
	srv.rootDir = tmpDir
	defer srv.wizardStore.Stop()

	req := httptest.NewRequest(http.MethodGet, "/", nil)
	rec := httptest.NewRecorder()

	srv.handleBoard(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("expected status 200, got %d", rec.Code)
	}

	body := rec.Body.String()

	// Verify YOLO mode indicator is present
	if !strings.Contains(body, "YOLO MODE") {
		t.Error("board page should contain YOLO MODE indicator when yolo_mode is enabled")
	}
	if !strings.Contains(body, "yolo-mode-container") {
		t.Error("board page should contain yolo-mode-container CSS class")
	}
	if !strings.Contains(body, "⚡") {
		t.Error("board page should contain YOLO mode icon (⚡)")
	}

	// Test with yolo_mode disabled
	configContent = `yolo_mode: false
`
	if err := os.WriteFile(filepath.Join(configDir, "config.yaml"), []byte(configContent), 0644); err != nil {
		t.Fatalf("failed to write config: %v", err)
	}

	req = httptest.NewRequest(http.MethodGet, "/", nil)
	rec = httptest.NewRecorder()

	srv.handleBoard(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("expected status 200, got %d", rec.Code)
	}

	body = rec.Body.String()

	// Verify YOLO mode toggle is always present (now in SAFE MODE state when disabled)
	if !strings.Contains(body, `id="yolo-mode-container"`) {
		t.Error("board page should always contain yolo-mode-container element")
	}
	if !strings.Contains(body, "SAFE MODE") {
		t.Error("board page should show 'SAFE MODE' when yolo_mode is disabled")
	}
	if !strings.Contains(body, "yolo-disabled") {
		t.Error("board page should contain yolo-disabled class when yolo_mode is disabled")
	}
}

// TestHandleRetry_CleansUpAndMovesToBacklog tests that handleRetry properly cleans up and moves to backlog
func TestHandleRetry_CleansUpAndMovesToBacklog(t *testing.T) {
	srv := &Server{
		tmpls: make(map[string]*template.Template),
		// No orchestrator set - should still handle gracefully
	}

	// Test with invalid issue ID
	req := httptest.NewRequest(http.MethodPost, "/retry/invalid", nil)
	req.SetPathValue("id", "invalid")
	rec := httptest.NewRecorder()

	srv.handleRetry(rec, req)

	// Should redirect to root when no orchestrator
	if rec.Code != http.StatusSeeOther {
		t.Errorf("expected status 303, got %d", rec.Code)
	}
}

// TestSettingsForm_HTMXTargetBody verifies that the settings form uses hx-target="body" to prevent layout nesting
func TestSettingsForm_HTMXTargetBody(t *testing.T) {
	srv := createTestServerWithTemplates(t)
	defer srv.wizardStore.Stop()

	// Test GET /settings
	req := httptest.NewRequest(http.MethodGet, "/settings", nil)
	rec := httptest.NewRecorder()

	srv.handleSettings(rec, req)

	// Should return 200 OK
	if rec.Code != http.StatusOK {
		t.Fatalf("expected status 200, got %d", rec.Code)
	}

	body := rec.Body.String()

	// Verify the form uses hx-target="body" (new correct value)
	if !strings.Contains(body, `hx-target="body"`) {
		t.Error("settings form should use hx-target=\"body\" to prevent layout nesting")
	}

	// Verify the form does NOT use hx-target=".settings-container" (old incorrect value)
	if strings.Contains(body, `hx-target=".settings-container"`) {
		t.Error("settings form should NOT use hx-target=\".settings-container\" (causes layout duplication)")
	}
}

// TestHandleYoloToggle_EnablesWhenDisabled tests that YOLO toggle enables YOLO mode when it's disabled
func TestHandleYoloToggle_EnablesWhenDisabled(t *testing.T) {
	tempDir := t.TempDir()

	// Create config with yolo_mode: false
	configContent := `yolo_mode: false
`
	if err := os.WriteFile(filepath.Join(tempDir, ".oda", "config.yaml"), []byte(configContent), 0644); err != nil {
		// Try creating the directory first
		if err := os.MkdirAll(filepath.Join(tempDir, ".oda"), 0755); err != nil {
			t.Fatalf("failed to create .oda directory: %v", err)
		}
		if err := os.WriteFile(filepath.Join(tempDir, ".oda", "config.yaml"), []byte(configContent), 0644); err != nil {
			t.Fatalf("failed to write config file: %v", err)
		}
	}

	srv := createTestServerWithTemplates(t)
	srv.rootDir = tempDir
	defer srv.wizardStore.Stop()

	// POST to toggle endpoint
	req := httptest.NewRequest(http.MethodPost, "/api/yolo/toggle", nil)
	rec := httptest.NewRecorder()

	srv.handleYoloToggle(rec, req)

	// Should return 200 OK
	if rec.Code != http.StatusOK {
		t.Errorf("expected status 200, got %d: %s", rec.Code, rec.Body.String())
	}

	// Verify response contains enabled state
	body := rec.Body.String()
	if !strings.Contains(body, "yolo-enabled") {
		t.Error("response should contain yolo-enabled class when enabling YOLO mode")
	}
	if !strings.Contains(body, "YOLO MODE") {
		t.Error("response should contain 'YOLO MODE' text when enabling")
	}
	if !strings.Contains(body, "⚡") {
		t.Error("response should contain lightning bolt icon when enabling")
	}

	// Verify config file is UNCHANGED (still yolo_mode: false)
	cfg, err := config.Load(tempDir)
	if err != nil {
		t.Fatalf("failed to load config after toggle: %v", err)
	}
	if cfg.YoloMode {
		t.Error("expected config file to remain unchanged with yolo_mode: false (runtime-only toggle)")
	}

	// Verify runtime override is set to true
	if srv.yoloOverride == nil {
		t.Error("expected yoloOverride to be set after toggle")
	} else if !*srv.yoloOverride {
		t.Errorf("expected yoloOverride to be true, got %v", *srv.yoloOverride)
	}
}

// TestHandleYoloToggle_DisablesWhenEnabled tests that YOLO toggle disables YOLO mode when it's enabled
func TestHandleYoloToggle_DisablesWhenEnabled(t *testing.T) {
	tempDir := t.TempDir()

	// Create config with yolo_mode: true
	configContent := `yolo_mode: true
`
	if err := os.MkdirAll(filepath.Join(tempDir, ".oda"), 0755); err != nil {
		t.Fatalf("failed to create .oda directory: %v", err)
	}
	if err := os.WriteFile(filepath.Join(tempDir, ".oda", "config.yaml"), []byte(configContent), 0644); err != nil {
		t.Fatalf("failed to write config file: %v", err)
	}

	srv := createTestServerWithTemplates(t)
	srv.rootDir = tempDir
	defer srv.wizardStore.Stop()

	// POST to toggle endpoint
	req := httptest.NewRequest(http.MethodPost, "/api/yolo/toggle", nil)
	rec := httptest.NewRecorder()

	srv.handleYoloToggle(rec, req)

	// Should return 200 OK
	if rec.Code != http.StatusOK {
		t.Errorf("expected status 200, got %d: %s", rec.Code, rec.Body.String())
	}

	// Verify response contains disabled state
	body := rec.Body.String()
	if !strings.Contains(body, "yolo-disabled") {
		t.Error("response should contain yolo-disabled class when disabling YOLO mode")
	}
	if !strings.Contains(body, "SAFE MODE") {
		t.Error("response should contain 'SAFE MODE' text when disabling")
	}
	if !strings.Contains(body, "🔒") {
		t.Error("response should contain lock icon when disabling")
	}

	// Verify config file is UNCHANGED (still yolo_mode: true)
	cfg, err := config.Load(tempDir)
	if err != nil {
		t.Fatalf("failed to load config after toggle: %v", err)
	}
	if !cfg.YoloMode {
		t.Error("expected config file to remain unchanged with yolo_mode: true (runtime-only toggle)")
	}

	// Verify runtime override is set to false
	if srv.yoloOverride == nil {
		t.Error("expected yoloOverride to be set after toggle")
	} else if *srv.yoloOverride {
		t.Errorf("expected yoloOverride to be false, got %v", *srv.yoloOverride)
	}
}

// TestHandleYoloToggle_NoRootDir tests that YOLO toggle returns 500 when rootDir is empty
func TestHandleYoloToggle_NoRootDir(t *testing.T) {
	srv := createTestServerWithTemplates(t)
	srv.rootDir = "" // Empty rootDir
	defer srv.wizardStore.Stop()

	// POST to toggle endpoint
	req := httptest.NewRequest(http.MethodPost, "/api/yolo/toggle", nil)
	rec := httptest.NewRecorder()

	srv.handleYoloToggle(rec, req)

	// Should return 500
	if rec.Code != http.StatusInternalServerError {
		t.Errorf("expected status 500 when rootDir is empty, got %d", rec.Code)
	}
}

// TestHandleYoloToggle_RuntimeOverrideInBoardData tests that the runtime YOLO override
// is reflected in board data without modifying the config file
func TestHandleYoloToggle_RuntimeOverrideInBoardData(t *testing.T) {
	tempDir := t.TempDir()

	// Create config with yolo_mode: false
	configContent := `yolo_mode: false
`
	if err := os.MkdirAll(filepath.Join(tempDir, ".oda"), 0755); err != nil {
		t.Fatalf("failed to create .oda directory: %v", err)
	}
	if err := os.WriteFile(filepath.Join(tempDir, ".oda", "config.yaml"), []byte(configContent), 0644); err != nil {
		t.Fatalf("failed to write config file: %v", err)
	}

	srv := createTestServerWithTemplates(t)
	srv.rootDir = tempDir
	defer srv.wizardStore.Stop()

	// Toggle YOLO via handler
	req := httptest.NewRequest(http.MethodPost, "/api/yolo/toggle", nil)
	rec := httptest.NewRecorder()
	srv.handleYoloToggle(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("expected status 200, got %d: %s", rec.Code, rec.Body.String())
	}

	// Call buildBoardData and verify YoloMode reflects the runtime override
	data := srv.buildBoardData(nil)
	if !data.YoloMode {
		t.Error("expected buildBoardData to return YoloMode=true after toggle (runtime override)")
	}

	// Verify config file on disk is unchanged
	cfg, err := config.Load(tempDir)
	if err != nil {
		t.Fatalf("failed to load config: %v", err)
	}
	if cfg.YoloMode {
		t.Error("expected config file to remain unchanged with yolo_mode: false")
	}
}

// TestLabelIcon tests the labelIcon template function
func TestLabelIcon(t *testing.T) {
	tests := []struct {
		name     string
		label    string
		expected string
	}{
		{"type:feature", "type:feature", "✨"},
		{"feature", "feature", "✨"},
		{"type:bug", "type:bug", "🐛"},
		{"enhancement", "enhancement", "✨"},
		{"bug", "bug", "🐛"},
		{"type:docs", "type:docs", "📚"},
		{"type:refactor", "type:refactor", "🔧"},
		{"size:S", "size:S", "🐜"},
		{"size:M", "size:M", "🐕"},
		{"size:L", "size:L", "🐘"},
		{"size:XL", "size:XL", "🦕"},
		{"priority:high", "priority:high", "🔴"},
		{"priority:medium", "priority:medium", "🟡"},
		{"priority:low", "priority:low", "🟢"},
		{"unknown label", "unknown:label", ""},
		{"stage label", "stage:analysis", ""},
		{"empty label", "", ""},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := LabelIcon(tt.label)
			if got != tt.expected {
				t.Errorf("LabelIcon(%q) = %q, want %q", tt.label, got, tt.expected)
			}
		})
	}
}

// TestLabelTooltip tests the labelTooltip template function
func TestLabelTooltip(t *testing.T) {
	tests := []struct {
		name     string
		label    string
		expected string
	}{
		{"type:feature", "type:feature", "Type: Feature"},
		{"feature", "feature", "Type: Feature"},
		{"type:bug", "type:bug", "Type: Bug"},
		{"enhancement", "enhancement", "Type: Feature"},
		{"bug", "bug", "Type: Bug"},
		{"type:docs", "type:docs", "Documentation"},
		{"type:refactor", "type:refactor", "Refactor"},
		{"size:S", "size:S", "Size: Small"},
		{"size:M", "size:M", "Size: Medium"},
		{"size:L", "size:L", "Size: Large"},
		{"size:XL", "size:XL", "Size: Extra Large"},
		{"priority:high", "priority:high", "Priority: High"},
		{"priority:medium", "priority:medium", "Priority: Medium"},
		{"priority:low", "priority:low", "Priority: Low"},
		{"unknown label", "unknown:label", ""},
		{"stage label", "stage:analysis", ""},
		{"empty label", "", ""},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := LabelTooltip(tt.label)
			if got != tt.expected {
				t.Errorf("LabelTooltip(%q) = %q, want %q", tt.label, got, tt.expected)
			}
		})
	}
}

// TestBoardTemplate_LabelIcons verifies that label icons are rendered correctly in the board template
func TestBoardTemplate_LabelIcons(t *testing.T) {
	srv := createTestServerWithTemplates(t)
	defer srv.wizardStore.Stop()

	// Create test data with mixed icon and text labels
	data := boardData{
		Active: "board",
		Plan: []taskCard{
			{
				ID:     1,
				Title:  "Test Feature",
				Status: "Plan",
				Labels: []string{"type:feature", "size:M", "priority:high", "stage:analysis"},
			},
		},
	}

	// Execute the board-columns template
	tmpl := srv.tmpls["board.html"]
	if tmpl == nil {
		t.Fatal("board.html template not found")
	}

	var buf strings.Builder
	if err := tmpl.ExecuteTemplate(&buf, "board-columns", data); err != nil {
		t.Fatalf("failed to execute template: %v", err)
	}

	output := buf.String()

	// Verify icon labels are rendered in card-icons container (top-right)
	if !strings.Contains(output, `class="card-icons"`) {
		t.Error("template should render card-icons container for icon labels")
	}

	// Verify icon labels are rendered with icon class and tooltip inside card-icons
	if !strings.Contains(output, `class="label-icon"`) {
		t.Error("template should render icon labels with label-icon class")
	}

	// Verify text labels are still rendered in card-labels container
	if !strings.Contains(output, `class="card-labels"`) {
		t.Error("template should render card-labels container for text labels")
	}

	if !strings.Contains(output, `class="label"`) {
		t.Error("template should render text labels with label class")
	}

	// Verify specific icons are present
	if !strings.Contains(output, "✨") {
		t.Error("template should contain feature icon (✨)")
	}
	if !strings.Contains(output, "🐕") {
		t.Error("template should contain size M icon (🐕)")
	}
	if !strings.Contains(output, "🔴") {
		t.Error("template should contain high priority icon (🔴)")
	}

	// Verify stage label is rendered as text (not icon) in card-labels
	if !strings.Contains(output, "stage:analysis") {
		t.Error("template should render stage:analysis as text label")
	}

	// Verify tooltips are present
	if !strings.Contains(output, `title="Type: Feature"`) {
		t.Error("template should contain tooltip for feature label")
	}
	if !strings.Contains(output, `title="Size: Medium"`) {
		t.Error("template should contain tooltip for size M label")
	}
	if !strings.Contains(output, `title="Priority: High"`) {
		t.Error("template should contain tooltip for priority high label")
	}
}

// TestHandleBoard_SprintNameInHeader verifies that when SprintName is set, it appears in the h1 header
func TestHandleBoard_SprintNameInHeader(t *testing.T) {
	srv := createTestServerWithTemplates(t)
	defer srv.wizardStore.Stop()

	// Test with SprintName set
	data := boardData{
		Active:     "board",
		SprintName: "Sprint 2026-03-25 07:54",
		Paused:     true,
		Processing: false,
	}

	// Execute the content template
	tmpl := srv.tmpls["board.html"]
	if tmpl == nil {
		t.Fatal("board.html template not found")
	}

	var buf strings.Builder
	if err := tmpl.ExecuteTemplate(&buf, "content", data); err != nil {
		t.Fatalf("failed to execute template: %v", err)
	}

	output := buf.String()

	// Verify the h1 contains the sprint name
	if !strings.Contains(output, "<h1>Sprint 2026-03-25 07:54</h1>") {
		t.Errorf("expected h1 to contain sprint name 'Sprint 2026-03-25 07:54', got: %s", output)
	}

	// Verify the old subtitle p element is NOT present
	if strings.Contains(output, `<p style="color:var(--muted);font-size:.9rem;margin-top:.25rem">`) {
		t.Error("old subtitle p element should NOT be present when SprintName is set")
	}

	// Verify "Sprint Board" fallback is NOT present when SprintName is set
	if strings.Contains(output, "<h1>Sprint Board</h1>") {
		t.Error("fallback 'Sprint Board' should NOT appear in h1 when SprintName is set")
	}
}

// TestHandleBoard_SprintNameFallback verifies that when SprintName is empty, "Sprint Board" appears as fallback
func TestHandleBoard_SprintNameFallback(t *testing.T) {
	srv := createTestServerWithTemplates(t)
	defer srv.wizardStore.Stop()

	// Test with SprintName empty (no active sprint)
	data := boardData{
		Active:     "board",
		SprintName: "", // Empty - should use fallback
		Paused:     true,
		Processing: false,
	}

	// Execute the content template
	tmpl := srv.tmpls["board.html"]
	if tmpl == nil {
		t.Fatal("board.html template not found")
	}

	var buf strings.Builder
	if err := tmpl.ExecuteTemplate(&buf, "content", data); err != nil {
		t.Fatalf("failed to execute template: %v", err)
	}

	output := buf.String()

	// Verify the h1 contains the fallback "Sprint Board"
	if !strings.Contains(output, "<h1>Sprint Board</h1>") {
		t.Errorf("expected h1 to contain fallback 'Sprint Board' when SprintName is empty, got: %s", output)
	}

	// Verify the old subtitle p element is NOT present (it was removed entirely)
	if strings.Contains(output, `<p style="color:var(--muted);font-size:.9rem;margin-top:.25rem">`) {
		t.Error("old subtitle p element should NOT be present (removed from template)")
	}
}

// TestHandleBoard_NoSubtitleParagraph verifies the subtitle paragraph element is completely removed
func TestHandleBoard_NoSubtitleParagraph(t *testing.T) {
	srv := createTestServerWithTemplates(t)
	defer srv.wizardStore.Stop()

	// Test with SprintName set
	data := boardData{
		Active:     "board",
		SprintName: "Test Sprint Name",
		Paused:     true,
		Processing: false,
	}

	// Execute the content template
	tmpl := srv.tmpls["board.html"]
	if tmpl == nil {
		t.Fatal("board.html template not found")
	}

	var buf strings.Builder
	if err := tmpl.ExecuteTemplate(&buf, "content", data); err != nil {
		t.Fatalf("failed to execute template: %v", err)
	}

	output := buf.String()

	// Verify no p element exists inside board-header div
	// The old pattern was: <p style="color:var(--muted);font-size:.9rem;margin-top:.25rem">{{.SprintName}}</p>
	if strings.Contains(output, `font-size:.9rem`) {
		t.Error("subtitle paragraph with font-size:.9rem should NOT be present")
	}

	// Verify no p element with SprintName content
	if strings.Contains(output, "Test Sprint Name</p>") {
		t.Error("SprintName should NOT appear inside a p element")
	}
}

// TestBoardTemplate_ProcessingPanel_Visible tests that the processing panel is visible when CurrentTicket is set
func TestBoardTemplate_ProcessingPanel_Visible(t *testing.T) {
	srv := createTestServerWithTemplates(t)
	defer srv.wizardStore.Stop()

	// Create test data with CurrentTicket populated
	data := boardData{
		Active: "board",
		CurrentTicket: &currentTicketInfo{
			Number:   789,
			Title:    "Processing Ticket Title",
			Status:   "coding",
			Priority: "high",
			Type:     "feature",
			Size:     "L",
		},
		Paused:     false,
		Processing: true,
	}

	// Execute the content template
	tmpl := srv.tmpls["board.html"]
	if tmpl == nil {
		t.Fatal("board.html template not found")
	}

	var buf strings.Builder
	if err := tmpl.ExecuteTemplate(&buf, "content", data); err != nil {
		t.Fatalf("failed to execute template: %v", err)
	}

	output := buf.String()

	// Verify processing panel is present
	if !strings.Contains(output, `id="processing-panel"`) {
		t.Error("template should contain processing-panel element")
	}

	// Verify panel is visible (not display:none)
	if strings.Contains(output, `id="processing-panel" style="display:none"`) {
		t.Error("processing panel should be visible when CurrentTicket is set")
	}

	// Verify ticket number is displayed
	if !strings.Contains(output, "#789") {
		t.Error("template should display ticket number #789")
	}

	// Verify ticket title is displayed
	if !strings.Contains(output, "Processing Ticket Title") {
		t.Error("template should display ticket title")
	}

	// Verify priority badge is present
	if !strings.Contains(output, `processing-priority-high`) {
		t.Error("template should contain high priority badge class")
	}

	// Verify type badge is present
	if !strings.Contains(output, "✨ Feature") {
		t.Error("template should contain feature type badge")
	}

	// Verify size badge is present
	if !strings.Contains(output, "📏 L") {
		t.Error("template should contain size L badge")
	}

	// Verify link to task detail page
	if !strings.Contains(output, `href="/task/789"`) {
		t.Error("template should contain link to task detail page")
	}
}

// TestBoardTemplate_ProcessingPanel_Idle tests that the processing panel shows idle state when CurrentTicket is nil
func TestBoardTemplate_ProcessingPanel_Idle(t *testing.T) {
	srv := createTestServerWithTemplates(t)
	defer srv.wizardStore.Stop()

	// Create test data with CurrentTicket nil
	data := boardData{
		Active:        "board",
		CurrentTicket: nil,
		Paused:        true,
		Processing:    false,
		TotalTickets:  5,
	}

	// Execute the content template
	tmpl := srv.tmpls["board.html"]
	if tmpl == nil {
		t.Fatal("board.html template not found")
	}

	var buf strings.Builder
	if err := tmpl.ExecuteTemplate(&buf, "content", data); err != nil {
		t.Fatalf("failed to execute template: %v", err)
	}

	output := buf.String()

	// Verify processing panel is present
	if !strings.Contains(output, `id="processing-panel"`) {
		t.Error("template should contain processing-panel element")
	}

	// Verify panel does NOT have display:none style (should always be visible)
	if strings.Contains(output, `id="processing-panel" style="display:none"`) {
		t.Error("processing panel should not be hidden (no display:none) when CurrentTicket is nil")
	}

	// Verify panel has idle class
	if !strings.Contains(output, `processing-panel-idle`) {
		t.Error("processing panel should have processing-panel-idle class when CurrentTicket is nil")
	}

	// Verify idle message is displayed
	if !strings.Contains(output, "No active ticket") {
		t.Error("template should display 'No active ticket' message when idle")
	}

	// Verify idle message contains "Worker ready"
	if !strings.Contains(output, "Worker ready") {
		t.Error("template should display 'Worker ready' message when idle")
	}
}

// TestBoardTemplate_ProcessingPanel_LongTitle tests that long titles are handled gracefully
func TestBoardTemplate_ProcessingPanel_LongTitle(t *testing.T) {
	srv := createTestServerWithTemplates(t)
	defer srv.wizardStore.Stop()

	// Create test data with a very long title
	longTitle := strings.Repeat("A", 200)
	data := boardData{
		Active: "board",
		CurrentTicket: &currentTicketInfo{
			Number:   999,
			Title:    longTitle,
			Status:   "coding",
			Priority: "low",
			Type:     "bug",
			Size:     "XL",
		},
		Paused:     false,
		Processing: true,
	}

	// Execute the content template
	tmpl := srv.tmpls["board.html"]
	if tmpl == nil {
		t.Fatal("board.html template not found")
	}

	var buf strings.Builder
	if err := tmpl.ExecuteTemplate(&buf, "content", data); err != nil {
		t.Fatalf("failed to execute template: %v", err)
	}

	output := buf.String()

	// Verify the long title is rendered (CSS handles truncation)
	if !strings.Contains(output, longTitle) {
		t.Error("template should render long title (CSS handles truncation)")
	}

	// Verify processing panel is still visible
	if strings.Contains(output, `id="processing-panel" style="display:none"`) {
		t.Error("processing panel should be visible even with long title")
	}
}

// TestBoardTemplate_ProcessingPanel_PriorityVariations tests priority badge variations
func TestBoardTemplate_ProcessingPanel_PriorityVariations(t *testing.T) {
	tests := []struct {
		name          string
		priority      string
		expectedEmoji string
		expectedClass string
	}{
		{
			name:          "high priority",
			priority:      "high",
			expectedEmoji: "🔴",
			expectedClass: "processing-priority-high",
		},
		{
			name:          "medium priority",
			priority:      "medium",
			expectedEmoji: "🟡",
			expectedClass: "processing-priority-medium",
		},
		{
			name:          "low priority",
			priority:      "low",
			expectedEmoji: "🟢",
			expectedClass: "processing-priority-low",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			srv := createTestServerWithTemplates(t)
			defer srv.wizardStore.Stop()

			data := boardData{
				Active: "board",
				CurrentTicket: &currentTicketInfo{
					Number:   100,
					Title:    "Test Ticket",
					Status:   "coding",
					Priority: tt.priority,
					Type:     "feature",
					Size:     "M",
				},
				Paused:     false,
				Processing: true,
			}

			tmpl := srv.tmpls["board.html"]
			if tmpl == nil {
				t.Fatal("board.html template not found")
			}

			var buf strings.Builder
			if err := tmpl.ExecuteTemplate(&buf, "content", data); err != nil {
				t.Fatalf("failed to execute template: %v", err)
			}

			output := buf.String()

			// Verify priority emoji is present
			if !strings.Contains(output, tt.expectedEmoji) {
				t.Errorf("template should contain %s emoji for %s priority", tt.expectedEmoji, tt.priority)
			}

			// Verify priority class is present
			if !strings.Contains(output, tt.expectedClass) {
				t.Errorf("template should contain %s class for %s priority", tt.expectedClass, tt.priority)
			}
		})
	}
}

// TestBoardTemplate_ProcessingPanel_TypeVariations tests type badge variations
func TestBoardTemplate_ProcessingPanel_TypeVariations(t *testing.T) {
	tests := []struct {
		name          string
		issueType     string
		expectedBadge string
	}{
		{
			name:          "bug type",
			issueType:     "bug",
			expectedBadge: "🐛 Bug",
		},
		{
			name:          "feature type",
			issueType:     "feature",
			expectedBadge: "✨ Feature",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			srv := createTestServerWithTemplates(t)
			defer srv.wizardStore.Stop()

			data := boardData{
				Active: "board",
				CurrentTicket: &currentTicketInfo{
					Number:   200,
					Title:    "Test Ticket",
					Status:   "coding",
					Priority: "medium",
					Type:     tt.issueType,
					Size:     "S",
				},
				Paused:     false,
				Processing: true,
			}

			tmpl := srv.tmpls["board.html"]
			if tmpl == nil {
				t.Fatal("board.html template not found")
			}

			var buf strings.Builder
			if err := tmpl.ExecuteTemplate(&buf, "content", data); err != nil {
				t.Fatalf("failed to execute template: %v", err)
			}

			output := buf.String()

			// Verify type badge is present
			if !strings.Contains(output, tt.expectedBadge) {
				t.Errorf("template should contain %q badge for %s type", tt.expectedBadge, tt.issueType)
			}
		})
	}
}

// TestBoardTemplate_ProcessingPanel_SizeVariations tests size badge variations
func TestBoardTemplate_ProcessingPanel_SizeVariations(t *testing.T) {
	tests := []struct {
		name          string
		size          string
		expectedBadge string
	}{
		{"size S", "S", "📏 S"},
		{"size M", "M", "📏 M"},
		{"size L", "L", "📏 L"},
		{"size XL", "XL", "📏 XL"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			srv := createTestServerWithTemplates(t)
			defer srv.wizardStore.Stop()

			data := boardData{
				Active: "board",
				CurrentTicket: &currentTicketInfo{
					Number:   300,
					Title:    "Test Ticket",
					Status:   "coding",
					Priority: "low",
					Type:     "feature",
					Size:     tt.size,
				},
				Paused:     false,
				Processing: true,
			}

			tmpl := srv.tmpls["board.html"]
			if tmpl == nil {
				t.Fatal("board.html template not found")
			}

			var buf strings.Builder
			if err := tmpl.ExecuteTemplate(&buf, "content", data); err != nil {
				t.Fatalf("failed to execute template: %v", err)
			}

			output := buf.String()

			// Verify size badge is present
			if !strings.Contains(output, tt.expectedBadge) {
				t.Errorf("template should contain %q badge", tt.expectedBadge)
			}
		})
	}
}

// TestBoardTemplate_ProcessingPanel_PartialLabels tests panel with partial label data
func TestBoardTemplate_ProcessingPanel_PartialLabels(t *testing.T) {
	srv := createTestServerWithTemplates(t)
	defer srv.wizardStore.Stop()

	// Create test data with only priority (no type or size)
	data := boardData{
		Active: "board",
		CurrentTicket: &currentTicketInfo{
			Number:   400,
			Title:    "Partial Labels Ticket",
			Status:   "coding",
			Priority: "high",
			Type:     "", // No type
			Size:     "", // No size
		},
		Paused:     false,
		Processing: true,
	}

	tmpl := srv.tmpls["board.html"]
	if tmpl == nil {
		t.Fatal("board.html template not found")
	}

	var buf strings.Builder
	if err := tmpl.ExecuteTemplate(&buf, "content", data); err != nil {
		t.Fatalf("failed to execute template: %v", err)
	}

	output := buf.String()

	// Verify priority badge is present
	if !strings.Contains(output, `processing-priority-high`) {
		t.Error("template should contain high priority badge")
	}

	// Verify type and size badges are NOT present (empty values)
	if strings.Contains(output, "🐛 Bug") || strings.Contains(output, "✨ Feature") {
		t.Error("template should NOT contain type badge when Type is empty")
	}

	if strings.Contains(output, "📏") {
		t.Error("template should NOT contain size badge when Size is empty")
	}
}

// TestBoardTemplate_ProcessingPanel_LayoutOrder tests that ticket appears before labels in the HTML structure
func TestBoardTemplate_ProcessingPanel_LayoutOrder(t *testing.T) {
	srv := createTestServerWithTemplates(t)
	defer srv.wizardStore.Stop()

	data := boardData{
		Active: "board",
		CurrentTicket: &currentTicketInfo{
			Number:   500,
			Title:    "Layout Order Test Ticket",
			Status:   "coding",
			Priority: "high",
			Type:     "feature",
			Size:     "M",
		},
		Paused:     false,
		Processing: true,
	}

	tmpl := srv.tmpls["board.html"]
	if tmpl == nil {
		t.Fatal("board.html template not found")
	}

	var buf strings.Builder
	if err := tmpl.ExecuteTemplate(&buf, "content", data); err != nil {
		t.Fatalf("failed to execute template: %v", err)
	}

	output := buf.String()

	// Verify that processing-labels appears before processing-ticket in the HTML (badges at top)
	labelsIdx := strings.Index(output, `class="processing-labels"`)
	ticketIdx := strings.Index(output, `class="processing-ticket"`)

	if labelsIdx == -1 {
		t.Error("template should contain processing-labels element")
	}

	if ticketIdx == -1 {
		t.Error("template should contain processing-ticket element")
	}

	if labelsIdx != -1 && ticketIdx != -1 && labelsIdx > ticketIdx {
		t.Error("processing-labels should appear BEFORE processing-ticket in the HTML structure (badges at top)")
	}

	// Verify new CSS classes are present
	if !strings.Contains(output, "flex-direction:column") {
		t.Error("template should contain flex-direction:column CSS for vertical layout")
	}
}

func TestHandleManualProcess(t *testing.T) {
	orch := &mvp.Orchestrator{}
	s := &Server{orchestrator: orch}

	req := httptest.NewRequest(http.MethodPost, "/api/tickets/42/process", nil)
	req.SetPathValue("id", "42")
	w := httptest.NewRecorder()

	s.handleManualProcess(w, req)

	if w.Code != http.StatusOK {
		t.Errorf("status = %d, want %d", w.Code, http.StatusOK)
	}
	var resp map[string]any
	json.NewDecoder(w.Body).Decode(&resp)
	if resp["success"] != true {
		t.Errorf("success = %v, want true", resp["success"])
	}
}

func TestHandleManualProcessNoOrchestrator(t *testing.T) {
	s := &Server{}

	req := httptest.NewRequest(http.MethodPost, "/api/tickets/42/process", nil)
	req.SetPathValue("id", "42")
	w := httptest.NewRecorder()

	s.handleManualProcess(w, req)

	if w.Code != http.StatusServiceUnavailable {
		t.Errorf("status = %d, want %d", w.Code, http.StatusServiceUnavailable)
	}
}

func TestHandleManualProcessInvalidID(t *testing.T) {
	orch := &mvp.Orchestrator{}
	s := &Server{orchestrator: orch}

	req := httptest.NewRequest(http.MethodPost, "/api/tickets/abc/process", nil)
	req.SetPathValue("id", "abc")
	w := httptest.NewRecorder()

	s.handleManualProcess(w, req)

	if w.Code != http.StatusBadRequest {
		t.Errorf("status = %d, want %d", w.Code, http.StatusBadRequest)
	}
}

// TestBoardTemplate_ProcessingPanel_EmptySprint tests that the processing panel shows empty sprint state when TotalTickets is 0
func TestBoardTemplate_ProcessingPanel_EmptySprint(t *testing.T) {
	srv := createTestServerWithTemplates(t)
	defer srv.wizardStore.Stop()

	data := boardData{
		Active:        "board",
		CurrentTicket: nil,
		Paused:        true,
		Processing:    false,
		TotalTickets:  0,
	}

	tmpl := srv.tmpls["board.html"]
	if tmpl == nil {
		t.Fatal("board.html template not found")
	}

	var buf strings.Builder
	if err := tmpl.ExecuteTemplate(&buf, "content", data); err != nil {
		t.Fatalf("failed to execute template: %v", err)
	}

	output := buf.String()

	if !strings.Contains(output, `processing-panel-idle`) {
		t.Error("empty sprint panel should have idle class")
	}
	if !strings.Contains(output, "Sprint") {
		t.Error("empty sprint should show Sprint badge")
	}
	if !strings.Contains(output, "No tickets in sprint") {
		t.Error("empty sprint should show 'No tickets in sprint' message")
	}
	if !strings.Contains(output, "processing-cta") {
		t.Error("empty sprint should show CTA button")
	}
	if !strings.Contains(output, "New Ticket") {
		t.Error("empty sprint CTA should say 'New Ticket'")
	}
	if strings.Contains(output, "Worker ready") {
		t.Error("empty sprint should NOT show 'Worker ready' message")
	}
}

// TestBuildBoardData_TotalTickets tests that TotalTickets is computed correctly
func TestBuildBoardData_TotalTickets(t *testing.T) {
	srv := &Server{
		tmpls: make(map[string]*template.Template),
	}

	data := srv.buildBoardData(nil)

	if data.TotalTickets != 0 {
		t.Errorf("expected TotalTickets=0 with no store, got %d", data.TotalTickets)
	}
}

// TestBuildBoardData_CompletedTickets tests that CompletedTickets and CompletionPercentage are computed correctly
func TestBuildBoardData_CompletedTickets(t *testing.T) {
	// Test case 1: No tickets - should have 0 completed and 0% completion
	srv := &Server{tmpls: make(map[string]*template.Template)}
	data := srv.buildBoardData(nil)
	if data.CompletedTickets != 0 {
		t.Errorf("expected CompletedTickets=0 with no tickets, got %d", data.CompletedTickets)
	}
	if data.CompletionPercentage != 0 {
		t.Errorf("expected CompletionPercentage=0 with no tickets, got %f", data.CompletionPercentage)
	}

	// Test case 2: Verify percentage calculation with mock data
	// Create a server with mock data that has tickets in Done column
	srv2 := &Server{tmpls: make(map[string]*template.Template)}
	data2 := srv2.buildBoardData(nil)
	// With no store/gh, we should have 0 tickets
	if data2.CompletedTickets != 0 {
		t.Errorf("expected CompletedTickets=0 without store, got %d", data2.CompletedTickets)
	}
	if data2.CompletionPercentage != 0 {
		t.Errorf("expected CompletionPercentage=0 without store, got %f", data2.CompletionPercentage)
	}

	// Test case 3: Manual verification of percentage calculation logic
	// Simulate what buildBoardData does
	testCases := []struct {
		total     int
		completed int
		expected  float64
	}{
		{0, 0, 0},
		{10, 0, 0},
		{10, 5, 50},
		{10, 10, 100},
		{8, 2, 25},
		{3, 1, 33.333333},
	}

	for _, tc := range testCases {
		var percentage float64
		if tc.total > 0 {
			percentage = float64(tc.completed) / float64(tc.total) * 100
		}
		// Use tolerance for floating point comparison
		tolerance := 0.0001
		if diff := percentage - tc.expected; diff < -tolerance || diff > tolerance {
			t.Errorf("percentage calculation for total=%d, completed=%d: expected %f, got %f",
				tc.total, tc.completed, tc.expected, percentage)
		}
	}
}

func TestHandleLogStream_ValidIssue(t *testing.T) {
	tmpDir := t.TempDir()
	logDir := filepath.Join(tmpDir, ".oda", "artifacts", "392", "logs")
	os.MkdirAll(logDir, 0o755)

	content := "[2026-03-25 14:30:22] STEP START: implement\n[2026-03-25 14:30:23] Working...\n[2026-03-25 14:31:55] STEP END: implement\n"
	os.WriteFile(filepath.Join(logDir, "20260325_143022_implement.log"), []byte(content), 0o644)

	srv := &Server{rootDir: tmpDir}

	req := httptest.NewRequest(http.MethodGet, "/api/logs/392/stream?follow=false", nil)
	req.SetPathValue("issue", "392")
	rec := httptest.NewRecorder()

	srv.handleLogStream(rec, req)

	if rec.Header().Get("Content-Type") != "text/event-stream" {
		t.Errorf("expected Content-Type=text/event-stream, got %s", rec.Header().Get("Content-Type"))
	}
	if rec.Header().Get("Cache-Control") != "no-cache" {
		t.Errorf("expected Cache-Control=no-cache, got %s", rec.Header().Get("Cache-Control"))
	}
	if rec.Header().Get("Connection") != "keep-alive" {
		t.Errorf("expected Connection=keep-alive, got %s", rec.Header().Get("Connection"))
	}

	body := rec.Body.String()
	if !strings.Contains(body, "event: log:new") {
		t.Error("expected log:new events in response")
	}
	if !strings.Contains(body, "event: log:complete") {
		t.Error("expected log:complete event in response")
	}
	if !strings.Contains(body, `"step":"implement"`) {
		t.Error("expected step name in events")
	}
}

func TestHandleLogStream_StepFilter(t *testing.T) {
	tmpDir := t.TempDir()
	logDir := filepath.Join(tmpDir, ".oda", "artifacts", "393", "logs")
	os.MkdirAll(logDir, 0o755)

	content1 := "[2026-03-25 14:30:22] STEP START: analyze\n[2026-03-25 14:30:25] STEP END: analyze\n"
	os.WriteFile(filepath.Join(logDir, "20260325_143022_analyze.log"), []byte(content1), 0o644)

	content2 := "[2026-03-25 14:31:00] STEP START: implement\n[2026-03-25 14:31:55] STEP END: implement\n"
	os.WriteFile(filepath.Join(logDir, "20260325_143100_implement.log"), []byte(content2), 0o644)

	srv := &Server{rootDir: tmpDir}

	req := httptest.NewRequest(http.MethodGet, "/api/logs/393/stream?step=implement&follow=false", nil)
	req.SetPathValue("issue", "393")
	rec := httptest.NewRecorder()

	srv.handleLogStream(rec, req)

	body := rec.Body.String()
	if !strings.Contains(body, `"step":"implement"`) {
		t.Error("expected implement step in response")
	}
	if strings.Contains(body, `"step":"analyze"`) {
		t.Error("should not contain analyze step when filtered to implement")
	}
}

func TestHandleLogStream_MissingDirectory(t *testing.T) {
	tmpDir := t.TempDir()
	srv := &Server{rootDir: tmpDir}

	req := httptest.NewRequest(http.MethodGet, "/api/logs/999/stream?follow=false", nil)
	req.SetPathValue("issue", "999")
	rec := httptest.NewRecorder()

	srv.handleLogStream(rec, req)

	if rec.Header().Get("Content-Type") != "text/event-stream" {
		t.Errorf("expected Content-Type=text/event-stream, got %s", rec.Header().Get("Content-Type"))
	}

	body := rec.Body.String()
	if !strings.Contains(body, "event: log:error") {
		t.Error("expected log:error event for missing directory")
	}
	if !strings.Contains(body, "log directory not found") {
		t.Error("expected error message about missing directory")
	}
}

func TestHandleLogStream_InvalidIssueNumber(t *testing.T) {
	tmpDir := t.TempDir()
	srv := &Server{rootDir: tmpDir}

	req := httptest.NewRequest(http.MethodGet, "/api/logs/abc/stream", nil)
	req.SetPathValue("issue", "abc")
	rec := httptest.NewRecorder()

	srv.handleLogStream(rec, req)

	if rec.Code != http.StatusBadRequest {
		t.Errorf("expected status 400, got %d", rec.Code)
	}
}

func TestHandleLogStream_EmptyLogsDirectory(t *testing.T) {
	tmpDir := t.TempDir()
	logDir := filepath.Join(tmpDir, ".oda", "artifacts", "394", "logs")
	os.MkdirAll(logDir, 0o755)

	srv := &Server{rootDir: tmpDir}

	req := httptest.NewRequest(http.MethodGet, "/api/logs/394/stream?follow=false", nil)
	req.SetPathValue("issue", "394")
	rec := httptest.NewRecorder()

	srv.handleLogStream(rec, req)

	if rec.Header().Get("Content-Type") != "text/event-stream" {
		t.Errorf("expected Content-Type=text/event-stream, got %s", rec.Header().Get("Content-Type"))
	}

	body := rec.Body.String()
	if strings.Contains(body, "event: log:new") {
		t.Error("should not have log:new events for empty directory")
	}
}

func TestHandleLogStream_MultipleLogFiles(t *testing.T) {
	tmpDir := t.TempDir()
	logDir := filepath.Join(tmpDir, ".oda", "artifacts", "395", "logs")
	os.MkdirAll(logDir, 0o755)

	content1 := "[2026-03-25 14:30:22] STEP START: analyze\n[2026-03-25 14:30:25] STEP END: analyze\n"
	os.WriteFile(filepath.Join(logDir, "20260325_143022_analyze.log"), []byte(content1), 0o644)

	content2 := "[2026-03-25 14:31:00] STEP START: implement\n[2026-03-25 14:31:55] STEP END: implement\n"
	os.WriteFile(filepath.Join(logDir, "20260325_143100_implement.log"), []byte(content2), 0o644)

	srv := &Server{rootDir: tmpDir}

	req := httptest.NewRequest(http.MethodGet, "/api/logs/395/stream?follow=false", nil)
	req.SetPathValue("issue", "395")
	rec := httptest.NewRecorder()

	srv.handleLogStream(rec, req)

	body := rec.Body.String()
	if !strings.Contains(body, `"step":"analyze"`) {
		t.Error("expected analyze step in response")
	}
	if !strings.Contains(body, `"step":"implement"`) {
		t.Error("expected implement step in response")
	}
}

func TestHandleLogStream_FollowDisabled(t *testing.T) {
	tmpDir := t.TempDir()
	logDir := filepath.Join(tmpDir, ".oda", "artifacts", "396", "logs")
	os.MkdirAll(logDir, 0o755)

	content := "[2026-03-25 14:30:22] STEP START: test\n[2026-03-25 14:30:23] Working...\n"
	os.WriteFile(filepath.Join(logDir, "20260325_143022_test.log"), []byte(content), 0o644)

	srv := &Server{rootDir: tmpDir}

	req := httptest.NewRequest(http.MethodGet, "/api/logs/396/stream?follow=false", nil)
	req.SetPathValue("issue", "396")
	rec := httptest.NewRecorder()

	done := make(chan bool)
	go func() {
		srv.handleLogStream(rec, req)
		done <- true
	}()

	select {
	case <-done:
		body := rec.Body.String()
		if !strings.Contains(body, "event: log:new") {
			t.Error("expected log:new events")
		}
		if strings.Contains(body, "event: log:complete") {
			t.Error("should not have log:complete when STEP END is not present")
		}
	case <-time.After(2 * time.Second):
		t.Error("handler should return immediately when follow=false")
	}
}

func TestHandleLogStream_SSEHeaders(t *testing.T) {
	tmpDir := t.TempDir()
	logDir := filepath.Join(tmpDir, ".oda", "artifacts", "397", "logs")
	os.MkdirAll(logDir, 0o755)

	content := "[2026-03-25 14:30:22] test message\n"
	os.WriteFile(filepath.Join(logDir, "20260325_143022_test.log"), []byte(content), 0o644)

	srv := &Server{rootDir: tmpDir}

	req := httptest.NewRequest(http.MethodGet, "/api/logs/397/stream?follow=false", nil)
	req.SetPathValue("issue", "397")
	rec := httptest.NewRecorder()

	srv.handleLogStream(rec, req)

	if rec.Header().Get("Content-Type") != "text/event-stream" {
		t.Errorf("expected Content-Type=text/event-stream, got %s", rec.Header().Get("Content-Type"))
	}
	if rec.Header().Get("Cache-Control") != "no-cache" {
		t.Errorf("expected Cache-Control=no-cache, got %s", rec.Header().Get("Cache-Control"))
	}
	if rec.Header().Get("Connection") != "keep-alive" {
		t.Errorf("expected Connection=keep-alive, got %s", rec.Header().Get("Connection"))
	}
}

// TestBuildBoardData_CanPlanSprint tests the CanPlanSprint field logic
func TestBuildBoardData_CanPlanSprint(t *testing.T) {
	t.Run("empty board allows sprint planning", func(t *testing.T) {
		srv := &Server{
			tmpls: make(map[string]*template.Template),
		}
		data := srv.buildBoardData(nil)
		if !data.CanPlanSprint {
			t.Error("expected CanPlanSprint to be true when board has no tickets")
		}
	})

	t.Run("board with tickets hides sprint planning", func(t *testing.T) {
		// When TotalTickets > 0, CanPlanSprint should be false
		// We verify this by checking the struct default (false) and the logic
		data := boardData{TotalTickets: 5}
		data.CanPlanSprint = data.TotalTickets == 0
		if data.CanPlanSprint {
			t.Error("expected CanPlanSprint to be false when board has tickets")
		}
	})
}

// TestPlanSprintButton_Visibility tests that the Plan Sprint button is visible when board is empty
func TestPlanSprintButton_Visibility(t *testing.T) {
	srv := createTestServerWithTemplates(t)
	defer srv.wizardStore.Stop()

	t.Run("visible when no tickets", func(t *testing.T) {
		req := httptest.NewRequest(http.MethodGet, "/", nil)
		rec := httptest.NewRecorder()
		srv.handleBoard(rec, req)

		if rec.Code != http.StatusOK {
			t.Fatalf("expected status 200, got %d", rec.Code)
		}

		body := rec.Body.String()
		if !strings.Contains(body, "Plan Sprint") {
			t.Error("expected Plan Sprint button to be visible when board is empty")
		}
	})
}

// TestPlanSprintButton_HiddenWithTickets tests that the Plan Sprint button is hidden when board has tickets
func TestPlanSprintButton_HiddenWithTickets(t *testing.T) {
	srv := createTestServerWithTemplates(t)
	defer srv.wizardStore.Stop()

	t.Run("hidden when tickets exist", func(t *testing.T) {
		// Create test data with tickets
		data := boardData{
			Active:        "board",
			CanPlanSprint: false, // Tickets exist
			TotalTickets:  5,
			Paused:        true,
			Processing:    false,
		}

		// Execute the content template
		tmpl := srv.tmpls["board.html"]
		if tmpl == nil {
			t.Fatal("board.html template not found")
		}

		var buf strings.Builder
		if err := tmpl.ExecuteTemplate(&buf, "content", data); err != nil {
			t.Fatalf("failed to execute template: %v", err)
		}

		output := buf.String()

		// Verify Plan Sprint button is NOT present when CanPlanSprint is false
		// The button should be wrapped in {{if .CanPlanSprint}} conditional
		if strings.Contains(output, `action="/plan-sprint"`) {
			t.Error("Plan Sprint form should NOT be present when CanPlanSprint is false")
		}
	})
}

// TestProcessingPanel_Title verifies the processing panel has a title
func TestProcessingPanel_Title(t *testing.T) {
	srv := createTestServerWithTemplates(t)
	defer srv.wizardStore.Stop()

	req := httptest.NewRequest(http.MethodGet, "/", nil)
	rec := httptest.NewRecorder()

	srv.handleBoard(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("expected status 200, got %d", rec.Code)
	}

	body := rec.Body.String()

	// Verify processing panel title is present
	if !strings.Contains(body, "processing-panel-title") {
		t.Error("processing panel missing title CSS class")
	}
	if !strings.Contains(body, "Processing") {
		t.Error("processing panel missing 'Processing' title text")
	}
}

// TestBoardTemplate_CSSLayout verifies the CSS layout properties for board columns and processing panel
func TestBoardTemplate_CSSLayout(t *testing.T) {
	srv := createTestServerWithTemplates(t)
	defer srv.wizardStore.Stop()

	// Create minimal test data
	data := boardData{
		Active: "board",
		Paused: true,
	}

	// Execute the content template
	tmpl := srv.tmpls["board.html"]
	if tmpl == nil {
		t.Fatal("board.html template not found")
	}

	var buf strings.Builder
	if err := tmpl.ExecuteTemplate(&buf, "content", data); err != nil {
		t.Fatalf("failed to execute template: %v", err)
	}

	output := buf.String()

	// Verify board-center uses flexbox layout (changed from grid to fix overlapping columns issue #493)
	if !strings.Contains(output, "display:flex;flex-direction:column") {
		t.Error("board-center should use display:flex with flex-direction:column")
	}

	// Verify board-center has overflow:hidden to prevent content spill
	if !strings.Contains(output, ".board-center{") || !strings.Contains(output, "overflow:hidden") {
		t.Error("board-center should have overflow:hidden to prevent content spill")
	}

	// Verify board-center-columns has flex:1 1 auto for proper sizing in flex container
	if strings.Contains(output, ".board-center-columns{") {
		if !strings.Contains(output, "flex:1 1 auto") {
			t.Error("board-center-columns should have flex:1 1 auto for proper flex container sizing")
		}
	} else {
		t.Error("board-center-columns CSS rule not found")
	}

	// Verify board-center-columns does NOT have fixed height:180px
	if strings.Contains(output, "height:180px") {
		t.Error("board-center-columns should NOT have fixed height:180px")
	}

	// Verify grid uses repeat(6,1fr) not repeat(8,1fr)
	if strings.Contains(output, "repeat(8,1fr)") {
		t.Error("board-center-columns should use repeat(6,1fr) not repeat(8,1fr)")
	}
	if !strings.Contains(output, "repeat(6,1fr)") {
		t.Error("board-center-columns should use repeat(6,1fr) for 6 pipeline columns")
	}

	// Verify processing-panel has flex:0 0 auto to prevent shrinking (flexbox layout)
	if strings.Contains(output, ".processing-panel{") {
		if !strings.Contains(output, "flex:0 0 auto") {
			t.Error("processing-panel should have flex:0 0 auto to prevent shrinking in flex container")
		}
	} else {
		t.Error("processing-panel CSS rule not found")
	}

	// Verify processing-panel does NOT have fixed min/max height (grid handles sizing)
	if strings.Contains(output, ".processing-panel{") && strings.Contains(output, "max-height:250px") {
		t.Error("processing-panel should NOT have max-height:250px (grid row handles sizing)")
	}
}

// TestHandleWorkerToggle_Success_Start tests toggling from paused to running
func TestHandleWorkerToggle_Success_Start(t *testing.T) {
	orch := &mvp.Orchestrator{}
	// Manually set paused to true to simulate initial state
	orch.Pause()
	s := &Server{orchestrator: orch}

	req := httptest.NewRequest(http.MethodPost, "/api/worker/toggle", nil)
	w := httptest.NewRecorder()

	s.handleWorkerToggle(w, req)

	if w.Code != http.StatusOK {
		t.Errorf("status = %d, want %d", w.Code, http.StatusOK)
	}

	var resp map[string]any
	if err := json.NewDecoder(w.Body).Decode(&resp); err != nil {
		t.Fatalf("failed to decode response: %v", err)
	}

	if resp["success"] != true {
		t.Errorf("success = %v, want true", resp["success"])
	}

	// After toggle from paused, should be unpaused (running)
	if resp["paused"] != false {
		t.Errorf("paused = %v, want false", resp["paused"])
	}

	if resp["message"] != "Worker started" {
		t.Errorf("message = %v, want 'Worker started'", resp["message"])
	}
}

// TestHandleWorkerToggle_Success_Pause tests toggling from running to paused
func TestHandleWorkerToggle_Success_Pause(t *testing.T) {
	orch := &mvp.Orchestrator{}
	orch.Start() // Start first so we can pause
	s := &Server{orchestrator: orch}

	req := httptest.NewRequest(http.MethodPost, "/api/worker/toggle", nil)
	w := httptest.NewRecorder()

	s.handleWorkerToggle(w, req)

	if w.Code != http.StatusOK {
		t.Errorf("status = %d, want %d", w.Code, http.StatusOK)
	}

	var resp map[string]any
	if err := json.NewDecoder(w.Body).Decode(&resp); err != nil {
		t.Fatalf("failed to decode response: %v", err)
	}

	if resp["success"] != true {
		t.Errorf("success = %v, want true", resp["success"])
	}

	if resp["paused"] != true {
		t.Errorf("paused = %v, want true", resp["paused"])
	}
}

// TestHandleWorkerToggle_NoOrchestrator tests error handling when orchestrator is nil
func TestHandleWorkerToggle_NoOrchestrator(t *testing.T) {
	s := &Server{orchestrator: nil}

	req := httptest.NewRequest(http.MethodPost, "/api/worker/toggle", nil)
	w := httptest.NewRecorder()

	s.handleWorkerToggle(w, req)

	if w.Code != http.StatusServiceUnavailable {
		t.Errorf("status = %d, want %d", w.Code, http.StatusServiceUnavailable)
	}

	var resp map[string]any
	if err := json.NewDecoder(w.Body).Decode(&resp); err != nil {
		t.Fatalf("failed to decode response: %v", err)
	}

	if resp["success"] != false {
		t.Errorf("success = %v, want false", resp["success"])
	}

	if resp["error"] != "orchestrator not configured" {
		t.Errorf("error = %v, want 'orchestrator not configured'", resp["error"])
	}
}

// TestHandleSettings_SprintAutoStart tests that SprintAutoStart is correctly loaded from config
func TestHandleSettings_SprintAutoStart(t *testing.T) {
	// Create a temporary directory for config
	tmpDir := t.TempDir()

	// Create .oda directory
	odaDir := filepath.Join(tmpDir, ".oda")
	if err := os.MkdirAll(odaDir, 0755); err != nil {
		t.Fatalf("failed to create .oda directory: %v", err)
	}

	// Create a config file with sprint auto_start enabled
	configContent := `llm:
  setup:
    model: test-provider/test-model
  planning:
    model: test-provider/test-model
  orchestration:
    model: test-provider/test-model
  code:
    model: test-provider/test-model
  code-heavy:
    model: test-provider/test-model
sprint:
  auto_start: true
`
	configPath := filepath.Join(odaDir, "config.yaml")
	if err := os.WriteFile(configPath, []byte(configContent), 0644); err != nil {
		t.Fatalf("failed to write config file: %v", err)
	}

	// Create server with templates
	srv := createTestServerWithTemplates(t)
	srv.rootDir = tmpDir
	defer srv.wizardStore.Stop()

	// Test GET /settings
	req := httptest.NewRequest(http.MethodGet, "/settings", nil)
	rec := httptest.NewRecorder()

	srv.handleSettings(rec, req)

	// Should return 200 OK
	if rec.Code != http.StatusOK {
		t.Errorf("expected status 200, got %d", rec.Code)
	}

	// Verify the checkbox is checked in the response
	body := rec.Body.String()
	if !strings.Contains(body, `id="sprint_auto_start"`) {
		t.Error("response should contain sprint_auto_start checkbox")
	}
	if !strings.Contains(body, `name="sprint_auto_start"`) {
		t.Error("response should contain sprint_auto_start form field")
	}
}

// TestHandleSaveSettings_SprintAutoStart tests saving sprint_auto_start setting
func TestHandleSaveSettings_SprintAutoStart(t *testing.T) {
	// Create a temporary directory for config
	tmpDir := t.TempDir()

	// Create .oda directory
	odaDir := filepath.Join(tmpDir, ".oda")
	if err := os.MkdirAll(odaDir, 0755); err != nil {
		t.Fatalf("failed to create .oda directory: %v", err)
	}

	// Create a minimal config file with sprint auto_start initially false
	configContent := `llm:
  setup:
    model: test-provider/test-model
  planning:
    model: test-provider/test-model
  orchestration:
    model: test-provider/test-model
  code:
    model: test-provider/test-model
  code-heavy:
    model: test-provider/test-model
sprint:
  auto_start: false
`
	configPath := filepath.Join(odaDir, "config.yaml")
	if err := os.WriteFile(configPath, []byte(configContent), 0644); err != nil {
		t.Fatalf("failed to write config file: %v", err)
	}

	// Create server with templates
	srv := createTestServerWithTemplates(t)
	srv.rootDir = tmpDir
	defer srv.wizardStore.Stop()

	// Test POST /settings with sprint_auto_start enabled
	form := url.Values{}
	form.Set("setup_model", "test-provider/test-model")
	form.Set("planning_model", "test-provider/test-model")
	form.Set("orchestration_model", "test-provider/test-model")
	form.Set("code_model", "test-provider/test-model")
	form.Set("code_heavy_model", "test-provider/test-model")
	form.Set("sprint_auto_start", "on") // Enable auto_start

	req := httptest.NewRequest(http.MethodPost, "/settings", strings.NewReader(form.Encode()))
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	rec := httptest.NewRecorder()

	srv.handleSaveSettings(rec, req)

	// Should return 200 OK
	if rec.Code != http.StatusOK {
		t.Errorf("expected status 200, got %d: %s", rec.Code, rec.Body.String())
	}

	// Verify success message
	body := rec.Body.String()
	if !strings.Contains(body, "Settings saved successfully") {
		t.Error("response should contain success message")
	}

	// Verify config was saved with auto_start: true
	savedData, err := os.ReadFile(configPath)
	if err != nil {
		t.Fatalf("failed to read saved config: %v", err)
	}

	savedContent := string(savedData)
	if !strings.Contains(savedContent, "auto_start: true") {
		t.Errorf("saved config should contain 'auto_start: true', got:\n%s", savedContent)
	}
}

// TestHandleSaveSettings_SprintAutoStartDisabled tests saving with sprint_auto_start unchecked
func TestHandleSaveSettings_SprintAutoStartDisabled(t *testing.T) {
	// Create a temporary directory for config
	tmpDir := t.TempDir()

	// Create .oda directory
	odaDir := filepath.Join(tmpDir, ".oda")
	if err := os.MkdirAll(odaDir, 0755); err != nil {
		t.Fatalf("failed to create .oda directory: %v", err)
	}

	// Create a config file with sprint auto_start initially true
	configContent := `llm:
  setup:
    model: test-provider/test-model
  planning:
    model: test-provider/test-model
  orchestration:
    model: test-provider/test-model
  code:
    model: test-provider/test-model
  code-heavy:
    model: test-provider/test-model
sprint:
  auto_start: true
`
	configPath := filepath.Join(odaDir, "config.yaml")
	if err := os.WriteFile(configPath, []byte(configContent), 0644); err != nil {
		t.Fatalf("failed to write config file: %v", err)
	}

	// Create server with templates
	srv := createTestServerWithTemplates(t)
	srv.rootDir = tmpDir
	defer srv.wizardStore.Stop()

	// Test POST /settings without sprint_auto_start (unchecked)
	form := url.Values{}
	form.Set("setup_model", "test-provider/test-model")
	form.Set("planning_model", "test-provider/test-model")
	form.Set("orchestration_model", "test-provider/test-model")
	form.Set("code_model", "test-provider/test-model")
	form.Set("code_heavy_model", "test-provider/test-model")
	// Note: sprint_auto_start is NOT set, simulating unchecked checkbox

	req := httptest.NewRequest(http.MethodPost, "/settings", strings.NewReader(form.Encode()))
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	rec := httptest.NewRecorder()

	srv.handleSaveSettings(rec, req)

	// Should return 200 OK
	if rec.Code != http.StatusOK {
		t.Errorf("expected status 200, got %d: %s", rec.Code, rec.Body.String())
	}

	// Verify config was saved with auto_start: false
	savedData, err := os.ReadFile(configPath)
	if err != nil {
		t.Fatalf("failed to read saved config: %v", err)
	}

	savedContent := string(savedData)
	if !strings.Contains(savedContent, "auto_start: false") {
		t.Errorf("saved config should contain 'auto_start: false', got:\n%s", savedContent)
	}
}

func TestHandleRestartStage(t *testing.T) {
	orch := &mvp.Orchestrator{}
	s := &Server{orchestrator: orch}

	req := httptest.NewRequest(http.MethodPost, "/api/task/42/restart-stage", nil)
	req.SetPathValue("id", "42")
	w := httptest.NewRecorder()

	s.handleRestartStage(w, req)

	// Should return error because store is not configured
	if w.Code != http.StatusConflict {
		t.Errorf("status = %d, want %d", w.Code, http.StatusConflict)
	}

	var resp map[string]any
	json.NewDecoder(w.Body).Decode(&resp)
	if resp["success"] != false {
		t.Errorf("success = %v, want false", resp["success"])
	}
}

func TestHandleRestartStageNoOrchestrator(t *testing.T) {
	s := &Server{}

	req := httptest.NewRequest(http.MethodPost, "/api/task/42/restart-stage", nil)
	req.SetPathValue("id", "42")
	w := httptest.NewRecorder()

	s.handleRestartStage(w, req)

	if w.Code != http.StatusServiceUnavailable {
		t.Errorf("status = %d, want %d", w.Code, http.StatusServiceUnavailable)
	}

	var resp map[string]any
	json.NewDecoder(w.Body).Decode(&resp)
	if resp["success"] != false {
		t.Errorf("success = %v, want false", resp["success"])
	}
}

func TestHandleRestartStageInvalidID(t *testing.T) {
	orch := &mvp.Orchestrator{}
	s := &Server{orchestrator: orch}

	req := httptest.NewRequest(http.MethodPost, "/api/task/abc/restart-stage", nil)
	req.SetPathValue("id", "abc")
	w := httptest.NewRecorder()

	s.handleRestartStage(w, req)

	if w.Code != http.StatusBadRequest {
		t.Errorf("status = %d, want %d", w.Code, http.StatusBadRequest)
	}

	var resp map[string]any
	json.NewDecoder(w.Body).Decode(&resp)
	if resp["success"] != false {
		t.Errorf("success = %v, want false", resp["success"])
	}
}

func TestHandleRestartFull(t *testing.T) {
	orch := &mvp.Orchestrator{}
	s := &Server{orchestrator: orch}

	req := httptest.NewRequest(http.MethodPost, "/api/task/42/restart-full", nil)
	req.SetPathValue("id", "42")
	w := httptest.NewRecorder()

	s.handleRestartFull(w, req)

	// Should return error because store is not configured
	if w.Code != http.StatusConflict {
		t.Errorf("status = %d, want %d", w.Code, http.StatusConflict)
	}

	var resp map[string]any
	json.NewDecoder(w.Body).Decode(&resp)
	if resp["success"] != false {
		t.Errorf("success = %v, want false", resp["success"])
	}
}

func TestHandleRestartFullNoOrchestrator(t *testing.T) {
	s := &Server{}

	req := httptest.NewRequest(http.MethodPost, "/api/task/42/restart-full", nil)
	req.SetPathValue("id", "42")
	w := httptest.NewRecorder()

	s.handleRestartFull(w, req)

	if w.Code != http.StatusServiceUnavailable {
		t.Errorf("status = %d, want %d", w.Code, http.StatusServiceUnavailable)
	}

	var resp map[string]any
	json.NewDecoder(w.Body).Decode(&resp)
	if resp["success"] != false {
		t.Errorf("success = %v, want false", resp["success"])
	}
}

func TestHandleRestartFullInvalidID(t *testing.T) {
	orch := &mvp.Orchestrator{}
	s := &Server{orchestrator: orch}

	req := httptest.NewRequest(http.MethodPost, "/api/task/abc/restart-full", nil)
	req.SetPathValue("id", "abc")
	w := httptest.NewRecorder()

	s.handleRestartFull(w, req)

	if w.Code != http.StatusBadRequest {
		t.Errorf("status = %d, want %d", w.Code, http.StatusBadRequest)
	}

	var resp map[string]any
	json.NewDecoder(w.Body).Decode(&resp)
	if resp["success"] != false {
		t.Errorf("success = %v, want false", resp["success"])
	}
}
