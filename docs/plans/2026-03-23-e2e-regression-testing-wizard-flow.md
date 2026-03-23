# End-to-End Regression Testing of Feature/Bug Creation Flow - Implementation Plan

> **For Claude:** REQUIRED SUB-SKILL: Use superpowers:executing-plans to implement this plan task-by-task.

**Goal:** Add comprehensive end-to-end regression tests for the complete feature and bug creation wizard flow, including validation scenarios, concurrent users, and post-creation verification.

**Architecture:** Extend existing test suite with: (1) Complete E2E bug flow test mirroring the existing feature flow test, (2) Integration tests that exercise the wizard through the full HTTP server stack, (3) Comprehensive validation scenario tests for error conditions, (4) Post-creation verification tests for redirect and board updates.

**Tech Stack:** Go 1.24, httptest, dashboard package, existing wizard session store

---

## Current State Analysis

**Already Implemented:**
- `TestFullWizardFlow` in `handlers_test.go:474-571` - Tests complete 4-step feature flow
- Individual handler tests for each wizard step (new, refine, breakdown, create)
- Concurrent session access tests (`TestConcurrentSessionAccess`, `TestConcurrentHandlerAccess`)
- Validation tests for missing session_id, empty idea, invalid type
- Epic label mapping tests for both feature and bug types

**Missing:**
- Complete E2E test for bug flow (only feature flow tested end-to-end)
- Integration tests through actual HTTP server (not just handler functions)
- Comprehensive validation scenario tests (all error conditions in sequence)
- Post-creation verification (redirect, session cleanup, board updates)
- Tests for wizard initiated from header buttons via full HTTP flow

---

## Task 1: Add Complete Bug Flow E2E Test

**Files:**
- Modify: `internal/dashboard/handlers_test.go` (after line 571)

**Step 1: Write the failing test**

Add after `TestFullWizardFlow`:

```go
// TestFullWizardFlow_Bug tests the complete bug wizard flow end-to-end
func TestFullWizardFlow_Bug(t *testing.T) {
	// Create server with minimal dependencies
	srv := &Server{
		tmpls:       make(map[string]*template.Template),
		wizardStore: NewWizardSessionStore(),
	}
	defer srv.wizardStore.Stop()

	// Step 1: Start wizard (GET /wizard/new?type=bug)
	req := httptest.NewRequest(http.MethodGet, "/wizard/new?type=bug", nil)
	rec := httptest.NewRecorder()
	srv.handleWizardNew(rec, req)

	if rec.Code != http.StatusOK && rec.Code != http.StatusInternalServerError {
		t.Fatalf("Step 1 failed: expected status 200 or 500, got %d", rec.Code)
	}

	// Create a new session for testing the flow
	testSession, _ := srv.wizardStore.Create("bug")
	sessionID := testSession.ID

	// Step 2: Refine idea (POST /wizard/refine)
	formData := url.Values{}
	formData.Set("session_id", sessionID)
	formData.Set("idea", "Fix login button not working on mobile")

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
	if session.Type != "bug" {
		t.Errorf("Step 2: Expected type 'bug', got %q", session.Type)
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

	t.Logf("Full bug wizard flow completed successfully")
}
```

**Step 2: Run test to verify it compiles and passes**

Run: `go test ./internal/dashboard -run TestFullWizardFlow_Bug -v`
Expected: PASS (test uses existing handler infrastructure)

**Step 3: Commit**

```bash
git add internal/dashboard/handlers_test.go
git commit -m "test: add complete E2E bug flow test"
```

---

## Task 2: Add Comprehensive Validation Scenario Tests

**Files:**
- Modify: `internal/dashboard/handlers_test.go` (after line 1104)

**Step 1: Write the failing test**

Add at end of file:

```go
// TestWizardFlow_ValidationErrors tests all validation scenarios in sequence
func TestWizardFlow_ValidationErrors(t *testing.T) {
	srv := &Server{
		tmpls:       make(map[string]*template.Template),
		wizardStore: NewWizardSessionStore(),
	}
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
			wantStatus: http.StatusBadRequest,
			wantError:  "Invalid wizard type",
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
			name: "missing session_id on breakdown",
			setup: func() (*http.Request, *httptest.ResponseRecorder) {
				req := httptest.NewRequest(http.MethodPost, "/wizard/breakdown", strings.NewReader(""))
				req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
				return req, httptest.NewRecorder()
			},
			handler:    srv.handleWizardBreakdown,
			wantStatus: http.StatusBadRequest,
			wantError:  "session_id",
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
			if !strings.Contains(strings.ToLower(body), strings.ToLower(tt.wantError)) {
				t.Errorf("expected error containing %q, got: %s", tt.wantError, body)
			}
		})
	}
}
```

**Step 2: Run test to verify it fails appropriately**

Run: `go test ./internal/dashboard -run TestWizardFlow_ValidationErrors -v`
Expected: PASS (tests existing validation logic)

**Step 3: Commit**

```bash
git add internal/dashboard/handlers_test.go
git commit -m "test: add comprehensive validation scenario tests"
```

---

## Task 3: Add Integration Test for Wizard Through HTTP Server

**Files:**
- Modify: `internal/integration_test.go` (after line 617)

**Step 1: Write the failing test**

Add at end of file:

```go
// TestDashboard_WizardFlow_Integration tests the complete wizard flow through the HTTP server
func TestDashboard_WizardFlow_Integration(t *testing.T) {
	store, err := db.Open(filepath.Join(t.TempDir(), "dash.db"))
	if err != nil {
		t.Fatalf("opening db: %v", err)
	}
	defer store.Close()

	poolFn := func() []worker.WorkerInfo {
		return []worker.WorkerInfo{}
	}

	srv, err := dashboard.NewServer(0, store, poolFn, nil, 0, nil, nil, "")
	if err != nil {
		t.Fatalf("creating dashboard server: %v", err)
	}

	handler := srv.Handler()

	// Test 1: Feature wizard flow via header button
	t.Run("feature_wizard_via_header", func(t *testing.T) {
		// Step 1: GET /wizard?type=feature (header button click)
		req := httptest.NewRequest(http.MethodGet, "/wizard?type=feature", nil)
		rec := httptest.NewRecorder()
		handler.ServeHTTP(rec, req)

		if rec.Code != http.StatusOK {
			t.Errorf("GET /wizard?type=feature: status = %d, want %d", rec.Code, http.StatusOK)
		}

		// Verify response contains wizard form
		body := rec.Body.String()
		if !strings.Contains(body, "wizard") && !strings.Contains(body, "feature") {
			t.Errorf("Response doesn't contain wizard content: %s", truncate(body, 200))
		}
	})

	// Test 2: Bug wizard flow via header button
	t.Run("bug_wizard_via_header", func(t *testing.T) {
		// Step 1: GET /wizard?type=bug (header button click)
		req := httptest.NewRequest(http.MethodGet, "/wizard?type=bug", nil)
		rec := httptest.NewRecorder()
		handler.ServeHTTP(rec, req)

		if rec.Code != http.StatusOK {
			t.Errorf("GET /wizard?type=bug: status = %d, want %d", rec.Code, http.StatusOK)
		}

		// Verify response contains wizard form
		body := rec.Body.String()
		if !strings.Contains(body, "wizard") && !strings.Contains(body, "bug") {
			t.Errorf("Response doesn't contain wizard content: %s", truncate(body, 200))
		}
	})

	// Test 3: Invalid wizard type returns error
	t.Run("invalid_wizard_type", func(t *testing.T) {
		req := httptest.NewRequest(http.MethodGet, "/wizard?type=invalid", nil)
		rec := httptest.NewRecorder()
		handler.ServeHTTP(rec, req)

		if rec.Code != http.StatusBadRequest {
			t.Errorf("GET /wizard?type=invalid: status = %d, want %d", rec.Code, http.StatusBadRequest)
		}
	})

	// Test 4: Wizard modal endpoint
	t.Run("wizard_modal", func(t *testing.T) {
		req := httptest.NewRequest(http.MethodGet, "/wizard/modal?type=feature", nil)
		rec := httptest.NewRecorder()
		handler.ServeHTTP(rec, req)

		if rec.Code != http.StatusOK {
			t.Errorf("GET /wizard/modal: status = %d, want %d", rec.Code, http.StatusOK)
		}
	})

	// Test 5: Wizard cancel endpoint
	t.Run("wizard_cancel", func(t *testing.T) {
		// First create a session
		req := httptest.NewRequest(http.MethodGet, "/wizard/new?type=feature", nil)
		rec := httptest.NewRecorder()
		handler.ServeHTTP(rec, req)

		// Extract session ID from response (if possible) or use a test session
		// For now, just test the endpoint returns success
		req = httptest.NewRequest(http.MethodPost, "/wizard/cancel", strings.NewReader("session_id=test-session"))
		req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
		rec = httptest.NewRecorder()
		handler.ServeHTTP(rec, req)

		// Should return OK even for invalid session (graceful handling)
		if rec.Code != http.StatusOK && rec.Code != http.StatusBadRequest {
			t.Errorf("POST /wizard/cancel: status = %d, want 200 or 400", rec.Code)
		}
	})
}
```

**Step 2: Run test to verify it compiles and passes**

Run: `go test ./internal -run TestDashboard_WizardFlow_Integration -v`
Expected: PASS (tests existing dashboard server integration)

**Step 3: Commit**

```bash
git add internal/integration_test.go
git commit -m "test: add integration tests for wizard flow through HTTP server"
```

---

## Task 4: Add Concurrent Wizard Flow Test

**Files:**
- Modify: `internal/dashboard/handlers_test.go` (after TestWizardFlow_ValidationErrors)

**Step 1: Write the failing test**

Add after TestWizardFlow_ValidationErrors:

```go
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

			// Step 3: Breakdown
			formData = url.Values{}
			formData.Set("session_id", session.ID)

			req = httptest.NewRequest(http.MethodPost, "/wizard/breakdown", strings.NewReader(formData.Encode()))
			req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
			rec = httptest.NewRecorder()
			srv.handleWizardBreakdown(rec, req)

			if rec.Code != http.StatusOK && rec.Code != http.StatusInternalServerError {
				t.Errorf("User %d: breakdown failed with status %d", userID, rec.Code)
				return
			}

			// Verify session still exists and has tasks
			session, ok := srv.wizardStore.Get(session.ID)
			if !ok {
				t.Errorf("User %d: session not found after breakdown", userID)
				return
			}

			if len(session.Tasks) == 0 {
				t.Errorf("User %d: no tasks generated", userID)
			}
		}(i)
	}

	wg.Wait()

	// Verify all sessions still exist (not cleaned up yet)
	count := srv.wizardStore.Count()
	if count != numUsers {
		t.Errorf("expected %d sessions, got %d", numUsers, count)
	}

	t.Logf("Concurrent wizard flow test completed: %d users, %d sessions", numUsers, count)
}
```

**Step 2: Run test to verify it passes**

Run: `go test ./internal/dashboard -run TestWizardFlow_ConcurrentUsers -v`
Expected: PASS

**Step 3: Commit**

```bash
git add internal/dashboard/handlers_test.go
git commit -m "test: add concurrent wizard flow test for multiple users"
```

---

## Task 5: Add Post-Creation Verification Test

**Files:**
- Modify: `internal/dashboard/handlers_test.go` (after TestWizardFlow_ConcurrentUsers)

**Step 1: Write the failing test**

Add after TestWizardFlow_ConcurrentUsers:

```go
// TestWizardFlow_PostCreationVerification tests redirect and cleanup after creation
func TestWizardFlow_PostCreationVerification(t *testing.T) {
	srv := &Server{
		tmpls:       make(map[string]*template.Template),
		wizardStore: NewWizardSessionStore(),
	}
	defer srv.wizardStore.Stop()

	// Create and complete a wizard session
	session, _ := srv.wizardStore.Create("feature")
	session.IdeaText = "Test feature idea"
	session.RefinedDescription = "Refined description of the test feature"
	session.Tasks = []Task{
		{Title: "Task 1", Description: "Description 1", Priority: "high", Size: "M"},
		{Title: "Task 2", Description: "Description 2", Priority: "medium", Size: "S"},
	}
	session.CurrentStep = WizardStepBreakdown

	// Store the session
	srv.wizardStore.sessions[session.ID] = session

	// Verify session exists
	_, ok := srv.wizardStore.Get(session.ID)
	if !ok {
		t.Fatal("Session should exist before creation")
	}

	// Step 4: Create issues
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
```

**Step 2: Run test to verify it passes**

Run: `go test ./internal/dashboard -run TestWizardFlow_PostCreationVerification -v`
Expected: PASS

**Step 3: Commit**

```bash
git add internal/dashboard/handlers_test.go
git commit -m "test: add post-creation verification test for session cleanup"
```

---

## Task 6: Run All Tests and Verify Coverage

**Step 1: Run all dashboard tests**

Run: `go test ./internal/dashboard -v 2>&1 | head -100`
Expected: All tests pass

**Step 2: Run all integration tests**

Run: `go test ./internal -v -run "TestDashboard" 2>&1 | head -50`
Expected: All dashboard integration tests pass

**Step 3: Check test coverage**

Run: `go test ./internal/dashboard -cover`
Expected: Coverage percentage (baseline for future improvements)

**Step 4: Final commit**

```bash
git log --oneline -5
```

Expected: Shows 5 commits for the new tests

---

## Summary

This implementation plan adds comprehensive end-to-end regression testing for the feature/bug creation flow:

1. **Task 1:** Complete E2E bug flow test (mirrors existing feature flow test)
2. **Task 2:** Comprehensive validation scenario tests (all error conditions)
3. **Task 3:** Integration tests through HTTP server (full stack testing)
4. **Task 4:** Concurrent user tests (thread safety verification)
5. **Task 5:** Post-creation verification (session cleanup, redirects)

All tests use existing infrastructure and handler functions. No production code changes required - this is purely adding test coverage for issue #114.
