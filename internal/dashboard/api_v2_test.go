package dashboard

import (
	"bytes"
	"encoding/json"
	"html/template"
	"net/http"
	"net/http/httptest"
	"os"
	"strings"
	"testing"

	"github.com/crazy-goat/one-dev-army/internal/db"
)

// ---------------------------------------------------------------------------
// Test helpers
// ---------------------------------------------------------------------------

// newTestServerV2 creates a minimal Server suitable for v2 API tests.
// It has no orchestrator, no GitHub client, no store — only the wizard store
// and basic fields needed to exercise the JSON handlers.
func newTestServerV2(t *testing.T) *Server {
	t.Helper()
	return &Server{
		tmpls:       make(map[string]*template.Template),
		wizardStore: NewWizardSessionStore(),
		mux:         http.NewServeMux(),
	}
}

// jsonBody encodes v as JSON and returns a *bytes.Reader suitable for http.NewRequest.
func jsonBody(t *testing.T, v any) *bytes.Reader {
	t.Helper()
	b, err := json.Marshal(v)
	if err != nil {
		t.Fatalf("jsonBody: %v", err)
	}
	return bytes.NewReader(b)
}

// decodeResponse decodes the response body into v.
func decodeResponse(t *testing.T, rec *httptest.ResponseRecorder, v any) {
	t.Helper()
	if err := json.NewDecoder(rec.Body).Decode(v); err != nil {
		t.Fatalf("decodeResponse: %v (body: %s)", err, rec.Body.String())
	}
}

// assertStatus checks that the response has the expected status code.
func assertStatus(t *testing.T, rec *httptest.ResponseRecorder, expected int) {
	t.Helper()
	if rec.Code != expected {
		t.Errorf("expected status %d, got %d (body: %s)", expected, rec.Code, rec.Body.String())
	}
}

// assertJSONContentType checks that the response has application/json content type.
func assertJSONContentType(t *testing.T, rec *httptest.ResponseRecorder) {
	t.Helper()
	ct := rec.Header().Get("Content-Type")
	if ct != "application/json" {
		t.Errorf("expected Content-Type %q, got %q", "application/json", ct)
	}
}

// ---------------------------------------------------------------------------
// Board tests
// ---------------------------------------------------------------------------

func TestHandleBoardV2(t *testing.T) {
	srv := newTestServerV2(t)
	defer srv.wizardStore.Stop()

	req := httptest.NewRequest(http.MethodGet, "/api/v2/board", nil)
	rec := httptest.NewRecorder()

	srv.handleBoardV2(rec, req)

	assertStatus(t, rec, http.StatusOK)
	assertJSONContentType(t, rec)

	var resp boardResponseV2
	decodeResponse(t, rec, &resp)

	// Default state: paused, not processing, no tickets
	if !resp.Paused {
		t.Error("expected Paused to be true by default")
	}
	if resp.Processing {
		t.Error("expected Processing to be false by default")
	}
	if resp.TotalTickets != 0 {
		t.Errorf("expected TotalTickets to be 0, got %d", resp.TotalTickets)
	}
	if resp.Columns == nil {
		t.Error("expected Columns to be non-nil")
	}

	// Verify all expected column keys exist
	expectedColumns := []string{"blocked", "backlog", "plan", "code", "ai_review", "check_pipeline", "approve", "merge", "done", "failed"}
	for _, col := range expectedColumns {
		if _, ok := resp.Columns[col]; !ok {
			t.Errorf("expected column %q to exist in response", col)
		}
	}
}

func TestHandleBoardV2_CanPlanSprint(t *testing.T) {
	srv := newTestServerV2(t)
	defer srv.wizardStore.Stop()

	req := httptest.NewRequest(http.MethodGet, "/api/v2/board", nil)
	rec := httptest.NewRecorder()

	srv.handleBoardV2(rec, req)

	var resp boardResponseV2
	decodeResponse(t, rec, &resp)

	// With no GitHub client and no store, CanPlanSprint should be true
	if !resp.CanPlanSprint {
		t.Error("expected CanPlanSprint to be true when no GitHub client")
	}
}

// ---------------------------------------------------------------------------
// Sprint status tests
// ---------------------------------------------------------------------------

func TestHandleSprintStatusV2(t *testing.T) {
	srv := newTestServerV2(t)
	defer srv.wizardStore.Stop()

	req := httptest.NewRequest(http.MethodGet, "/api/v2/sprint/status", nil)
	rec := httptest.NewRecorder()

	srv.handleSprintStatusV2(rec, req)

	assertStatus(t, rec, http.StatusOK)
	assertJSONContentType(t, rec)

	var resp sprintStatusV2
	decodeResponse(t, rec, &resp)

	// No orchestrator → paused
	if !resp.Paused {
		t.Error("expected Paused to be true when no orchestrator")
	}
	if resp.Processing {
		t.Error("expected Processing to be false when no orchestrator")
	}
}

func TestHandleSprintStartV2_NoOrchestrator(t *testing.T) {
	srv := newTestServerV2(t)
	defer srv.wizardStore.Stop()

	req := httptest.NewRequest(http.MethodPost, "/api/v2/sprint/start", nil)
	rec := httptest.NewRecorder()

	srv.handleSprintStartV2(rec, req)

	assertStatus(t, rec, http.StatusServiceUnavailable)
}

func TestHandleSprintPauseV2_NoOrchestrator(t *testing.T) {
	srv := newTestServerV2(t)
	defer srv.wizardStore.Stop()

	req := httptest.NewRequest(http.MethodPost, "/api/v2/sprint/pause", nil)
	rec := httptest.NewRecorder()

	srv.handleSprintPauseV2(rec, req)

	assertStatus(t, rec, http.StatusServiceUnavailable)
}

// ---------------------------------------------------------------------------
// Workers tests
// ---------------------------------------------------------------------------

func TestHandleWorkersV2(t *testing.T) {
	srv := newTestServerV2(t)
	defer srv.wizardStore.Stop()

	req := httptest.NewRequest(http.MethodGet, "/api/v2/workers", nil)
	rec := httptest.NewRecorder()

	srv.handleWorkersV2(rec, req)

	assertStatus(t, rec, http.StatusOK)
	assertJSONContentType(t, rec)

	var resp map[string]any
	decodeResponse(t, rec, &resp)

	workers, ok := resp["workers"].([]any)
	if !ok {
		t.Fatal("expected 'workers' to be an array")
	}
	if len(workers) != 0 {
		t.Errorf("expected empty workers array, got %d", len(workers))
	}

	paused, ok := resp["paused"].(bool)
	if !ok || !paused {
		t.Error("expected paused to be true when no orchestrator")
	}
}

func TestHandleWorkerToggleV2_NoOrchestrator(t *testing.T) {
	srv := newTestServerV2(t)
	defer srv.wizardStore.Stop()

	req := httptest.NewRequest(http.MethodPost, "/api/v2/workers/toggle", nil)
	rec := httptest.NewRecorder()

	srv.handleWorkerToggleV2(rec, req)

	assertStatus(t, rec, http.StatusServiceUnavailable)
}

// ---------------------------------------------------------------------------
// Ticket action tests
// ---------------------------------------------------------------------------

func TestHandleApproveV2_InvalidID(t *testing.T) {
	srv := newTestServerV2(t)
	defer srv.wizardStore.Stop()

	// Register route so PathValue works
	srv.mux.HandleFunc("POST /api/v2/issues/{id}/approve", srv.handleApproveV2)

	req := httptest.NewRequest(http.MethodPost, "/api/v2/issues/abc/approve", nil)
	rec := httptest.NewRecorder()

	srv.mux.ServeHTTP(rec, req)

	assertStatus(t, rec, http.StatusBadRequest)

	var resp map[string]string
	decodeResponse(t, rec, &resp)

	if resp["error"] == "" {
		t.Error("expected error message in response")
	}
}

func TestHandleApproveV2_ZeroID(t *testing.T) {
	srv := newTestServerV2(t)
	defer srv.wizardStore.Stop()

	srv.mux.HandleFunc("POST /api/v2/issues/{id}/approve", srv.handleApproveV2)

	req := httptest.NewRequest(http.MethodPost, "/api/v2/issues/0/approve", nil)
	rec := httptest.NewRecorder()

	srv.mux.ServeHTTP(rec, req)

	assertStatus(t, rec, http.StatusBadRequest)
}

func TestHandleApproveV2_NoOrchestrator(t *testing.T) {
	srv := newTestServerV2(t)
	defer srv.wizardStore.Stop()

	srv.mux.HandleFunc("POST /api/v2/issues/{id}/approve", srv.handleApproveV2)

	req := httptest.NewRequest(http.MethodPost, "/api/v2/issues/42/approve", nil)
	rec := httptest.NewRecorder()

	srv.mux.ServeHTTP(rec, req)

	assertStatus(t, rec, http.StatusServiceUnavailable)
}

func TestHandleRejectV2_InvalidID(t *testing.T) {
	srv := newTestServerV2(t)
	defer srv.wizardStore.Stop()

	srv.mux.HandleFunc("POST /api/v2/issues/{id}/reject", srv.handleRejectV2)

	req := httptest.NewRequest(http.MethodPost, "/api/v2/issues/-5/reject", nil)
	rec := httptest.NewRecorder()

	srv.mux.ServeHTTP(rec, req)

	assertStatus(t, rec, http.StatusBadRequest)
}

func TestHandleBlockV2_NoOrchestrator(t *testing.T) {
	srv := newTestServerV2(t)
	defer srv.wizardStore.Stop()

	srv.mux.HandleFunc("POST /api/v2/issues/{id}/block", srv.handleBlockV2)

	req := httptest.NewRequest(http.MethodPost, "/api/v2/issues/42/block", nil)
	rec := httptest.NewRecorder()

	srv.mux.ServeHTTP(rec, req)

	assertStatus(t, rec, http.StatusServiceUnavailable)
}

func TestHandleUnblockV2_NoOrchestrator(t *testing.T) {
	srv := newTestServerV2(t)
	defer srv.wizardStore.Stop()

	srv.mux.HandleFunc("POST /api/v2/issues/{id}/unblock", srv.handleUnblockV2)

	req := httptest.NewRequest(http.MethodPost, "/api/v2/issues/42/unblock", nil)
	rec := httptest.NewRecorder()

	srv.mux.ServeHTTP(rec, req)

	assertStatus(t, rec, http.StatusServiceUnavailable)
}

func TestHandleProcessV2_NoOrchestrator(t *testing.T) {
	srv := newTestServerV2(t)
	defer srv.wizardStore.Stop()

	srv.mux.HandleFunc("POST /api/v2/issues/{id}/process", srv.handleProcessV2)

	req := httptest.NewRequest(http.MethodPost, "/api/v2/issues/42/process", nil)
	rec := httptest.NewRecorder()

	srv.mux.ServeHTTP(rec, req)

	assertStatus(t, rec, http.StatusServiceUnavailable)
}

func TestHandleDeclineV2_InvalidBody(t *testing.T) {
	srv := newTestServerV2(t)
	defer srv.wizardStore.Stop()

	srv.mux.HandleFunc("POST /api/v2/issues/{id}/decline", srv.handleDeclineV2)

	req := httptest.NewRequest(http.MethodPost, "/api/v2/issues/42/decline", bytes.NewReader([]byte("not json")))
	rec := httptest.NewRecorder()

	srv.mux.ServeHTTP(rec, req)

	// Should fail with bad request (invalid JSON) or service unavailable (no orchestrator)
	// Since we check orchestrator first after parsing ID, and then parse body, let's check
	if rec.Code != http.StatusServiceUnavailable && rec.Code != http.StatusBadRequest {
		t.Errorf("expected 400 or 503, got %d", rec.Code)
	}
}

// ---------------------------------------------------------------------------
// Issue detail tests
// ---------------------------------------------------------------------------

func TestHandleIssueDetailV2_InvalidID(t *testing.T) {
	srv := newTestServerV2(t)
	defer srv.wizardStore.Stop()

	srv.mux.HandleFunc("GET /api/v2/issues/{id}", srv.handleIssueDetailV2)

	req := httptest.NewRequest(http.MethodGet, "/api/v2/issues/abc", nil)
	rec := httptest.NewRecorder()

	srv.mux.ServeHTTP(rec, req)

	assertStatus(t, rec, http.StatusBadRequest)
}

func TestHandleIssueDetailV2_ValidID(t *testing.T) {
	srv := newTestServerV2(t)
	defer srv.wizardStore.Stop()

	srv.mux.HandleFunc("GET /api/v2/issues/{id}", srv.handleIssueDetailV2)

	req := httptest.NewRequest(http.MethodGet, "/api/v2/issues/42", nil)
	rec := httptest.NewRecorder()

	srv.mux.ServeHTTP(rec, req)

	assertStatus(t, rec, http.StatusOK)
	assertJSONContentType(t, rec)

	var resp issueDetailV2
	decodeResponse(t, rec, &resp)

	if resp.IssueNumber != 42 {
		t.Errorf("expected IssueNumber 42, got %d", resp.IssueNumber)
	}
	if resp.Steps == nil {
		t.Error("expected Steps to be non-nil (empty array)")
	}
}

func TestHandleIssueStepsV2_ValidID(t *testing.T) {
	srv := newTestServerV2(t)
	defer srv.wizardStore.Stop()

	srv.mux.HandleFunc("GET /api/v2/issues/{id}/steps", srv.handleIssueStepsV2)

	req := httptest.NewRequest(http.MethodGet, "/api/v2/issues/42/steps", nil)
	rec := httptest.NewRecorder()

	srv.mux.ServeHTTP(rec, req)

	assertStatus(t, rec, http.StatusOK)

	var resp map[string]any
	decodeResponse(t, rec, &resp)

	if resp["issue_number"] != float64(42) {
		t.Errorf("expected issue_number 42, got %v", resp["issue_number"])
	}
}

// ---------------------------------------------------------------------------
// Settings tests
// ---------------------------------------------------------------------------

func TestHandleSettingsV2_NoRootDir(t *testing.T) {
	srv := newTestServerV2(t)
	defer srv.wizardStore.Stop()

	req := httptest.NewRequest(http.MethodGet, "/api/v2/settings", nil)
	rec := httptest.NewRecorder()

	srv.handleSettingsV2(rec, req)

	assertStatus(t, rec, http.StatusOK)
	assertJSONContentType(t, rec)

	var resp settingsResponseV2
	decodeResponse(t, rec, &resp)

	// Should return default config when rootDir is empty
	if resp.Config.Code.Model == "" {
		t.Error("expected default Code model to be non-empty")
	}
}

func TestHandleYoloToggleV2_NoRootDir(t *testing.T) {
	srv := newTestServerV2(t)
	defer srv.wizardStore.Stop()

	req := httptest.NewRequest(http.MethodPost, "/api/v2/settings/yolo", nil)
	rec := httptest.NewRecorder()

	srv.handleYoloToggleV2(rec, req)

	assertStatus(t, rec, http.StatusInternalServerError)
}

// ---------------------------------------------------------------------------
// Sync tests
// ---------------------------------------------------------------------------

func TestHandleSyncV2_NoSyncService(t *testing.T) {
	srv := newTestServerV2(t)
	defer srv.wizardStore.Stop()

	req := httptest.NewRequest(http.MethodPost, "/api/v2/sync", nil)
	rec := httptest.NewRecorder()

	srv.handleSyncV2(rec, req)

	assertStatus(t, rec, http.StatusServiceUnavailable)
}

// ---------------------------------------------------------------------------
// Rate limit tests
// ---------------------------------------------------------------------------

func TestHandleRateLimitV2_NoService(t *testing.T) {
	srv := newTestServerV2(t)
	defer srv.wizardStore.Stop()

	req := httptest.NewRequest(http.MethodGet, "/api/v2/rate-limit", nil)
	rec := httptest.NewRecorder()

	srv.handleRateLimitV2(rec, req)

	assertStatus(t, rec, http.StatusServiceUnavailable)
}

// ---------------------------------------------------------------------------
// Wizard tests
// ---------------------------------------------------------------------------

func TestHandleWizardCreateSessionV2(t *testing.T) {
	srv := newTestServerV2(t)
	defer srv.wizardStore.Stop()

	body := jsonBody(t, map[string]string{"type": "feature"})
	req := httptest.NewRequest(http.MethodPost, "/api/v2/wizard/sessions", body)
	req.Header.Set("Content-Type", "application/json")
	rec := httptest.NewRecorder()

	srv.handleWizardCreateSessionV2(rec, req)

	assertStatus(t, rec, http.StatusCreated)
	assertJSONContentType(t, rec)

	var resp wizardSessionResponseV2
	decodeResponse(t, rec, &resp)

	if resp.ID == "" {
		t.Error("expected session ID to be non-empty")
	}
	if resp.Type != "feature" {
		t.Errorf("expected type 'feature', got %q", resp.Type)
	}
	if resp.CurrentStep != "new" {
		t.Errorf("expected current_step 'new', got %q", resp.CurrentStep)
	}

	// Verify session was created in store
	if srv.wizardStore.Count() != 1 {
		t.Errorf("expected 1 session in store, got %d", srv.wizardStore.Count())
	}
}

func TestHandleWizardCreateSessionV2_Bug(t *testing.T) {
	srv := newTestServerV2(t)
	defer srv.wizardStore.Stop()

	body := jsonBody(t, map[string]string{"type": "bug"})
	req := httptest.NewRequest(http.MethodPost, "/api/v2/wizard/sessions", body)
	req.Header.Set("Content-Type", "application/json")
	rec := httptest.NewRecorder()

	srv.handleWizardCreateSessionV2(rec, req)

	assertStatus(t, rec, http.StatusCreated)

	var resp wizardSessionResponseV2
	decodeResponse(t, rec, &resp)

	if resp.Type != "bug" {
		t.Errorf("expected type 'bug', got %q", resp.Type)
	}
}

func TestHandleWizardCreateSessionV2_InvalidType(t *testing.T) {
	srv := newTestServerV2(t)
	defer srv.wizardStore.Stop()

	body := jsonBody(t, map[string]string{"type": "invalid"})
	req := httptest.NewRequest(http.MethodPost, "/api/v2/wizard/sessions", body)
	req.Header.Set("Content-Type", "application/json")
	rec := httptest.NewRecorder()

	srv.handleWizardCreateSessionV2(rec, req)

	assertStatus(t, rec, http.StatusBadRequest)

	// No session should be created
	if srv.wizardStore.Count() != 0 {
		t.Errorf("expected 0 sessions, got %d", srv.wizardStore.Count())
	}
}

func TestHandleWizardCreateSessionV2_InvalidJSON(t *testing.T) {
	srv := newTestServerV2(t)
	defer srv.wizardStore.Stop()

	req := httptest.NewRequest(http.MethodPost, "/api/v2/wizard/sessions", bytes.NewReader([]byte("not json")))
	req.Header.Set("Content-Type", "application/json")
	rec := httptest.NewRecorder()

	srv.handleWizardCreateSessionV2(rec, req)

	assertStatus(t, rec, http.StatusBadRequest)
}

func TestHandleWizardGetSessionV2(t *testing.T) {
	srv := newTestServerV2(t)
	defer srv.wizardStore.Stop()

	// Create a session first
	session, err := srv.wizardStore.Create("feature")
	if err != nil {
		t.Fatalf("failed to create session: %v", err)
	}

	srv.mux.HandleFunc("GET /api/v2/wizard/sessions/{id}", srv.handleWizardGetSessionV2)

	req := httptest.NewRequest(http.MethodGet, "/api/v2/wizard/sessions/"+session.ID, nil)
	rec := httptest.NewRecorder()

	srv.mux.ServeHTTP(rec, req)

	assertStatus(t, rec, http.StatusOK)

	var resp wizardSessionResponseV2
	decodeResponse(t, rec, &resp)

	if resp.ID != session.ID {
		t.Errorf("expected session ID %q, got %q", session.ID, resp.ID)
	}
}

func TestHandleWizardGetSessionV2_NotFound(t *testing.T) {
	srv := newTestServerV2(t)
	defer srv.wizardStore.Stop()

	srv.mux.HandleFunc("GET /api/v2/wizard/sessions/{id}", srv.handleWizardGetSessionV2)

	req := httptest.NewRequest(http.MethodGet, "/api/v2/wizard/sessions/nonexistent", nil)
	rec := httptest.NewRecorder()

	srv.mux.ServeHTTP(rec, req)

	assertStatus(t, rec, http.StatusNotFound)
}

func TestHandleWizardDeleteSessionV2(t *testing.T) {
	srv := newTestServerV2(t)
	defer srv.wizardStore.Stop()

	// Create a session first
	session, err := srv.wizardStore.Create("bug")
	if err != nil {
		t.Fatalf("failed to create session: %v", err)
	}

	if srv.wizardStore.Count() != 1 {
		t.Fatalf("expected 1 session, got %d", srv.wizardStore.Count())
	}

	srv.mux.HandleFunc("DELETE /api/v2/wizard/sessions/{id}", srv.handleWizardDeleteSessionV2)

	req := httptest.NewRequest(http.MethodDelete, "/api/v2/wizard/sessions/"+session.ID, nil)
	rec := httptest.NewRecorder()

	srv.mux.ServeHTTP(rec, req)

	assertStatus(t, rec, http.StatusOK)

	// Session should be deleted
	if srv.wizardStore.Count() != 0 {
		t.Errorf("expected 0 sessions after delete, got %d", srv.wizardStore.Count())
	}
}

func TestHandleWizardRefineV2_NoSession(t *testing.T) {
	srv := newTestServerV2(t)
	defer srv.wizardStore.Stop()

	srv.mux.HandleFunc("POST /api/v2/wizard/sessions/{id}/refine", srv.handleWizardRefineV2)

	body := jsonBody(t, map[string]string{"idea": "test idea"})
	req := httptest.NewRequest(http.MethodPost, "/api/v2/wizard/sessions/nonexistent/refine", body)
	req.Header.Set("Content-Type", "application/json")
	rec := httptest.NewRecorder()

	srv.mux.ServeHTTP(rec, req)

	assertStatus(t, rec, http.StatusNotFound)
}

func TestHandleWizardRefineV2_EmptyIdea(t *testing.T) {
	srv := newTestServerV2(t)
	defer srv.wizardStore.Stop()

	session, err := srv.wizardStore.Create("feature")
	if err != nil {
		t.Fatalf("failed to create session: %v", err)
	}

	srv.mux.HandleFunc("POST /api/v2/wizard/sessions/{id}/refine", srv.handleWizardRefineV2)

	body := jsonBody(t, map[string]string{"idea": ""})
	req := httptest.NewRequest(http.MethodPost, "/api/v2/wizard/sessions/"+session.ID+"/refine", body)
	req.Header.Set("Content-Type", "application/json")
	rec := httptest.NewRecorder()

	srv.mux.ServeHTTP(rec, req)

	assertStatus(t, rec, http.StatusBadRequest)
}

func TestHandleWizardRefineV2_MockResponse(t *testing.T) {
	srv := newTestServerV2(t)
	defer srv.wizardStore.Stop()

	session, err := srv.wizardStore.Create("feature")
	if err != nil {
		t.Fatalf("failed to create session: %v", err)
	}

	srv.mux.HandleFunc("POST /api/v2/wizard/sessions/{id}/refine", srv.handleWizardRefineV2)

	body := jsonBody(t, map[string]string{"idea": "Add user authentication"})
	req := httptest.NewRequest(http.MethodPost, "/api/v2/wizard/sessions/"+session.ID+"/refine", body)
	req.Header.Set("Content-Type", "application/json")
	rec := httptest.NewRecorder()

	srv.mux.ServeHTTP(rec, req)

	assertStatus(t, rec, http.StatusOK)

	var resp wizardSessionResponseV2
	decodeResponse(t, rec, &resp)

	if resp.GeneratedTitle == "" {
		t.Error("expected generated title to be non-empty")
	}
	if resp.TechnicalPlanning == "" {
		t.Error("expected technical planning to be non-empty")
	}
	if resp.CurrentStep != "refine" {
		t.Errorf("expected current_step 'refine', got %q", resp.CurrentStep)
	}
}

func TestHandleWizardCreateIssueV2_MockResponse(t *testing.T) {
	srv := newTestServerV2(t)
	defer srv.wizardStore.Stop()

	// Create and prepare a session
	session, err := srv.wizardStore.Create("feature")
	if err != nil {
		t.Fatalf("failed to create session: %v", err)
	}
	session.SetTechnicalPlanning("## Test planning")
	session.SetGeneratedTitle("[Feature] Test feature")

	srv.mux.HandleFunc("POST /api/v2/wizard/sessions/{id}/create", srv.handleWizardCreateIssueV2)

	body := jsonBody(t, map[string]any{
		"title":         "[Feature] Custom title",
		"add_to_sprint": false,
	})
	req := httptest.NewRequest(http.MethodPost, "/api/v2/wizard/sessions/"+session.ID+"/create", body)
	req.Header.Set("Content-Type", "application/json")
	rec := httptest.NewRecorder()

	srv.mux.ServeHTTP(rec, req)

	assertStatus(t, rec, http.StatusCreated)

	var resp map[string]any
	decodeResponse(t, rec, &resp)

	if resp["success"] != true {
		t.Error("expected success to be true")
	}
	issue, ok := resp["issue"].(map[string]any)
	if !ok {
		t.Fatal("expected 'issue' in response")
	}
	if issue["number"] != float64(100) {
		t.Errorf("expected mock issue number 100, got %v", issue["number"])
	}

	// Session should be cleaned up
	if srv.wizardStore.Count() != 0 {
		t.Errorf("expected 0 sessions after create, got %d", srv.wizardStore.Count())
	}
}

func TestHandleWizardLogsV2(t *testing.T) {
	srv := newTestServerV2(t)
	defer srv.wizardStore.Stop()

	session, err := srv.wizardStore.Create("feature")
	if err != nil {
		t.Fatalf("failed to create session: %v", err)
	}
	session.AddLog("user", "test idea")
	session.AddLog("system", "processing...")

	srv.mux.HandleFunc("GET /api/v2/wizard/sessions/{id}/logs", srv.handleWizardLogsV2)

	req := httptest.NewRequest(http.MethodGet, "/api/v2/wizard/sessions/"+session.ID+"/logs", nil)
	rec := httptest.NewRecorder()

	srv.mux.ServeHTTP(rec, req)

	assertStatus(t, rec, http.StatusOK)

	var resp map[string]any
	decodeResponse(t, rec, &resp)

	logs, ok := resp["logs"].([]any)
	if !ok {
		t.Fatal("expected 'logs' to be an array")
	}
	if len(logs) != 2 {
		t.Errorf("expected 2 log entries, got %d", len(logs))
	}
}

// ---------------------------------------------------------------------------
// Sprint close preview tests
// ---------------------------------------------------------------------------

func TestHandleSprintClosePreviewV2_NoMilestone(t *testing.T) {
	srv := newTestServerV2(t)
	defer srv.wizardStore.Stop()

	body := jsonBody(t, map[string]string{"bump_type": "patch"})
	req := httptest.NewRequest(http.MethodPost, "/api/v2/sprint/close/preview", body)
	req.Header.Set("Content-Type", "application/json")
	rec := httptest.NewRecorder()

	srv.handleSprintClosePreviewV2(rec, req)

	assertStatus(t, rec, http.StatusBadRequest)
}

func TestHandleSprintCloseConfirmV2_NoMilestone(t *testing.T) {
	srv := newTestServerV2(t)
	defer srv.wizardStore.Stop()

	body := jsonBody(t, map[string]any{
		"bump_type":     "patch",
		"release_title": "Test Release",
		"release_body":  "Test body",
	})
	req := httptest.NewRequest(http.MethodPost, "/api/v2/sprint/close/confirm", body)
	req.Header.Set("Content-Type", "application/json")
	rec := httptest.NewRecorder()

	srv.handleSprintCloseConfirmV2(rec, req)

	assertStatus(t, rec, http.StatusBadRequest)
}

// ---------------------------------------------------------------------------
// Helper function tests
// ---------------------------------------------------------------------------

func TestParseIssueID(t *testing.T) {
	tests := []struct {
		name    string
		pathVal string
		wantID  int
		wantErr bool
	}{
		{"valid", "42", 42, false},
		{"zero", "0", 0, true},
		{"negative", "-1", 0, true},
		{"non-numeric", "abc", 0, true},
		// Note: empty path value is not testable via mux routing since the
		// mux won't match an empty segment. In practice, the mux would 404.
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			mux := http.NewServeMux()
			var gotID int
			var gotErr error

			mux.HandleFunc("GET /test/{id}", func(_ http.ResponseWriter, r *http.Request) {
				gotID, gotErr = parseIssueID(r)
			})

			req := httptest.NewRequest(http.MethodGet, "/test/"+tt.pathVal, nil)
			rec := httptest.NewRecorder()
			mux.ServeHTTP(rec, req)

			if tt.wantErr && gotErr == nil {
				t.Error("expected error, got nil")
			}
			if !tt.wantErr && gotErr != nil {
				t.Errorf("unexpected error: %v", gotErr)
			}
			if !tt.wantErr && gotID != tt.wantID {
				t.Errorf("expected ID %d, got %d", tt.wantID, gotID)
			}
		})
	}
}

func TestWriteJSON(t *testing.T) {
	rec := httptest.NewRecorder()
	writeJSON(rec, http.StatusOK, map[string]string{"key": "value"})

	assertStatus(t, rec, http.StatusOK)
	assertJSONContentType(t, rec)

	var resp map[string]string
	if err := json.NewDecoder(rec.Body).Decode(&resp); err != nil {
		t.Fatalf("failed to decode: %v", err)
	}
	if resp["key"] != "value" {
		t.Errorf("expected key=value, got %v", resp)
	}
}

func TestWriteError(t *testing.T) {
	rec := httptest.NewRecorder()
	writeError(rec, http.StatusBadRequest, "test error")

	assertStatus(t, rec, http.StatusBadRequest)

	var resp map[string]string
	if err := json.NewDecoder(rec.Body).Decode(&resp); err != nil {
		t.Fatalf("failed to decode: %v", err)
	}
	if resp["error"] != "test error" {
		t.Errorf("expected error='test error', got %v", resp)
	}
}

func TestCalculateNewVersion(t *testing.T) {
	tests := []struct {
		current  string
		bump     string
		expected string
	}{
		{"1.2.3", "patch", "1.2.4"},
		{"1.2.3", "minor", "1.3.0"},
		{"1.2.3", "major", "2.0.0"},
		{"0.0.0", "patch", "0.0.1"},
		{"invalid", "patch", "0.0.1"},
	}

	for _, tt := range tests {
		t.Run(tt.current+"_"+tt.bump, func(t *testing.T) {
			result := calculateNewVersion(tt.current, tt.bump)
			if result != tt.expected {
				t.Errorf("calculateNewVersion(%q, %q) = %q, want %q", tt.current, tt.bump, result, tt.expected)
			}
		})
	}
}

func TestConvertSteps(t *testing.T) {
	steps := convertSteps(nil)
	if len(steps) != 0 {
		t.Errorf("expected empty slice for nil input, got %d", len(steps))
	}

	steps = convertSteps([]db.TaskStep{})
	if len(steps) != 0 {
		t.Errorf("expected empty slice for empty input, got %d", len(steps))
	}
}

func TestHandleSprintCloseConfirmV2_UsesGetDefaultBranch(t *testing.T) {
	src, err := os.ReadFile("api_v2.go")
	if err != nil {
		t.Fatalf("failed to read api_v2.go: %v", err)
	}
	content := string(src)

	if strings.Contains(content, `CreateTag(tagName, "master"`) {
		t.Error("api_v2.go still contains hardcoded 'master' in CreateTag call; should use GetDefaultBranch()")
	}

	if !strings.Contains(content, "GetDefaultBranch()") {
		t.Error("api_v2.go should call GetDefaultBranch() to determine the branch for tag creation")
	}
}
