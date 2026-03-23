package plan

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

func TestPlanToMarkdown(t *testing.T) {
	plan := &Plan{
		IssueNumber: 159,
		Analysis:    "This is a test analysis.",
		ImplementationSteps: []Step{
			{
				Order:       1,
				Description: "Create plan.go file",
				Files:       []string{"internal/plan/plan.go"},
				Details:     "Add Plan struct and ToMarkdown method",
			},
			{
				Order:       2,
				Description: "Create attachment.go file",
				Files:       []string{"internal/plan/attachment.go"},
				Details:     "Add AttachmentManager for GitHub integration",
			},
		},
		TestPlan:  []string{"Test ToMarkdown", "Test ParseFromMarkdown"},
		CreatedAt: time.Date(2024, 1, 1, 0, 0, 0, 0, time.UTC),
	}

	markdown := plan.ToMarkdown()

	// Check that markdown contains expected content
	if !strings.Contains(markdown, "# Implementation Plan for Issue #159") {
		t.Error("Expected markdown to contain issue number in title")
	}
	if !strings.Contains(markdown, "This is a test analysis.") {
		t.Error("Expected markdown to contain analysis")
	}
	if !strings.Contains(markdown, "## Implementation Steps") {
		t.Error("Expected markdown to contain implementation steps section")
	}
	if !strings.Contains(markdown, "Step 1: Create plan.go file") {
		t.Error("Expected markdown to contain step 1")
	}
	if !strings.Contains(markdown, "internal/plan/plan.go") {
		t.Error("Expected markdown to contain file reference")
	}
	if !strings.Contains(markdown, "## Test Plan") {
		t.Error("Expected markdown to contain test plan section")
	}
}

func TestParseFromMarkdown(t *testing.T) {
	markdown := `# Implementation Plan for Issue #159

**Created:** 2024-01-01T00:00:00Z

## Analysis

This is a test analysis.

## Implementation Steps

### Step 1: Create plan.go file

**Files:**
- ` + "`internal/plan/plan.go`" + `

Add Plan struct and ToMarkdown method

### Step 2: Create attachment.go file

**Files:**
- ` + "`internal/plan/attachment.go`" + `

Add AttachmentManager for GitHub integration

## Test Plan

- [ ] Test ToMarkdown
- [ ] Test ParseFromMarkdown
`

	plan, err := ParseFromMarkdown(markdown)
	if err != nil {
		t.Fatalf("Failed to parse markdown: %v", err)
	}

	if plan.IssueNumber != 159 {
		t.Errorf("Expected issue number 159, got %d", plan.IssueNumber)
	}

	if !strings.Contains(plan.Analysis, "This is a test analysis.") {
		t.Error("Expected analysis to be parsed correctly")
	}

	if len(plan.ImplementationSteps) != 2 {
		t.Errorf("Expected 2 steps, got %d", len(plan.ImplementationSteps))
	}

	if len(plan.TestPlan) != 2 {
		t.Errorf("Expected 2 test items, got %d", len(plan.TestPlan))
	}
}

func TestSaveAndLoadPlan(t *testing.T) {
	// Create a temporary directory
	tmpDir := t.TempDir()
	planPath := filepath.Join(tmpDir, "plan.md")

	// Create a plan
	original := &Plan{
		IssueNumber: 42,
		Analysis:    "Test analysis for issue #42",
		ImplementationSteps: []Step{
			{
				Order:       1,
				Description: "First step",
				Files:       []string{"file1.go", "file2.go"},
				Details:     "Details for first step",
			},
		},
		TestPlan:  []string{"Test 1", "Test 2"},
		CreatedAt: time.Now(),
	}

	// Save the plan
	if err := original.SaveToFile(planPath); err != nil {
		t.Fatalf("Failed to save plan: %v", err)
	}

	// Verify file exists
	if _, err := os.Stat(planPath); err != nil {
		t.Fatalf("Plan file was not created: %v", err)
	}

	// Load the plan
	loaded, err := ParseFromFile(planPath)
	if err != nil {
		t.Fatalf("Failed to load plan: %v", err)
	}

	// Verify loaded data
	if loaded.IssueNumber != original.IssueNumber {
		t.Errorf("Expected issue number %d, got %d", original.IssueNumber, loaded.IssueNumber)
	}

	if !strings.Contains(loaded.Analysis, original.Analysis) {
		t.Error("Analysis was not preserved correctly")
	}

	if len(loaded.ImplementationSteps) != len(original.ImplementationSteps) {
		t.Errorf("Expected %d steps, got %d", len(original.ImplementationSteps), len(loaded.ImplementationSteps))
	}
}

func TestGetPlanFilePath(t *testing.T) {
	repoDir := "/tmp/test-repo"
	expected := filepath.Join(repoDir, "plan.md")
	actual := GetPlanFilePath(repoDir)
	if actual != expected {
		t.Errorf("Expected %s, got %s", expected, actual)
	}
}

func TestParseSteps(t *testing.T) {
	content := `1. Create plan.go file
Files:
- internal/plan/plan.go
- internal/plan/attachment.go

Add the Plan struct and methods

2. Update worker.go
Files:
- internal/mvp/worker.go

Integrate plan creation in analyze and plan methods`

	steps := parseSteps(content)

	if len(steps) != 2 {
		t.Errorf("Expected 2 steps, got %d", len(steps))
	}

	if len(steps) > 0 {
		if steps[0].Order != 1 {
			t.Errorf("Expected step order 1, got %d", steps[0].Order)
		}
		if !strings.Contains(steps[0].Description, "Create plan.go") {
			t.Errorf("Expected description to contain 'Create plan.go', got %s", steps[0].Description)
		}
		if len(steps[0].Files) != 2 {
			t.Errorf("Expected 2 files, got %d", len(steps[0].Files))
		}
	}
}

func TestMatchesStepHeader(t *testing.T) {
	tests := []struct {
		line     string
		expected bool
	}{
		{"1. First step", true},
		{"Step 1: First step", true},
		{"1) First step", true},
		{"Regular text", false},
		{"## Header", false},
	}

	for _, test := range tests {
		result := matchesStepHeader(test.line)
		if result != test.expected {
			t.Errorf("matchesStepHeader(%q) = %v, expected %v", test.line, result, test.expected)
		}
	}
}

func TestExtractStepInfo(t *testing.T) {
	tests := []struct {
		line          string
		expectedOrder int
		expectedDesc  string
	}{
		{"1. First step", 1, "First step"},
		{"Step 2: Second step", 2, "Second step"},
		{"3) Third step", 3, "Third step"},
		{"42. Complex step description", 42, "Complex step description"},
	}

	for _, test := range tests {
		order, desc := extractStepInfo(test.line)
		if order != test.expectedOrder {
			t.Errorf("extractStepInfo(%q) order = %d, expected %d", test.line, order, test.expectedOrder)
		}
		if desc != test.expectedDesc {
			t.Errorf("extractStepInfo(%q) desc = %q, expected %q", test.line, desc, test.expectedDesc)
		}
	}
}
