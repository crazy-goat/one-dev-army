package dashboard

import (
	"encoding/json"
	"strings"
	"testing"

	"github.com/crazy-goat/one-dev-army/internal/prompts"
)

// TestRefinementPrompt_EnglishOnlyConstraint verifies prompt requires English output
func TestRefinementPrompt_EnglishOnlyConstraint(t *testing.T) {
	refinementPrompt := prompts.MustGet(prompts.DashboardRefinement)
	if !strings.Contains(refinementPrompt, "ALL output MUST be in English") {
		t.Error("RefinementPromptTemplate must enforce English-only output")
	}
}

func TestBuildIssueGenerationPrompt(t *testing.T) {
	prompt := BuildIssueGenerationPrompt(WizardTypeFeature, "Add user authentication", "Go web service", "en-US")

	// Verify prompt contains required sections from new template
	if !strings.Contains(prompt, "## Description") {
		t.Error("Prompt missing Description section")
	}
	if !strings.Contains(prompt, "## Tasks") {
		t.Error("Prompt missing Tasks section")
	}
	if !strings.Contains(prompt, "## Files to Modify") {
		t.Error("Prompt missing Files to Modify section")
	}
	if !strings.Contains(prompt, "## Acceptance Criteria") {
		t.Error("Prompt missing Acceptance Criteria section")
	}
	if !strings.Contains(prompt, "Add user authentication") {
		t.Error("Prompt missing original idea")
	}
	if !strings.Contains(prompt, "Feature") {
		t.Error("Prompt missing Feature type label")
	}
}

// TestBuildIssueGenerationPrompt_BackwardCompat verifies the alias works
func TestBuildIssueGenerationPrompt_BackwardCompat(t *testing.T) {
	prompt1 := BuildIssueGenerationPrompt(WizardTypeFeature, "Add user auth", "Go web service", "en-US")
	prompt2 := BuildTechnicalPlanningPrompt(WizardTypeFeature, "Add user auth", "Go web service", "en-US")

	if prompt1 != prompt2 {
		t.Error("BuildTechnicalPlanningPrompt should be an alias for BuildIssueGenerationPrompt")
	}
}

func TestBuildIssueGenerationPrompt_Bug(t *testing.T) {
	prompt := BuildIssueGenerationPrompt(WizardTypeBug, "Fix login error", "Go web service", "en-US")

	if !strings.Contains(prompt, "Bug") {
		t.Error("Bug prompt should mention Bug type")
	}
}

func TestBuildIssueGenerationPrompt_AlwaysEnglish(t *testing.T) {
	// Regardless of language parameter, prompt must enforce English
	for _, lang := range []string{"pl-PL", "en-US", "de-DE", ""} {
		prompt := BuildIssueGenerationPrompt(WizardTypeFeature, "Add user auth", "Go service", lang)
		if !strings.Contains(prompt, "ALL output MUST be in English") {
			t.Errorf("Prompt with language=%q must enforce English-only output", lang)
		}
	}
}

func TestBuildIssueGenerationPrompt_LanguageParamIgnored(t *testing.T) {
	// Language parameter should be accepted but not affect the prompt content
	prompt1 := BuildIssueGenerationPrompt(WizardTypeFeature, "Add auth", "Go service", "pl-PL")
	prompt2 := BuildIssueGenerationPrompt(WizardTypeFeature, "Add auth", "Go service", "en-US")
	prompt3 := BuildIssueGenerationPrompt(WizardTypeFeature, "Add auth", "Go service", "")

	if prompt1 != prompt2 || prompt2 != prompt3 {
		t.Error("Language parameter should not affect prompt output — always English")
	}
}

func TestBuildIssueGenerationPrompt_EmptyCodebaseContext(t *testing.T) {
	prompt := BuildIssueGenerationPrompt(WizardTypeFeature, "Add user auth", "", "en-US")

	if !strings.Contains(prompt, "No codebase context provided.") {
		t.Error("Prompt should use default codebase context when empty")
	}
}

// TestGeneratedIssueSchema_ValidJSON verifies the schema is valid JSON
func TestGeneratedIssueSchema_ValidJSON(t *testing.T) {
	var schema map[string]any
	if err := json.Unmarshal(GeneratedIssueSchema, &schema); err != nil {
		t.Fatalf("GeneratedIssueSchema is not valid JSON: %v", err)
	}

	// Verify required fields
	if schema["type"] != "object" {
		t.Error("Schema type should be 'object'")
	}

	props, ok := schema["properties"].(map[string]any)
	if !ok {
		t.Fatal("Schema missing 'properties' field")
	}

	if _, ok := props["title"]; !ok {
		t.Error("Schema missing 'title' property")
	}

	if _, ok := props["description"]; !ok {
		t.Error("Schema missing 'description' property")
	}

	required, ok := schema["required"].([]any)
	if !ok {
		t.Fatal("Schema missing 'required' field")
	}

	requiredFields := make(map[string]bool)
	for _, r := range required {
		requiredFields[r.(string)] = true
	}

	if !requiredFields["title"] {
		t.Error("Schema should require 'title'")
	}
	if !requiredFields["description"] {
		t.Error("Schema should require 'description'")
	}

	if schema["additionalProperties"] != false {
		t.Error("Schema should disallow additional properties")
	}
}

// TestGeneratedIssue_Unmarshal verifies the struct can be unmarshaled from JSON
func TestGeneratedIssue_Unmarshal(t *testing.T) {
	jsonData := `{"title": "[Feature] Add auth", "description": "## Description\n\nAdd authentication.", "priority": "high", "complexity": "M"}`

	var issue GeneratedIssue
	if err := json.Unmarshal([]byte(jsonData), &issue); err != nil {
		t.Fatalf("Failed to unmarshal GeneratedIssue: %v", err)
	}

	if issue.Title != "[Feature] Add auth" {
		t.Errorf("Expected title '[Feature] Add auth', got %q", issue.Title)
	}

	if !strings.Contains(issue.Description, "## Description") {
		t.Error("Expected description to contain '## Description'")
	}

	if issue.Priority != "high" {
		t.Errorf("Expected priority 'high', got %q", issue.Priority)
	}

	if issue.Complexity != "M" {
		t.Errorf("Expected complexity 'M', got %q", issue.Complexity)
	}
}

func TestGeneratedIssue_PriorityLabel(t *testing.T) {
	tests := []struct {
		priority string
		expected string
	}{
		{"high", "priority:high"},
		{"medium", "priority:medium"},
		{"low", "priority:low"},
		{"High", "priority:high"},
		{"MEDIUM", "priority:medium"},
		{"critical", ""},
		{"", ""},
		{"unknown", ""},
	}

	for _, tt := range tests {
		gi := GeneratedIssue{Priority: tt.priority}
		got := gi.PriorityLabel()
		if got != tt.expected {
			t.Errorf("PriorityLabel(%q) = %q, want %q", tt.priority, got, tt.expected)
		}
	}
}

func TestGeneratedIssue_ComplexityLabel(t *testing.T) {
	tests := []struct {
		complexity string
		expected   string
	}{
		{"S", "size:S"},
		{"M", "size:M"},
		{"L", "size:L"},
		{"XL", "size:XL"},
		{"s", "size:S"},
		{"m", "size:M"},
		{"xl", "size:XL"},
		{"XXL", ""},
		{"", ""},
		{"unknown", ""},
	}

	for _, tt := range tests {
		gi := GeneratedIssue{Complexity: tt.complexity}
		got := gi.ComplexityLabel()
		if got != tt.expected {
			t.Errorf("ComplexityLabel(%q) = %q, want %q", tt.complexity, got, tt.expected)
		}
	}
}

func TestGeneratedIssueSchema_HasPriorityAndComplexity(t *testing.T) {
	var schema map[string]any
	if err := json.Unmarshal(GeneratedIssueSchema, &schema); err != nil {
		t.Fatalf("GeneratedIssueSchema is not valid JSON: %v", err)
	}

	props := schema["properties"].(map[string]any)

	// Verify priority field exists with enum
	priorityProp, ok := props["priority"].(map[string]any)
	if !ok {
		t.Fatal("Schema missing 'priority' property")
	}
	priorityEnum, ok := priorityProp["enum"].([]any)
	if !ok {
		t.Fatal("Priority property missing 'enum'")
	}
	expectedPriorities := map[string]bool{"high": false, "medium": false, "low": false}
	for _, v := range priorityEnum {
		expectedPriorities[v.(string)] = true
	}
	for k, found := range expectedPriorities {
		if !found {
			t.Errorf("Priority enum missing value %q", k)
		}
	}

	// Verify complexity field exists with enum
	complexityProp, ok := props["complexity"].(map[string]any)
	if !ok {
		t.Fatal("Schema missing 'complexity' property")
	}
	complexityEnum, ok := complexityProp["enum"].([]any)
	if !ok {
		t.Fatal("Complexity property missing 'enum'")
	}
	expectedComplexities := map[string]bool{"S": false, "M": false, "L": false, "XL": false}
	for _, v := range complexityEnum {
		expectedComplexities[v.(string)] = true
	}
	for k, found := range expectedComplexities {
		if !found {
			t.Errorf("Complexity enum missing value %q", k)
		}
	}

	// Verify both are required
	required := schema["required"].([]any)
	requiredFields := make(map[string]bool)
	for _, r := range required {
		requiredFields[r.(string)] = true
	}
	if !requiredFields["priority"] {
		t.Error("Schema should require 'priority'")
	}
	if !requiredFields["complexity"] {
		t.Error("Schema should require 'complexity'")
	}
}

func TestBuildIssueGenerationPrompt_ContainsPriorityAndComplexityInstructions(t *testing.T) {
	prompt := BuildIssueGenerationPrompt(WizardTypeFeature, "Add auth", "Go service", "en-US")

	if !strings.Contains(prompt, `"priority"`) {
		t.Error("Prompt should mention priority field")
	}
	if !strings.Contains(prompt, `"complexity"`) {
		t.Error("Prompt should mention complexity field")
	}
	if !strings.Contains(prompt, "S") && !strings.Contains(prompt, "1-2 hours") {
		t.Error("Prompt should explain complexity scale")
	}
}
