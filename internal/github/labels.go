package github

import (
	"fmt"
	"log"
	"strconv"
	"strings"
	"sync"
)

// Stage represents a workflow stage with a type-safe enum.
// Every stage maps to exactly one GitHub label with the "stage:" prefix.
type Stage string

const (
	StageBacklog   Stage = "stage:backlog"
	StagePlan      Stage = "stage:analysis"
	StageCode      Stage = "stage:coding"
	StageReview    Stage = "stage:code-review"
	StageCreatePR  Stage = "stage:create-pr"
	StageApprove   Stage = "stage:awaiting-approval"
	StageMerge     Stage = "stage:merging"
	StageDone      Stage = "stage:done"
	StageFailed    Stage = "stage:failed"
	StageBlocked   Stage = "stage:blocked"
	StageNeedsUser Stage = "stage:needs-user"
)

// AllStages lists every valid Stage value.
var AllStages = []Stage{
	StageBacklog,
	StagePlan,
	StageCode,
	StageReview,
	StageCreatePR,
	StageApprove,
	StageMerge,
	StageDone,
	StageFailed,
	StageBlocked,
	StageNeedsUser,
}

// Label returns the GitHub label string for this stage (e.g. "stage:backlog").
func (s Stage) Label() string {
	return string(s)
}

// Column returns the dashboard column name for this stage.
func (s Stage) Column() string {
	switch s {
	case StageBacklog:
		return "Backlog"
	case StagePlan:
		return "Plan"
	case StageCode:
		return "Code"
	case StageReview, StageCreatePR:
		return "AI Review"
	case StageApprove:
		return "Approve"
	case StageMerge:
		return "Merge"
	case StageDone:
		return "Done"
	case StageFailed:
		return "Failed"
	case StageBlocked:
		return "Blocked"
	case StageNeedsUser:
		return "Blocked"
	default:
		return "Backlog"
	}
}

// stagesByLabel allows O(1) lookup from label string to Stage.
var stagesByLabel map[string]Stage

func init() {
	stagesByLabel = make(map[string]Stage, len(AllStages))
	for _, s := range AllStages {
		stagesByLabel[s.Label()] = s
	}
}

// StageFromLabel converts a GitHub label string to a Stage.
// Returns the stage and true if found, or StageBacklog and false if not.
func StageFromLabel(label string) (Stage, bool) {
	s, ok := stagesByLabel[label]
	return s, ok
}

// StageLabelPrefixes contains all label prefixes that should be removed when changing stages.
// Includes legacy bare labels for backward compatibility during migration.
var StageLabelPrefixes = []string{
	"stage:",
	"awaiting-approval",
	"failed",
	"blocked",
	"in-progress",
}

type Label struct {
	Name  string
	Color string
}

var RequiredLabels = []Label{
	{Name: "sprint", Color: "0E8A16"},
	{Name: "insight", Color: "D93F0B"},
	{Name: "size:S", Color: "C2E0C6"},
	{Name: "size:M", Color: "BFDADC"},
	{Name: "size:L", Color: "BFD4F2"},
	{Name: "size:XL", Color: "D4C5F9"},
	{Name: "stage:backlog", Color: "EEEEEE"},
	{Name: "stage:analysis", Color: "FBCA04"},
	{Name: "stage:coding", Color: "1D76DB"},
	{Name: "stage:code-review", Color: "1D76DB"},
	{Name: "stage:create-pr", Color: "1D76DB"},
	{Name: "stage:awaiting-approval", Color: "0E8A16"},
	{Name: "stage:merging", Color: "0E8A16"},
	{Name: "stage:done", Color: "0E8A16"},
	{Name: "stage:failed", Color: "D93F0B"},
	{Name: "stage:blocked", Color: "B60205"},
	{Name: "stage:needs-user", Color: "FBCA04"},
	{Name: "priority:high", Color: "B60205"},
	{Name: "priority:medium", Color: "FBCA04"},
	{Name: "priority:low", Color: "0E8A16"},
	{Name: "epic", Color: "5319E7"},
	{Name: "wizard", Color: "7C3AED"},
	{Name: "merge-failed", Color: "D93F0B"},
}

// SetStageLabel sets the stage label for an issue, removing all previous stage labels.
// It returns the updated issue with fresh data from GitHub.
// Special case: StageDone also closes the issue.
func (c *Client) SetStageLabel(issueNumber int, stage Stage) (Issue, error) {
	log.Printf("[GitHub] Setting stage %s for issue #%d", stage.Label(), issueNumber)

	// Get current issue to check existing labels
	issue, err := c.GetIssue(issueNumber)
	if err != nil {
		return Issue{}, fmt.Errorf("getting issue #%d: %w", issueNumber, err)
	}

	// Remove all stage-related labels
	labelsToRemove := c.getStageLabelsToRemove(issue)
	for _, label := range labelsToRemove {
		_ = c.RemoveLabel(issueNumber, label)
	}

	// Add new stage label (bypass guard — this IS the correct path for stage labels)
	if err := c.addLabelRaw(issueNumber, stage.Label()); err != nil {
		return Issue{}, fmt.Errorf("adding label %s to issue #%d: %w", stage.Label(), issueNumber, err)
	}

	// Handle special case: Done stage closes the issue
	if stage == StageDone {
		if err := c.CloseIssue(issueNumber); err != nil {
			return Issue{}, fmt.Errorf("closing issue #%d: %w", issueNumber, err)
		}
	}

	log.Printf("[GitHub] ✓ Set stage %s for issue #%d", stage.Label(), issueNumber)

	// Build updated issue locally instead of fetching from GitHub
	updatedIssue := Issue{
		Number:    issue.Number,
		Title:     issue.Title,
		Body:      issue.Body,
		State:     issue.State,
		Assignees: issue.Assignees,
		UpdatedAt: issue.UpdatedAt,
	}

	// Rebuild labels: remove stage labels, add new one
	for _, label := range issue.Labels {
		if !IsStageLabel(label.Name) {
			updatedIssue.Labels = append(updatedIssue.Labels, label)
		}
	}
	updatedIssue.Labels = append(updatedIssue.Labels, struct {
		Name string `json:"name"`
	}{Name: stage.Label()})

	if stage == StageDone {
		updatedIssue.State = "closed"
	}

	return updatedIssue, nil
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

// GetStageFromLabels returns the Stage based on the labels present on an issue.
// Returns StageBacklog if no stage label is found.
func GetStageFromLabels(labels []string) Stage {
	for _, l := range labels {
		if s, ok := StageFromLabel(l); ok {
			return s
		}
	}
	return StageBacklog
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
		log.Println("  ✓ all GitHub labels exist")
		return nil
	}

	log.Printf("  → creating %d missing label(s)...", len(missing))

	var wg sync.WaitGroup
	errChan := make(chan error, len(missing))
	createdChan := make(chan string, len(missing))

	for _, l := range missing {
		wg.Add(1)
		go func(l Label) {
			defer wg.Done()
			_, err := c.gh("label", "create", l.Name, "--color", l.Color, "--force")
			if err != nil {
				errChan <- fmt.Errorf("creating label %s: %w", l.Name, err)
			} else {
				createdChan <- l.Name
			}
		}(l)
	}

	wg.Wait()
	close(errChan)
	close(createdChan)

	for err := range errChan {
		return err
	}

	var created []string
	for name := range createdChan {
		created = append(created, name)
	}
	log.Printf("  ✓ created %d label(s): %v", len(created), created)

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
