package plan

import (
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"time"
)

// Plan represents an implementation plan for a GitHub issue
type Plan struct {
	IssueNumber         int
	Analysis            string
	ImplementationSteps []Step
	TestPlan            []string
	CreatedAt           time.Time
	UpdatedAt           time.Time
	FilePath            string
	GitHubURL           string
}

// Step represents a single implementation step
type Step struct {
	Order       int
	Description string
	Files       []string
	Details     string
}

// ToMarkdown converts the plan to markdown format
func (p *Plan) ToMarkdown() string {
	var sb strings.Builder

	sb.WriteString(fmt.Sprintf("# Implementation Plan for Issue #%d\n\n", p.IssueNumber))
	sb.WriteString(fmt.Sprintf("**Created:** %s\n", p.CreatedAt.Format(time.RFC3339)))
	if !p.UpdatedAt.IsZero() && p.UpdatedAt != p.CreatedAt {
		sb.WriteString(fmt.Sprintf("**Updated:** %s\n", p.UpdatedAt.Format(time.RFC3339)))
	}
	sb.WriteString("\n")

	if p.Analysis != "" {
		sb.WriteString("## Analysis\n\n")
		sb.WriteString(p.Analysis)
		sb.WriteString("\n\n")
	}

	if len(p.ImplementationSteps) > 0 {
		sb.WriteString("## Implementation Steps\n\n")
		for _, step := range p.ImplementationSteps {
			sb.WriteString(fmt.Sprintf("### Step %d: %s\n\n", step.Order, step.Description))
			if len(step.Files) > 0 {
				sb.WriteString("**Files:**\n")
				for _, file := range step.Files {
					sb.WriteString(fmt.Sprintf("- `%s`\n", file))
				}
				sb.WriteString("\n")
			}
			if step.Details != "" {
				sb.WriteString(step.Details)
				sb.WriteString("\n\n")
			}
		}
	}

	if len(p.TestPlan) > 0 {
		sb.WriteString("## Test Plan\n\n")
		for _, test := range p.TestPlan {
			sb.WriteString(fmt.Sprintf("- [ ] %s\n", test))
		}
		sb.WriteString("\n")
	}

	return sb.String()
}

// SaveToFile saves the plan to a file
func (p *Plan) SaveToFile(path string) error {
	content := p.ToMarkdown()
	if err := os.WriteFile(path, []byte(content), 0644); err != nil {
		return fmt.Errorf("writing plan to file: %w", err)
	}
	p.FilePath = path
	return nil
}

// ParseFromMarkdown parses a plan from markdown content
func ParseFromMarkdown(content string) (*Plan, error) {
	plan := &Plan{}

	// Extract issue number from title
	titleRe := regexp.MustCompile(`# Implementation Plan for Issue #(\d+)`)
	if matches := titleRe.FindStringSubmatch(content); len(matches) > 1 {
		fmt.Sscanf(matches[1], "%d", &plan.IssueNumber)
	}

	// Extract created date
	createdRe := regexp.MustCompile(`\*\*Created:\*\* ([^\n]+)`)
	if matches := createdRe.FindStringSubmatch(content); len(matches) > 1 {
		if t, err := time.Parse(time.RFC3339, matches[1]); err == nil {
			plan.CreatedAt = t
		}
	}

	// Extract updated date
	updatedRe := regexp.MustCompile(`\*\*Updated:\*\* ([^\n]+)`)
	if matches := updatedRe.FindStringSubmatch(content); len(matches) > 1 {
		if t, err := time.Parse(time.RFC3339, matches[1]); err == nil {
			plan.UpdatedAt = t
		}
	}

	// Extract analysis section
	analysisRe := regexp.MustCompile(`(?s)## Analysis\n\n(.+?)(?:\n\n## |\z)`)
	if matches := analysisRe.FindStringSubmatch(content); len(matches) > 1 {
		plan.Analysis = strings.TrimSpace(matches[1])
	}

	// Extract implementation steps - find all step sections
	stepHeaderRe := regexp.MustCompile(`(?m)^### Step (\d+): ([^\n]+)$`)
	stepMatches := stepHeaderRe.FindAllStringSubmatchIndex(content, -1)

	for i, match := range stepMatches {
		if len(match) >= 6 {
			orderStr := content[match[2]:match[3]]
			desc := content[match[4]:match[5]]

			var order int
			fmt.Sscanf(orderStr, "%d", &order)

			step := Step{
				Order:       order,
				Description: desc,
			}

			// Find the content of this step (from end of header to next step or end)
			startIdx := match[1]
			endIdx := len(content)
			if i < len(stepMatches)-1 {
				endIdx = stepMatches[i+1][0]
			}
			stepContent := content[startIdx:endIdx]

			// Extract files from step content
			filesRe := regexp.MustCompile(`(?m)^\*\*Files:\*\*\n((?:- [^\n]+\n?)+)`)
			if fileMatches := filesRe.FindStringSubmatch(stepContent); len(fileMatches) > 1 {
				fileLines := strings.Split(fileMatches[1], "\n")
				for _, line := range fileLines {
					line = strings.TrimSpace(line)
					if strings.HasPrefix(line, "- `") && strings.HasSuffix(line, "`") {
						file := line[3 : len(line)-1]
						step.Files = append(step.Files, file)
					}
				}
			}

			// Extract details (everything after files section or after header)
			detailsRe := regexp.MustCompile(`(?s)(?:\*\*Files:\*\*\n(?:- [^\n]+\n?)+)?\n*(.+)`)
			if detailsMatches := detailsRe.FindStringSubmatch(stepContent); len(detailsMatches) > 1 {
				step.Details = strings.TrimSpace(detailsMatches[1])
			}

			plan.ImplementationSteps = append(plan.ImplementationSteps, step)
		}
	}

	// Extract test plan - find the section and extract items
	testSectionRe := regexp.MustCompile(`(?s)## Test Plan\n\n(.+?)(?:\n\n## |\z)`)
	if matches := testSectionRe.FindStringSubmatch(content); len(matches) > 1 {
		testLines := strings.Split(matches[1], "\n")
		for _, line := range testLines {
			line = strings.TrimSpace(line)
			if strings.HasPrefix(line, "- [ ] ") {
				plan.TestPlan = append(plan.TestPlan, line[6:])
			} else if strings.HasPrefix(line, "- ") {
				plan.TestPlan = append(plan.TestPlan, line[2:])
			}
		}
	}

	return plan, nil
}

// ParseFromFile parses a plan from a file
func ParseFromFile(path string) (*Plan, error) {
	content, err := os.ReadFile(path)
	if err != nil {
		return nil, fmt.Errorf("reading plan file: %w", err)
	}
	plan, err := ParseFromMarkdown(string(content))
	if err != nil {
		return nil, err
	}
	plan.FilePath = path
	return plan, nil
}

// GetPlanFilePath returns the default plan.md file path for a repository
func GetPlanFilePath(repoDir string) string {
	return filepath.Join(repoDir, "plan.md")
}
