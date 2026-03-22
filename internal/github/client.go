package github

import (
	"encoding/json"
	"fmt"
	"os/exec"
	"strings"
)

type Client struct {
	Repo            string
	ActiveMilestone *Milestone
	ProjectID       string
	StatusFieldID   string
	StatusOptionIDs map[string]string
}

func NewClient(repo string) *Client {
	return &Client{Repo: repo}
}

// SetActiveMilestone sets the currently active milestone (oldest open sprint)
func (c *Client) SetActiveMilestone(m *Milestone) {
	c.ActiveMilestone = m
}

// GetActiveMilestone returns the currently active milestone or nil if none set
func (c *Client) GetActiveMilestone() *Milestone {
	return c.ActiveMilestone
}

func (c *Client) gh(args ...string) ([]byte, error) {
	fullArgs := append([]string{"-R", c.Repo}, args...)
	cmd := exec.Command("gh", fullArgs...)
	out, err := cmd.CombinedOutput()
	if err != nil {
		return nil, fmt.Errorf("gh %s: %w\n%s", strings.Join(args, " "), err, out)
	}
	return out, nil
}

// ghNoRepo runs gh without the -R flag, for commands that don't support it (e.g. gh project).
func (c *Client) ghNoRepo(args ...string) ([]byte, error) {
	cmd := exec.Command("gh", args...)
	out, err := cmd.CombinedOutput()
	if err != nil {
		return nil, fmt.Errorf("gh %s: %w\n%s", strings.Join(args, " "), err, out)
	}
	return out, nil
}

func (c *Client) ghJSON(result interface{}, args ...string) error {
	out, err := c.gh(args...)
	if err != nil {
		return err
	}
	if err := json.Unmarshal(out, result); err != nil {
		return fmt.Errorf("parsing gh JSON output: %w", err)
	}
	return nil
}

// ghNoRepoJSON runs gh without -R flag and parses JSON output
func (c *Client) ghNoRepoJSON(result interface{}, args ...string) error {
	out, err := c.ghNoRepo(args...)
	if err != nil {
		return err
	}
	if err := json.Unmarshal(out, result); err != nil {
		return fmt.Errorf("parsing gh JSON output: %w", err)
	}
	return nil
}

func DetectRepo() (string, error) {
	cmd := exec.Command("gh", "repo", "view", "--json", "nameWithOwner", "-q", ".nameWithOwner")
	out, err := cmd.CombinedOutput()
	if err != nil {
		return "", fmt.Errorf("detecting repo: %w\n%s", err, out)
	}
	repo := strings.TrimSpace(string(out))
	if repo == "" {
		return "", fmt.Errorf("detecting repo: empty result from gh")
	}
	return repo, nil
}
