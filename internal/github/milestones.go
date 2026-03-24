package github

import (
	"fmt"
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
