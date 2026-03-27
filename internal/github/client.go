package github

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"os/exec"
	"regexp"
	"strconv"
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
func (*Client) ghNoRepo(args ...string) ([]byte, error) {
	cmd := exec.Command("gh", args...)
	out, err := cmd.CombinedOutput()
	if err != nil {
		return nil, fmt.Errorf("gh %s: %w\n%s", strings.Join(args, " "), err, out)
	}
	return out, nil
}

func (c *Client) ghJSON(result any, args ...string) error {
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
func (c *Client) ghNoRepoJSON(result any, args ...string) error {
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
		return "", errors.New("detecting repo: empty result from gh")
	}
	return repo, nil
}

// GetToken retrieves the GitHub authentication token from gh CLI
func GetToken() (string, error) {
	cmd := exec.Command("gh", "auth", "token")
	out, err := cmd.CombinedOutput()
	if err != nil {
		return "", fmt.Errorf("getting token: %w\n%s", err, out)
	}
	token := strings.TrimSpace(string(out))
	if token == "" {
		return "", errors.New("getting token: empty result from gh")
	}
	return token, nil
}

// TagInfo represents information about a Git tag/release
type TagInfo struct {
	Tag  string `json:"tag"`
	Date string `json:"date,omitempty"`
	SHA  string `json:"sha,omitempty"`
}

// GetLastTag returns the most recent tag/release
func (c *Client) GetLastTag(ctx context.Context) (*TagInfo, error) {
	// Try to get the latest release first
	output, err := c.gh("api", "repos/"+c.Repo+"/releases/latest")
	if err == nil {
		var release struct {
			TagName string `json:"tag_name"`
			Created string `json:"created_at"`
		}
		if err := json.Unmarshal(output, &release); err == nil && release.TagName != "" {
			return &TagInfo{
				Tag:  release.TagName,
				Date: release.Created,
			}, nil
		}
	}

	// Fallback to tags if no releases
	output, err = c.gh("api", "repos/"+c.Repo+"/tags?per_page=1")
	if err != nil {
		return nil, fmt.Errorf("getting last tag: %w", err)
	}

	var tags []struct {
		Name   string `json:"name"`
		Commit struct {
			SHA string `json:"sha"`
			URL string `json:"url"`
		} `json:"commit"`
	}

	if err := json.Unmarshal(output, &tags); err != nil {
		return nil, fmt.Errorf("parsing tags: %w", err)
	}

	if len(tags) == 0 {
		return nil, nil
	}

	return &TagInfo{
		Tag: tags[0].Name,
		SHA: tags[0].Commit.SHA,
	}, nil
}

// LinkedIssue represents a linked issue relationship
type LinkedIssue struct {
	Number       int    `json:"number"`
	Relationship string `json:"relationship"` // blocked_by, blocks, relates_to
}

// GetLinkedIssues fetches linked issues using GitHub GraphQL API
// Falls back to parsing issue body if GraphQL not available
func (c *Client) GetLinkedIssues(ctx context.Context, issueNumber int) ([]LinkedIssue, error) {
	// Try GraphQL API first (GitHub Linked Issues)
	linked, err := c.getLinkedIssuesGraphQL(ctx, issueNumber)
	if err == nil {
		return linked, nil
	}

	// Fallback: parse issue body for #123 references
	return c.getLinkedIssuesFromBody(ctx, issueNumber)
}

func (c *Client) getLinkedIssuesGraphQL(ctx context.Context, issueNumber int) ([]LinkedIssue, error) {
	// Parse owner and repo from c.Repo
	parts := strings.Split(c.Repo, "/")
	if len(parts) != 2 {
		return nil, fmt.Errorf("invalid repo format: %s", c.Repo)
	}
	owner, repo := parts[0], parts[1]

	// GraphQL query for linked issues
	query := fmt.Sprintf(`{
		repository(owner: "%s", name: "%s") {
			issue(number: %d) {
				trackedIssues(first: 100) {
					nodes {
						number
					}
				}
				trackedInIssues(first: 100) {
					nodes {
						number
					}
				}
			}
		}
	}`, owner, repo, issueNumber)

	// Execute GraphQL query using gh api graphql
	output, err := c.gh("api", "graphql", "-f", "query="+query)
	if err != nil {
		return nil, err
	}

	// Parse response
	var result struct {
		Data struct {
			Repository struct {
				Issue struct {
					TrackedIssues struct {
						Nodes []struct {
							Number int `json:"number"`
						} `json:"nodes"`
					} `json:"trackedIssues"`
					TrackedInIssues struct {
						Nodes []struct {
							Number int `json:"number"`
						} `json:"nodes"`
					} `json:"trackedInIssues"`
				} `json:"issue"`
			} `json:"repository"`
		} `json:"data"`
	}

	if err := json.Unmarshal(output, &result); err != nil {
		return nil, err
	}

	var linked []LinkedIssue
	for _, node := range result.Data.Repository.Issue.TrackedIssues.Nodes {
		linked = append(linked, LinkedIssue{
			Number:       node.Number,
			Relationship: "blocks",
		})
	}
	for _, node := range result.Data.Repository.Issue.TrackedInIssues.Nodes {
		linked = append(linked, LinkedIssue{
			Number:       node.Number,
			Relationship: "blocked_by",
		})
	}

	return linked, nil
}

func (c *Client) getLinkedIssuesFromBody(ctx context.Context, issueNumber int) ([]LinkedIssue, error) {
	// Get issue details
	issue, err := c.GetIssue(issueNumber)
	if err != nil {
		return nil, err
	}

	// Parse body for #123 references
	re := regexp.MustCompile(`#(\d+)`)
	matches := re.FindAllStringSubmatch(issue.Body, -1)

	var linked []LinkedIssue
	seen := make(map[int]bool)

	for _, match := range matches {
		if len(match) > 1 {
			num, _ := strconv.Atoi(match[1])
			if num > 0 && num != issueNumber && !seen[num] {
				seen[num] = true
				linked = append(linked, LinkedIssue{
					Number:       num,
					Relationship: "relates_to",
				})
			}
		}
	}

	return linked, nil
}
