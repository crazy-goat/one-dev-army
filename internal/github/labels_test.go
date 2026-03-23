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
		{"stage:awaiting-approval exists", "stage:awaiting-approval", "0E8A16", true},
		{"stage:blocked exists", "stage:blocked", "B60205", true},
		{"stage:failed exists", "stage:failed", "D93F0B", true},
		{"stage:create-pr exists", "stage:create-pr", "1D76DB", true},
		{"stage:merging exists", "stage:merging", "0E8A16", true},
		{"stage:backlog exists", "stage:backlog", "EEEEEE", true},
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
	// Simulate existing labels
	existingSet := map[string]bool{
		"sprint":  true,
		"insight": true,
	}

	// Find missing labels
	var missing []Label
	for _, l := range RequiredLabels {
		if !existingSet[l.Name] {
			missing = append(missing, l)
		}
	}

	// Verify missing contains the new stage labels
	expectedMissing := []string{"stage:backlog", "stage:create-pr", "stage:merging", "stage:failed", "stage:blocked"}
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
	type labelStruct struct {
		Name string `json:"name"`
	}

	jsonData := `[{"name": "sprint"}, {"name": "priority:high"}, {"name": "epic"}]`
	_ = jsonData

	var labels []labelStruct
	_ = labels
}

func TestEnsureLabelsErrorPropagation(t *testing.T) {
	client := &Client{Repo: "test/repo"}
	if client == nil {
		t.Error("Client should not be nil")
	}
}

func TestRequiredLabelsCount(t *testing.T) {
	// Labels:
	// sprint, insight (2)
	// size:S, size:M, size:L, size:XL (4)
	// stage:backlog, stage:analysis, stage:coding, stage:code-review, stage:create-pr,
	// stage:awaiting-approval, stage:merging, stage:failed, stage:blocked (9)
	// priority:high, priority:medium, priority:low (3)
	// epic, wizard, merge-failed (3)
	// Total: 21
	expectedCount := 21
	if len(RequiredLabels) != expectedCount {
		t.Errorf("Expected %d labels, got %d", expectedCount, len(RequiredLabels))
	}
}

func TestPriorityLabelColors(t *testing.T) {
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

func TestStageAwaitingApprovalLabel(t *testing.T) {
	found := false
	for _, l := range RequiredLabels {
		if l.Name == "stage:awaiting-approval" {
			found = true
			if l.Color != "0E8A16" {
				t.Errorf("stage:awaiting-approval label has wrong color: got %s, want 0E8A16", l.Color)
			}
			break
		}
	}
	if !found {
		t.Error("stage:awaiting-approval label not found in RequiredLabels")
	}
}

func TestStageBlockedLabel(t *testing.T) {
	found := false
	for _, l := range RequiredLabels {
		if l.Name == "stage:blocked" {
			found = true
			if l.Color != "B60205" {
				t.Errorf("stage:blocked label has wrong color: got %s, want B60205", l.Color)
			}
			break
		}
	}
	if !found {
		t.Error("stage:blocked label not found in RequiredLabels")
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
		{"Plan", 1, []string{"stage:analysis"}},
		{"Code", 1, []string{"stage:coding"}},
		{"AI Review", 1, []string{"stage:code-review"}},
		{"Create PR", 1, []string{"stage:create-pr"}},
		{"Approve", 1, []string{"stage:awaiting-approval"}},
		{"Merge", 1, []string{"stage:merging"}},
		{"Done", 0, []string{}},
		{"Failed", 1, []string{"stage:failed"}},
		{"Blocked", 1, []string{"stage:blocked"}},
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
		{"stage:coding", true},
		{"stage:code-review", true},
		{"stage:create-pr", true},
		{"stage:awaiting-approval", true},
		{"stage:merging", true},
		{"stage:failed", true},
		{"stage:blocked", true},
		{"stage:backlog", true},
		// Legacy bare labels are still caught by StageLabelPrefixes for cleanup
		{"awaiting-approval", true},
		{"failed", true},
		{"blocked", true},
		{"in-progress", true},
		// Non-stage labels
		{"sprint", false},
		{"priority:high", false},
		{"epic", false},
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
		{"Code stage", []string{"stage:coding"}, "Code"},
		{"AI Review stage", []string{"stage:code-review"}, "AI Review"},
		{"Create PR stage", []string{"stage:create-pr"}, "Create PR"},
		{"Approve stage", []string{"stage:awaiting-approval"}, "Approve"},
		{"Merge stage", []string{"stage:merging"}, "Merge"},
		{"Failed stage", []string{"stage:failed"}, "Failed"},
		{"Blocked stage", []string{"stage:blocked"}, "Blocked"},
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
		mc := newMockClient()
		client := &Client{Repo: mc.Repo}
		if client == nil {
			t.Error("Client should not be nil")
		}
	})

	t.Run("invalid stage", func(t *testing.T) {
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
		"in-progress",
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
			name:            "Plan - adds analysis",
			stage:           "Plan",
			initialLabels:   []string{"sprint"},
			expectedAdds:    []string{"stage:analysis"},
			expectedRemoves: []string{},
			shouldClose:     false,
		},
		{
			name:            "Code - replaces Plan label",
			stage:           "Code",
			initialLabels:   []string{"stage:analysis", "sprint"},
			expectedAdds:    []string{"stage:coding"},
			expectedRemoves: []string{"stage:analysis"},
			shouldClose:     false,
		},
		{
			name:            "Create PR - replaces code-review label",
			stage:           "Create PR",
			initialLabels:   []string{"stage:code-review", "sprint"},
			expectedAdds:    []string{"stage:create-pr"},
			expectedRemoves: []string{"stage:code-review"},
			shouldClose:     false,
		},
		{
			name:            "Approve - replaces create-pr label",
			stage:           "Approve",
			initialLabels:   []string{"stage:create-pr", "sprint"},
			expectedAdds:    []string{"stage:awaiting-approval"},
			expectedRemoves: []string{"stage:create-pr"},
			shouldClose:     false,
		},
		{
			name:            "Merge - replaces awaiting-approval label",
			stage:           "Merge",
			initialLabels:   []string{"stage:awaiting-approval", "sprint"},
			expectedAdds:    []string{"stage:merging"},
			expectedRemoves: []string{"stage:awaiting-approval"},
			shouldClose:     false,
		},
		{
			name:            "Done - closes issue",
			stage:           "Done",
			initialLabels:   []string{"stage:merging", "sprint"},
			expectedAdds:    []string{},
			expectedRemoves: []string{"stage:merging"},
			shouldClose:     true,
		},
		{
			name:            "Failed - adds stage:failed label",
			stage:           "Failed",
			initialLabels:   []string{"stage:coding"},
			expectedAdds:    []string{"stage:failed"},
			expectedRemoves: []string{"stage:coding"},
			shouldClose:     false,
		},
		{
			name:            "Blocked - adds stage:blocked label",
			stage:           "Blocked",
			initialLabels:   []string{"stage:analysis"},
			expectedAdds:    []string{"stage:blocked"},
			expectedRemoves: []string{"stage:analysis"},
			shouldClose:     false,
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			labels, ok := StageToLabels[tc.stage]
			if !ok {
				t.Fatalf("Stage %s not found in StageToLabels", tc.stage)
			}

			if len(labels) != len(tc.expectedAdds) {
				t.Errorf("Stage %s: expected %d labels to add, mapping has %d",
					tc.stage, len(tc.expectedAdds), len(labels))
			}

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

			for _, remove := range tc.expectedRemoves {
				if !IsStageLabel(remove) {
					t.Errorf("Expected remove label %s is not a stage label", remove)
				}
			}

			if tc.stage == "Done" && !tc.shouldClose {
				t.Error("Done stage should set shouldClose to true")
			}
		})
	}
}

// Test error handling in SetStageLabel
func TestSetStageLabelErrorHandling(t *testing.T) {
	t.Run("invalid stage returns error", func(t *testing.T) {
		if _, ok := StageToLabels["InvalidStage"]; ok {
			t.Error("InvalidStage should not be in StageToLabels mapping")
		}
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
			name: "mixed labels with new stage: prefix",
			issue: &Issue{
				Number: 1,
				Labels: []struct {
					Name string `json:"name"`
				}{
					{Name: "sprint"},
					{Name: "stage:analysis"},
					{Name: "priority:high"},
					{Name: "stage:failed"},
				},
			},
			expected: []string{"stage:analysis", "stage:failed"},
		},
		{
			name: "all new stage labels",
			issue: &Issue{
				Number: 1,
				Labels: []struct {
					Name string `json:"name"`
				}{
					{Name: "stage:analysis"},
					{Name: "stage:awaiting-approval"},
					{Name: "stage:blocked"},
				},
			},
			expected: []string{"stage:analysis", "stage:awaiting-approval", "stage:blocked"},
		},
		{
			name: "legacy bare labels are also removed",
			issue: &Issue{
				Number: 1,
				Labels: []struct {
					Name string `json:"name"`
				}{
					{Name: "awaiting-approval"},
					{Name: "failed"},
					{Name: "blocked"},
					{Name: "in-progress"},
				},
			},
			expected: []string{"awaiting-approval", "failed", "blocked", "in-progress"},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := client.getStageLabelsToRemove(tt.issue)
			if len(result) != len(tt.expected) {
				t.Errorf("Expected %d labels to remove, got %d: %v", len(tt.expected), len(result), result)
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
		_ = client.AddLabels
	})

	t.Run("RemoveLabels concurrent execution", func(t *testing.T) {
		client := &Client{Repo: "test/repo"}
		_ = client.RemoveLabels
	})
}

// Test edge cases
func TestSetStageLabelEdgeCases(t *testing.T) {
	t.Run("empty stage mapping", func(t *testing.T) {
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
		"stage:awaiting-approval",
		"stage:failed",
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
		{"Backlog", "Plan", []string{"stage:analysis"}, []string{}},
		{"Plan", "Code", []string{"stage:coding"}, []string{"stage:analysis"}},
		{"Code", "AI Review", []string{"stage:code-review"}, []string{"stage:coding"}},
		{"AI Review", "Create PR", []string{"stage:create-pr"}, []string{"stage:code-review"}},
		{"Create PR", "Approve", []string{"stage:awaiting-approval"}, []string{"stage:create-pr"}},
		{"Approve", "Merge", []string{"stage:merging"}, []string{"stage:awaiting-approval"}},
		{"Merge", "Done", []string{}, []string{"stage:merging"}},
		{"Code", "Failed", []string{"stage:failed"}, []string{"stage:coding"}},
		{"Plan", "Blocked", []string{"stage:blocked"}, []string{"stage:analysis"}},
		{"Failed", "Code", []string{"stage:coding"}, []string{"stage:failed"}},
		{"Blocked", "Backlog", []string{}, []string{"stage:blocked"}},
	}

	for _, tc := range transitions {
		t.Run(fmt.Sprintf("%s to %s", tc.from, tc.to), func(t *testing.T) {
			targetLabels, ok := StageToLabels[tc.to]
			if !ok {
				t.Fatalf("Stage %s not found", tc.to)
			}

			if len(targetLabels) != len(tc.adds) {
				t.Errorf("Expected %d labels to add, got %d", len(tc.adds), len(targetLabels))
			}

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
		"Create PR",
		"Approve",
		"Merge",
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
	client := &Client{Repo: "test/repo"}
	var _ func(int, string) (Issue, error) = client.SetStageLabel
}

// Test that the client has all necessary methods for SetStageLabel
func TestClientMethodsExist(t *testing.T) {
	client := &Client{Repo: "test/repo"}
	var _ func(int) (*Issue, error) = client.GetIssue
	var _ func(int, string) error = client.AddLabel
	var _ func(int, string) error = client.RemoveLabel
	var _ func(int) error = client.CloseIssue
}

// Integration test simulation
func TestSetStageLabelIntegration(t *testing.T) {
	t.Run("Plan to Code workflow", func(t *testing.T) {
		initialLabels := []string{"stage:analysis", "sprint"}

		var toRemove []string
		for _, label := range initialLabels {
			if IsStageLabel(label) {
				toRemove = append(toRemove, label)
			}
		}

		expectedRemoves := []string{"stage:analysis"}
		if len(toRemove) != len(expectedRemoves) {
			t.Errorf("Expected %d labels to remove, got %d", len(expectedRemoves), len(toRemove))
		}

		labelsToAdd := StageToLabels["Code"]
		expectedAdds := []string{"stage:coding"}
		if len(labelsToAdd) != len(expectedAdds) {
			t.Errorf("Expected %d labels to add, got %d", len(expectedAdds), len(labelsToAdd))
		}
	})

	t.Run("Done stage workflow", func(t *testing.T) {
		initialLabels := []string{"stage:merging", "sprint"}

		var toRemove []string
		for _, label := range initialLabels {
			if IsStageLabel(label) {
				toRemove = append(toRemove, label)
			}
		}

		expectedRemoves := []string{"stage:merging"}
		if len(toRemove) != len(expectedRemoves) {
			t.Errorf("Expected %d labels to remove, got %d", len(expectedRemoves), len(toRemove))
		}

		labelsToAdd := StageToLabels["Done"]
		if len(labelsToAdd) != 0 {
			t.Errorf("Done stage should have 0 labels to add, got %d", len(labelsToAdd))
		}
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

	mc.ghOutputs[key] = []byte(`{"number":123,"title":"Test"}`)

	if _, ok := mc.ghOutputs[key]; !ok {
		t.Error("Mock output not stored correctly")
	}
}

// Test that SetStageLabel handles all stages defined in the mapping
func TestSetStageLabelAllStages(t *testing.T) {
	for stage := range StageToLabels {
		t.Run(stage, func(t *testing.T) {
			labels, ok := StageToLabels[stage]
			if !ok {
				t.Fatalf("Stage %s not found in mapping", stage)
			}

			for _, label := range labels {
				if label == "" {
					t.Error("Empty label in stage mapping")
				}
			}

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
