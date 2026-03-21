package github

import (
	"encoding/json"
	"fmt"
	"strconv"
	"strings"
)

type Issue struct {
	Number int    `json:"number"`
	Title  string `json:"title"`
	Body   string `json:"body"`
	State  string `json:"state"`
	Labels []struct {
		Name string `json:"name"`
	} `json:"labels"`
}

func (c *Client) CreateIssue(title, body string, labels []string) (int, error) {
	args := []string{"issue", "create", "--title", title, "--body", body}
	for _, l := range labels {
		args = append(args, "--label", l)
	}
	out, err := c.gh(args...)
	if err != nil {
		return 0, fmt.Errorf("creating issue: %w", err)
	}

	url := strings.TrimSpace(string(out))
	parts := strings.Split(url, "/")
	numStr := parts[len(parts)-1]
	num, err := strconv.Atoi(numStr)
	if err != nil {
		return 0, fmt.Errorf("parsing issue number from %q: %w", url, err)
	}
	return num, nil
}

func (c *Client) ListIssues(milestone string) ([]Issue, error) {
	args := []string{"issue", "list", "--state", "all", "--json", "number,title,body,state,labels"}
	if milestone != "" {
		args = append(args, "--milestone", milestone)
	}
	var issues []Issue
	if err := c.ghJSON(&issues, args...); err != nil {
		return nil, fmt.Errorf("listing issues: %w", err)
	}
	return issues, nil
}

func (c *Client) AddComment(issueNum int, body string) error {
	_, err := c.gh("issue", "comment", strconv.Itoa(issueNum), "--body", body)
	if err != nil {
		return fmt.Errorf("adding comment to #%d: %w", issueNum, err)
	}
	return nil
}

func (c *Client) CreateMilestone(title string) error {
	_, err := c.gh("api", "repos/{owner}/{repo}/milestones", "-f", "title="+title)
	if err != nil {
		return fmt.Errorf("creating milestone %s: %w", title, err)
	}
	return nil
}

func parseJSON(data []byte, v interface{}) error {
	if err := json.Unmarshal(data, v); err != nil {
		return fmt.Errorf("parsing JSON: %w", err)
	}
	return nil
}
