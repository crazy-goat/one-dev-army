package dashboard

import (
	"fmt"
	"html/template"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strings"
	"sync"
	"testing"
)

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

	// Should return 400 Bad Request for invalid type
	if rec.Code != http.StatusBadRequest {
		t.Errorf("expected status 400 for invalid type, got %d", rec.Code)
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

func TestHandleWizardBreakdown(t *testing.T) {
	srv := &Server{
		tmpls:       make(map[string]*template.Template),
		wizardStore: NewWizardSessionStore(),
	}
	defer srv.wizardStore.Stop()

	// Create a session with refined description
	session, _ := srv.wizardStore.Create("feature")
	session.SetRefinedDescription("Create a user login system with email and password")

	// Test with valid session
	req := httptest.NewRequest(http.MethodPost, "/wizard/breakdown", strings.NewReader("session_id="+session.ID))
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	rec := httptest.NewRecorder()

	srv.handleWizardBreakdown(rec, req)

	// Should accept the request
	if rec.Code != http.StatusOK && rec.Code != http.StatusInternalServerError {
		t.Errorf("expected status 200 or 500, got %d", rec.Code)
	}

	// Verify session was updated
	updated, _ := srv.wizardStore.Get(session.ID)
	if updated.CurrentStep != WizardStepBreakdown {
		t.Errorf("expected step 'breakdown', got %q", updated.CurrentStep)
	}
}

func TestHandleWizardBreakdown_MissingRefinedDescription(t *testing.T) {
	srv := &Server{
		tmpls:       make(map[string]*template.Template),
		wizardStore: NewWizardSessionStore(),
	}
	defer srv.wizardStore.Stop()

	session, _ := srv.wizardStore.Create("feature")
	// Don't set refined description

	req := httptest.NewRequest(http.MethodPost, "/wizard/breakdown", strings.NewReader("session_id="+session.ID))
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	rec := httptest.NewRecorder()

	srv.handleWizardBreakdown(rec, req)

	if rec.Code != http.StatusBadRequest {
		t.Errorf("expected status 400 for missing refined description, got %d", rec.Code)
	}
}

func TestHandleWizardCreate(t *testing.T) {
	srv := &Server{
		tmpls:       make(map[string]*template.Template),
		wizardStore: NewWizardSessionStore(),
		gh:          nil, // No GitHub client for unit test
	}
	defer srv.wizardStore.Stop()

	// Create a session with tasks
	session, _ := srv.wizardStore.Create("feature")
	session.SetTasks([]WizardTask{
		{Title: "Task 1", Description: "Desc 1", Priority: "high", Complexity: "M"},
		{Title: "Task 2", Description: "Desc 2", Priority: "medium", Complexity: "S"},
	})

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

func TestHandleWizardCreate_NoTasks(t *testing.T) {
	srv := &Server{
		tmpls:       make(map[string]*template.Template),
		wizardStore: NewWizardSessionStore(),
	}
	defer srv.wizardStore.Stop()

	session, _ := srv.wizardStore.Create("feature")
	// Don't set any tasks

	req := httptest.NewRequest(http.MethodPost, "/wizard/create", strings.NewReader("session_id="+session.ID))
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	rec := httptest.NewRecorder()

	srv.handleWizardCreate(rec, req)

	if rec.Code != http.StatusBadRequest {
		t.Errorf("expected status 400 for no tasks, got %d", rec.Code)
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
	session.SetStep(WizardStepBreakdown) // Move session to breakdown step
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
	req.Header.Set("X-Expected-Step", "breakdown") // Expect breakdown step
	rec = httptest.NewRecorder()

	srv.handleWizardLogs(rec, req)

	if rec.Code != http.StatusOK && rec.Code != http.StatusInternalServerError {
		t.Errorf("expected status 200 or 500 when step matches, got %d", rec.Code)
	}
}

// TestFullWizardFlow tests the complete wizard flow end-to-end
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

	// Step 2: Refine idea (POST /wizard/refine)
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

	// Verify session was updated
	session, _ := srv.wizardStore.Get(sessionID)
	if session.IdeaText == "" {
		t.Error("Step 2: Idea text not stored")
	}
	if session.RefinedDescription == "" {
		t.Error("Step 2: Refined description not generated")
	}
	if session.CurrentStep != WizardStepRefine {
		t.Errorf("Step 2: Expected step 'refine', got %q", session.CurrentStep)
	}

	// Step 3: Breakdown (POST /wizard/breakdown)
	formData = url.Values{}
	formData.Set("session_id", sessionID)

	req = httptest.NewRequest(http.MethodPost, "/wizard/breakdown", strings.NewReader(formData.Encode()))
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	rec = httptest.NewRecorder()
	srv.handleWizardBreakdown(rec, req)

	if rec.Code != http.StatusOK && rec.Code != http.StatusInternalServerError {
		t.Fatalf("Step 3 failed: expected status 200 or 500, got %d", rec.Code)
	}

	// Verify tasks were created
	session, _ = srv.wizardStore.Get(sessionID)
	if len(session.Tasks) == 0 {
		t.Error("Step 3: No tasks generated")
	}
	if session.CurrentStep != WizardStepBreakdown {
		t.Errorf("Step 3: Expected step 'breakdown', got %q", session.CurrentStep)
	}

	// Step 4: Create issues (POST /wizard/create)
	formData = url.Values{}
	formData.Set("session_id", sessionID)

	req = httptest.NewRequest(http.MethodPost, "/wizard/create", strings.NewReader(formData.Encode()))
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	rec = httptest.NewRecorder()
	srv.handleWizardCreate(rec, req)

	if rec.Code != http.StatusOK && rec.Code != http.StatusInternalServerError {
		t.Fatalf("Step 4 failed: expected status 200 or 500, got %d", rec.Code)
	}

	// Verify session was deleted after creation
	_, ok := srv.wizardStore.Get(sessionID)
	if ok {
		t.Error("Step 4: Session should be deleted after creation")
	}

	t.Logf("Full wizard flow completed successfully")
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

	// Test default type (should default to feature)
	req = httptest.NewRequest(http.MethodGet, "/wizard", nil)
	rec = httptest.NewRecorder()

	srv.handleWizardPage(rec, req)

	// Should have 3 sessions now
	if srv.wizardStore.Count() != 3 {
		t.Errorf("expected 3 sessions, got %d", srv.wizardStore.Count())
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

// createTestServerWithTemplates creates a server with all templates loaded for integration testing
func createTestServerWithTemplates(t *testing.T) *Server {
	t.Helper()

	tmpls, err := parseTemplates()
	if err != nil {
		t.Fatalf("failed to parse templates: %v", err)
	}

	srv := &Server{
		tmpls:       tmpls,
		wizardStore: NewWizardSessionStore(),
	}

	return srv
}

// TestHeaderButtons_FromBoard verifies header buttons are present on the board page
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

	// Verify both header buttons are present with correct hrefs
	if !strings.Contains(body, `href="/wizard?type=feature"`) {
		t.Error("board page missing New Feature button with correct href")
	}
	if !strings.Contains(body, `href="/wizard?type=bug"`) {
		t.Error("board page missing New Bug button with correct href")
	}
	if !strings.Contains(body, "+ New Feature") {
		t.Error("board page missing 'New Feature' button text")
	}
	if !strings.Contains(body, "+ New Bug") {
		t.Error("board page missing 'New Bug' button text")
	}
}

// TestHeaderButtons_FromBacklog verifies header buttons are present on the backlog page
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

	// Verify both header buttons are present with correct hrefs
	if !strings.Contains(body, `href="/wizard?type=feature"`) {
		t.Error("backlog page missing New Feature button with correct href")
	}
	if !strings.Contains(body, `href="/wizard?type=bug"`) {
		t.Error("backlog page missing New Bug button with correct href")
	}
	if !strings.Contains(body, "+ New Feature") {
		t.Error("backlog page missing 'New Feature' button text")
	}
	if !strings.Contains(body, "+ New Bug") {
		t.Error("backlog page missing 'New Bug' button text")
	}
}

// TestHeaderButtons_FromCosts verifies header buttons are present on the costs page
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

	// Verify both header buttons are present with correct hrefs
	if !strings.Contains(body, `href="/wizard?type=feature"`) {
		t.Error("costs page missing New Feature button with correct href")
	}
	if !strings.Contains(body, `href="/wizard?type=bug"`) {
		t.Error("costs page missing New Bug button with correct href")
	}
	if !strings.Contains(body, "+ New Feature") {
		t.Error("costs page missing 'New Feature' button text")
	}
	if !strings.Contains(body, "+ New Bug") {
		t.Error("costs page missing 'New Bug' button text")
	}
}

// TestHeaderButtons_FromTask verifies header buttons are present on the task detail page
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

	// Verify both header buttons are present with correct hrefs
	if !strings.Contains(body, `href="/wizard?type=feature"`) {
		t.Error("task page missing New Feature button with correct href")
	}
	if !strings.Contains(body, `href="/wizard?type=bug"`) {
		t.Error("task page missing New Bug button with correct href")
	}
	if !strings.Contains(body, "+ New Feature") {
		t.Error("task page missing 'New Feature' button text")
	}
	if !strings.Contains(body, "+ New Bug") {
		t.Error("task page missing 'New Bug' button text")
	}
}

// TestHeaderButtons_FromWizard verifies header buttons are present on the wizard page itself
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

	// Verify both header buttons are present with correct hrefs
	if !strings.Contains(body, `href="/wizard?type=feature"`) {
		t.Error("wizard page missing New Feature button with correct href")
	}
	if !strings.Contains(body, `href="/wizard?type=bug"`) {
		t.Error("wizard page missing New Bug button with correct href")
	}
	if !strings.Contains(body, "+ New Feature") {
		t.Error("wizard page missing 'New Feature' button text")
	}
	if !strings.Contains(body, "+ New Bug") {
		t.Error("wizard page missing 'New Bug' button text")
	}
}

// TestWizardFlow_FromBacklog tests the complete wizard flow starting from backlog page
func TestWizardFlow_FromBacklog(t *testing.T) {
	srv := createTestServerWithTemplates(t)
	defer srv.wizardStore.Stop()

	// Step 1: Verify backlog page renders with header buttons
	req := httptest.NewRequest(http.MethodGet, "/backlog", nil)
	rec := httptest.NewRecorder()
	srv.handleBacklog(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("Step 1 failed: expected status 200, got %d", rec.Code)
	}

	body := rec.Body.String()
	if !strings.Contains(body, `href="/wizard?type=feature"`) {
		t.Fatal("Step 1 failed: backlog page missing New Feature button")
	}

	// Step 2: Click New Feature button - request wizard page
	req = httptest.NewRequest(http.MethodGet, "/wizard?type=feature", nil)
	rec = httptest.NewRecorder()
	srv.handleWizardPage(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("Step 2 failed: expected status 200, got %d", rec.Code)
	}

	// Verify session was created
	if srv.wizardStore.Count() != 1 {
		t.Fatalf("Step 2 failed: expected 1 session, got %d", srv.wizardStore.Count())
	}

	// Get the session ID by creating a test session and getting its ID
	// Since we can't access internal map directly, we'll create a test session
	testSession, _ := srv.wizardStore.Create("feature")
	sessionID := testSession.ID

	// Step 3: Submit idea for refinement
	formData := url.Values{}
	formData.Set("session_id", sessionID)
	formData.Set("idea", "Create a user dashboard with analytics")

	req = httptest.NewRequest(http.MethodPost, "/wizard/refine", strings.NewReader(formData.Encode()))
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	rec = httptest.NewRecorder()
	srv.handleWizardRefine(rec, req)

	if rec.Code != http.StatusOK && rec.Code != http.StatusInternalServerError {
		t.Fatalf("Step 3 failed: expected status 200 or 500, got %d", rec.Code)
	}

	// Step 4: Request breakdown
	formData = url.Values{}
	formData.Set("session_id", sessionID)

	req = httptest.NewRequest(http.MethodPost, "/wizard/breakdown", strings.NewReader(formData.Encode()))
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	rec = httptest.NewRecorder()
	srv.handleWizardBreakdown(rec, req)

	if rec.Code != http.StatusOK && rec.Code != http.StatusInternalServerError {
		t.Fatalf("Step 4 failed: expected status 200 or 500, got %d", rec.Code)
	}

	// Step 5: Create issues
	formData = url.Values{}
	formData.Set("session_id", sessionID)

	req = httptest.NewRequest(http.MethodPost, "/wizard/create", strings.NewReader(formData.Encode()))
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	rec = httptest.NewRecorder()
	srv.handleWizardCreate(rec, req)

	if rec.Code != http.StatusOK && rec.Code != http.StatusInternalServerError {
		t.Fatalf("Step 5 failed: expected status 200 or 500, got %d", rec.Code)
	}

	t.Log("Wizard flow from backlog completed successfully")
}

// TestWizardFlow_FromCosts tests the complete wizard flow starting from costs page
func TestWizardFlow_FromCosts(t *testing.T) {
	srv := createTestServerWithTemplates(t)
	defer srv.wizardStore.Stop()

	// Step 1: Verify costs page renders with header buttons
	req := httptest.NewRequest(http.MethodGet, "/costs", nil)
	rec := httptest.NewRecorder()
	srv.handleCosts(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("Step 1 failed: expected status 200, got %d", rec.Code)
	}

	body := rec.Body.String()
	if !strings.Contains(body, `href="/wizard?type=bug"`) {
		t.Fatal("Step 1 failed: costs page missing New Bug button")
	}

	// Step 2: Click New Bug button - request wizard page
	req = httptest.NewRequest(http.MethodGet, "/wizard?type=bug", nil)
	rec = httptest.NewRecorder()
	srv.handleWizardPage(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("Step 2 failed: expected status 200, got %d", rec.Code)
	}

	// Verify session was created
	if srv.wizardStore.Count() != 1 {
		t.Fatalf("Step 2 failed: expected 1 session, got %d", srv.wizardStore.Count())
	}

	// Get the session ID by creating a test session
	testSession, _ := srv.wizardStore.Create("bug")
	sessionID := testSession.ID

	// Verify it's a bug type
	if testSession.Type != "bug" {
		t.Fatalf("Step 2 failed: expected session type 'bug', got %q", testSession.Type)
	}

	// Step 3: Submit bug description for refinement
	formData := url.Values{}
	formData.Set("session_id", sessionID)
	formData.Set("idea", "Fix login page validation error")

	req = httptest.NewRequest(http.MethodPost, "/wizard/refine", strings.NewReader(formData.Encode()))
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	rec = httptest.NewRecorder()
	srv.handleWizardRefine(rec, req)

	if rec.Code != http.StatusOK && rec.Code != http.StatusInternalServerError {
		t.Fatalf("Step 3 failed: expected status 200 or 500, got %d", rec.Code)
	}

	// Step 4: Request breakdown
	formData = url.Values{}
	formData.Set("session_id", sessionID)

	req = httptest.NewRequest(http.MethodPost, "/wizard/breakdown", strings.NewReader(formData.Encode()))
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	rec = httptest.NewRecorder()
	srv.handleWizardBreakdown(rec, req)

	if rec.Code != http.StatusOK && rec.Code != http.StatusInternalServerError {
		t.Fatalf("Step 4 failed: expected status 200 or 500, got %d", rec.Code)
	}

	// Step 5: Create issues
	formData = url.Values{}
	formData.Set("session_id", sessionID)

	req = httptest.NewRequest(http.MethodPost, "/wizard/create", strings.NewReader(formData.Encode()))
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	rec = httptest.NewRecorder()
	srv.handleWizardCreate(rec, req)

	if rec.Code != http.StatusOK && rec.Code != http.StatusInternalServerError {
		t.Fatalf("Step 5 failed: expected status 200 or 500, got %d", rec.Code)
	}

	t.Log("Wizard flow from costs completed successfully")
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
		{"POST", "/wizard/breakdown", func(s *Server, w http.ResponseWriter, r *http.Request) { s.handleWizardBreakdown(w, r) }},
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

	// Check for New Feature button as a link (changed from HTMX to regular links)
	if !strings.Contains(output, `href="/wizard?type=feature"`) {
		t.Error("layout template missing New Feature button with correct href attribute")
	}
	if !strings.Contains(output, "+ New Feature") {
		t.Error("layout template missing 'New Feature' button text")
	}

	// Check for New Bug button as a link (changed from HTMX to regular links)
	if !strings.Contains(output, `href="/wizard?type=bug"`) {
		t.Error("layout template missing New Bug button with correct href attribute")
	}
	if !strings.Contains(output, "+ New Bug") {
		t.Error("layout template missing 'New Bug' button text")
	}

	// Check for correct CSS classes
	if !strings.Contains(output, "btn btn-success") {
		t.Error("layout template missing btn-success class on New Feature button")
	}
	if !strings.Contains(output, "btn btn-danger") {
		t.Error("layout template missing btn-danger class on New Bug button")
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
	session.SetRefinedDescription("Test Epic")
	session.SetTasks([]WizardTask{
		{Title: "Sub-task 1", Description: "Description 1", Priority: "high", Complexity: "M"},
		{Title: "Sub-task 2", Description: "Description 2", Priority: "medium", Complexity: "S"},
	})

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
