package github

import (
	"encoding/json"
	"fmt"
	"log"
	"regexp"
	"sort"
	"strconv"
	"strings"
	"time"
)

type Issue struct {
	Number    int    `json:"number"`
	Title     string `json:"title"`
	Body      string `json:"body"`
	State     string `json:"state"`
	Assignees []struct {
		Login string `json:"login"`
	} `json:"assignees"`
	Labels []struct {
		Name string `json:"name"`
	} `json:"labels"`
	// PR merge status fields for distinguishing merged vs manually closed issues
	PRMerged  bool       `json:"pr_merged,omitempty"`
	MergedAt  *time.Time `json:"merged_at,omitempty"`
	UpdatedAt *time.Time `json:"updatedAt,omitempty"`
}

// GetAssignee returns the first assignee's login or empty string if unassigned
func (i Issue) GetAssignee() string {
	if len(i.Assignees) > 0 {
		return i.Assignees[0].Login
	}
	return ""
}

// GetLabelNames returns a slice of label names
func (i Issue) GetLabelNames() []string {
	var names []string
	for _, l := range i.Labels {
		names = append(names, l.Name)
	}
	return names
}

func (c *Client) GetIssue(number int) (*Issue, error) {
	out, err := c.gh("issue", "view", strconv.Itoa(number), "--json", "number,title,body,state,labels,assignees")
	if err != nil {
		return nil, fmt.Errorf("getting issue #%d: %w", number, err)
	}
	var issue Issue
	if err := json.Unmarshal(out, &issue); err != nil {
		return nil, fmt.Errorf("parsing issue #%d: %w", number, err)
	}
	return &issue, nil
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

// CreateIssueWithMilestone creates a new issue with labels and assigns it to a milestone
func (c *Client) CreateIssueWithMilestone(title, body string, labels []string, milestone string) (int, error) {
	args := []string{"issue", "create", "--title", title, "--body", body}

	for _, l := range labels {
		args = append(args, "--label", l)
	}

	if milestone != "" {
		args = append(args, "--milestone", milestone)
	}

	out, err := c.gh(args...)
	if err != nil {
		return 0, fmt.Errorf("creating issue with milestone: %w", err)
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
	args := []string{"issue", "list", "--state", "all", "--json", "number,title,body,state,labels,updatedAt"}
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

// AddLabel adds a non-stage label to an issue.
// Stage labels (stage:*) are rejected — use SetStageLabel instead.
func (c *Client) AddLabel(issueNum int, label string) error {
	if IsStageLabel(label) {
		return fmt.Errorf("cannot add stage label %q via AddLabel — use SetStageLabel instead", label)
	}
	return c.addLabelRaw(issueNum, label)
}

// addLabelRaw adds any label without restrictions. Internal use only.
func (c *Client) addLabelRaw(issueNum int, label string) error {
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

func (c *Client) ClosePR(branch string) error {
	_, err := c.gh("pr", "close", branch, "--delete-branch")
	if err != nil {
		return fmt.Errorf("closing PR for branch %s: %w", branch, err)
	}
	return nil
}

type PRCheck struct {
	Name       string `json:"name"`
	Status     string `json:"status"`
	Conclusion string `json:"conclusion"`
	DetailsURL string `json:"detailsUrl"`
}

type PRChecksResult struct {
	Status         string
	Logs           string
	TotalCount     int
	CompletedCount int
	PendingChecks  []string
}

// runIDPattern extracts the run ID from a GitHub Actions URL like
// https://github.com/OWNER/REPO/actions/runs/12345678/job/98765432
var runIDPattern = regexp.MustCompile(`/actions/runs/(\d+)`)

func (c *Client) GetPRChecks(branch string) (*PRChecksResult, error) {
	var result struct {
		StatusCheckRollup []PRCheck `json:"statusCheckRollup"`
	}
	err := c.ghJSON(&result, "pr", "view", branch, "--json", "statusCheckRollup")
	if err != nil {
		return nil, fmt.Errorf("getting PR checks for %s: %w", branch, err)
	}

	checks := result.StatusCheckRollup
	if len(checks) == 0 {
		return &PRChecksResult{Status: "pending"}, nil
	}

	var failedNames []string
	var failedLinks []string
	var pendingNames []string
	allComplete := true
	completedCount := 0

	for _, check := range checks {
		if check.Status == "COMPLETED" {
			completedCount++
		} else {
			allComplete = false
			pendingNames = append(pendingNames, check.Name)
		}
		if check.Conclusion == "FAILURE" {
			failedNames = append(failedNames, check.Name)
			failedLinks = append(failedLinks, check.DetailsURL)
		}
	}

	totalCount := len(checks)

	if !allComplete {
		return &PRChecksResult{
			Status:         "pending",
			TotalCount:     totalCount,
			CompletedCount: completedCount,
			PendingChecks:  pendingNames,
		}, nil
	}

	if len(failedNames) > 0 {
		logs := c.buildFailureLogs(failedNames, failedLinks)
		return &PRChecksResult{Status: "fail", Logs: logs}, nil
	}

	return &PRChecksResult{Status: "pass"}, nil
}

// buildFailureLogs fetches actual CI logs for failed checks.
// It deduplicates run IDs (multiple jobs can share one workflow run)
// and calls `gh run view <id> --log-failed` for each unique run.
// Falls back to just listing check names if log fetching fails.
func (c *Client) buildFailureLogs(failedNames, failedLinks []string) string {
	// Collect unique run IDs from links
	seen := make(map[string]bool)
	var runIDs []string
	for _, link := range failedLinks {
		if id := extractRunID(link); id != "" && !seen[id] {
			seen[id] = true
			runIDs = append(runIDs, id)
		}
	}

	var sb strings.Builder
	sb.WriteString("## Failed CI Checks\n\n")
	sb.WriteString("Failed: ")
	sb.WriteString(strings.Join(failedNames, ", "))
	sb.WriteString("\n\n")

	if len(runIDs) == 0 {
		// No run IDs extracted — fall back to names only
		return sb.String()
	}

	for _, runID := range runIDs {
		runLogs := c.getFailedRunLogs(runID)
		if runLogs != "" {
			sb.WriteString("### Run ")
			sb.WriteString(runID)
			sb.WriteString("\n\n```\n")
			sb.WriteString(runLogs)
			sb.WriteString("\n```\n\n")
		}
	}

	return sb.String()
}

// extractRunID parses a GitHub Actions run ID from a check link URL.
// Returns empty string if the link doesn't match the expected pattern.
func extractRunID(link string) string {
	matches := runIDPattern.FindStringSubmatch(link)
	if len(matches) < 2 {
		return ""
	}
	return matches[1]
}

// getFailedRunLogs fetches the failed step logs for a workflow run.
// Returns empty string on any error (graceful degradation).
func (c *Client) getFailedRunLogs(runID string) string {
	out, err := c.gh("run", "view", runID, "--log-failed")
	if err != nil {
		log.Printf("[GitHub] Warning: failed to fetch logs for run %s: %v", runID, err)
		return ""
	}
	return strings.TrimSpace(string(out))
}

func (c *Client) FindPRBranch(issueNumber int) (string, error) {
	out, err := c.gh("pr", "list", "--json", "headRefName,number",
		"-q", fmt.Sprintf(".[] | select(.headRefName | startswith(\"oda-%d-\")) | .headRefName", issueNumber))
	if err != nil {
		return "", fmt.Errorf("finding PR branch for issue #%d: %w", issueNumber, err)
	}
	branch := strings.TrimSpace(string(out))
	if branch == "" {
		return "", fmt.Errorf("no open PR found for issue #%d", issueNumber)
	}
	return branch, nil
}

type Milestone struct {
	Title     string    `json:"title"`
	Number    int       `json:"number"`
	State     string    `json:"state"`
	CreatedAt time.Time `json:"created_at"`
	DueOn     time.Time `json:"due_on"`
}

// ListMilestones returns open milestones sorted by creation date (newest first).
// GitHub API defaults to state=open which is the desired behavior — EnsureMilestone
// only needs to know if there's an active sprint to work with.
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

func (c *Client) CloseIssue(issueNum int) error {
	_, err := c.gh("issue", "close", strconv.Itoa(issueNum))
	if err != nil {
		return fmt.Errorf("closing issue #%d: %w", issueNum, err)
	}
	return nil
}

// UpdateIssueBody updates the body of an existing issue
func (c *Client) UpdateIssueBody(issueNum int, body string) error {
	_, err := c.gh("issue", "edit", strconv.Itoa(issueNum), "--body", body)
	if err != nil {
		return fmt.Errorf("updating body of issue #%d: %w", issueNum, err)
	}
	return nil
}

func (c *Client) ListOpenIssues() ([]Issue, error) {
	args := []string{"issue", "list", "--state", "open", "--json", "number,title,body,state,assignees,labels", "--limit", "500"}
	var issues []Issue
	if err := c.ghJSON(&issues, args...); err != nil {
		return nil, fmt.Errorf("listing open issues: %w", err)
	}
	return issues, nil
}

// ListIssuesForMilestone fetches all issues assigned to a specific milestone with full details
func (c *Client) ListIssuesForMilestone(milestone string) ([]Issue, error) {
	args := []string{"issue", "list", "--state", "all", "--json", "number,title,body,state,assignees,labels,updatedAt", "--milestone", milestone, "--limit", "500"}

	out, err := c.gh(args...)
	if err != nil {
		return nil, fmt.Errorf("listing issues for milestone %s: %w", milestone, err)
	}

	var issues []Issue
	if err := json.Unmarshal(out, &issues); err != nil {
		return nil, fmt.Errorf("parsing issues for milestone %s: %w", milestone, err)
	}

	log.Printf("[GitHub] Fetched %d issues for milestone '%s'", len(issues), milestone)
	return issues, nil
}

// GetIssuePRStatus checks if an issue was closed via a merged PR
// Returns true if the issue has a merged PR associated with it
func (c *Client) GetIssuePRStatus(issueNumber int) (bool, *time.Time, error) {
	// Use GraphQL timelineItems to find PRs that closed this issue
	query := `query($repo: String!, $owner: String!, $number: Int!) {
		repository(owner: $owner, name: $repo) {
			issue(number: $number) {
				timelineItems(first: 25, itemTypes: [CLOSED_EVENT]) {
					nodes {
						... on ClosedEvent {
							closer {
								... on PullRequest {
									state
									mergedAt
								}
							}
						}
					}
				}
			}
		}
	}`

	parts := strings.SplitN(c.Repo, "/", 2)
	if len(parts) != 2 {
		return false, nil, fmt.Errorf("invalid repo format: %s", c.Repo)
	}
	owner, repo := parts[0], parts[1]

	out, err := c.ghNoRepo("api", "graphql",
		"-f", "query="+query,
		"-f", "owner="+owner,
		"-f", "repo="+repo,
		"-F", fmt.Sprintf("number=%d", issueNumber),
	)
	if err != nil {
		return false, nil, fmt.Errorf("graphql query for issue #%d: %w", issueNumber, err)
	}

	var result struct {
		Data struct {
			Repository struct {
				Issue struct {
					TimelineItems struct {
						Nodes []struct {
							Closer struct {
								State    string     `json:"state"`
								MergedAt *time.Time `json:"mergedAt"`
							} `json:"closer"`
						} `json:"nodes"`
					} `json:"timelineItems"`
				} `json:"issue"`
			} `json:"repository"`
		} `json:"data"`
	}

	if err := json.Unmarshal(out, &result); err != nil {
		return false, nil, fmt.Errorf("parsing graphql response for issue #%d: %w", issueNumber, err)
	}

	for _, node := range result.Data.Repository.Issue.TimelineItems.Nodes {
		if node.Closer.State == "MERGED" {
			return true, node.Closer.MergedAt, nil
		}
	}

	return false, nil, nil
}

// ListIssuesWithPRStatus fetches all issues for a milestone with PR merge status
// in a single GraphQL query, replacing the N+1 pattern of ListIssuesForMilestone + N × GetIssuePRStatus.
func (c *Client) ListIssuesWithPRStatus(milestone string) ([]Issue, error) {
	parts := strings.SplitN(c.Repo, "/", 2)
	if len(parts) != 2 {
		return nil, fmt.Errorf("invalid repo format: %s", c.Repo)
	}
	owner, repo := parts[0], parts[1]

	query := `query($repo: String!, $owner: String!, $milestone: String!, $cursor: String) {
		repository(owner: $owner, name: $repo) {
			milestones(first: 1, query: $milestone) {
				nodes {
					issues(first: 100, after: $cursor) {
						pageInfo {
							hasNextPage
							endCursor
						}
						nodes {
							number
							title
							body
							state
							updatedAt
							assignees(first: 5) {
								nodes {
									login
								}
							}
							labels(first: 20) {
								nodes {
									name
								}
							}
							timelineItems(first: 5, itemTypes: [CLOSED_EVENT]) {
								nodes {
									... on ClosedEvent {
										closer {
											... on PullRequest {
												state
												mergedAt
											}
										}
									}
								}
							}
						}
					}
				}
			}
		}
	}`

	type gqlIssueNode struct {
		Number    int        `json:"number"`
		Title     string     `json:"title"`
		Body      string     `json:"body"`
		State     string     `json:"state"`
		UpdatedAt *time.Time `json:"updatedAt"`
		Assignees struct {
			Nodes []struct {
				Login string `json:"login"`
			} `json:"nodes"`
		} `json:"assignees"`
		Labels struct {
			Nodes []struct {
				Name string `json:"name"`
			} `json:"nodes"`
		} `json:"labels"`
		TimelineItems struct {
			Nodes []struct {
				Closer struct {
					State    string     `json:"state"`
					MergedAt *time.Time `json:"mergedAt"`
				} `json:"closer"`
			} `json:"nodes"`
		} `json:"timelineItems"`
	}

	var allIssues []Issue
	var cursor *string

	for {
		args := []string{
			"api", "graphql",
			"-f", "query=" + query,
			"-f", "owner=" + owner,
			"-f", "repo=" + repo,
			"-f", "milestone=" + milestone,
		}
		if cursor != nil {
			args = append(args, "-f", "cursor="+*cursor)
		}

		out, err := c.ghNoRepo(args...)
		if err != nil {
			return nil, fmt.Errorf("graphql query for milestone %s: %w", milestone, err)
		}

		var result struct {
			Data struct {
				Repository struct {
					Milestones struct {
						Nodes []struct {
							Issues struct {
								PageInfo struct {
									HasNextPage bool   `json:"hasNextPage"`
									EndCursor   string `json:"endCursor"`
								} `json:"pageInfo"`
								Nodes []gqlIssueNode `json:"nodes"`
							} `json:"issues"`
						} `json:"nodes"`
					} `json:"milestones"`
				} `json:"repository"`
			} `json:"data"`
		}

		if err := json.Unmarshal(out, &result); err != nil {
			return nil, fmt.Errorf("parsing graphql response for milestone %s: %w", milestone, err)
		}

		milestones := result.Data.Repository.Milestones.Nodes
		if len(milestones) == 0 {
			log.Printf("[GitHub] No milestone found matching %q", milestone)
			return nil, nil
		}

		issuesData := milestones[0].Issues

		for _, node := range issuesData.Nodes {
			issue := Issue{
				Number:    node.Number,
				Title:     node.Title,
				Body:      node.Body,
				State:     node.State,
				UpdatedAt: node.UpdatedAt,
			}

			// Map assignees
			for _, a := range node.Assignees.Nodes {
				issue.Assignees = append(issue.Assignees, struct {
					Login string `json:"login"`
				}{Login: a.Login})
			}

			// Map labels
			for _, l := range node.Labels.Nodes {
				issue.Labels = append(issue.Labels, struct {
					Name string `json:"name"`
				}{Name: l.Name})
			}

			// Extract PR merge status from timeline
			for _, te := range node.TimelineItems.Nodes {
				if te.Closer.State == "MERGED" {
					issue.PRMerged = true
					issue.MergedAt = te.Closer.MergedAt
					break
				}
			}

			allIssues = append(allIssues, issue)
		}

		if !issuesData.PageInfo.HasNextPage {
			break
		}
		endCursor := issuesData.PageInfo.EndCursor
		cursor = &endCursor
	}

	log.Printf("[GitHub] Fetched %d issues with PR status for milestone %q (1 GraphQL query)", len(allIssues), milestone)
	return allIssues, nil
}

func (c *Client) CloseMilestone(number int) error {
	_, err := c.ghNoRepo("api", "repos/"+c.Repo+"/milestones/"+strconv.Itoa(number), "-f", "state=closed")
	if err != nil {
		return fmt.Errorf("closing milestone %d: %w", number, err)
	}
	return nil
}

// GetMostRecentlyClosedSprint returns the most recently closed milestone
func (c *Client) GetMostRecentlyClosedSprint() (*Milestone, error) {
	var milestones []Milestone
	if err := c.ghNoRepoJSON(&milestones, "api", "repos/"+c.Repo+"/milestones?state=closed"); err != nil {
		return nil, fmt.Errorf("listing closed milestones: %w", err)
	}

	if len(milestones) == 0 {
		return nil, nil
	}

	// Sort by creation date, newest first
	sort.Slice(milestones, func(i, j int) bool {
		return milestones[i].CreatedAt.After(milestones[j].CreatedAt)
	})

	return &milestones[0], nil
}

// GetClosedTicketsForSprint returns closed (merged) tickets from a sprint
func (c *Client) GetClosedTicketsForSprint(milestoneNumber int) ([]Issue, error) {
	// Get milestone title first
	var milestone Milestone
	if err := c.ghNoRepoJSON(&milestone, "api", "repos/"+c.Repo+"/milestones/"+strconv.Itoa(milestoneNumber)); err != nil {
		return nil, fmt.Errorf("getting milestone %d: %w", milestoneNumber, err)
	}

	// Use ListIssuesWithPRStatus to get all issues with PR merge status
	issues, err := c.ListIssuesWithPRStatus(milestone.Title)
	if err != nil {
		return nil, fmt.Errorf("listing issues for sprint: %w", err)
	}

	// Filter to only merged issues
	var mergedIssues []Issue
	for _, issue := range issues {
		if issue.PRMerged {
			mergedIssues = append(mergedIssues, issue)
		}
	}

	return mergedIssues, nil
}

func parseJSON(data []byte, v any) error {
	if err := json.Unmarshal(data, v); err != nil {
		return fmt.Errorf("parsing JSON: %w", err)
	}
	return nil
}
