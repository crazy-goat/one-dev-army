package dashboard

import (
	"strings"
	"testing"
)

func TestCleanupMarkdown(t *testing.T) {
	tests := []struct {
		name     string
		input    string
		expected string
	}{
		{
			name:     "empty content",
			input:    "",
			expected: "",
		},
		{
			name:     "simple content",
			input:    "Hello world",
			expected: "Hello world\n",
		},
		{
			name:     "content with CRLF line endings",
			input:    "Line 1\r\nLine 2\r\nLine 3",
			expected: "Line 1\nLine 2\nLine 3\n",
		},
		{
			name:     "content with CR line endings",
			input:    "Line 1\rLine 2\rLine 3",
			expected: "Line 1\nLine 2\nLine 3\n",
		},
		{
			name:     "trailing whitespace",
			input:    "Line with trailing spaces   \nAnother line\t\t",
			expected: "Line with trailing spaces\nAnother line\n",
		},
		{
			name:     "multiple blank lines",
			input:    "Line 1\n\n\n\n\nLine 2",
			expected: "Line 1\n\nLine 2\n",
		},
		{
			name:     "section heading without spacing",
			input:    "## Description\nSome text\n## Tasks\nMore text",
			expected: "## Description\n\nSome text\n\n## Tasks\n\nMore text\n",
		},
		{
			name:     "bullet list with asterisks",
			input:    "* Item 1\n* Item 2\n* Item 3",
			expected: "- Item 1\n- Item 2\n- Item 3\n",
		},
		{
			name:     "numbered list",
			input:    "1. First\n2. Second\n3. Third",
			expected: "1. First\n2. Second\n3. Third\n",
		},
		{
			name:     "checkbox list",
			input:    "- [ ] Task 1\n- [x] Task 2",
			expected: "- [ ] Task 1\n- [x] Task 2\n",
		},
		{
			name:     "code block",
			input:    "```go\nfunc main() {}\n```",
			expected: "```go\nfunc main() {}\n\n```\n",
		},
		{
			name:     "mixed content",
			input:    "## Description\r\nSome text   \n\n\n\n## Tasks\n* Task 1\n* Task 2\n\n## Files\n1. file.go\n2. file2.go",
			expected: "## Description\n\nSome text\n\n## Tasks\n\n- Task 1\n- Task 2\n\n## Files\n\n1. file.go\n2. file2.go\n",
		},
		{
			name:     "content without final newline",
			input:    "Some content",
			expected: "Some content\n",
		},
		{
			name:     "content with multiple final newlines",
			input:    "Some content\n\n\n",
			expected: "Some content\n",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := CleanupMarkdown(tt.input)
			if result != tt.expected {
				t.Errorf("CleanupMarkdown() = %q, want %q", result, tt.expected)
			}
		})
	}
}

func TestValidateMarkdown(t *testing.T) {
	tests := []struct {
		name     string
		input    string
		expected []string
	}{
		{
			name:     "empty content",
			input:    "",
			expected: []string{},
		},
		{
			name:     "valid markdown",
			input:    "## Description\n\nSome text\n",
			expected: []string{},
		},
		{
			name:     "unclosed code block",
			input:    "```go\nfunc main() {}",
			expected: []string{"Unclosed code block starting at line 1"},
		},
		{
			name:     "closed code block",
			input:    "```go\nfunc main() {}\n```",
			expected: []string{},
		},
		{
			name:     "single level heading",
			input:    "# Title\n\n## Description\n\nText",
			expected: []string{"Line 1 uses single # heading (should use ## for sections)"},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := ValidateMarkdown(tt.input)
			if len(result) != len(tt.expected) {
				t.Errorf("ValidateMarkdown() returned %d errors, want %d", len(result), len(tt.expected))
				return
			}
			for i, err := range result {
				if !strings.Contains(err, tt.expected[i]) {
					t.Errorf("ValidateMarkdown() error[%d] = %q, should contain %q", i, err, tt.expected[i])
				}
			}
		})
	}
}

func TestNormalizeLineEndings(t *testing.T) {
	tests := []struct {
		name     string
		input    string
		expected string
	}{
		{
			name:     "CRLF to LF",
			input:    "Line 1\r\nLine 2\r\n",
			expected: "Line 1\nLine 2\n",
		},
		{
			name:     "CR to LF",
			input:    "Line 1\rLine 2\r",
			expected: "Line 1\nLine 2\n",
		},
		{
			name:     "mixed line endings",
			input:    "Line 1\r\nLine 2\rLine 3\n",
			expected: "Line 1\nLine 2\nLine 3\n",
		},
		{
			name:     "already LF",
			input:    "Line 1\nLine 2\n",
			expected: "Line 1\nLine 2\n",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := normalizeLineEndings(tt.input)
			if result != tt.expected {
				t.Errorf("normalizeLineEndings() = %q, want %q", result, tt.expected)
			}
		})
	}
}

func TestRemoveTrailingWhitespace(t *testing.T) {
	tests := []struct {
		name     string
		input    string
		expected string
	}{
		{
			name:     "spaces at end",
			input:    "Line with spaces   ",
			expected: "Line with spaces",
		},
		{
			name:     "tabs at end",
			input:    "Line with tabs\t\t",
			expected: "Line with tabs",
		},
		{
			name:     "mixed whitespace",
			input:    "Line with mixed \t \t",
			expected: "Line with mixed",
		},
		{
			name:     "multiple lines",
			input:    "Line 1  \nLine 2\t\nLine 3",
			expected: "Line 1\nLine 2\nLine 3",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := removeTrailingWhitespace(tt.input)
			if result != tt.expected {
				t.Errorf("removeTrailingWhitespace() = %q, want %q", result, tt.expected)
			}
		})
	}
}

func TestNormalizeMultipleBlankLines(t *testing.T) {
	tests := []struct {
		name     string
		input    string
		expected string
	}{
		{
			name:     "three newlines",
			input:    "Line 1\n\n\nLine 2",
			expected: "Line 1\n\nLine 2",
		},
		{
			name:     "four newlines",
			input:    "Line 1\n\n\n\nLine 2",
			expected: "Line 1\n\nLine 2",
		},
		{
			name:     "two newlines (OK)",
			input:    "Line 1\n\nLine 2",
			expected: "Line 1\n\nLine 2",
		},
		{
			name:     "one newline (OK)",
			input:    "Line 1\nLine 2",
			expected: "Line 1\nLine 2",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := normalizeMultipleBlankLines(tt.input)
			if result != tt.expected {
				t.Errorf("normalizeMultipleBlankLines() = %q, want %q", result, tt.expected)
			}
		})
	}
}

func TestEnsureFinalNewline(t *testing.T) {
	tests := []struct {
		name     string
		input    string
		expected string
	}{
		{
			name:     "no newline",
			input:    "content",
			expected: "content\n",
		},
		{
			name:     "one newline",
			input:    "content\n",
			expected: "content\n",
		},
		{
			name:     "multiple newlines",
			input:    "content\n\n\n",
			expected: "content\n",
		},
		{
			name:     "empty string",
			input:    "",
			expected: "\n",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := ensureFinalNewline(tt.input)
			if result != tt.expected {
				t.Errorf("ensureFinalNewline() = %q, want %q", result, tt.expected)
			}
		})
	}
}

func TestFixListFormatting(t *testing.T) {
	tests := []struct {
		name     string
		input    string
		expected string
	}{
		{
			name:     "asterisk to dash",
			input:    "* Item 1\n* Item 2",
			expected: "- Item 1\n- Item 2",
		},
		{
			name:     "dash stays dash",
			input:    "- Item 1\n- Item 2",
			expected: "- Item 1\n- Item 2",
		},
		{
			name:     "numbered list",
			input:    "1. First\n2. Second",
			expected: "1. First\n2. Second",
		},
		{
			name:     "checkbox list",
			input:    "- [ ] Unchecked\n- [x] Checked",
			expected: "- [ ] Unchecked\n- [x] Checked",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := fixListFormatting(tt.input)
			if result != tt.expected {
				t.Errorf("fixListFormatting() = %q, want %q", result, tt.expected)
			}
		})
	}
}

func TestEnsureSectionSpacing(t *testing.T) {
	tests := []struct {
		name     string
		input    string
		expected string
	}{
		{
			name:     "section without spacing",
			input:    "Text\n## Section\nMore text",
			expected: "Text\n\n## Section\n\nMore text",
		},
		{
			name:     "section at start",
			input:    "## Section\nText",
			expected: "## Section\n\nText",
		},
		{
			name:     "multiple sections",
			input:    "## First\nText\n## Second\nMore",
			expected: "## First\n\nText\n\n## Second\n\nMore",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := ensureSectionSpacing(tt.input)
			if result != tt.expected {
				t.Errorf("ensureSectionSpacing() = %q, want %q", result, tt.expected)
			}
		})
	}
}
