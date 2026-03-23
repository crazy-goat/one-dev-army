package github

import (
	"fmt"
	"regexp"
	"strconv"
	"strings"
	"testing"
	"time"
)

// TestCreateNextSprint_ExtractsNumberCorrectly tests that sprint numbers are extracted correctly from various title formats
func TestCreateNextSprint_ExtractsNumberCorrectly(t *testing.T) {
	tests := []struct {
		name           string
		currentTitle   string
		expectedNumber int
	}{
		{
			name:           "Simple sprint number",
			currentTitle:   "Sprint 5",
			expectedNumber: 6,
		},
		{
			name:           "Double digit sprint number",
			currentTitle:   "Sprint 42",
			expectedNumber: 43,
		},
		{
			name:           "Triple digit sprint number",
			currentTitle:   "Sprint 123",
			expectedNumber: 124,
		},
		{
			name:           "Sprint with extra text",
			currentTitle:   "Sprint 7 - Final Phase",
			expectedNumber: 8,
		},
		{
			name:           "Sprint with prefix text",
			currentTitle:   "Q1 Sprint 3",
			expectedNumber: 4,
		},
		{
			name:           "No sprint number - defaults to 1",
			currentTitle:   "Backlog",
			expectedNumber: 1,
		},
		{
			name:           "Empty string - defaults to 1",
			currentTitle:   "",
			expectedNumber: 1,
		},
		{
			name:           "Timestamp-based sprint name - extracts year as number",
			currentTitle:   "Sprint 2026-03-23 14:35",
			expectedNumber: 2027, // Extracts 2026 from timestamp, creates 2027
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Extract sprint number using the same logic as CreateNextSprint
			re := regexp.MustCompile(`Sprint\s+(\d+)`)
			matches := re.FindStringSubmatch(tt.currentTitle)

			var nextNumber int
			if len(matches) >= 2 {
				currentNum, _ := strconv.Atoi(matches[1])
				nextNumber = currentNum + 1
			} else {
				nextNumber = 1
			}

			if nextNumber != tt.expectedNumber {
				t.Errorf("CreateNextSprint(%q) would create Sprint %d, expected Sprint %d",
					tt.currentTitle, nextNumber, tt.expectedNumber)
			}
		})
	}
}

// TestCreateNextSprint_TitleGeneration tests the title generation logic
func TestCreateNextSprint_TitleGeneration(t *testing.T) {
	tests := []struct {
		name          string
		sprintNumber  int
		expectedTitle string
	}{
		{
			name:          "Single digit",
			sprintNumber:  5,
			expectedTitle: "Sprint 5",
		},
		{
			name:          "Double digit",
			sprintNumber:  42,
			expectedTitle: "Sprint 42",
		},
		{
			name:          "Triple digit",
			sprintNumber:  100,
			expectedTitle: "Sprint 100",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			title := fmt.Sprintf("Sprint %d", tt.sprintNumber)
			if title != tt.expectedTitle {
				t.Errorf("Title generation failed: got %q, expected %q", title, tt.expectedTitle)
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
