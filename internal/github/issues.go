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
	_, err := c.ghNoRepo("api", "repos/"+c.Repo+"/milestones", "-f", "title="+title)
	if err != nil {
		return fmt.Errorf("creating milestone %s: %w", title, err)
	}
	return nil
}

func (c *Client) AddLabel(issueNum int, label string) error {
	_, err := c.gh("issue", "edit", strconv.Itoa(issueNum), "--add-label", label)
	if err != nil {
		return fmt.Errorf("adding label %s to #%d: %w", label, issueNum, err)
	}
	return nil
}

func (c *Client) RemoveLabel(issueNum int, label string) error {
	_, err := c.gh("issue", "edit", strconv.Itoa(issueNum), "--remove-label", label)
	if err != nil {
		return fmt.Errorf("removing label %s from #%d: %w", label, issueNum, err)
	}
	return nil
}

func (c *Client) CreatePR(branch, title, body string) (string, error) {
	out, err := c.gh("pr", "create", "--head", branch, "--title", title, "--body", body)
	if err != nil {
		return "", fmt.Errorf("creating PR for branch %s: %w", branch, err)
	}
	return strings.TrimSpace(string(out)), nil
}

func (c *Client) MergePR(branch string) error {
	_, err := c.gh("pr", "merge", branch, "--merge", "--delete-branch")
	if err != nil {
		return fmt.Errorf("merging PR for branch %s: %w", branch, err)
	}
	return nil
}

type Milestone struct {
	Title  string `json:"title"`
	Number int    `json:"number"`
	State  string `json:"state"`
}

func (c *Client) ListMilestones() ([]Milestone, error) {
	out, err := c.ghNoRepo("api", "repos/"+c.Repo+"/milestones", "--jq", ".[].title")
	if err != nil {
		var milestones []Milestone
		if err2 := c.ghJSON(&milestones, "api", "repos/"+c.Repo+"/milestones"); err2 != nil {
			return nil, fmt.Errorf("listing milestones: %w", err)
		}
		return milestones, nil
	}
	var milestones []Milestone
	for _, line := range strings.Split(strings.TrimSpace(string(out)), "\n") {
		if line != "" {
			milestones = append(milestones, Milestone{Title: line})
		}
	}
	return milestones, nil
}

func (c *Client) SetMilestone(issueNum int, milestone string) error {
	_, err := c.gh("issue", "edit", strconv.Itoa(issueNum), "--milestone", milestone)
	if err != nil {
		return fmt.Errorf("setting milestone %s on #%d: %w", milestone, issueNum, err)
	}
	return nil
}

type Comment struct {
	Body   string `json:"body"`
	Author struct {
		Login string `json:"login"`
	} `json:"author"`
}

func (c *Client) ListComments(issueNum int) ([]Comment, error) {
	var comments []Comment
	if err := c.ghJSON(&comments, "issue", "view", strconv.Itoa(issueNum), "--json", "comments", "--jq", ".comments"); err != nil {
		return nil, fmt.Errorf("listing comments for #%d: %w", issueNum, err)
	}
	return comments, nil
}

func (c *Client) ListOpenIssues() ([]Issue, error) {
	args := []string{"issue", "list", "--state", "open", "--json", "number,title,body,state,labels", "--limit", "500"}
	var issues []Issue
	if err := c.ghJSON(&issues, args...); err != nil {
		return nil, fmt.Errorf("listing open issues: %w", err)
	}
	return issues, nil
}

func parseJSON(data []byte, v interface{}) error {
	if err := json.Unmarshal(data, v); err != nil {
		return fmt.Errorf("parsing JSON: %w", err)
	}
	return nil
}
