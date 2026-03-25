package dashboard

import (
	"encoding/json"
	"fmt"
	"strings"

	"github.com/crazy-goat/one-dev-army/internal/prompts"
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
// The actual templates are stored in internal/prompts/dashboard/

// BuildRefinementPrompt creates the prompt for idea refinement with codebase context
// wizardType: the type of wizard (feature or bug)
// idea: the original user idea
// codebaseContext: information about the existing codebase (file structure, key files, etc.)
// language: accepted for API compatibility but ignored — output is always English
func BuildRefinementPrompt(wizardType WizardType, idea string, codebaseContext string, _ string) string {
	if codebaseContext == "" {
		codebaseContext = "No codebase context provided."
	}

	if wizardType == WizardTypeBug {
		return fmt.Sprintf(prompts.MustGet(prompts.DashboardRefinement),
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

	return fmt.Sprintf(prompts.MustGet(prompts.DashboardRefinement),
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

	return fmt.Sprintf(prompts.MustGet(prompts.DashboardBreakdown), typeLabel, description)
}

// BuildIssueGenerationPrompt creates the unified prompt for issue generation.
// This generates both title and description in a single LLM call using structured JSON output.
// The language parameter is accepted for API compatibility but ignored — output is always English.
func BuildIssueGenerationPrompt(wizardType WizardType, idea string, codebaseContext string, _ string) string {
	if codebaseContext == "" {
		codebaseContext = "No codebase context provided."
	}

	var typeLabel string
	if wizardType == WizardTypeBug {
		typeLabel = "Bug"
	} else {
		typeLabel = "Feature"
	}

	return fmt.Sprintf(prompts.MustGet(prompts.DashboardIssueGeneration),
		codebaseContext,
		typeLabel,
		idea,
	)
}

// BuildTechnicalPlanningPrompt is an alias for backward compatibility.
func BuildTechnicalPlanningPrompt(wizardType WizardType, idea string, codebaseContext string, language string) string {
	return BuildIssueGenerationPrompt(wizardType, idea, codebaseContext, language)
}

// ReleaseNotes represents the structured output for release notes generation
type ReleaseNotes struct {
	Title       string `json:"title"`
	Description string `json:"description"`
}

// ReleaseNotesSchema is the JSON schema for structured LLM output for release notes
var ReleaseNotesSchema = json.RawMessage(`{
	"type": "object",
	"properties": {
		"title": {
			"type": "string",
			"description": "Concise release title, 5-15 words, summarizing the main theme of this release"
		},
		"description": {
			"type": "string",
			"description": "Release notes in markdown format. Include sections: What's New, Bug Fixes, Improvements, and Breaking Changes (if any). Group related items together."
		}
	},
	"required": ["title", "description"],
	"additionalProperties": false
}`)

// BuildReleaseNotesPrompt creates the prompt for generating release notes
// milestoneTitle: the title of the milestone being closed
// version: the new version tag (e.g., "v1.2.3")
// closedIssues: list of closed issues with their titles and numbers
func BuildReleaseNotesPrompt(milestoneTitle, version string, closedIssues []string) string {
	issuesList := "No closed issues."
	if len(closedIssues) > 0 {
		issuesList = strings.Join(closedIssues, "\n")
	}

	return fmt.Sprintf(`Generate release notes for version %s (milestone: %s).

Closed Issues:
%s

Based on these closed issues, create:
1. A concise, engaging release title that captures the main theme
2. Well-organized release notes in markdown format with these sections:
   - What's New (features and enhancements)
   - Bug Fixes
   - Improvements (performance, refactoring, etc.)
   - Breaking Changes (if any issues indicate breaking changes)

If there are no closed issues, create a generic release note about routine maintenance and improvements.

Output must be valid JSON matching the provided schema.`, version, milestoneTitle, issuesList)
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
