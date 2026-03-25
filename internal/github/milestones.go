package github

import (
	"encoding/json"
	"fmt"
	"log"
	"strconv"
	"time"
)

// EnsureMilestone checks if at least one open milestone exists for the repository.
// If no open milestones exist, it creates a new one with a timestamp-based name
// (e.g. "Sprint 2026-03-23 14:35") and a due date 2 weeks from now.
// Returns the title of the created milestone (empty string if one already existed) and an error.
func (c *Client) EnsureMilestone() (string, error) {
	milestones, err := c.ListMilestones()
	if err != nil {
		return "", fmt.Errorf("listing milestones: %w", err)
	}

	if len(milestones) > 0 {
		return "", nil
	}

	// No open milestones — create a new one with a timestamp-based title
	now := time.Now()
	title := "Sprint " + now.Format("2006-01-02 15:04")
	dueDate := now.AddDate(0, 0, 14).Format("2006-01-02T15:04:05Z")

	_, err = c.ghNoRepo("api", "repos/"+c.Repo+"/milestones",
		"-f", "title="+title,
		"-f", "due_on="+dueDate)
	if err != nil {
		return "", fmt.Errorf("creating milestone: %w", err)
	}

	return title, nil
}

// CreateNextSprint creates a new sprint with a timestamp-based name.
// Uses the same naming convention as EnsureMilestone (e.g. "Sprint 2026-03-24 08:15")
// with a due date 2 weeks from now.
// Returns the title of the created milestone and an error if creation fails.
func (c *Client) CreateNextSprint(_ string) (string, error) {
	now := time.Now()
	title := "Sprint " + now.Format("2006-01-02 15:04")
	dueDate := now.AddDate(0, 0, 14).Format("2006-01-02T15:04:05Z")

	_, err := c.ghNoRepo("api", "repos/"+c.Repo+"/milestones",
		"-f", "title="+title,
		"-f", "due_on="+dueDate)
	if err != nil {
		return "", fmt.Errorf("creating milestone %s: %w", title, err)
	}

	return title, nil
}

// GetClosedIssuesForMilestone returns closed issues assigned to a specific milestone
// that were actually completed (merged via PR), not just manually closed
func (c *Client) GetClosedIssuesForMilestone(milestoneNumber int) ([]Issue, error) {
	// Use gh CLI to list closed issues for the milestone
	// We need to fetch more fields to determine if issue was merged or just closed
	out, err := c.gh("issue", "list",
		"--milestone", strconv.Itoa(milestoneNumber),
		"--state", "closed",
		"--json", "number,title,state,labels,updatedAt")
	if err != nil {
		return nil, fmt.Errorf("listing closed issues for milestone %d: %w", milestoneNumber, err)
	}

	var issues []Issue
	if err := json.Unmarshal(out, &issues); err != nil {
		return nil, fmt.Errorf("parsing closed issues for milestone %d: %w", milestoneNumber, err)
	}

	// Filter only issues that were actually merged (not just manually closed)
	var mergedIssues []Issue
	for _, issue := range issues {
		// Check if this issue was closed via merged PR
		isMerged, _, err := c.GetIssuePRStatus(issue.Number)
		if err != nil {
			// If we can't determine status, include it anyway (better to include than miss)
			log.Printf("[GitHub] Warning: could not determine PR status for issue #%d: %v", issue.Number, err)
			mergedIssues = append(mergedIssues, issue)
			continue
		}

		if isMerged {
			mergedIssues = append(mergedIssues, issue)
			log.Printf("[GitHub] Issue #%d was merged, including in release notes", issue.Number)
		} else {
			log.Printf("[GitHub] Issue #%d was closed but not merged, skipping", issue.Number)
		}
	}

	return mergedIssues, nil
}
