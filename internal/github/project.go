package github

import (
	"fmt"
	"strings"
)

var ProjectColumns = []string{
	"Backlog",
	"In Progress",
	"Review",
	"Merging",
	"Done",
	"Blocked",
}

func (c *Client) EnsureProject(name string) (string, error) {
	owner := strings.Split(c.Repo, "/")[0]

	var projects []struct {
		ID    string `json:"id"`
		Title string `json:"title"`
	}
	err := c.ghJSON(&projects, "project", "list", "--owner", owner, "--format", "json")
	if err != nil {
		return "", fmt.Errorf("listing projects: %w", err)
	}

	for _, p := range projects {
		if p.Title == name {
			return p.ID, nil
		}
	}

	out, err := c.gh("project", "create", "--owner", owner, "--title", name, "--format", "json")
	if err != nil {
		return "", fmt.Errorf("creating project %s: %w", name, err)
	}

	var created struct {
		ID string `json:"id"`
	}
	if err := parseJSON(out, &created); err != nil {
		return "", err
	}
	return created.ID, nil
}
