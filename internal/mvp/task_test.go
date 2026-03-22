package mvp

import (
	"testing"

	"github.com/crazy-goat/one-dev-army/internal/github"
)

func TestTaskStatusConstants(t *testing.T) {
	statuses := []TaskStatus{
		StatusPending,
		StatusAnalyzing,
		StatusPlanning,
		StatusCoding,
		StatusReviewing,
		StatusCreatingPR,
		StatusDone,
		StatusFailed,
	}

	expected := []string{
		"pending",
		"analyzing",
		"planning",
		"coding",
		"reviewing",
		"creating_pr",
		"done",
		"failed",
	}

	if len(statuses) != len(expected) {
		t.Fatalf("expected %d statuses, got %d", len(expected), len(statuses))
	}

	for i, s := range statuses {
		if string(s) != expected[i] {
			t.Errorf("status[%d] = %q, want %q", i, s, expected[i])
		}
	}
}

func TestTaskZeroValue(t *testing.T) {
	var task Task
	if task.Status != "" {
		t.Errorf("zero-value Status = %q, want empty", task.Status)
	}
	if task.Result != nil {
		t.Error("zero-value Result should be nil")
	}
	if task.Branch != "" {
		t.Errorf("zero-value Branch = %q, want empty", task.Branch)
	}
	if task.Worktree != "" {
		t.Errorf("zero-value Worktree = %q, want empty", task.Worktree)
	}
}

func TestTaskWithIssue(t *testing.T) {
	issue := github.Issue{
		Number: 42,
		Title:  "Add feature X",
		Body:   "Description of feature X",
		State:  "open",
	}

	task := Task{
		Issue:     issue,
		Milestone: "Sprint 1",
		Status:    StatusPending,
	}

	if task.Issue.Number != 42 {
		t.Errorf("Issue.Number = %d, want 42", task.Issue.Number)
	}
	if task.Milestone != "Sprint 1" {
		t.Errorf("Milestone = %q, want %q", task.Milestone, "Sprint 1")
	}
	if task.Status != StatusPending {
		t.Errorf("Status = %q, want %q", task.Status, StatusPending)
	}
}

func TestTaskResult(t *testing.T) {
	result := &TaskResult{
		PRURL:   "https://github.com/owner/repo/pull/1",
		Summary: "Implemented feature X",
	}

	if result.PRURL != "https://github.com/owner/repo/pull/1" {
		t.Errorf("PRURL = %q, want PR URL", result.PRURL)
	}
	if result.Error != nil {
		t.Error("Error should be nil for successful result")
	}
	if result.Summary != "Implemented feature X" {
		t.Errorf("Summary = %q, want %q", result.Summary, "Implemented feature X")
	}
}
