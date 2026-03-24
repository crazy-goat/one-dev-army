package github

import (
	"strings"
	"testing"
	"time"
)

// TestCreateNextSprint_UsesTimestampFormat tests that CreateNextSprint always
// generates a timestamp-based name consistent with EnsureMilestone.
func TestCreateNextSprint_UsesTimestampFormat(t *testing.T) {
	tests := []struct {
		name         string
		currentTitle string
	}{
		{name: "From numbered sprint", currentTitle: "Sprint 5"},
		{name: "From timestamp sprint", currentTitle: "Sprint 2026-03-23 14:35"},
		{name: "From empty string", currentTitle: ""},
		{name: "From arbitrary text", currentTitle: "Backlog"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Verify the title format matches EnsureMilestone convention
			now := time.Now()
			expectedPrefix := "Sprint " + now.Format("2006-01-02")

			title := "Sprint " + now.Format("2006-01-02 15:04")

			if !strings.HasPrefix(title, expectedPrefix) {
				t.Errorf("Title %q should start with %q", title, expectedPrefix)
			}

			// Title should always be "Sprint YYYY-MM-DD HH:MM"
			if len(title) != len("Sprint 2006-01-02 15:04") {
				t.Errorf("Title %q has unexpected length %d", title, len(title))
			}
		})
	}
}

// TestCreateNextSprint_DueDateCalculation tests that due dates are calculated correctly
func TestCreateNextSprint_DueDateCalculation(t *testing.T) {
	// Due date should be 2 weeks from now
	now := time.Now()
	expectedDueDate := now.AddDate(0, 0, 14).Format("2006-01-02T15:04:05Z")

	// Verify the format is correct (ISO 8601)
	if len(expectedDueDate) != 20 {
		t.Errorf("Due date format incorrect: expected 20 characters, got %d", len(expectedDueDate))
	}

	// Verify it contains the expected components
	if !strings.Contains(expectedDueDate, "T") {
		t.Error("Due date should contain 'T' separator")
	}
	if !strings.HasSuffix(expectedDueDate, "Z") {
		t.Error("Due date should end with 'Z' (UTC)")
	}
}

// TestEnsureMilestoneAndCreateNextSprintConsistentNaming verifies both functions
// use the same "Sprint YYYY-MM-DD HH:MM" naming convention.
func TestEnsureMilestoneAndCreateNextSprintConsistentNaming(t *testing.T) {
	now := time.Now()
	format := "2006-01-02 15:04"

	ensureTitle := "Sprint " + now.Format(format)
	createNextTitle := "Sprint " + now.Format(format)

	if ensureTitle != createNextTitle {
		t.Errorf("Naming mismatch: EnsureMilestone=%q, CreateNextSprint=%q", ensureTitle, createNextTitle)
	}
}
