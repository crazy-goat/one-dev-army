package db_test

import (
	"math"
	"path/filepath"
	"testing"
	"time"

	"github.com/crazy-goat/one-dev-army/internal/db"
	"github.com/crazy-goat/one-dev-army/internal/github"
)

func openTestStore(t *testing.T) *db.Store {
	t.Helper()
	path := filepath.Join(t.TempDir(), "test.db")
	store, err := db.Open(path)
	if err != nil {
		t.Fatalf("opening store: %v", err)
	}
	t.Cleanup(func() { store.Close() })
	return store
}

func TestOpen_Migrate(t *testing.T) {
	path := filepath.Join(t.TempDir(), "migrate.db")
	store, err := db.Open(path)
	if err != nil {
		t.Fatalf("first open: %v", err)
	}
	store.Close()

	store, err = db.Open(path)
	if err != nil {
		t.Fatalf("second open (idempotent migration): %v", err)
	}
	store.Close()
}

func TestSaveAndGetTaskMetrics(t *testing.T) {
	store := openTestStore(t)

	metrics := []db.StageMetric{
		{TaskID: 42, SprintID: 1, Stage: "analysis", LLM: "claude-sonnet-4", TokensIn: 1000, TokensOut: 500, CostUSD: 0.015, DurationS: 30, Retries: 0},
		{TaskID: 42, SprintID: 1, Stage: "coding", LLM: "claude-sonnet-4", TokensIn: 2000, TokensOut: 1500, CostUSD: 0.045, DurationS: 120, Retries: 1},
		{TaskID: 99, SprintID: 1, Stage: "analysis", LLM: "claude-opus-4", TokensIn: 500, TokensOut: 200, CostUSD: 0.030, DurationS: 15, Retries: 0},
	}

	for _, m := range metrics {
		if err := store.SaveStageMetric(m); err != nil {
			t.Fatalf("saving metric: %v", err)
		}
	}

	got, err := store.GetTaskMetrics(42)
	if err != nil {
		t.Fatalf("getting task metrics: %v", err)
	}

	if len(got) != 2 {
		t.Fatalf("got %d metrics, want 2", len(got))
	}

	first := got[0]
	if first.ID == 0 {
		t.Error("expected non-zero ID")
	}
	if first.TaskID != 42 {
		t.Errorf("task_id = %d, want 42", first.TaskID)
	}
	if first.Stage != "analysis" {
		t.Errorf("stage = %q, want %q", first.Stage, "analysis")
	}
	if first.LLM != "claude-sonnet-4" {
		t.Errorf("llm = %q, want %q", first.LLM, "claude-sonnet-4")
	}
	if first.TokensIn != 1000 {
		t.Errorf("tokens_in = %d, want 1000", first.TokensIn)
	}
	if first.TokensOut != 500 {
		t.Errorf("tokens_out = %d, want 500", first.TokensOut)
	}
	if math.Abs(first.CostUSD-0.015) > 1e-9 {
		t.Errorf("cost_usd = %f, want 0.015", first.CostUSD)
	}
	if first.DurationS != 30 {
		t.Errorf("duration_s = %d, want 30", first.DurationS)
	}
	if first.Retries != 0 {
		t.Errorf("retries = %d, want 0", first.Retries)
	}

	second := got[1]
	if second.Stage != "coding" {
		t.Errorf("stage = %q, want %q", second.Stage, "coding")
	}
	if second.Retries != 1 {
		t.Errorf("retries = %d, want 1", second.Retries)
	}

	got99, err := store.GetTaskMetrics(99)
	if err != nil {
		t.Fatalf("getting task 99 metrics: %v", err)
	}
	if len(got99) != 1 {
		t.Fatalf("got %d metrics for task 99, want 1", len(got99))
	}
}

func TestGetTaskMetrics_Empty(t *testing.T) {
	store := openTestStore(t)

	got, err := store.GetTaskMetrics(999)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(got) != 0 {
		t.Errorf("got %d metrics, want 0", len(got))
	}
}

func TestGetSprintCost(t *testing.T) {
	store := openTestStore(t)

	metrics := []db.StageMetric{
		{TaskID: 1, SprintID: 10, Stage: "analysis", LLM: "claude-sonnet-4", TokensIn: 100, TokensOut: 50, CostUSD: 0.10, DurationS: 10, Retries: 0},
		{TaskID: 1, SprintID: 10, Stage: "coding", LLM: "claude-sonnet-4", TokensIn: 200, TokensOut: 100, CostUSD: 0.25, DurationS: 60, Retries: 0},
		{TaskID: 2, SprintID: 10, Stage: "analysis", LLM: "claude-opus-4", TokensIn: 300, TokensOut: 150, CostUSD: 0.50, DurationS: 20, Retries: 1},
		{TaskID: 3, SprintID: 20, Stage: "coding", LLM: "claude-sonnet-4", TokensIn: 400, TokensOut: 200, CostUSD: 1.00, DurationS: 90, Retries: 0},
	}

	for _, m := range metrics {
		if err := store.SaveStageMetric(m); err != nil {
			t.Fatalf("saving metric: %v", err)
		}
	}

	cost10, err := store.GetSprintCost(10)
	if err != nil {
		t.Fatalf("getting sprint 10 cost: %v", err)
	}
	want10 := 0.10 + 0.25 + 0.50
	if math.Abs(cost10-want10) > 1e-9 {
		t.Errorf("sprint 10 cost = %f, want %f", cost10, want10)
	}

	cost20, err := store.GetSprintCost(20)
	if err != nil {
		t.Fatalf("getting sprint 20 cost: %v", err)
	}
	if math.Abs(cost20-1.00) > 1e-9 {
		t.Errorf("sprint 20 cost = %f, want 1.00", cost20)
	}

	costEmpty, err := store.GetSprintCost(999)
	if err != nil {
		t.Fatalf("getting empty sprint cost: %v", err)
	}
	if costEmpty != 0 {
		t.Errorf("empty sprint cost = %f, want 0", costEmpty)
	}
}

func TestGetLastCompletedStep_Migration(t *testing.T) {
	store := openTestStore(t)

	// Insert old "analyze" step
	stepID, err := store.InsertStep(100, "analyze", "test prompt", "session-1")
	if err != nil {
		t.Fatalf("inserting analyze step: %v", err)
	}
	if err := store.FinishStep(stepID, "analysis response"); err != nil {
		t.Fatalf("finishing analyze step: %v", err)
	}

	// Should return "technical-planning" for old "analyze" step
	lastStep, err := store.GetLastCompletedStep(100)
	if err != nil {
		t.Fatalf("getting last completed step: %v", err)
	}
	if lastStep != "technical-planning" {
		t.Errorf("last step = %q, want %q", lastStep, "technical-planning")
	}

	// Insert old "plan" step
	stepID2, err := store.InsertStep(101, "plan", "test prompt 2", "session-2")
	if err != nil {
		t.Fatalf("inserting plan step: %v", err)
	}
	if err := store.FinishStep(stepID2, "plan response"); err != nil {
		t.Fatalf("finishing plan step: %v", err)
	}

	// Should return "technical-planning" for old "plan" step
	lastStep2, err := store.GetLastCompletedStep(101)
	if err != nil {
		t.Fatalf("getting last completed step: %v", err)
	}
	if lastStep2 != "technical-planning" {
		t.Errorf("last step = %q, want %q", lastStep2, "technical-planning")
	}

	// Insert new "technical-planning" step
	stepID3, err := store.InsertStep(102, "technical-planning", "test prompt 3", "session-3")
	if err != nil {
		t.Fatalf("inserting technical-planning step: %v", err)
	}
	if err := store.FinishStep(stepID3, "combined response"); err != nil {
		t.Fatalf("finishing technical-planning step: %v", err)
	}

	// Should return "technical-planning" for new step
	lastStep3, err := store.GetLastCompletedStep(102)
	if err != nil {
		t.Fatalf("getting last completed step: %v", err)
	}
	if lastStep3 != "technical-planning" {
		t.Errorf("last step = %q, want %q", lastStep3, "technical-planning")
	}
}

func TestGetStepResponse_Migration(t *testing.T) {
	store := openTestStore(t)

	// Insert old "plan" step
	stepID, err := store.InsertStep(200, "plan", "test prompt", "session-1")
	if err != nil {
		t.Fatalf("inserting plan step: %v", err)
	}
	if err := store.FinishStep(stepID, "plan response content"); err != nil {
		t.Fatalf("finishing plan step: %v", err)
	}

	// Request "technical-planning" should fall back to "plan" response
	response, err := store.GetStepResponse(200, "technical-planning")
	if err != nil {
		t.Fatalf("getting step response: %v", err)
	}
	if response != "plan response content" {
		t.Errorf("response = %q, want %q", response, "plan response content")
	}

	// Insert new "technical-planning" step
	stepID2, err := store.InsertStep(201, "technical-planning", "test prompt 2", "session-2")
	if err != nil {
		t.Fatalf("inserting technical-planning step: %v", err)
	}
	if err := store.FinishStep(stepID2, "combined response content"); err != nil {
		t.Fatalf("finishing technical-planning step: %v", err)
	}

	// Should return the new step response directly
	response2, err := store.GetStepResponse(201, "technical-planning")
	if err != nil {
		t.Fatalf("getting step response: %v", err)
	}
	if response2 != "combined response content" {
		t.Errorf("response = %q, want %q", response2, "combined response content")
	}
}

func TestSaveAndGetIssueCache(t *testing.T) {
	store := openTestStore(t)

	issue := github.Issue{
		Number: 177,
		Title:  "Create SQLite cache schema for issues",
		Body:   "This is a test issue body",
		State:  "open",
		Labels: []struct {
			Name string `json:"name"`
		}{
			{Name: "enhancement"},
			{Name: "database"},
		},
		Assignees: []struct {
			Login string `json:"login"`
		}{
			{Login: "testuser"},
		},
	}

	if err := store.SaveIssueCache(issue, "v1.0"); err != nil {
		t.Fatalf("saving issue cache: %v", err)
	}

	got, err := store.GetIssueCache(177)
	if err != nil {
		t.Fatalf("getting issue cache: %v", err)
	}

	if got.Number != 177 {
		t.Errorf("number = %d, want 177", got.Number)
	}
	if got.Title != "Create SQLite cache schema for issues" {
		t.Errorf("title = %q, want %q", got.Title, "Create SQLite cache schema for issues")
	}
	if got.Body != "This is a test issue body" {
		t.Errorf("body = %q, want %q", got.Body, "This is a test issue body")
	}
	if got.State != "open" {
		t.Errorf("state = %q, want %q", got.State, "open")
	}
	if len(got.Labels) != 2 {
		t.Errorf("labels count = %d, want 2", len(got.Labels))
	} else {
		if got.Labels[0].Name != "enhancement" {
			t.Errorf("label[0] = %q, want %q", got.Labels[0].Name, "enhancement")
		}
		if got.Labels[1].Name != "database" {
			t.Errorf("label[1] = %q, want %q", got.Labels[1].Name, "database")
		}
	}
	if len(got.Assignees) != 1 || got.Assignees[0].Login != "testuser" {
		t.Errorf("assignee = %v, want testuser", got.Assignees)
	}
}

func TestGetIssueCache_NotFound(t *testing.T) {
	store := openTestStore(t)

	_, err := store.GetIssueCache(99999)
	if err == nil {
		t.Fatal("expected error for non-existent issue, got nil")
	}
}

func TestGetIssuesCacheByMilestone(t *testing.T) {
	store := openTestStore(t)

	issues := []github.Issue{
		{Number: 1, Title: "Issue 1", State: "open", Labels: nil, Assignees: nil},
		{Number: 2, Title: "Issue 2", State: "closed", Labels: nil, Assignees: nil},
		{Number: 3, Title: "Issue 3", State: "open", Labels: nil, Assignees: nil},
	}

	for _, issue := range issues {
		milestone := "v1.0"
		if issue.Number == 3 {
			milestone = "v2.0"
		}
		if err := store.SaveIssueCache(issue, milestone); err != nil {
			t.Fatalf("saving issue cache: %v", err)
		}
	}

	got, err := store.GetIssuesCacheByMilestone("v1.0")
	if err != nil {
		t.Fatalf("getting issues by milestone: %v", err)
	}

	if len(got) != 2 {
		t.Errorf("got %d issues, want 2", len(got))
	}

	for _, issue := range got {
		if issue.Number != 1 && issue.Number != 2 {
			t.Errorf("unexpected issue number: %d", issue.Number)
		}
	}
}

func TestGetAllCachedIssues(t *testing.T) {
	store := openTestStore(t)

	issues := []github.Issue{
		{Number: 10, Title: "Issue 10", State: "open", Labels: nil, Assignees: nil},
		{Number: 20, Title: "Issue 20", State: "closed", Labels: nil, Assignees: nil},
		{Number: 30, Title: "Issue 30", State: "open", Labels: nil, Assignees: nil},
	}

	for _, issue := range issues {
		if err := store.SaveIssueCache(issue, "backlog"); err != nil {
			t.Fatalf("saving issue cache: %v", err)
		}
	}

	got, err := store.GetAllCachedIssues()
	if err != nil {
		t.Fatalf("getting all cached issues: %v", err)
	}

	if len(got) != 3 {
		t.Errorf("got %d issues, want 3", len(got))
	}
}

func TestClearIssueCache(t *testing.T) {
	store := openTestStore(t)

	issue := github.Issue{
		Number: 100,
		Title:  "Test Issue",
		State:  "open",
		Labels: nil,
	}

	if err := store.SaveIssueCache(issue, "v1.0"); err != nil {
		t.Fatalf("saving issue cache: %v", err)
	}

	got, err := store.GetAllCachedIssues()
	if err != nil {
		t.Fatalf("getting all cached issues: %v", err)
	}
	if len(got) != 1 {
		t.Fatalf("expected 1 issue before clear, got %d", len(got))
	}

	if err := store.ClearIssueCache(); err != nil {
		t.Fatalf("clearing issue cache: %v", err)
	}

	got, err = store.GetAllCachedIssues()
	if err != nil {
		t.Fatalf("getting all cached issues after clear: %v", err)
	}
	if len(got) != 0 {
		t.Errorf("expected 0 issues after clear, got %d", len(got))
	}
}

func TestSaveAndGetIssueCache_WithMergeStatus(t *testing.T) {
	store := openTestStore(t)

	mergedAt := time.Now().UTC().Truncate(time.Second)
	issue := github.Issue{
		Number:    178,
		Title:     "Issue with merged PR",
		Body:      "This issue was closed via PR merge",
		State:     "closed",
		PRMerged:  true,
		MergedAt:  &mergedAt,
		Labels:    nil,
		Assignees: nil,
	}

	if err := store.SaveIssueCache(issue, "v1.0"); err != nil {
		t.Fatalf("saving issue cache with merge status: %v", err)
	}

	got, err := store.GetIssueCache(178)
	if err != nil {
		t.Fatalf("getting issue cache: %v", err)
	}

	if got.Number != 178 {
		t.Errorf("number = %d, want 178", got.Number)
	}
	if got.State != "closed" {
		t.Errorf("state = %q, want %q", got.State, "closed")
	}
	if !got.PRMerged {
		t.Errorf("pr_merged = %v, want true", got.PRMerged)
	}
	if got.MergedAt == nil {
		t.Error("merged_at should not be nil")
	} else if !got.MergedAt.Equal(mergedAt) {
		t.Errorf("merged_at = %v, want %v", got.MergedAt, mergedAt)
	}
}

func TestSaveAndGetIssueCache_NotMerged(t *testing.T) {
	store := openTestStore(t)

	issue := github.Issue{
		Number:    179,
		Title:     "Issue closed without merge",
		Body:      "This issue was manually closed",
		State:     "closed",
		PRMerged:  false,
		MergedAt:  nil,
		Labels:    nil,
		Assignees: nil,
	}

	if err := store.SaveIssueCache(issue, "v1.0"); err != nil {
		t.Fatalf("saving issue cache without merge status: %v", err)
	}

	got, err := store.GetIssueCache(179)
	if err != nil {
		t.Fatalf("getting issue cache: %v", err)
	}

	if got.PRMerged {
		t.Errorf("pr_merged = %v, want false", got.PRMerged)
	}
	if got.MergedAt != nil {
		t.Errorf("merged_at should be nil, got %v", got.MergedAt)
	}
}
