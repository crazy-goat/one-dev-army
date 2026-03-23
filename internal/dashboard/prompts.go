package dashboard

import (
	"fmt"
	"strings"
)

// Prompt templates for the Feature/Bug Creation Wizard
// These prompts are designed to work with LLMs to refine ideas and break them down into tasks

// RefinementPromptTemplate is the base template for idea refinement
// It instructs the LLM to analyze the idea in the context of the existing codebase
const RefinementPromptTemplate = `You are a GitHub issue writer. Your ONLY output is a markdown issue body. You NEVER explain, narrate, or think out loud.

RULES:
- Output ONLY the issue body in markdown. Nothing else.
- Do NOT start with "Now I", "Let me", "Here's", "Based on", "I'll", "After analyzing", or ANY preamble.
- Do NOT include phrases like "comprehensive understanding", "I have analyzed", "Let me create".
- First character of your response MUST be "#" (a markdown heading) or "-" (a list item).
- Output MUST be in English regardless of input language.

Codebase context (for your reference only, do NOT discuss it):
%s

Original %s:
%s

Write a professional GitHub issue body for this %s that covers:
1. %s
2. %s
3. %s
4. %s
5. %s
6. How it fits with existing codebase patterns

Format as a well-structured markdown %s suitable for a GitHub issue.`

// BreakdownPromptTemplate is the base template for task breakdown
// It instructs the LLM to break down a technical description into actionable tasks
const BreakdownPromptTemplate = `You are a technical project manager breaking down work into GitHub issues.

%s description:
%s

Break this down into 3-7 specific, actionable tasks. For each task provide:
- title: concise task title (max 80 chars)
- description: detailed description that MUST include clear acceptance criteria (what "done" looks like)
- priority: one of [low, medium, high, critical]
- complexity: one of [S, M, L, XL] (S=1-2 hours, M=half day, L=1-2 days, XL=3+ days)

CRITICAL CONSTRAINTS:
1. DO NOT include implementation details, code snippets, or specific technical solutions in the description
2. Focus on WHAT needs to be done and the acceptance criteria, NOT HOW to do it
3. The description should be understandable by any team member, not just developers
4. Each task description MUST end with a "Acceptance Criteria:" section listing 2-4 specific, verifiable criteria

Return ONLY a JSON array in this exact format:
[
  {
    "title": "Task title",
    "description": "Task description with clear acceptance criteria at the end.\n\nAcceptance Criteria:\n- Criterion 1\n- Criterion 2\n- Criterion 3",
    "priority": "high",
    "complexity": "M"
  }
]

No markdown, no explanation, just the JSON array.`

// BuildRefinementPrompt creates the prompt for idea refinement with codebase context
// wizardType: the type of wizard (feature or bug)
// idea: the original user idea
// codebaseContext: information about the existing codebase (file structure, key files, etc.)
func BuildRefinementPrompt(wizardType WizardType, idea string, codebaseContext string) string {
	if codebaseContext == "" {
		codebaseContext = "No codebase context provided."
	}

	if wizardType == WizardTypeBug {
		return fmt.Sprintf(RefinementPromptTemplate,
			codebaseContext,               // %s - codebase context
			"bug description",             // %s - original type
			idea,                          // %s - original content
			"bug report",                  // %s - output type
			"description of the issue",    // %s - point 1
			"Steps to reproduce",          // %s - point 2
			"Expected vs actual behavior", // %s - point 3
			"Impact/severity assessment",  // %s - point 4
			"Any additional context that would help developers", // %s - point 5
			"bug report", // %s - final output type
		)
	}

	return fmt.Sprintf(RefinementPromptTemplate,
		codebaseContext,              // %s - codebase context
		"idea",                       // %s - original type
		idea,                         // %s - original content
		"feature description",        // %s - output type
		"problem statement",          // %s - point 1
		"Target users/personas",      // %s - point 2
		"Proposed solution overview", // %s - point 3
		"Key acceptance criteria",    // %s - point 4
		"Technical considerations or constraints", // %s - point 5
		"feature description",                     // %s - final output type
	)
}

// BuildBreakdownPrompt creates the prompt for task breakdown
// wizardType: the type of wizard (feature or bug)
// description: the refined technical description
func BuildBreakdownPrompt(wizardType WizardType, description string) string {
	var typeLabel string
	if wizardType == WizardTypeBug {
		typeLabel = "Bug fix"
	} else {
		typeLabel = "Feature"
	}

	return fmt.Sprintf(BreakdownPromptTemplate, typeLabel, description)
}

// GetCodebaseContext gathers context about the existing codebase
// This is a placeholder implementation that can be enhanced to:
// - Read key configuration files (package.json, go.mod, etc.)
// - Get file structure from git
// - Analyze existing patterns in the codebase
// Returns a string summarizing the codebase context
func GetCodebaseContext() string {
	// For now, return a minimal context
	// In a full implementation, this could:
	// 1. Run git ls-files to get file structure
	// 2. Read key config files (go.mod, package.json, etc.)
	// 3. Sample existing code patterns
	// 4. Limit to top N files to avoid token limits

	var context strings.Builder
	context.WriteString("Project Structure:\n")
	context.WriteString("- Go-based backend service\n")
	context.WriteString("- Uses standard project layout (internal/, cmd/, etc.)\n")
	context.WriteString("- Dashboard package for web UI and wizard functionality\n")
	context.WriteString("- GitHub integration for issue management\n")
	context.WriteString("- LLM integration via OpenCode API\n")

	return context.String()
}

// stripLLMPreamble removes conversational preamble that LLMs sometimes prepend
// before the actual content. It looks for the first markdown heading or list item
// and discards everything before it.
func stripLLMPreamble(text string) string {
	text = strings.TrimSpace(text)
	if text == "" {
		return ""
	}

	// If it already starts with markdown content, return as-is
	if text[0] == '#' || text[0] == '-' || text[0] == '*' {
		return text
	}

	// Find the first markdown heading
	lines := strings.Split(text, "\n")
	for i, line := range lines {
		trimmed := strings.TrimSpace(line)
		if strings.HasPrefix(trimmed, "#") {
			return strings.TrimSpace(strings.Join(lines[i:], "\n"))
		}
	}

	// No heading found — return the whole thing (better than nothing)
	return text
}
