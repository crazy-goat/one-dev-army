package github

import (
	"encoding/json"
	"fmt"
	"sort"
	"strconv"
	"strings"
	"time"
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
	Title     string    `json:"title"`
	Number    int       `json:"number"`
	State     string    `json:"state"`
	CreatedAt time.Time `json:"created_at"`
	DueOn     time.Time `json:"due_on"`
}

// ListMilestones returns all milestones sorted by creation date (newest first)
func (c *Client) ListMilestones() ([]Milestone, error) {
	var milestones []Milestone
	if err := c.ghNoRepoJSON(&milestones, "api", "repos/"+c.Repo+"/milestones"); err != nil {
		return nil, fmt.Errorf("listing milestones: %w", err)
	}

	// Sort by creation date, newest first
	sort.Slice(milestones, func(i, j int) bool {
		return milestones[i].CreatedAt.After(milestones[j].CreatedAt)
	})

	return milestones, nil
}

// ListOpenMilestones returns only open (active) milestones sorted by due date
func (c *Client) ListOpenMilestones() ([]Milestone, error) {
	all, err := c.ListMilestones()
	if err != nil {
		return nil, err
	}

	var open []Milestone
	for _, m := range all {
		if m.State == "open" {
			open = append(open, m)
		}
	}

	// Sort by due date (closest first)
	sort.Slice(open, func(i, j int) bool {
		return open[i].DueOn.Before(open[j].DueOn)
	})

	return open, nil
}

// GetOldestOpenMilestone returns the oldest open milestone by creation date
// This is considered the "active" sprint
func (c *Client) GetOldestOpenMilestone() (*Milestone, error) {
	open, err := c.ListOpenMilestones()
	if err != nil {
		return nil, err
	}
	if len(open) == 0 {
		return nil, nil
	}
	// ListOpenMilestones returns sorted by DueOn, we need oldest by CreatedAt
	// So we need to find the one with earliest CreatedAt
	oldest := &open[0]
	for i := 1; i < len(open); i++ {
		if open[i].CreatedAt.Before(oldest.CreatedAt) {
			oldest = &open[i]
		}
	}
	return oldest, nil
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
