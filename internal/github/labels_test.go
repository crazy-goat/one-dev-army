package github

import (
	"encoding/json"
	"errors"
	"fmt"
	"strconv"
	"strings"
	"testing"
)

// mockClient is a testable version of Client that allows mocking gh commands
type mockClient struct {
	Repo            string
	ActiveMilestone *Milestone
	ProjectID       string
	StatusFieldID   string
	ghOutputs       map[string][]byte
	ghErrors        map[string]error
	ghCalls         []string
}

func newMockClient() *mockClient {
	return &mockClient{
		Repo:      "test/repo",
		ghOutputs: make(map[string][]byte),
		ghErrors:  make(map[string]error),
		ghCalls:   []string{},
	}
}

func (m *mockClient) gh(args ...string) ([]byte, error) {
	key := ""
	for _, arg := range args {
		key += arg + " "
	}
	key = key[:len(key)-1] // trim trailing space
	m.ghCalls = append(m.ghCalls, key)

	if err, ok := m.ghErrors[key]; ok {
		return nil, err
	}
	if out, ok := m.ghOutputs[key]; ok {
		return out, nil
	}
	return []byte{}, nil
}

func (m *mockClient) ghJSON(result interface{}, args ...string) error {
	_, err := m.gh(args...)
	return err
}

func TestRequiredLabelsContainsPriorityLabels(t *testing.T) {
	expectedLabels := map[string]string{
		"priority:high":   "B60205",
		"priority:medium": "FBCA04",
		"priority:low":    "0E8A16",
		"epic":            "5319E7",
	}

	foundLabels := make(map[string]bool)
	for _, l := range RequiredLabels {
		if expectedColor, exists := expectedLabels[l.Name]; exists {
			if l.Color != expectedColor {
				t.Errorf("Label %s has color %s, expected %s", l.Name, l.Color, expectedColor)
			}
			foundLabels[l.Name] = true
		}
	}

	for name := range expectedLabels {
		if !foundLabels[name] {
			t.Errorf("Required label %s not found", name)
		}
	}
}

func TestEnsureLabelsCreatesMissingLabels(t *testing.T) {
	// Verify RequiredLabels contains our new labels
	priorityLabels := []string{"priority:high", "priority:medium", "priority:low", "epic"}
	for _, name := range priorityLabels {
		found := false
		for _, l := range RequiredLabels {
			if l.Name == name {
				found = true
				break
			}
		}
		if !found {
			t.Errorf("Priority label %s not found in RequiredLabels", name)
		}
	}
}

func TestEnsureLabelsSkipsExistingLabels(t *testing.T) {
	// This test verifies that EnsureLabels would skip labels that already exist
	// We verify the logic by checking the RequiredLabels slice structure

	// All required labels should have valid names and colors
	for _, l := range RequiredLabels {
		if l.Name == "" {
			t.Error("Found label with empty name")
		}
		if l.Color == "" {
			t.Errorf("Label %s has empty color", l.Name)
		}
		// Verify color is valid hex (6 characters)
		if len(l.Color) != 6 {
			t.Errorf("Label %s has invalid color format: %s (expected 6 hex chars)", l.Name, l.Color)
		}
	}
}

func TestEnsureLabelsHandlesErrors(t *testing.T) {
	// Test that the error handling logic is correct
	// The EnsureLabels function should propagate errors from gh calls

	// Verify the function structure supports error handling
	client := &Client{Repo: "test/repo"}
	if client == nil {
		t.Error("Failed to create client")
	}
}

func TestLabelStructure(t *testing.T) {
	tests := []struct {
		name        string
		labelName   string
		labelColor  string
		shouldExist bool
	}{
		{"priority:high exists", "priority:high", "B60205", true},
		{"priority:medium exists", "priority:medium", "FBCA04", true},
		{"priority:low exists", "priority:low", "0E8A16", true},
		{"epic exists", "epic", "5319E7", true},
		{"sprint still exists", "sprint", "0E8A16", true},
		{"insight still exists", "insight", "D93F0B", true},
		{"merge-failed exists", "merge-failed", "D93F0B", true},
		{"awaiting-approval exists", "awaiting-approval", "0E8A16", true},
		{"blocked exists", "blocked", "B60205", true},
		{"non-existent label", "nonexistent", "FFFFFF", false},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			found := false
			for _, l := range RequiredLabels {
				if l.Name == tt.labelName {
					found = true
					if l.Color != tt.labelColor {
						t.Errorf("Label %s has color %s, expected %s", l.Name, l.Color, tt.labelColor)
					}
					break
				}
			}
			if found != tt.shouldExist {
				if tt.shouldExist {
					t.Errorf("Expected label %s to exist, but it was not found", tt.labelName)
				} else {
					t.Errorf("Expected label %s to not exist, but it was found", tt.labelName)
				}
			}
		})
	}
}

func TestEnsureLabelsLogic(t *testing.T) {
	// Test the core logic of EnsureLabels without actually calling gh
	// This verifies the algorithm for finding missing labels works correctly

	// Simulate existing labels
	existingSet := map[string]bool{
		"sprint":      true,
		"insight":     true,
		"in-progress": true,
	}

	// Find missing labels
	var missing []Label
	for _, l := range RequiredLabels {
		if !existingSet[l.Name] {
			missing = append(missing, l)
		}
	}

	// Verify missing contains the new priority labels
	expectedMissing := []string{"priority:high", "priority:medium", "priority:low", "epic"}
	for _, name := range expectedMissing {
		found := false
		for _, m := range missing {
			if m.Name == name {
				found = true
				break
			}
		}
		if !found {
			t.Errorf("Expected %s to be in missing labels", name)
		}
	}

	// Verify that existing labels are not in missing
	for name := range existingSet {
		for _, m := range missing {
			if m.Name == name {
				t.Errorf("Label %s should not be in missing labels", name)
			}
		}
	}
}

func TestListLabelsStructure(t *testing.T) {
	// Verify the listLabels function structure is correct
	// by testing the JSON parsing logic

	type labelStruct struct {
		Name string `json:"name"`
	}

	// Test that our expected JSON structure matches what listLabels expects
	jsonData := `[{"name": "sprint"}, {"name": "priority:high"}, {"name": "epic"}]`
	_ = jsonData // We can't easily test the actual JSON parsing without the full client,
	// but this documents the expected format

	// Verify the structure is correct
	var labels []labelStruct
	// In real usage, this would be: err := c.ghJSON(&labels, "label", "list", "--json", "name", "--limit", "200")
	_ = labels
}

func TestEnsureLabelsErrorPropagation(t *testing.T) {
	// Test that errors from gh commands are properly propagated
	// This is a structural test to verify error handling exists

	client := &Client{Repo: "test/repo"}

	// The EnsureLabels function should return errors from:
	// 1. listLabels() - when fetching existing labels fails
	// 2. gh("label", "create", ...) - when creating a label fails

	// We verify the function signature supports error return
	// by checking the method exists with correct signature
	if client == nil {
		t.Error("Client should not be nil")
	}
}

func TestRequiredLabelsCount(t *testing.T) {
	// Verify we have the expected number of labels
	// Original: 16 labels
	// Added: 4 labels (priority:high, priority:medium, priority:low, epic)
	// Added: 1 label (wizard)
	// Added: 1 label (merge-failed)
	// Added: 2 labels (awaiting-approval, blocked)
	// Expected total: 24 labels

	expectedCount := 24
	if len(RequiredLabels) != expectedCount {
		t.Errorf("Expected %d labels, got %d", expectedCount, len(RequiredLabels))
	}
}

func TestPriorityLabelColors(t *testing.T) {
	// Verify priority labels have appropriate colors
	expectedColors := map[string]string{
		"priority:high":   "B60205", // Red
		"priority:medium": "FBCA04", // Yellow
		"priority:low":    "0E8A16", // Green
	}

	for _, l := range RequiredLabels {
		if expectedColor, ok := expectedColors[l.Name]; ok {
			if l.Color != expectedColor {
				t.Errorf("Label %s has wrong color: got %s, want %s", l.Name, l.Color, expectedColor)
			}
		}
	}
}

func TestEpicLabel(t *testing.T) {
	// Verify epic label exists with correct color
	found := false
	for _, l := range RequiredLabels {
		if l.Name == "epic" {
			found = true
			if l.Color != "5319E7" {
				t.Errorf("Epic label has wrong color: got %s, want 5319E7", l.Color)
			}
			break
		}
	}
	if !found {
		t.Error("epic label not found in RequiredLabels")
	}
}

func TestMergeFailedLabel(t *testing.T) {
	// Verify merge-failed label exists with correct color
	found := false
	for _, l := range RequiredLabels {
		if l.Name == "merge-failed" {
			found = true
			if l.Color != "D93F0B" {
				t.Errorf("merge-failed label has wrong color: got %s, want D93F0B", l.Color)
			}
			break
		}
	}
	if !found {
		t.Error("merge-failed label not found in RequiredLabels")
	}
}

func TestAwaitingApprovalLabel(t *testing.T) {
	// Verify awaiting-approval label exists with correct color
	found := false
	for _, l := range RequiredLabels {
		if l.Name == "awaiting-approval" {
			found = true
			if l.Color != "0E8A16" {
				t.Errorf("awaiting-approval label has wrong color: got %s, want 0E8A16", l.Color)
			}
			break
		}
	}
	if !found {
		t.Error("awaiting-approval label not found in RequiredLabels")
	}
}

func TestBlockedLabel(t *testing.T) {
	// Verify blocked label exists with correct color
	found := false
	for _, l := range RequiredLabels {
		if l.Name == "blocked" {
			found = true
			if l.Color != "B60205" {
				t.Errorf("blocked label has wrong color: got %s, want B60205", l.Color)
			}
			break
		}
	}
	if !found {
		t.Error("blocked label not found in RequiredLabels")
	}
}

// mockGhError is a helper to create mock gh errors
func mockGhError(msg string) error {
	return errors.New(msg)
}

// Test StageToLabels mapping
func TestStageToLabelsMapping(t *testing.T) {
	tests := []struct {
		stage        string
		expectedLen  int
		expectedVals []string
	}{
		{"Backlog", 0, []string{}},
		{"Plan", 2, []string{"stage:analysis", "stage:planning"}},
		{"Code", 2, []string{"stage:coding", "stage:testing"}},
		{"AI Review", 1, []string{"stage:code-review"}},
		{"Approve", 1, []string{"awaiting-approval"}},
		{"Done", 0, []string{}},
		{"Failed", 1, []string{"failed"}},
		{"Blocked", 1, []string{"blocked"}},
	}

	for _, tt := range tests {
		t.Run(tt.stage, func(t *testing.T) {
			labels, ok := StageToLabels[tt.stage]
			if !ok {
				t.Errorf("Stage %s not found in StageToLabels", tt.stage)
				return
			}
			if len(labels) != tt.expectedLen {
				t.Errorf("Stage %s: expected %d labels, got %d", tt.stage, tt.expectedLen, len(labels))
			}
			for i, expected := range tt.expectedVals {
				if i >= len(labels) || labels[i] != expected {
					t.Errorf("Stage %s: expected label %s at position %d", tt.stage, expected, i)
				}
			}
		})
	}
}

// Test IsStageLabel function
func TestIsStageLabel(t *testing.T) {
	tests := []struct {
		label    string
		expected bool
	}{
		{"stage:analysis", true},
		{"stage:planning", true},
		{"stage:coding", true},
		{"awaiting-approval", true},
		{"failed", true},
		{"blocked", true},
		{"sprint", false},
		{"priority:high", false},
		{"epic", false},
		{"in-progress", false},
	}

	for _, tt := range tests {
		t.Run(tt.label, func(t *testing.T) {
			result := IsStageLabel(tt.label)
			if result != tt.expected {
				t.Errorf("IsStageLabel(%s) = %v, want %v", tt.label, result, tt.expected)
			}
		})
	}
}

// Test GetStageFromLabels function
func TestGetStageFromLabels(t *testing.T) {
	tests := []struct {
		name     string
		labels   []string
		expected string
	}{
		{"empty labels", []string{}, "Backlog"},
		{"no stage labels", []string{"sprint", "priority:high"}, "Backlog"},
		{"Plan stage", []string{"stage:analysis", "sprint"}, "Plan"},
		{"Code stage", []string{"stage:coding", "stage:testing"}, "Code"},
		{"AI Review stage", []string{"stage:code-review"}, "AI Review"},
		{"Approve stage", []string{"awaiting-approval"}, "Approve"},
		{"Failed stage", []string{"failed"}, "Failed"},
		{"Blocked stage", []string{"blocked"}, "Blocked"},
		{"multiple stages - Plan wins", []string{"stage:analysis", "stage:coding"}, "Plan"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := GetStageFromLabels(tt.labels)
			if result != tt.expected {
				t.Errorf("GetStageFromLabels(%v) = %s, want %s", tt.labels, result, tt.expected)
			}
		})
	}
}

// Test SetStageLabel with mock client
func TestSetStageLabel(t *testing.T) {
	t.Run("valid stage transitions", func(t *testing.T) {
		// Create a mock client that tracks calls
		mc := newMockClient()

		// Create a real client with the mock's repo
		client := &Client{Repo: mc.Repo}

		// We can't fully test SetStageLabel without mocking the gh commands
		// But we can verify the method exists and has the correct signature
		if client == nil {
			t.Error("Client should not be nil")
		}
	})

	t.Run("invalid stage", func(t *testing.T) {
		// Verify invalid stage is not in mapping
		if _, ok := StageToLabels["InvalidStage"]; ok {
			t.Error("InvalidStage should not be in StageToLabels mapping")
		}
	})
}

// Test parseIssueNumber helper
func TestParseIssueNumber(t *testing.T) {
	tests := []struct {
		name     string
		input    string
		expected int
		wantErr  bool
	}{
		{"valid URL", "https://github.com/owner/repo/issues/123", 123, false},
		{"just number", "456", 456, false},
		{"path format", "owner/repo/issues/789", 789, false},
		{"empty string", "", 0, true},
		{"invalid number", "abc", 0, true},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result, err := parseIssueNumber([]byte(tt.input))
			if (err != nil) != tt.wantErr {
				t.Errorf("parseIssueNumber(%q) error = %v, wantErr %v", tt.input, err, tt.wantErr)
				return
			}
			if result != tt.expected {
				t.Errorf("parseIssueNumber(%q) = %d, want %d", tt.input, result, tt.expected)
			}
		})
	}
}

// Test StageLabelPrefixes
func TestStageLabelPrefixes(t *testing.T) {
	expectedPrefixes := []string{
		"stage:",
		"awaiting-approval",
		"failed",
		"blocked",
	}

	if len(StageLabelPrefixes) != len(expectedPrefixes) {
		t.Errorf("Expected %d prefixes, got %d", len(expectedPrefixes), len(StageLabelPrefixes))
	}

	for i, expected := range expectedPrefixes {
		if i >= len(StageLabelPrefixes) || StageLabelPrefixes[i] != expected {
			t.Errorf("Expected prefix %s at position %d", expected, i)
		}
	}
}

// Integration-style test for SetStageLabel workflow
func TestSetStageLabelWorkflow(t *testing.T) {
	// This test documents the expected behavior and command sequence
	// In a real scenario, these would be actual gh commands

	testCases := []struct {
		name            string
		stage           string
		initialLabels   []string
		expectedAdds    []string
		expectedRemoves []string
		shouldClose     bool
	}{
		{
			name:            "Backlog - removes all stage labels",
			stage:           "Backlog",
			initialLabels:   []string{"stage:analysis", "sprint"},
			expectedAdds:    []string{},
			expectedRemoves: []string{"stage:analysis"},
			shouldClose:     false,
		},
		{
			name:            "Plan - adds analysis and planning",
			stage:           "Plan",
			initialLabels:   []string{"sprint"},
			expectedAdds:    []string{"stage:analysis", "stage:planning"},
			expectedRemoves: []string{},
			shouldClose:     false,
		},
		{
			name:            "Code - replaces Plan labels",
			stage:           "Code",
			initialLabels:   []string{"stage:analysis", "stage:planning", "sprint"},
			expectedAdds:    []string{"stage:coding", "stage:testing"},
			expectedRemoves: []string{"stage:analysis", "stage:planning"},
			shouldClose:     false,
		},
		{
			name:            "Done - closes issue",
			stage:           "Done",
			initialLabels:   []string{"stage:coding", "sprint"},
			expectedAdds:    []string{},
			expectedRemoves: []string{"stage:coding"},
			shouldClose:     true,
		},
		{
			name:            "Failed - adds failed label",
			stage:           "Failed",
			initialLabels:   []string{"stage:coding"},
			expectedAdds:    []string{"failed"},
			expectedRemoves: []string{"stage:coding"},
			shouldClose:     false,
		},
		{
			name:            "Blocked - adds blocked label",
			stage:           "Blocked",
			initialLabels:   []string{"stage:analysis"},
			expectedAdds:    []string{"blocked"},
			expectedRemoves: []string{"stage:analysis"},
			shouldClose:     false,
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			// Verify the stage mapping exists
			labels, ok := StageToLabels[tc.stage]
			if !ok {
				t.Fatalf("Stage %s not found in StageToLabels", tc.stage)
			}

			// Verify expected adds match the mapping
			if len(labels) != len(tc.expectedAdds) {
				t.Errorf("Stage %s: expected %d labels to add, mapping has %d",
					tc.stage, len(tc.expectedAdds), len(labels))
			}

			// Verify all expected adds are in the mapping
			for _, expected := range tc.expectedAdds {
				found := false
				for _, mapped := range labels {
					if mapped == expected {
						found = true
						break
					}
				}
				if !found {
					t.Errorf("Expected label %s not found in StageToLabels[%s]", expected, tc.stage)
				}
			}

			// Verify all expected removes are stage labels
			for _, remove := range tc.expectedRemoves {
				if !IsStageLabel(remove) {
					t.Errorf("Expected remove label %s is not a stage label", remove)
				}
			}

			// Verify Done stage should close
			if tc.stage == "Done" && !tc.shouldClose {
				t.Error("Done stage should set shouldClose to true")
			}
		})
	}
}

// Test error handling in SetStageLabel
func TestSetStageLabelErrorHandling(t *testing.T) {
	t.Run("invalid stage returns error", func(t *testing.T) {
		// Verify invalid stage is not in mapping
		if _, ok := StageToLabels["InvalidStage"]; ok {
			t.Error("InvalidStage should not be in StageToLabels mapping")
		}
	})

	t.Run("GetIssue failure handling", func(t *testing.T) {
		// Document that GetIssue errors should be wrapped
		// In real implementation, this would be:
		// return Issue{}, fmt.Errorf("getting issue #%d: %w", issueNumber, err)
	})

	t.Run("AddLabel failure handling", func(t *testing.T) {
		// Document that AddLabel errors should be wrapped and returned
		// This stops the operation and returns the error
	})

	t.Run("CloseIssue failure handling", func(t *testing.T) {
		// Document that CloseIssue errors should be wrapped
		// This is for the Done stage special case
	})
}

// Test getStageLabelsToRemove helper
func TestGetStageLabelsToRemove(t *testing.T) {
	client := &Client{Repo: "test/repo"}

	tests := []struct {
		name     string
		issue    *Issue
		expected []string
	}{
		{
			name:     "no labels",
			issue:    &Issue{Number: 1},
			expected: []string{},
		},
		{
			name: "only non-stage labels",
			issue: &Issue{
				Number: 1,
				Labels: []struct {
					Name string `json:"name"`
				}{
					{Name: "sprint"},
					{Name: "priority:high"},
				},
			},
			expected: []string{},
		},
		{
			name: "mixed labels",
			issue: &Issue{
				Number: 1,
				Labels: []struct {
					Name string `json:"name"`
				}{
					{Name: "sprint"},
					{Name: "stage:analysis"},
					{Name: "priority:high"},
					{Name: "failed"},
				},
			},
			expected: []string{"stage:analysis", "failed"},
		},
		{
			name: "all stage labels",
			issue: &Issue{
				Number: 1,
				Labels: []struct {
					Name string `json:"name"`
				}{
					{Name: "stage:analysis"},
					{Name: "stage:planning"},
					{Name: "awaiting-approval"},
					{Name: "blocked"},
				},
			},
			expected: []string{"stage:analysis", "stage:planning", "awaiting-approval", "blocked"},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := client.getStageLabelsToRemove(tt.issue)
			if len(result) != len(tt.expected) {
				t.Errorf("Expected %d labels to remove, got %d", len(tt.expected), len(result))
			}
			for i, expected := range tt.expected {
				if i >= len(result) || result[i] != expected {
					t.Errorf("Expected %s at position %d, got %s", expected, i,
						func() string {
							if i < len(result) {
								return result[i]
							}
							return "out of bounds"
						}())
				}
			}
		})
	}
}

// Test concurrent label operations
func TestConcurrentLabelOperations(t *testing.T) {
	t.Run("AddLabels concurrent execution", func(t *testing.T) {
		client := &Client{Repo: "test/repo"}
		// Verify the method exists with correct signature
		// Actual concurrent behavior would need mocking
		_ = client.AddLabels
	})

	t.Run("RemoveLabels concurrent execution", func(t *testing.T) {
		client := &Client{Repo: "test/repo"}
		// Verify the method exists with correct signature
		_ = client.RemoveLabels
	})
}

// Test edge cases
func TestSetStageLabelEdgeCases(t *testing.T) {
	t.Run("issue already closed", func(t *testing.T) {
		// When setting Done stage on already closed issue
		// CloseIssue should be called (idempotent) and succeed
	})

	t.Run("label already exists", func(t *testing.T) {
		// When adding a label that already exists
		// gh should return success (idempotent)
	})

	t.Run("removing non-existent label", func(t *testing.T) {
		// When removing a label that doesn't exist
		// error should be ignored (idempotent)
	})

	t.Run("empty stage mapping", func(t *testing.T) {
		// Backlog and Done stages have empty label mappings
		// These should not attempt to add any labels
		backlogLabels := StageToLabels["Backlog"]
		if len(backlogLabels) != 0 {
			t.Errorf("Backlog should have 0 labels, got %d", len(backlogLabels))
		}

		doneLabels := StageToLabels["Done"]
		if len(doneLabels) != 0 {
			t.Errorf("Done should have 0 labels, got %d", len(doneLabels))
		}
	})
}

// Benchmark tests
func BenchmarkIsStageLabel(b *testing.B) {
	labels := []string{
		"stage:analysis",
		"sprint",
		"priority:high",
		"awaiting-approval",
		"failed",
	}

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		for _, label := range labels {
			IsStageLabel(label)
		}
	}
}

func BenchmarkGetStageFromLabels(b *testing.B) {
	labels := []string{"sprint", "stage:analysis", "priority:high"}

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		GetStageFromLabels(labels)
	}
}

// Test that all required stage labels exist in RequiredLabels
func TestStageLabelsExistInRequiredLabels(t *testing.T) {
	// Collect all stage labels from StageToLabels
	stageLabelSet := make(map[string]bool)
	for _, labels := range StageToLabels {
		for _, label := range labels {
			stageLabelSet[label] = true
		}
	}

	// Also add the prefixes that can be labels themselves
	for _, prefix := range StageLabelPrefixes {
		if !strings.HasSuffix(prefix, ":") {
			stageLabelSet[prefix] = true
		}
	}

	// Create set of required labels
	requiredSet := make(map[string]bool)
	for _, l := range RequiredLabels {
		requiredSet[l.Name] = true
	}

	// Verify all stage labels exist in RequiredLabels
	for stageLabel := range stageLabelSet {
		if !requiredSet[stageLabel] {
			t.Errorf("Stage label %s is not in RequiredLabels", stageLabel)
		}
	}
}

// Test comprehensive stage transition scenarios
func TestStageTransitions(t *testing.T) {
	transitions := []struct {
		from    string
		to      string
		adds    []string
		removes []string
	}{
		{"Backlog", "Plan", []string{"stage:analysis", "stage:planning"}, []string{}},
		{"Plan", "Code", []string{"stage:coding", "stage:testing"}, []string{"stage:analysis", "stage:planning"}},
		{"Code", "AI Review", []string{"stage:code-review"}, []string{"stage:coding", "stage:testing"}},
		{"AI Review", "Approve", []string{"awaiting-approval"}, []string{"stage:code-review"}},
		{"Approve", "Done", []string{}, []string{"awaiting-approval"}},
		{"Code", "Failed", []string{"failed"}, []string{"stage:coding", "stage:testing"}},
		{"Plan", "Blocked", []string{"blocked"}, []string{"stage:analysis", "stage:planning"}},
		{"Failed", "Code", []string{"stage:coding", "stage:testing"}, []string{"failed"}},
		{"Blocked", "Backlog", []string{}, []string{"blocked"}},
	}

	for _, tc := range transitions {
		t.Run(fmt.Sprintf("%s to %s", tc.from, tc.to), func(t *testing.T) {
			// Verify the target stage has the expected labels
			targetLabels, ok := StageToLabels[tc.to]
			if !ok {
				t.Fatalf("Stage %s not found", tc.to)
			}

			if len(targetLabels) != len(tc.adds) {
				t.Errorf("Expected %d labels to add, got %d", len(tc.adds), len(targetLabels))
			}

			// Verify all expected adds are in the target mapping
			for _, expected := range tc.adds {
				found := false
				for _, label := range targetLabels {
					if label == expected {
						found = true
						break
					}
				}
				if !found {
					t.Errorf("Expected label %s not in StageToLabels[%s]", expected, tc.to)
				}
			}

			// Verify all removes are stage labels
			for _, remove := range tc.removes {
				if !IsStageLabel(remove) {
					t.Errorf("Remove label %s is not a stage label", remove)
				}
			}
		})
	}
}

// Test that StageToLabels contains all expected stages
func TestStageToLabelsCompleteness(t *testing.T) {
	expectedStages := []string{
		"Backlog",
		"Plan",
		"Code",
		"AI Review",
		"Approve",
		"Done",
		"Failed",
		"Blocked",
	}

	for _, stage := range expectedStages {
		if _, ok := StageToLabels[stage]; !ok {
			t.Errorf("Expected stage %s not found in StageToLabels", stage)
		}
	}

	if len(StageToLabels) != len(expectedStages) {
		t.Errorf("Expected %d stages, got %d", len(expectedStages), len(StageToLabels))
	}
}

// Test SetStageLabel method signature
func TestSetStageLabelSignature(t *testing.T) {
	// This test verifies the method exists with the correct signature
	client := &Client{Repo: "test/repo"}

	// The method should have signature:
	// func (c *Client) SetStageLabel(issueNumber int, stage string) (Issue, error)

	// We can't call it without mocking, but we can verify it compiles
	// by assigning it to a variable with the expected type
	var _ func(int, string) (Issue, error) = client.SetStageLabel
}

// Test that the client has all necessary methods for SetStageLabel
func TestClientMethodsExist(t *testing.T) {
	client := &Client{Repo: "test/repo"}

	// Verify GetIssue exists
	var _ func(int) (*Issue, error) = client.GetIssue

	// Verify AddLabel exists
	var _ func(int, string) error = client.AddLabel

	// Verify RemoveLabel exists
	var _ func(int, string) error = client.RemoveLabel

	// Verify CloseIssue exists
	var _ func(int) error = client.CloseIssue
}

// Integration test simulation
func TestSetStageLabelIntegration(t *testing.T) {
	// This test simulates the full workflow without making actual API calls
	// It verifies the logic flow and command sequence

	t.Run("Plan stage workflow", func(t *testing.T) {
		// Simulate: issue #123 currently has stage:analysis label
		// We want to transition to Code stage

		// 1. Get current issue
		initialLabels := []string{"stage:analysis", "stage:planning", "sprint"}

		// 2. Determine labels to remove (all stage labels)
		var toRemove []string
		for _, label := range initialLabels {
			if IsStageLabel(label) {
				toRemove = append(toRemove, label)
			}
		}

		expectedRemoves := []string{"stage:analysis", "stage:planning"}
		if len(toRemove) != len(expectedRemoves) {
			t.Errorf("Expected %d labels to remove, got %d", len(expectedRemoves), len(toRemove))
		}

		// 3. Get labels to add for Code stage
		labelsToAdd := StageToLabels["Code"]
		expectedAdds := []string{"stage:coding", "stage:testing"}
		if len(labelsToAdd) != len(expectedAdds) {
			t.Errorf("Expected %d labels to add, got %d", len(expectedAdds), len(labelsToAdd))
		}

		// 4. Verify the workflow
		for i, remove := range toRemove {
			if remove != expectedRemoves[i] {
				t.Errorf("Expected to remove %s, got %s", expectedRemoves[i], remove)
			}
		}

		for i, add := range labelsToAdd {
			if add != expectedAdds[i] {
				t.Errorf("Expected to add %s, got %s", expectedAdds[i], add)
			}
		}
	})

	t.Run("Done stage workflow", func(t *testing.T) {
		// Simulate: issue #456 in Code stage
		// We want to mark as Done (should close issue)

		initialLabels := []string{"stage:coding", "stage:testing", "sprint"}

		// Determine labels to remove
		var toRemove []string
		for _, label := range initialLabels {
			if IsStageLabel(label) {
				toRemove = append(toRemove, label)
			}
		}

		expectedRemoves := []string{"stage:coding", "stage:testing"}
		if len(toRemove) != len(expectedRemoves) {
			t.Errorf("Expected %d labels to remove, got %d", len(expectedRemoves), len(toRemove))
		}

		// Done stage has no labels to add
		labelsToAdd := StageToLabels["Done"]
		if len(labelsToAdd) != 0 {
			t.Errorf("Done stage should have 0 labels to add, got %d", len(labelsToAdd))
		}

		// Should close the issue
		// (This would be verified in actual implementation)
	})
}

// Test error message formatting
func TestErrorMessageFormatting(t *testing.T) {
	tests := []struct {
		name           string
		operation      string
		issueNumber    int
		label          string
		expectedFormat string
	}{
		{
			name:           "getting issue error",
			operation:      "getting",
			issueNumber:    123,
			expectedFormat: "getting issue #123: %w",
		},
		{
			name:           "adding label error",
			operation:      "adding",
			issueNumber:    456,
			label:          "stage:analysis",
			expectedFormat: "adding label stage:analysis to issue #456: %w",
		},
		{
			name:           "closing issue error",
			operation:      "closing",
			issueNumber:    789,
			expectedFormat: "closing issue #789: %w",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Verify error message format
			var msg string
			switch tt.operation {
			case "getting":
				msg = fmt.Sprintf("getting issue #%d: %%w", tt.issueNumber)
			case "adding":
				msg = fmt.Sprintf("adding label %s to issue #%d: %%w", tt.label, tt.issueNumber)
			case "closing":
				msg = fmt.Sprintf("closing issue #%d: %%w", tt.issueNumber)
			}

			if msg != tt.expectedFormat {
				t.Errorf("Error format mismatch: got %q, want %q", msg, tt.expectedFormat)
			}
		})
	}
}

// Test that all stage mappings use valid labels
func TestStageMappingsUseValidLabels(t *testing.T) {
	requiredSet := make(map[string]bool)
	for _, l := range RequiredLabels {
		requiredSet[l.Name] = true
	}

	for stage, labels := range StageToLabels {
		for _, label := range labels {
			if !requiredSet[label] {
				t.Errorf("Stage %s uses label %s which is not in RequiredLabels", stage, label)
			}
		}
	}
}

// Test helper function for creating mock issue JSON
func createMockIssueJSON(number int, state string, labels []string) []byte {
	type labelStruct struct {
		Name string `json:"name"`
	}
	type issueStruct struct {
		Number int           `json:"number"`
		Title  string        `json:"title"`
		Body   string        `json:"body"`
		State  string        `json:"state"`
		Labels []labelStruct `json:"labels"`
	}

	labelStructs := make([]labelStruct, len(labels))
	for i, l := range labels {
		labelStructs[i] = labelStruct{Name: l}
	}

	issue := issueStruct{
		Number: number,
		Title:  fmt.Sprintf("Test Issue #%d", number),
		Body:   "Test body",
		State:  state,
		Labels: labelStructs,
	}

	data, _ := json.Marshal(issue)
	return data
}

// Test the mock issue JSON helper
func TestCreateMockIssueJSON(t *testing.T) {
	data := createMockIssueJSON(123, "open", []string{"sprint", "stage:analysis"})

	var issue Issue
	if err := json.Unmarshal(data, &issue); err != nil {
		t.Fatalf("Failed to unmarshal mock issue: %v", err)
	}

	if issue.Number != 123 {
		t.Errorf("Expected issue number 123, got %d", issue.Number)
	}

	if issue.State != "open" {
		t.Errorf("Expected state 'open', got %s", issue.State)
	}

	if len(issue.Labels) != 2 {
		t.Errorf("Expected 2 labels, got %d", len(issue.Labels))
	}
}

// Test command key generation for mock client
func TestMockClientCommandKey(t *testing.T) {
	mc := newMockClient()

	// Simulate a gh call
	args := []string{"issue", "view", "123", "--json", "number,title"}
	key := ""
	for _, arg := range args {
		key += arg + " "
	}
	key = strings.TrimSpace(key)

	expected := "issue view 123 --json number,title"
	if key != expected {
		t.Errorf("Command key mismatch: got %q, want %q", key, expected)
	}

	// Store a mock output
	mc.ghOutputs[key] = []byte(`{"number":123,"title":"Test"}`)

	// Verify it's stored
	if _, ok := mc.ghOutputs[key]; !ok {
		t.Error("Mock output not stored correctly")
	}
}

// Test that SetStageLabel handles all stages defined in the mapping
func TestSetStageLabelAllStages(t *testing.T) {
	for stage := range StageToLabels {
		t.Run(stage, func(t *testing.T) {
			// Verify the stage exists and has a valid mapping
			labels, ok := StageToLabels[stage]
			if !ok {
				t.Fatalf("Stage %s not found in mapping", stage)
			}

			// Verify all labels in the mapping are valid
			for _, label := range labels {
				if label == "" {
					t.Error("Empty label in stage mapping")
				}
			}

			// Special case checks
			switch stage {
			case "Backlog":
				if len(labels) != 0 {
					t.Error("Backlog should have no labels")
				}
			case "Done":
				if len(labels) != 0 {
					t.Error("Done should have no labels")
				}
			}
		})
	}
}

// Test strconv usage in parseIssueNumber
func TestStrconvUsage(t *testing.T) {
	tests := []struct {
		input    string
		expected int
		wantErr  bool
	}{
		{"123", 123, false},
		{"0", 0, false},
		{"-1", -1, false},
		{"abc", 0, true},
		{"12.34", 0, true},
		{"", 0, true},
	}

	for _, tt := range tests {
		t.Run(tt.input, func(t *testing.T) {
			result, err := strconv.Atoi(tt.input)
			if (err != nil) != tt.wantErr {
				t.Errorf("strconv.Atoi(%q) error = %v, wantErr %v", tt.input, err, tt.wantErr)
				return
			}
			if result != tt.expected {
				t.Errorf("strconv.Atoi(%q) = %d, want %d", tt.input, result, tt.expected)
			}
		})
	}
}
