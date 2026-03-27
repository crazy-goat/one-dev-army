package dashboard

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
)

func TestGetLastTag(t *testing.T) {
	srv := newTestServerV2(t)
	defer srv.wizardStore.Stop()

	// Without a GitHub client configured, skip this test
	if srv.gh == nil {
		t.Skip("Skipping test: no GitHub client configured")
	}

	req := httptest.NewRequest(http.MethodGet, "/api/v2/sprint/last-tag", nil)
	rec := httptest.NewRecorder()

	srv.handleGetLastTag(rec, req)

	// The endpoint should exist and return a valid response
	// It may return 404 if no tags exist, or 200 with tag data
	if rec.Code != http.StatusOK && rec.Code != http.StatusNotFound {
		t.Errorf("expected status 200 or 404, got %d (body: %s)", rec.Code, rec.Body.String())
	}

	// If we got a 200, verify the response structure
	if rec.Code == http.StatusOK {
		var tag map[string]interface{}
		if err := json.Unmarshal(rec.Body.Bytes(), &tag); err != nil {
			t.Errorf("failed to decode response: %v (body: %s)", err, rec.Body.String())
		}
		if tag["tag"] == nil {
			t.Error("expected 'tag' field in response")
		}
	}
}

func TestGetUnassignedIssues(t *testing.T) {
	srv := newTestServerV2(t)
	defer srv.wizardStore.Stop()

	// Without a GitHub client configured, skip this test
	if srv.gh == nil {
		t.Skip("Skipping test: no GitHub client configured")
	}

	req := httptest.NewRequest(http.MethodGet, "/api/v2/issues/unassigned", nil)
	rec := httptest.NewRecorder()

	srv.handleGetUnassignedIssues(rec, req)

	// Should return 200 OK (may be empty array if no issues)
	if rec.Code != http.StatusOK {
		t.Errorf("expected status 200, got %d (body: %s)", rec.Code, rec.Body.String())
	}

	// Verify response is valid JSON array
	var issues []IssueCandidate
	if err := json.Unmarshal(rec.Body.Bytes(), &issues); err != nil {
		t.Errorf("failed to decode response: %v (body: %s)", err, rec.Body.String())
	}

	// Verify all returned issues have required fields
	for _, issue := range issues {
		if issue.Number == 0 {
			t.Error("issue number should not be zero")
		}
		if issue.Title == "" {
			t.Error("issue title should not be empty")
		}
	}
}
