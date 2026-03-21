package github

import (
	"encoding/json"
	"fmt"
	"os/exec"
	"strings"
)

type Client struct {
	Repo string
}

func NewClient(repo string) *Client {
	return &Client{Repo: repo}
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
