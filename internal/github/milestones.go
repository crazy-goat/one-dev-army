package github

import (
	"fmt"
	"time"
)

// EnsureMilestone checks if at least one milestone exists for the repository.
// If no milestones exist, it creates a default "Sprint 1" milestone with
// a due date 2 weeks from now.
// Returns true if a milestone was created, false if one already existed.
func (c *Client) EnsureMilestone() (bool, error) {
	milestones, err := c.ListMilestones()
	if err != nil {
		return false, fmt.Errorf("listing milestones: %w", err)
	}

	if len(milestones) > 0 {
		return false, nil
	}

	// No milestones exist, create a default one using gh api
	dueDate := time.Now().AddDate(0, 0, 14).Format("2006-01-02T15:04:05Z")
	_, err = c.ghNoRepo("api", "repos/"+c.Repo+"/milestones",
		"-f", "title=Sprint 1",
		"-f", "due_on="+dueDate)
	if err != nil {
		return false, fmt.Errorf("creating milestone: %w", err)
	}

	return true, nil
}
