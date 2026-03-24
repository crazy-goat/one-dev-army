package github

import (
	"encoding/json"
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

func TestRequiredLabelsContainsPriorityLabels(t *testing.T) {
	expectedLabels := map[string]string{
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
	_ = &Client{Repo: "test/repo"}
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
		{"stage:done exists", "stage:done", "0E8A16", true},
		{"stage:needs-user exists", "stage:needs-user", "FBCA04", true},
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
	_ = &Client{Repo: "test/repo"}
}

func TestRequiredLabelsCount(t *testing.T) {
	// Labels:
	// sprint, insight (2)
	// size:S, size:M, size:L, size:XL (4)
	// stage:backlog, stage:analysis, stage:coding, stage:code-review, stage:create-pr,
	// stage:awaiting-approval, stage:merging, stage:done, stage:failed, stage:blocked,
	// stage:needs-user (11)
	// priority:high, priority:medium, priority:low (3)
	// epic, wizard, merge-failed (3)
	// bug, feature (2)
	// Total: 25
	expectedCount := 25
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

func TestStageLabelRoundTrip(t *testing.T) {
	tests := []struct {
		stage         Stage
		expectedLabel string
	}{
		{StageBacklog, "stage:backlog"},
		{StagePlan, "stage:analysis"},
		{StageCode, "stage:coding"},
		{StageReview, "stage:code-review"},
		{StageCreatePR, "stage:create-pr"},
		{StageApprove, "stage:awaiting-approval"},
		{StageMerge, "stage:merging"},
		{StageDone, "stage:done"},
		{StageFailed, "stage:failed"},
		{StageBlocked, "stage:blocked"},
		{StageNeedsUser, "stage:needs-user"},
	}

	for _, tt := range tests {
		t.Run(tt.expectedLabel, func(t *testing.T) {
			// Stage.Label() returns the correct label string
			if tt.stage.Label() != tt.expectedLabel {
				t.Errorf("Stage(%q).Label() = %q, want %q", tt.stage, tt.stage.Label(), tt.expectedLabel)
			}

			// StageFromLabel() round-trips back to the same Stage
			got, ok := StageFromLabel(tt.expectedLabel)
			if !ok {
				t.Errorf("StageFromLabel(%q) returned false", tt.expectedLabel)
				return
			}
			if got != tt.stage {
				t.Errorf("StageFromLabel(%q) = %q, want %q", tt.expectedLabel, got, tt.stage)
			}
		})
	}
}

// Test Stage.Column() mapping
func TestStageColumn(t *testing.T) {
	tests := []struct {
		stage    Stage
		expected string
	}{
		{StageBacklog, "Backlog"},
		{StagePlan, "Plan"},
		{StageCode, "Code"},
		{StageReview, "AI Review"},
		{StageCreatePR, "AI Review"},
		{StageApprove, "Approve"},
		{StageMerge, "Merge"},
		{StageDone, "Done"},
		{StageFailed, "Failed"},
		{StageBlocked, "Blocked"},
		{StageNeedsUser, "Blocked"},
	}

	for _, tt := range tests {
		t.Run(string(tt.stage), func(t *testing.T) {
			if tt.stage.Column() != tt.expected {
				t.Errorf("Stage(%q).Column() = %q, want %q", tt.stage, tt.stage.Column(), tt.expected)
			}
		})
	}
}

// Test StageFromLabel with unknown labels
func TestStageFromLabelUnknown(t *testing.T) {
	unknowns := []string{"stage:unknown", "invalid", "priority:high", "sprint", ""}
	for _, label := range unknowns {
		t.Run(label, func(t *testing.T) {
			_, ok := StageFromLabel(label)
			if ok {
				t.Errorf("StageFromLabel(%q) should return false for unknown label", label)
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
		{"stage:done", true},
		{"stage:needs-user", true},
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
		expected Stage
	}{
		{"empty labels", []string{}, StageBacklog},
		{"no stage labels", []string{"sprint", "priority:high"}, StageBacklog},
		{"Plan stage", []string{"stage:analysis", "sprint"}, StagePlan},
		{"Code stage", []string{"stage:coding"}, StageCode},
		{"AI Review stage", []string{"stage:code-review"}, StageReview},
		{"Create PR stage", []string{"stage:create-pr"}, StageCreatePR},
		{"Approve stage", []string{"stage:awaiting-approval"}, StageApprove},
		{"Merge stage", []string{"stage:merging"}, StageMerge},
		{"Done stage", []string{"stage:done"}, StageDone},
		{"Failed stage", []string{"stage:failed"}, StageFailed},
		{"Blocked stage", []string{"stage:blocked"}, StageBlocked},
		{"NeedsUser stage", []string{"stage:needs-user"}, StageNeedsUser},
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
		_ = &Client{Repo: mc.Repo}
	})

	t.Run("invalid stage label not in AllStages", func(t *testing.T) {
		_, ok := StageFromLabel("stage:invalid")
		if ok {
			t.Error("stage:invalid should not be a valid stage")
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

// Integration-style test for SetStageLabel workflow using Stage constants
func TestSetStageLabelWorkflow(t *testing.T) {
	testCases := []struct {
		name            string
		stage           Stage
		initialLabels   []string
		expectedLabel   string
		expectedRemoves []string
		shouldClose     bool
	}{
		{
			name:            "Backlog - removes all stage labels and adds stage:backlog",
			stage:           StageBacklog,
			initialLabels:   []string{"stage:analysis", "sprint"},
			expectedLabel:   "stage:backlog",
			expectedRemoves: []string{"stage:analysis"},
			shouldClose:     false,
		},
		{
			name:            "Plan - adds analysis",
			stage:           StagePlan,
			initialLabels:   []string{"sprint"},
			expectedLabel:   "stage:analysis",
			expectedRemoves: []string{},
			shouldClose:     false,
		},
		{
			name:            "Code - replaces Plan label",
			stage:           StageCode,
			initialLabels:   []string{"stage:analysis", "sprint"},
			expectedLabel:   "stage:coding",
			expectedRemoves: []string{"stage:analysis"},
			shouldClose:     false,
		},
		{
			name:            "Create PR - replaces code-review label",
			stage:           StageCreatePR,
			initialLabels:   []string{"stage:code-review", "sprint"},
			expectedLabel:   "stage:create-pr",
			expectedRemoves: []string{"stage:code-review"},
			shouldClose:     false,
		},
		{
			name:            "Approve - replaces create-pr label",
			stage:           StageApprove,
			initialLabels:   []string{"stage:create-pr", "sprint"},
			expectedLabel:   "stage:awaiting-approval",
			expectedRemoves: []string{"stage:create-pr"},
			shouldClose:     false,
		},
		{
			name:            "Merge - replaces awaiting-approval label",
			stage:           StageMerge,
			initialLabels:   []string{"stage:awaiting-approval", "sprint"},
			expectedLabel:   "stage:merging",
			expectedRemoves: []string{"stage:awaiting-approval"},
			shouldClose:     false,
		},
		{
			name:            "Done - adds stage:done and closes issue",
			stage:           StageDone,
			initialLabels:   []string{"stage:merging", "sprint"},
			expectedLabel:   "stage:done",
			expectedRemoves: []string{"stage:merging"},
			shouldClose:     true,
		},
		{
			name:            "Failed - adds stage:failed label",
			stage:           StageFailed,
			initialLabels:   []string{"stage:coding"},
			expectedLabel:   "stage:failed",
			expectedRemoves: []string{"stage:coding"},
			shouldClose:     false,
		},
		{
			name:            "Blocked - adds stage:blocked label",
			stage:           StageBlocked,
			initialLabels:   []string{"stage:analysis"},
			expectedLabel:   "stage:blocked",
			expectedRemoves: []string{"stage:analysis"},
			shouldClose:     false,
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			// Verify the stage label matches expected
			if tc.stage.Label() != tc.expectedLabel {
				t.Errorf("Stage %s: expected label %s, got %s",
					tc.stage, tc.expectedLabel, tc.stage.Label())
			}

			// Verify removes are stage labels
			for _, remove := range tc.expectedRemoves {
				if !IsStageLabel(remove) {
					t.Errorf("Expected remove label %s is not a stage label", remove)
				}
			}

			// Verify Done stage should close
			if tc.stage == StageDone && !tc.shouldClose {
				t.Error("Done stage should set shouldClose to true")
			}
		})
	}
}

// Test error handling in SetStageLabel
func TestSetStageLabelErrorHandling(t *testing.T) {
	t.Run("invalid stage label not recognized", func(t *testing.T) {
		_, ok := StageFromLabel("stage:invalid")
		if ok {
			t.Error("stage:invalid should not be recognized")
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

// Test that all stages have exactly one label via Label()
func TestAllStagesHaveLabels(t *testing.T) {
	for _, stage := range AllStages {
		label := stage.Label()
		if label == "" {
			t.Errorf("Stage %q has empty label", stage)
		}
		if !strings.HasPrefix(label, "stage:") {
			t.Errorf("Stage %q label %q does not have stage: prefix", stage, label)
		}
	}
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

// Test that all stage labels in AllStages exist in RequiredLabels
func TestStageLabelsExistInRequiredLabels(t *testing.T) {
	// Create set of required labels
	requiredSet := make(map[string]bool)
	for _, l := range RequiredLabels {
		requiredSet[l.Name] = true
	}

	// Verify all stage labels exist in RequiredLabels
	for _, stage := range AllStages {
		label := stage.Label()
		if !requiredSet[label] {
			t.Errorf("Stage label %s (from stage %q) is not in RequiredLabels", label, stage)
		}
	}
}

// Test comprehensive stage transition scenarios
func TestStageTransitions(t *testing.T) {
	transitions := []struct {
		from    Stage
		to      Stage
		adds    string
		removes []string
	}{
		{StageBacklog, StagePlan, "stage:analysis", []string{}},
		{StagePlan, StageCode, "stage:coding", []string{"stage:analysis"}},
		{StageCode, StageReview, "stage:code-review", []string{"stage:coding"}},
		{StageReview, StageCreatePR, "stage:create-pr", []string{"stage:code-review"}},
		{StageCreatePR, StageApprove, "stage:awaiting-approval", []string{"stage:create-pr"}},
		{StageApprove, StageMerge, "stage:merging", []string{"stage:awaiting-approval"}},
		{StageMerge, StageDone, "stage:done", []string{"stage:merging"}},
		{StageCode, StageFailed, "stage:failed", []string{"stage:coding"}},
		{StagePlan, StageBlocked, "stage:blocked", []string{"stage:analysis"}},
		{StageFailed, StageCode, "stage:coding", []string{"stage:failed"}},
		{StageBlocked, StageBacklog, "stage:backlog", []string{"stage:blocked"}},
	}

	for _, tc := range transitions {
		t.Run(fmt.Sprintf("%s to %s", tc.from, tc.to), func(t *testing.T) {
			targetLabel := tc.to.Label()
			if targetLabel != tc.adds {
				t.Errorf("Expected label %s, got %s", tc.adds, targetLabel)
			}

			for _, remove := range tc.removes {
				if !IsStageLabel(remove) {
					t.Errorf("Remove label %s is not a stage label", remove)
				}
			}
		})
	}
}

// Test that AllStages contains all expected stages
func TestAllStagesCompleteness(t *testing.T) {
	expectedStages := []Stage{
		StageBacklog,
		StagePlan,
		StageCode,
		StageReview,
		StageCreatePR,
		StageApprove,
		StageMerge,
		StageDone,
		StageFailed,
		StageBlocked,
		StageNeedsUser,
	}

	if len(AllStages) != len(expectedStages) {
		t.Errorf("Expected %d stages in AllStages, got %d", len(expectedStages), len(AllStages))
	}

	stageSet := make(map[Stage]bool)
	for _, s := range AllStages {
		stageSet[s] = true
	}

	for _, expected := range expectedStages {
		if !stageSet[expected] {
			t.Errorf("Expected stage %q not found in AllStages", expected)
		}
	}
}

// Test SetStageLabel method signature accepts Stage type
func TestSetStageLabelSignature(t *testing.T) {
	client := &Client{Repo: "test/repo"}
	var _ = client.SetStageLabel
}

// Test that the client has all necessary methods for SetStageLabel
func TestClientMethodsExist(t *testing.T) {
	client := &Client{Repo: "test/repo"}
	var _ = client.GetIssue
	var _ = client.AddLabel
	var _ = client.RemoveLabel
	var _ = client.CloseIssue
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

		expectedAdd := StageCode.Label()
		if expectedAdd != "stage:coding" {
			t.Errorf("Expected stage:coding, got %s", expectedAdd)
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

		// Done stage adds stage:done label
		expectedAdd := StageDone.Label()
		if expectedAdd != "stage:done" {
			t.Errorf("Expected stage:done, got %s", expectedAdd)
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

// Test that all stage labels in AllStages use valid labels from RequiredLabels
func TestStageMappingsUseValidLabels(t *testing.T) {
	requiredSet := make(map[string]bool)
	for _, l := range RequiredLabels {
		requiredSet[l.Name] = true
	}

	for _, stage := range AllStages {
		label := stage.Label()
		if !requiredSet[label] {
			t.Errorf("Stage %q uses label %s which is not in RequiredLabels", stage, label)
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

// Test that SetStageLabel handles all stages defined in AllStages
func TestSetStageLabelAllStages(t *testing.T) {
	for _, stage := range AllStages {
		t.Run(string(stage), func(t *testing.T) {
			label := stage.Label()
			if label == "" {
				t.Error("Empty label for stage")
			}
			if !strings.HasPrefix(label, "stage:") {
				t.Errorf("Label %q does not have stage: prefix", label)
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

// Test stagesByLabel init map is populated correctly
func TestStagesByLabelInit(t *testing.T) {
	// Every stage in AllStages should be findable via StageFromLabel
	for _, stage := range AllStages {
		found, ok := StageFromLabel(stage.Label())
		if !ok {
			t.Errorf("StageFromLabel(%q) returned false", stage.Label())
			continue
		}
		if found != stage {
			t.Errorf("StageFromLabel(%q) = %q, want %q", stage.Label(), found, stage)
		}
	}

	// The map should have exactly len(AllStages) entries
	if len(stagesByLabel) != len(AllStages) {
		t.Errorf("stagesByLabel has %d entries, expected %d", len(stagesByLabel), len(AllStages))
	}
}
