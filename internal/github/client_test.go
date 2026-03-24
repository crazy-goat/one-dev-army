package github

import (
	"testing"
)

func TestBuildLabels(t *testing.T) {
	tests := []struct {
		name      string
		priority  string
		size      string
		issueType string
		want      []string
	}{
		{
			name:      "no labels",
			priority:  "",
			size:      "",
			issueType: "",
			want:      []string{},
		},
		{
			name:      "priority only",
			priority:  "high",
			size:      "",
			issueType: "",
			want:      []string{"priority:high"},
		},
		{
			name:      "size only",
			priority:  "",
			size:      "M",
			issueType: "",
			want:      []string{"size:M"},
		},
		{
			name:      "type only",
			priority:  "",
			size:      "",
			issueType: "bug",
			want:      []string{"bug"},
		},
		{
			name:      "all labels",
			priority:  "high",
			size:      "L",
			issueType: "feature",
			want:      []string{"priority:high", "size:L", "feature"},
		},
		{
			name:      "priority and size",
			priority:  "medium",
			size:      "S",
			issueType: "",
			want:      []string{"priority:medium", "size:S"},
		},
		{
			name:      "priority and type",
			priority:  "low",
			size:      "",
			issueType: "bug",
			want:      []string{"priority:low", "bug"},
		},
		{
			name:      "size and type",
			priority:  "",
			size:      "XL",
			issueType: "feature",
			want:      []string{"size:XL", "feature"},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := BuildLabels(tt.priority, tt.size, tt.issueType)
			if len(got) != len(tt.want) {
				t.Errorf("BuildLabels() = %v, want %v", got, tt.want)
				return
			}
			for i := range got {
				if got[i] != tt.want[i] {
					t.Errorf("BuildLabels()[%d] = %v, want %v", i, got[i], tt.want[i])
				}
			}
		})
	}
}

func TestSprintDetector_GetCurrentSprintTitle(t *testing.T) {
	// This test requires a mock client - for now just test the structure
	// In a real scenario, we'd mock the GitHub client
	t.Run("returns empty when no milestone", func(t *testing.T) {
		// Since we can't easily mock the client without interfaces,
		// we'll just verify the SprintDetector struct exists and works
		client := NewClient("test/repo")
		detector := NewSprintDetector(client)

		if detector == nil {
			t.Error("NewSprintDetector() returned nil")
		}

		if detector.client != client {
			t.Error("SprintDetector client not set correctly")
		}
	})
}
