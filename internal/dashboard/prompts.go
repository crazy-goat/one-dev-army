package dashboard

import (
	"encoding/json"
	"fmt"
	"strings"
)

// GeneratedIssue is the JSON structure returned by the LLM for issue generation.
type GeneratedIssue struct {
	Title       string `json:"title"`
	Description string `json:"description"`
	Priority    string `json:"priority"`
	Complexity  string `json:"complexity"`
}

// PriorityLabel returns the GitHub label for the priority (e.g. "priority:high").
// Returns empty string if the priority is not recognized.
func (g GeneratedIssue) PriorityLabel() string {
	switch strings.ToLower(g.Priority) {
	case "high":
		return "priority:high"
	case "medium":
		return "priority:medium"
	case "low":
		return "priority:low"
	default:
		return ""
	}
}

// ComplexityLabel returns the GitHub label for the complexity (e.g. "size:M").
// Returns empty string if the complexity is not recognized.
func (g GeneratedIssue) ComplexityLabel() string {
	switch strings.ToUpper(g.Complexity) {
	case "S":
		return "size:S"
	case "M":
		return "size:M"
	case "L":
		return "size:L"
	case "XL":
		return "size:XL"
	default:
		return ""
	}
}

// GeneratedIssueSchema is the JSON schema for structured LLM output.
var GeneratedIssueSchema = json.RawMessage(`{
	"type": "object",
	"properties": {
		"title": {
			"type": "string",
			"description": "Concise GitHub issue title, 5-10 words, max 80 characters. Must start with [Feature] or [Bug] prefix."
		},
		"description": {
			"type": "string",
			"description": "GitHub issue body in markdown format with sections: Description, Tasks, Files to Modify, Acceptance Criteria."
		},
		"priority": {
			"type": "string",
			"enum": ["high", "medium", "low"],
			"description": "Priority based on business impact and urgency. high = critical/blocking, medium = important but not urgent, low = nice-to-have."
		},
		"complexity": {
			"type": "string",
			"enum": ["S", "M", "L", "XL"],
			"description": "Estimated implementation complexity. S = 1-2 hours, M = half day, L = 1-2 days, XL = 3+ days."
		}
	},
	"required": ["title", "description", "priority", "complexity"],
	"additionalProperties": false
}`)

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
- ALL output MUST be in English. Even if the user's input is in another language, translate and write everything in English. No exceptions.

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

// IssueGenerationPromptTemplate is the unified template for generating a complete GitHub issue
// with both title and description in a single LLM call using structured JSON output.
// NOTE: The language parameter is accepted but ignored — output is ALWAYS English.
// This is critical because GitHub issues must be in English for consistency.
const IssueGenerationPromptTemplate = `You are a GitHub issue generator. You produce a JSON object with "title" and "description" fields.

CRITICAL LANGUAGE RULE: ALL output MUST be in English. The title and description MUST be written entirely in English. Even if the user's request is in Polish, German, Chinese, or any other language — you MUST translate it and write the issue in English. No exceptions.

The "title" field:
- 5-10 words, maximum 80 characters
- Must start with [Feature] or [Bug] prefix based on issue type
- Written in English
- Scannable and descriptive

The "priority" field — assess based on business impact and urgency:
- "high" — critical functionality, blocking other work, security issue, or data loss risk
- "medium" — important improvement, affects users but has workarounds
- "low" — nice-to-have, cosmetic, minor improvement

The "complexity" field — estimate implementation effort:
- "S" — 1-2 hours, small change, single file, well-defined scope
- "M" — half day, a few files, moderate logic changes
- "L" — 1-2 days, multiple files/components, requires careful design
- "XL" — 3+ days, cross-cutting changes, significant new functionality

The "description" field is a markdown document with exactly these sections:

## Description
[1-3 sentences in English: what needs to be done and why]

## Tasks
[Numbered list of concrete implementation steps in English. Each step is one action a developer can complete in 2-15 minutes. Be specific about file paths.]

## Files to Modify
[List of file paths that need changes, with a brief note in English on what changes]

## Acceptance Criteria
[2-5 specific, verifiable criteria for completion, in English]

CRITICAL RULES:
- ALL text MUST be in English — title, description, tasks, criteria, everything
- NO implementation code, algorithms, or design patterns
- NO architecture overviews or component dependency analysis
- Focus on WHAT to do, not HOW
- Be specific about file paths
- Tasks should be actionable steps, not abstract descriptions
- Keep it concise — a developer should read this in under 2 minutes

Codebase context (for reference only):
%s

Issue type: %s

Original request:
%s`

// BuildRefinementPrompt creates the prompt for idea refinement with codebase context
// wizardType: the type of wizard (feature or bug)
// idea: the original user idea
// codebaseContext: information about the existing codebase (file structure, key files, etc.)
// language: accepted for API compatibility but ignored — output is always English
func BuildRefinementPrompt(wizardType WizardType, idea string, codebaseContext string, language string) string {
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

// BuildIssueGenerationPrompt creates the unified prompt for issue generation.
// This generates both title and description in a single LLM call using structured JSON output.
// The language parameter is accepted for API compatibility but ignored — output is always English.
func BuildIssueGenerationPrompt(wizardType WizardType, idea string, codebaseContext string, language string) string {
	if codebaseContext == "" {
		codebaseContext = "No codebase context provided."
	}

	var typeLabel string
	if wizardType == WizardTypeBug {
		typeLabel = "Bug"
	} else {
		typeLabel = "Feature"
	}

	return fmt.Sprintf(IssueGenerationPromptTemplate,
		codebaseContext,
		typeLabel,
		idea,
	)
}

// BuildTechnicalPlanningPrompt is an alias for backward compatibility.
func BuildTechnicalPlanningPrompt(wizardType WizardType, idea string, codebaseContext string, language string) string {
	return BuildIssueGenerationPrompt(wizardType, idea, codebaseContext, language)
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
