package github

import (
	"fmt"
	"strconv"
	"strings"
	"sync"
)

// StageToLabels maps stage names to their corresponding GitHub labels
var StageToLabels = map[string][]string{
	"Backlog":   {},
	"Plan":      {"stage:analysis", "stage:planning"},
	"Code":      {"stage:coding", "stage:testing"},
	"AI Review": {"stage:code-review"},
	"Approve":   {"awaiting-approval"},
	"Done":      {},
	"Failed":    {"failed"},
	"Blocked":   {"blocked"},
}

// StageLabelPrefixes contains all label prefixes that should be removed when changing stages
var StageLabelPrefixes = []string{
	"stage:",
	"awaiting-approval",
	"failed",
	"blocked",
}

type Label struct {
	Name  string
	Color string
}

var RequiredLabels = []Label{
	{Name: "sprint", Color: "0E8A16"},
	{Name: "insight", Color: "D93F0B"},
	{Name: "in-progress", Color: "FBCA04"},
	{Name: "failed", Color: "D93F0B"},
	{Name: "size:S", Color: "C2E0C6"},
	{Name: "size:M", Color: "BFDADC"},
	{Name: "size:L", Color: "BFD4F2"},
	{Name: "size:XL", Color: "D4C5F9"},
	{Name: "stage:analysis", Color: "FBCA04"},
	{Name: "stage:planning", Color: "FBCA04"},
	{Name: "stage:plan-review", Color: "FBCA04"},
	{Name: "stage:coding", Color: "1D76DB"},
	{Name: "stage:testing", Color: "1D76DB"},
	{Name: "stage:code-review", Color: "1D76DB"},
	{Name: "stage:needs-user", Color: "B60205"},
	{Name: "stage:cancelled", Color: "EEEEEE"},
	{Name: "priority:high", Color: "B60205"},
	{Name: "priority:medium", Color: "FBCA04"},
	{Name: "priority:low", Color: "0E8A16"},
	{Name: "epic", Color: "5319E7"},
	{Name: "wizard", Color: "7C3AED"},
	{Name: "merge-failed", Color: "D93F0B"},
	{Name: "awaiting-approval", Color: "0E8A16"},
	{Name: "blocked", Color: "B60205"},
}

// SetStageLabel sets the stage label for an issue, removing all previous stage labels.
// It returns the updated issue with fresh data from GitHub.
// Special cases:
//   - "Done": closes the issue
//   - "Backlog": removes all stage labels without adding new ones
func (c *Client) SetStageLabel(issueNumber int, stage string) (Issue, error) {
	// Validate stage
	labels, ok := StageToLabels[stage]
	if !ok {
		return Issue{}, fmt.Errorf("invalid stage: %s", stage)
	}

	// Get current issue to check existing labels
	issue, err := c.GetIssue(issueNumber)
	if err != nil {
		return Issue{}, fmt.Errorf("getting issue #%d: %w", issueNumber, err)
	}

	// Remove all stage-related labels
	labelsToRemove := c.getStageLabelsToRemove(issue)
	for _, label := range labelsToRemove {
		// Continue on error - label might not exist (idempotent)
		_ = c.RemoveLabel(issueNumber, label)
	}

	// Add new labels (if not Backlog stage)
	if stage != "Backlog" {
		for _, label := range labels {
			if err := c.AddLabel(issueNumber, label); err != nil {
				return Issue{}, fmt.Errorf("adding label %s to issue #%d: %w", label, issueNumber, err)
			}
		}
	}

	// Handle special case: Done stage closes the issue
	if stage == "Done" {
		if err := c.CloseIssue(issueNumber); err != nil {
			return Issue{}, fmt.Errorf("closing issue #%d: %w", issueNumber, err)
		}
	}

	// Fetch fresh issue data
	updatedIssue, err := c.GetIssue(issueNumber)
	if err != nil {
		return Issue{}, fmt.Errorf("fetching updated issue #%d: %w", issueNumber, err)
	}

	return *updatedIssue, nil
}

// getStageLabelsToRemove returns all stage-related labels that should be removed from an issue
func (c *Client) getStageLabelsToRemove(issue *Issue) []string {
	var toRemove []string
	for _, label := range issue.Labels {
		for _, prefix := range StageLabelPrefixes {
			if strings.HasPrefix(label.Name, prefix) || label.Name == prefix {
				toRemove = append(toRemove, label.Name)
				break
			}
		}
	}
	return toRemove
}

// AddLabels adds multiple labels to an issue concurrently
func (c *Client) AddLabels(issueNum int, labels []string) error {
	if len(labels) == 0 {
		return nil
	}

	var wg sync.WaitGroup
	errChan := make(chan error, len(labels))

	for _, label := range labels {
		wg.Add(1)
		go func(l string) {
			defer wg.Done()
			if err := c.AddLabel(issueNum, l); err != nil {
				errChan <- fmt.Errorf("adding label %s: %w", l, err)
			}
		}(label)
	}

	wg.Wait()
	close(errChan)

	for err := range errChan {
		return err
	}
	return nil
}

// RemoveLabels removes multiple labels from an issue concurrently
func (c *Client) RemoveLabels(issueNum int, labels []string) error {
	if len(labels) == 0 {
		return nil
	}

	var wg sync.WaitGroup
	errChan := make(chan error, len(labels))

	for _, label := range labels {
		wg.Add(1)
		go func(l string) {
			defer wg.Done()
			if err := c.RemoveLabel(issueNum, l); err != nil {
				errChan <- fmt.Errorf("removing label %s: %w", l, err)
			}
		}(label)
	}

	wg.Wait()
	close(errChan)

	for err := range errChan {
		return err
	}
	return nil
}

// IsStageLabel checks if a label is a stage-related label
func IsStageLabel(label string) bool {
	for _, prefix := range StageLabelPrefixes {
		if strings.HasPrefix(label, prefix) || label == prefix {
			return true
		}
	}
	return false
}

// GetStageFromLabels returns the stage name based on the labels present on an issue
func GetStageFromLabels(labels []string) string {
	labelSet := make(map[string]bool)
	for _, l := range labels {
		labelSet[l] = true
	}

	// Check each stage mapping
	for stage, stageLabels := range StageToLabels {
		if stage == "Backlog" || stage == "Done" {
			continue
		}
		matches := 0
		for _, sl := range stageLabels {
			if labelSet[sl] {
				matches++
			}
		}
		if matches > 0 {
			return stage
		}
	}

	return "Backlog"
}

func (c *Client) EnsureLabels() error {
	existing, _ := c.listLabels()
	existingSet := make(map[string]bool, len(existing))
	for _, name := range existing {
		existingSet[name] = true
	}

	var missing []Label
	for _, l := range RequiredLabels {
		if !existingSet[l.Name] {
			missing = append(missing, l)
		}
	}

	if len(missing) == 0 {
		return nil
	}

	var wg sync.WaitGroup
	errs := make(chan error, len(missing))

	for _, l := range missing {
		wg.Add(1)
		go func(l Label) {
			defer wg.Done()
			_, err := c.gh("label", "create", l.Name, "--color", l.Color, "--force")
			if err != nil {
				errs <- fmt.Errorf("creating label %s: %w", l.Name, err)
			}
		}(l)
	}

	wg.Wait()
	close(errs)

	for err := range errs {
		return err
	}
	return nil
}

func (c *Client) listLabels() ([]string, error) {
	var labels []struct {
		Name string `json:"name"`
	}
	err := c.ghJSON(&labels, "label", "list", "--json", "name", "--limit", "200")
	if err != nil {
		return nil, err
	}
	names := make([]string, len(labels))
	for i, l := range labels {
		names[i] = l.Name
	}
	return names, nil
}

// parseIssueNumber extracts issue number from gh command output
func parseIssueNumber(output []byte) (int, error) {
	url := strings.TrimSpace(string(output))
	parts := strings.Split(url, "/")
	if len(parts) == 0 {
		return 0, fmt.Errorf("empty output")
	}
	numStr := parts[len(parts)-1]
	num, err := strconv.Atoi(numStr)
	if err != nil {
		return 0, fmt.Errorf("parsing issue number from %q: %w", url, err)
	}
	return num, nil
}
