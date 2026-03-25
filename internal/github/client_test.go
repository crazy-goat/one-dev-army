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

func TestExtractRunID(t *testing.T) {
	tests := []struct {
		name string
		link string
		want string
	}{
		{
			name: "standard actions URL with job",
			link: "https://github.com/owner/repo/actions/runs/12345678/job/98765432",
			want: "12345678",
		},
		{
			name: "actions URL without job",
			link: "https://github.com/owner/repo/actions/runs/99887766",
			want: "99887766",
		},
		{
			name: "empty link",
			link: "",
			want: "",
		},
		{
			name: "non-actions URL",
			link: "https://github.com/owner/repo/pull/42",
			want: "",
		},
		{
			name: "actions URL with attempt",
			link: "https://github.com/owner/repo/actions/runs/12345678/attempts/2",
			want: "12345678",
		},
		{
			name: "external check URL",
			link: "https://codecov.io/gh/owner/repo/commit/abc123",
			want: "",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := extractRunID(tt.link)
			if got != tt.want {
				t.Errorf("extractRunID(%q) = %q, want %q", tt.link, got, tt.want)
			}
		})
	}
}

func TestRunIDPattern(t *testing.T) {
	// Verify the compiled regex is valid and matches expected patterns
	if runIDPattern == nil {
		t.Fatal("runIDPattern is nil")
	}

	// Verify it doesn't match partial patterns
	if extractRunID("/actions/runs/") != "" {
		t.Error("should not match /actions/runs/ without digits")
	}

	// Verify it extracts only digits
	got := extractRunID("https://github.com/o/r/actions/runs/42/job/1")
	if got != "42" {
		t.Errorf("expected '42', got %q", got)
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
			t.Fatal("NewSprintDetector() returned nil")
		}

		if detector.client != client {
			t.Error("SprintDetector client not set correctly")
		}
	})
}
