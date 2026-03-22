package mvp

import (
	"testing"

	"github.com/crazy-goat/one-dev-army/internal/opencode"
)

func strPtr(s string) *string {
	return &s
}

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
					{Type: "text", Text: strPtr("hello world")},
				},
			},
			want: "hello world",
		},
		{
			name: "multiple text parts",
			msg: &opencode.Message{
				Parts: []opencode.Part{
					{Type: "text", Text: strPtr("part1")},
					{Type: "text", Text: strPtr("part2")},
				},
			},
			want: "part1part2",
		},
		{
			name: "mixed part types",
			msg: &opencode.Message{
				Parts: []opencode.Part{
					{Type: "text", Text: strPtr("hello")},
					{Type: "tool_use", Text: strPtr("ignored")},
					{Type: "text", Text: strPtr(" world")},
				},
			},
			want: "hello world",
		},
		{
			name: "no text parts",
			msg: &opencode.Message{
				Parts: []opencode.Part{
					{Type: "tool_use", Text: strPtr("ignored")},
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
