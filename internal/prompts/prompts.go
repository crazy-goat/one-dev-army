// Package prompts provides access to LLM prompts embedded in the binary.
// All prompts are stored as separate .md files and embedded using go:embed.
package prompts

import (
	"embed"
	"fmt"
)

//go:embed mvp/*.md worker/*.md dashboard/*.md scheduler/*.md
var promptFiles embed.FS

// Prompt names for each package
const (
	// MVP prompts
	MVPPickTicket        = "mvp/pick_ticket.md"
	MVPTechnicalPlanning = "mvp/technical_planning.md"
	MVPCodeReview        = "mvp/code_review.md"
	MVPFixFromReview     = "mvp/fix_from_review.md"
	MVPImplementation    = "mvp/implementation.md"

	// Worker prompts
	WorkerAutomatedPipeline = "worker/automated_pipeline_notice.md"
	WorkerAnalysis          = "worker/analysis.md"
	WorkerCoding            = "worker/coding.md"
	WorkerCodeReview        = "worker/code_review.md"

	// Dashboard prompts
	DashboardRefinement      = "dashboard/refinement.md"
	DashboardBreakdown       = "dashboard/breakdown.md"
	DashboardIssueGeneration = "dashboard/issue_generation.md"

	// Scheduler prompts (templates with placeholders)
	SchedulerSprintPlanning  = "scheduler/sprint_planning.md"
	SchedulerInsightAnalysis = "scheduler/insight_analysis.md"
)

// Get returns the content of a prompt file.
// Returns error if the prompt file is not found.
func Get(name string) (string, error) {
	content, err := promptFiles.ReadFile(name)
	if err != nil {
		return "", fmt.Errorf("failed to read prompt %s: %w", name, err)
	}
	return string(content), nil
}

// MustGet returns the content of a prompt file.
// Panics if the prompt file is not found (use only for compile-time known prompts).
func MustGet(name string) string {
	content, err := Get(name)
	if err != nil {
		panic(err)
	}
	return content
}

// SprintPlanningPrompt builds the sprint planning prompt with the given issues and max tasks.
func SprintPlanningPrompt(issues string, maxTasks int) string {
	template := MustGet(SchedulerSprintPlanning)
	prompt := fmt.Sprintf(template, issues)
	if maxTasks > 0 {
		prompt += fmt.Sprintf("\nSelect up to %d tasks for this sprint.", maxTasks)
	}
	return prompt
}

// InsightAnalysisPrompt builds the insight analysis prompt with the given insights.
func InsightAnalysisPrompt(insights string) string {
	template := MustGet(SchedulerInsightAnalysis)
	return fmt.Sprintf(template, insights)
}
