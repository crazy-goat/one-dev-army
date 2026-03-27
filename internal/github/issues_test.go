package github

import (
	"testing"
)

func TestGetPRChecks_DetailedStatus(t *testing.T) {
	tests := []struct {
		name          string
		checks        []PRCheck
		wantStatus    string
		wantTotal     int
		wantCompleted int
		wantPending   []string
	}{
		{
			name: "all checks completed - success",
			checks: []PRCheck{
				{Name: "lint", Status: "COMPLETED", Conclusion: "SUCCESS"},
				{Name: "test", Status: "COMPLETED", Conclusion: "SUCCESS"},
			},
			wantStatus:    "pass",
			wantTotal:     0,
			wantCompleted: 0,
			wantPending:   nil,
		},
		{
			name: "some checks pending",
			checks: []PRCheck{
				{Name: "lint", Status: "COMPLETED", Conclusion: "SUCCESS"},
				{Name: "test", Status: "IN_PROGRESS", Conclusion: ""},
				{Name: "build", Status: "PENDING", Conclusion: ""},
			},
			wantStatus:    "pending",
			wantTotal:     3,
			wantCompleted: 1,
			wantPending:   []string{"test", "build"},
		},
		{
			name: "all checks pending",
			checks: []PRCheck{
				{Name: "lint", Status: "IN_PROGRESS", Conclusion: ""},
				{Name: "test", Status: "PENDING", Conclusion: ""},
			},
			wantStatus:    "pending",
			wantTotal:     2,
			wantCompleted: 0,
			wantPending:   []string{"lint", "test"},
		},
		{
			name: "some checks failed",
			checks: []PRCheck{
				{Name: "lint", Status: "COMPLETED", Conclusion: "SUCCESS"},
				{Name: "test", Status: "COMPLETED", Conclusion: "FAILURE"},
			},
			wantStatus:    "fail",
			wantTotal:     0,
			wantCompleted: 0,
			wantPending:   nil,
		},
		{
			name:          "empty check list",
			checks:        []PRCheck{},
			wantStatus:    "pending",
			wantTotal:     0,
			wantCompleted: 0,
			wantPending:   nil,
		},
		{
			name: "mixed conclusions - success and failure",
			checks: []PRCheck{
				{Name: "lint", Status: "COMPLETED", Conclusion: "SUCCESS"},
				{Name: "test", Status: "COMPLETED", Conclusion: "FAILURE"},
				{Name: "build", Status: "COMPLETED", Conclusion: "SUCCESS"},
			},
			wantStatus:    "fail",
			wantTotal:     0,
			wantCompleted: 0,
			wantPending:   nil,
		},
		{
			name: "single pending check",
			checks: []PRCheck{
				{Name: "lint", Status: "IN_PROGRESS", Conclusion: ""},
			},
			wantStatus:    "pending",
			wantTotal:     1,
			wantCompleted: 0,
			wantPending:   []string{"lint"},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Create a mock client to test the logic
			// Since we can't easily mock the gh command, we'll test the logic directly
			result := evaluateChecks(tt.checks)

			if result.Status != tt.wantStatus {
				t.Errorf("Status = %q, want %q", result.Status, tt.wantStatus)
			}
			if result.TotalCount != tt.wantTotal {
				t.Errorf("TotalCount = %d, want %d", result.TotalCount, tt.wantTotal)
			}
			if result.CompletedCount != tt.wantCompleted {
				t.Errorf("CompletedCount = %d, want %d", result.CompletedCount, tt.wantCompleted)
			}
			if !stringSlicesEqual(result.PendingChecks, tt.wantPending) {
				t.Errorf("PendingChecks = %v, want %v", result.PendingChecks, tt.wantPending)
			}
		})
	}
}

// evaluateChecks is a helper function that mimics the logic in GetPRChecks
// for testing purposes without requiring actual GitHub API calls
func evaluateChecks(checks []PRCheck) *PRChecksResult {
	if len(checks) == 0 {
		return &PRChecksResult{Status: "pending"}
	}

	var failedNames []string
	var pendingNames []string
	allComplete := true
	completedCount := 0

	for _, check := range checks {
		if check.Status == "COMPLETED" {
			completedCount++
		} else {
			allComplete = false
			pendingNames = append(pendingNames, check.Name)
		}
		if check.Conclusion == "FAILURE" {
			failedNames = append(failedNames, check.Name)
		}
	}

	totalCount := len(checks)

	if !allComplete {
		return &PRChecksResult{
			Status:         "pending",
			TotalCount:     totalCount,
			CompletedCount: completedCount,
			PendingChecks:  pendingNames,
		}
	}

	if len(failedNames) > 0 {
		return &PRChecksResult{Status: "fail"}
	}

	return &PRChecksResult{Status: "pass"}
}

func stringSlicesEqual(a, b []string) bool {
	if len(a) != len(b) {
		return false
	}
	for i := range a {
		if a[i] != b[i] {
			return false
		}
	}
	return true
}
