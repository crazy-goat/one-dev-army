package plan

import (
	"context"
	"fmt"
	"log"
	"os"
	"regexp"
	"strconv"
	"strings"
	"time"

	"github.com/crazy-goat/one-dev-army/internal/git"
	"github.com/crazy-goat/one-dev-army/internal/github"
)

// AttachmentManager handles creating and attaching plan.md files to GitHub issues
type AttachmentManager struct {
	gh       *github.Client
	worktree *git.Worktree
}

// NewAttachmentManager creates a new AttachmentManager
func NewAttachmentManager(gh *github.Client, worktree *git.Worktree) *AttachmentManager {
	return &AttachmentManager{
		gh:       gh,
		worktree: worktree,
	}
}

// CreateAndAttach creates a plan.md file, commits it, pushes to the branch,
// and adds a comment to the GitHub issue with the plan URL.
// Returns the GitHub URL of the plan file.
func (am *AttachmentManager) CreateAndAttach(
	ctx context.Context,
	issueNum int,
	branch string,
	analysis string,
	planContent string,
) (string, error) {
	// Create initial plan with analysis
	plan := &Plan{
		IssueNumber: issueNum,
		Analysis:    analysis,
		CreatedAt:   time.Now(),
		UpdatedAt:   time.Now(),
	}

	// If we have implementation steps, parse them
	if planContent != "" {
		plan.ImplementationSteps = parseSteps(planContent)
	}

	// Save plan to file
	planPath := GetPlanFilePath(am.worktree.Path)
	if err := plan.SaveToFile(planPath); err != nil {
		return "", fmt.Errorf("saving plan to file: %w", err)
	}

	// Commit the plan file
	commitMsg := fmt.Sprintf("docs: add plan.md for issue #%d", issueNum)
	if err := am.commitFile(planPath, commitMsg); err != nil {
		return "", fmt.Errorf("committing plan file: %w", err)
	}

	// Push branch
	if err := am.pushBranch(branch); err != nil {
		return "", fmt.Errorf("pushing branch: %w", err)
	}

	// Generate GitHub URL
	planURL := fmt.Sprintf("https://github.com/%s/blob/%s/plan.md", am.gh.Repo, branch)
	plan.GitHubURL = planURL

	// Add comment to issue
	comment := fmt.Sprintf("📋 Implementation Plan: [plan.md](%s)", planURL)
	if err := am.gh.AddComment(issueNum, comment); err != nil {
		log.Printf("[AttachmentManager] Warning: failed to add plan comment to issue #%d: %v", issueNum, err)
		// Don't fail if comment addition fails
	}

	return planURL, nil
}

// GetFromIssue retrieves the plan from a GitHub issue by checking for plan.md
// in the associated branch. Returns nil if no plan is found.
func (am *AttachmentManager) GetFromIssue(
	ctx context.Context,
	issueNum int,
) (*Plan, error) {
	// Try to find the branch for this issue
	branch, err := am.gh.FindPRBranch(issueNum)
	if err != nil {
		// No PR/branch found, try to construct branch name
		branch = fmt.Sprintf("oda-%d-", issueNum)
	}

	// Check if plan.md exists in the worktree
	planPath := GetPlanFilePath(am.worktree.Path)
	if _, err := os.Stat(planPath); err != nil {
		return nil, fmt.Errorf("plan.md not found: %w", err)
	}

	// Parse the plan from file
	plan, err := ParseFromFile(planPath)
	if err != nil {
		return nil, fmt.Errorf("parsing plan from file: %w", err)
	}

	// Set the GitHub URL
	plan.GitHubURL = fmt.Sprintf("https://github.com/%s/blob/%s/plan.md", am.gh.Repo, branch)

	return plan, nil
}

// UpdateAttachment updates an existing plan.md file with new content,
// commits the changes, and pushes to the branch.
func (am *AttachmentManager) UpdateAttachment(
	ctx context.Context,
	issueNum int,
	branch string,
	plan *Plan,
) error {
	plan.UpdatedAt = time.Now()

	// Save updated plan
	planPath := GetPlanFilePath(am.worktree.Path)
	if err := plan.SaveToFile(planPath); err != nil {
		return fmt.Errorf("saving updated plan: %w", err)
	}

	// Commit the update
	commitMsg := fmt.Sprintf("docs: update plan.md for issue #%d", issueNum)
	if err := am.commitFile(planPath, commitMsg); err != nil {
		return fmt.Errorf("committing updated plan: %w", err)
	}

	// Push branch
	if err := am.pushBranch(branch); err != nil {
		return fmt.Errorf("pushing updated plan: %w", err)
	}

	return nil
}

// CreateInitialPlan creates an initial plan.md with just the analysis
func (am *AttachmentManager) CreateInitialPlan(
	issueNum int,
	branch string,
	analysis string,
) (string, error) {
	return am.CreateAndAttach(context.Background(), issueNum, branch, analysis, "")
}

// UpdatePlanWithImplementation updates the plan with implementation steps
func (am *AttachmentManager) UpdatePlanWithImplementation(
	issueNum int,
	branch string,
	analysis string,
	implementationPlan string,
) (string, error) {
	return am.CreateAndAttach(context.Background(), issueNum, branch, analysis, implementationPlan)
}

// CreateFullPlan creates a plan.md with both analysis and implementation in one call
func (am *AttachmentManager) CreateFullPlan(
	issueNum int,
	branch string,
	analysis string,
	implementationPlan string,
) (string, error) {
	return am.CreateAndAttach(context.Background(), issueNum, branch, analysis, implementationPlan)
}

// commitFile commits a file to the repository
func (am *AttachmentManager) commitFile(filePath, message string) error {
	// Add the file
	if _, err := git.RunInWorktree(am.worktree.Path, "git", "add", filePath); err != nil {
		return fmt.Errorf("adding file: %w", err)
	}

	// Check if there are changes to commit
	_, err := git.RunInWorktree(am.worktree.Path, "git", "diff", "--cached", "--quiet")
	if err == nil {
		// No changes to commit
		return nil
	}

	// Commit
	_, err = git.RunInWorktree(am.worktree.Path, "git", "commit", "-m", message)
	if err != nil {
		return fmt.Errorf("committing: %w", err)
	}

	return nil
}

// pushBranch pushes the current branch to origin
func (am *AttachmentManager) pushBranch(branch string) error {
	// Try force-with-lease first (safe for existing remote branches)
	_, err := git.RunInWorktree(am.worktree.Path, "git", "push", "-u", "--force-with-lease", "origin", branch)
	if err == nil {
		return nil
	}

	// If that fails, try regular push
	_, err = git.RunInWorktree(am.worktree.Path, "git", "push", "-u", "origin", branch)
	if err != nil {
		return fmt.Errorf("pushing branch: %w", err)
	}

	return nil
}

// parseSteps extracts implementation steps from plan content
func parseSteps(content string) []Step {
	var steps []Step
	lines := strings.Split(content, "\n")
	var currentStep *Step
	var inFilesSection bool

	for i, line := range lines {
		line = strings.TrimSpace(line)

		// Look for step headers (e.g., "1. ", "Step 1:", etc.)
		if matchesStepHeader(line) {
			// Save previous step if exists
			if currentStep != nil {
				steps = append(steps, *currentStep)
			}

			// Parse step number and description
			order, desc := extractStepInfo(line)
			currentStep = &Step{
				Order:       order,
				Description: desc,
			}
			inFilesSection = false
			continue
		}

		// Look for files section
		if currentStep != nil && strings.Contains(line, "Files:") {
			inFilesSection = true
			continue
		}

		// Extract files
		if inFilesSection && currentStep != nil {
			if strings.HasPrefix(line, "- ") || strings.HasPrefix(line, "* ") {
				file := strings.TrimPrefix(line, "- ")
				file = strings.TrimPrefix(file, "* ")
				file = strings.TrimSpace(file)
				if file != "" {
					currentStep.Files = append(currentStep.Files, file)
				}
			} else if line == "" {
				inFilesSection = false
			}
		}

		// Accumulate details
		if currentStep != nil && !inFilesSection && line != "" {
			if currentStep.Details != "" {
				currentStep.Details += "\n"
			}
			currentStep.Details += line
		}

		// Check for end of content or next major section
		if i < len(lines)-1 && strings.HasPrefix(lines[i+1], "## ") {
			if currentStep != nil {
				steps = append(steps, *currentStep)
				currentStep = nil
			}
		}
	}

	// Don't forget the last step
	if currentStep != nil {
		steps = append(steps, *currentStep)
	}

	return steps
}

// matchesStepHeader checks if a line is a step header
func matchesStepHeader(line string) bool {
	// Match patterns like "1. ", "Step 1:", "1) ", etc.
	patterns := []string{
		`^\d+\.\s+`,
		`^Step\s+\d+[:.]\s*`,
		`^\d+\)\s+`,
	}
	for _, pattern := range patterns {
		if matched, _ := regexp.MatchString(pattern, line); matched {
			return true
		}
	}
	return false
}

// extractStepInfo extracts step number and description from a header line
func extractStepInfo(line string) (int, string) {
	// Try to extract number
	re := regexp.MustCompile(`\d+`)
	match := re.FindString(line)
	order, _ := strconv.Atoi(match)

	// Extract description (everything after the number/prefix)
	desc := line
	// Remove common prefixes
	desc = regexp.MustCompile(`^\d+\.\s*`).ReplaceAllString(desc, "")
	desc = regexp.MustCompile(`^Step\s+\d+[:.]\s*`).ReplaceAllString(desc, "")
	desc = regexp.MustCompile(`^\d+\)\s*`).ReplaceAllString(desc, "")
	desc = strings.TrimSpace(desc)

	return order, desc
}

// FileExists checks if plan.md exists in the worktree
func (am *AttachmentManager) FileExists() bool {
	planPath := GetPlanFilePath(am.worktree.Path)
	_, err := os.Stat(planPath)
	return err == nil
}

// GetPlanPath returns the full path to plan.md
func (am *AttachmentManager) GetPlanPath() string {
	return GetPlanFilePath(am.worktree.Path)
}

// ReadPlan reads the current plan from disk if it exists
func (am *AttachmentManager) ReadPlan() (*Plan, error) {
	planPath := GetPlanFilePath(am.worktree.Path)
	return ParseFromFile(planPath)
}
