package github

import (
	"fmt"
	"regexp"
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

// CreateNextSprint creates a new sprint with a sequential name based on the current milestone.
// It extracts the sprint number from the current milestone title (e.g., "Sprint 5" -> 5),
// increments it by 1, and creates a new milestone with the next number.
// The new sprint has a due date 2 weeks from now.
// Returns the title of the created milestone and an error if creation fails.
func (c *Client) CreateNextSprint(currentMilestoneTitle string) (string, error) {
	// Extract sprint number from current milestone title using regex
	// Matches patterns like "Sprint 5", "Sprint 12", etc.
	re := regexp.MustCompile(`Sprint\s+(\d+)`)
	matches := re.FindStringSubmatch(currentMilestoneTitle)

	var nextNumber int
	if len(matches) >= 2 {
		// Found a sprint number, increment it
		currentNum, err := strconv.Atoi(matches[1])
		if err != nil {
			// Fallback: if parsing fails, use timestamp-based naming
			now := time.Now()
			title := "Sprint " + now.Format("2006-01-02 15:04")
			dueDate := now.AddDate(0, 0, 14).Format("2006-01-02T15:04:05Z")

			_, err = c.ghNoRepo("api", "repos/"+c.Repo+"/milestones",
				"-f", "title="+title,
				"-f", "due_on="+dueDate)
			if err != nil {
				return "", fmt.Errorf("creating fallback milestone: %w", err)
			}
			return title, nil
		}
		nextNumber = currentNum + 1
	} else {
		// No sprint number found in title, start from 1
		nextNumber = 1
	}

	// Create the new milestone with sequential name
	title := fmt.Sprintf("Sprint %d", nextNumber)
	now := time.Now()
	dueDate := now.AddDate(0, 0, 14).Format("2006-01-02T15:04:05Z")

	_, err := c.ghNoRepo("api", "repos/"+c.Repo+"/milestones",
		"-f", "title="+title,
		"-f", "due_on="+dueDate)
	if err != nil {
		return "", fmt.Errorf("creating milestone %s: %w", title, err)
	}

	return title, nil
}
