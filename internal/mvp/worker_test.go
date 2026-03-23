package mvp

import (
	"testing"

	"github.com/crazy-goat/one-dev-army/internal/opencode"
)

func TestSlug(t *testing.T) {
	tests := []struct {
		input string
		want  string
	}{
		{"Add feature X", "add-feature-x"},
		{"Fix bug #123", "fix-bug-123"},
		{"UPPERCASE TITLE", "uppercase-title"},
		{"  spaces  around  ", "spaces-around"},
		{"special!@#$%chars", "special-chars"},
		{"", ""},
		{"a", "a"},
		{"already-slugged", "already-slugged"},
		{"This is a very long title that should be truncated to forty characters maximum", "this-is-a-very-long-title-that-should-be"},
		{"Trailing-dash-at-exactly-forty-characters", "trailing-dash-at-exactly-forty-character"},
		{"dots.and.periods", "dots-and-periods"},
		{"multiple---dashes", "multiple-dashes"},
	}

	for _, tt := range tests {
		t.Run(tt.input, func(t *testing.T) {
			got := slug(tt.input)
			if got != tt.want {
				t.Errorf("slug(%q) = %q, want %q", tt.input, got, tt.want)
			}
		})
	}
}

func TestSlugMaxLength(t *testing.T) {
	long := "abcdefghijklmnopqrstuvwxyz-abcdefghijklmnopqrstuvwxyz"
	got := slug(long)
	if len(got) > 40 {
		t.Errorf("slug length = %d, want <= 40", len(got))
	}
}

func TestSlugNoTrailingDash(t *testing.T) {
	input := "this title is exactly forty one chars long"
	got := slug(input)
	if len(got) > 0 && got[len(got)-1] == '-' {
		t.Errorf("slug(%q) = %q, ends with dash", input, got)
	}
}

func TestExtractText(t *testing.T) {
	tests := []struct {
		name string
		msg  *opencode.Message
		want string
	}{
		{
			name: "nil message",
			msg:  nil,
			want: "",
		},
		{
			name: "empty parts",
			msg:  &opencode.Message{},
			want: "",
		},
		{
			name: "single text part",
			msg: &opencode.Message{
				Parts: []opencode.Part{
					{Type: "text", Text: "hello world"},
				},
			},
			want: "hello world",
		},
		{
			name: "multiple text parts",
			msg: &opencode.Message{
				Parts: []opencode.Part{
					{Type: "text", Text: "part1"},
					{Type: "text", Text: "part2"},
				},
			},
			want: "part1part2",
		},
		{
			name: "mixed part types",
			msg: &opencode.Message{
				Parts: []opencode.Part{
					{Type: "text", Text: "hello"},
					{Type: "tool_use", Text: "ignored"},
					{Type: "text", Text: " world"},
				},
			},
			want: "hello world",
		},
		{
			name: "no text parts",
			msg: &opencode.Message{
				Parts: []opencode.Part{
					{Type: "tool_use", Text: "ignored"},
				},
			},
			want: "",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := extractText(tt.msg)
			if got != tt.want {
				t.Errorf("extractText() = %q, want %q", got, tt.want)
			}
		})
	}
}

func TestParseTechnicalPlanningResponse(t *testing.T) {
	tests := []struct {
		name         string
		response     string
		wantAnalysis string
		wantPlan     string
	}{
		{
			name: "both sections present",
			response: `## Analysis

1. Core requirements: implement feature X
2. Files to change: main.go
3. Implementation approach: add new function
4. Testing strategy: unit tests

## Implementation Plan

1. Modify main.go to add feature
2. Add tests in main_test.go`,
			wantAnalysis: "1. Core requirements: implement feature X\n2. Files to change: main.go\n3. Implementation approach: add new function\n4. Testing strategy: unit tests",
			wantPlan:     "1. Modify main.go to add feature\n2. Add tests in main_test.go",
		},
		{
			name: "only analysis header",
			response: `## Analysis

Some analysis content here
More analysis content`,
			wantAnalysis: "Some analysis content here\nMore analysis content",
			wantPlan:     "Some analysis content here\nMore analysis content",
		},
		{
			name: "only plan header",
			response: `Some intro text

## Implementation Plan

1. Step one
2. Step two`,
			wantAnalysis: "Some intro text",
			wantPlan:     "1. Step one\n2. Step two",
		},
		{
			name: "no headers - heuristic split",
			response: `Analysis part here.

Implementation Plan:
1. First step
2. Second step`,
			wantAnalysis: "Analysis part here.",
			wantPlan:     "Implementation Plan:\n1. First step\n2. Second step",
		},
		{
			name:         "no headers - no split",
			response:     `Just some content without any markers`,
			wantAnalysis: "Just some content without any markers",
			wantPlan:     "Just some content without any markers",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			gotAnalysis, gotPlan := parseTechnicalPlanningResponse(tt.response)
			if gotAnalysis != tt.wantAnalysis {
				t.Errorf("parseTechnicalPlanningResponse() analysis = %q, want %q", gotAnalysis, tt.wantAnalysis)
			}
			if gotPlan != tt.wantPlan {
				t.Errorf("parseTechnicalPlanningResponse() plan = %q, want %q", gotPlan, tt.wantPlan)
			}
		})
	}
}

func TestCheckAlreadyDone(t *testing.T) {
	tests := []struct {
		name     string
		response string
		want     string
	}{
		{
			name:     "ALREADY_DONE prefix present",
			response: "ALREADY_DONE: method Foo already exists in bar.go:42",
			want:     "method Foo already exists in bar.go:42",
		},
		{
			name:     "ALREADY_DONE with extra text",
			response: "Some text\nALREADY_DONE: feature already implemented\nMore text",
			want:     "feature already implemented",
		},
		{
			name:     "no ALREADY_DONE",
			response: "This is a normal response without the prefix",
			want:     "",
		},
		{
			name:     "empty response",
			response: "",
			want:     "",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := checkAlreadyDone(tt.response)
			if got != tt.want {
				t.Errorf("checkAlreadyDone() = %q, want %q", got, tt.want)
			}
		})
	}
}
