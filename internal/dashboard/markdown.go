package dashboard

import (
	"regexp"
	"strconv"
	"strings"
)

var (
	// trailingWhitespaceRegex matches trailing whitespace on lines
	trailingWhitespaceRegex = regexp.MustCompile(`[ \t]+$`)

	// multipleBlankLinesRegex matches 3+ consecutive newlines
	multipleBlankLinesRegex = regexp.MustCompile(`\n{3,}`)

	// codeBlockRegex matches code blocks (both fenced and indented)
	codeBlockFenceRegex = regexp.MustCompile("^```")
)

// CleanupMarkdown fixes common markdown formatting issues in LLM-generated content.
// It normalizes line endings, fixes code blocks, normalizes headings, fixes list
// formatting, ensures proper section spacing, and removes trailing whitespace.
func CleanupMarkdown(content string) string {
	if content == "" {
		return ""
	}

	content = normalizeLineEndings(content)
	content = fixCodeBlocks(content)
	content = normalizeHeadings(content)
	content = fixListFormatting(content)
	content = ensureSectionSpacing(content)
	content = removeTrailingWhitespace(content)
	content = normalizeMultipleBlankLines(content)
	content = ensureFinalNewline(content)

	return content
}

// ValidateMarkdown checks if markdown has structural issues and returns a list of errors.
func ValidateMarkdown(content string) []string {
	var errors []string

	if content == "" {
		return errors
	}

	// Check for unclosed code blocks
	lines := strings.Split(content, "\n")
	inCodeBlock := false
	codeBlockStart := 0

	for i, line := range lines {
		if codeBlockFenceRegex.MatchString(line) {
			if inCodeBlock {
				inCodeBlock = false
			} else {
				inCodeBlock = true
				codeBlockStart = i + 1
			}
		}
	}

	if inCodeBlock {
		errors = append(errors, "Unclosed code block starting at line "+strconv.Itoa(codeBlockStart))
	}

	// Check for improper heading levels (should start with ## for sections)
	for i, line := range lines {
		if strings.HasPrefix(line, "# ") && !strings.HasPrefix(line, "## ") {
			// Single # heading found - might be okay for title, but warn
			errors = append(errors, "Line "+strconv.Itoa(i+1)+" uses single # heading (should use ## for sections)")
		}
	}

	return errors
}

// normalizeLineEndings converts all line endings to \n
func normalizeLineEndings(content string) string {
	content = strings.ReplaceAll(content, "\r\n", "\n")
	content = strings.ReplaceAll(content, "\r", "\n")
	return content
}

// fixCodeBlocks ensures proper code block formatting
func fixCodeBlocks(content string) string {
	lines := strings.Split(content, "\n")
	var result []string
	inCodeBlock := false

	for _, line := range lines {
		trimmed := strings.TrimSpace(line)

		// Check for code block fence
		if strings.HasPrefix(trimmed, "```") {
			// Ensure proper spacing around code blocks
			if len(result) > 0 && result[len(result)-1] != "" {
				result = append(result, "")
			}
			result = append(result, line)
			inCodeBlock = !inCodeBlock
			continue
		}

		result = append(result, line)
	}

	return strings.Join(result, "\n")
}

// normalizeHeadings ensures consistent heading levels
func normalizeHeadings(content string) string {
	lines := strings.Split(content, "\n")
	var result []string

	for i, line := range lines {
		trimmed := strings.TrimSpace(line)

		// Check if this is a heading
		if strings.HasPrefix(trimmed, "#") {
			// Ensure blank line before heading (except at start)
			if i > 0 && len(result) > 0 && result[len(result)-1] != "" {
				result = append(result, "")
			}

			result = append(result, line)

			// Ensure blank line after heading
			result = append(result, "")
		} else {
			result = append(result, line)
		}
	}

	return strings.Join(result, "\n")
}

// fixListFormatting normalizes list markers and spacing
func fixListFormatting(content string) string {
	lines := strings.Split(content, "\n")
	var result []string
	inList := false

	for i, line := range lines {
		trimmed := strings.TrimSpace(line)

		// Check if this is a list item
		isBullet := strings.HasPrefix(trimmed, "- ") || strings.HasPrefix(trimmed, "* ")
		isNumbered := regexp.MustCompile(`^\d+\.\s`).MatchString(trimmed)
		isCheckbox := strings.HasPrefix(trimmed, "- [") || strings.HasPrefix(trimmed, "* [")

		switch {
		case isBullet || isNumbered || isCheckbox:
			if !inList && i > 0 && len(result) > 0 && result[len(result)-1] != "" {
				// Add blank line before list starts
				result = append(result, "")
			}
			inList = true

			// Normalize bullet markers to "- "
			if strings.HasPrefix(trimmed, "* ") {
				line = strings.Replace(line, "* ", "- ", 1)
			}

			result = append(result, line)
		default:
			inList = false
			result = append(result, line)
		}
	}

	return strings.Join(result, "\n")
}

// ensureSectionSpacing adds proper spacing between sections
func ensureSectionSpacing(content string) string {
	// Split by sections (lines starting with ##)
	lines := strings.Split(content, "\n")
	var result []string

	for i, line := range lines {
		trimmed := strings.TrimSpace(line)

		if strings.HasPrefix(trimmed, "## ") {
			// This is a section heading
			if i > 0 && len(result) > 0 && result[len(result)-1] != "" {
				// Add blank line before section
				result = append(result, "")
			}
			result = append(result, line)
			// Add blank line after section heading
			result = append(result, "")
		} else {
			result = append(result, line)
		}
	}

	return strings.Join(result, "\n")
}

// removeTrailingWhitespace removes trailing whitespace from all lines
func removeTrailingWhitespace(content string) string {
	lines := strings.Split(content, "\n")
	for i, line := range lines {
		lines[i] = trailingWhitespaceRegex.ReplaceAllString(line, "")
	}
	return strings.Join(lines, "\n")
}

// normalizeMultipleBlankLines collapses 3+ consecutive newlines to 2
func normalizeMultipleBlankLines(content string) string {
	return multipleBlankLinesRegex.ReplaceAllString(content, "\n\n")
}

// ensureFinalNewline ensures content ends with exactly one newline
func ensureFinalNewline(content string) string {
	content = strings.TrimRight(content, "\n")
	return content + "\n"
}
