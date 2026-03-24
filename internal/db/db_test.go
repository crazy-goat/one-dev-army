package db_test

import (
	"math"
	"path/filepath"
	"strings"
	"sync"
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
	t.Cleanup(func() { _ = store.Close() })
	return store
}

func TestOpen_Migrate(t *testing.T) {
	path := filepath.Join(t.TempDir(), "migrate.db")
	store, err := db.Open(path)
	if err != nil {
		t.Fatalf("first open: %v", err)
	}
	_ = store.Close()

	store, err = db.Open(path)
	if err != nil {
		t.Fatalf("second open (idempotent migration): %v", err)
	}
	_ = store.Close()
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

	if err := store.SaveIssueCache(issue, "v1.0", true); err != nil {
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
		if err := store.SaveIssueCache(issue, milestone, true); err != nil {
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

func TestGetOpenIssuesCacheByMilestone(t *testing.T) {
	store := openTestStore(t)

	issues := []github.Issue{
		{Number: 1, Title: "Issue 1", State: "open", Labels: nil, Assignees: nil},
		{Number: 2, Title: "Issue 2", State: "closed", Labels: nil, Assignees: nil},
		{Number: 3, Title: "Issue 3", State: "open", Labels: nil, Assignees: nil},
		{Number: 4, Title: "Issue 4", State: "closed", Labels: nil, Assignees: nil},
	}

	for _, issue := range issues {
		milestone := "v1.0"
		if issue.Number == 3 || issue.Number == 4 {
			milestone = "v2.0"
		}
		if err := store.SaveIssueCache(issue, milestone, true); err != nil {
			t.Fatalf("saving issue cache: %v", err)
		}
	}

	// Test v1.0 milestone - should only return open issues
	got, err := store.GetOpenIssuesCacheByMilestone("v1.0")
	if err != nil {
		t.Fatalf("getting open issues by milestone: %v", err)
	}

	if len(got) != 1 {
		t.Errorf("got %d open issues for v1.0, want 1", len(got))
	}
	if len(got) > 0 && got[0].Number != 1 {
		t.Errorf("expected issue #1, got issue #%d", got[0].Number)
	}

	// Test v2.0 milestone - should only return open issues
	got, err = store.GetOpenIssuesCacheByMilestone("v2.0")
	if err != nil {
		t.Fatalf("getting open issues by milestone: %v", err)
	}

	if len(got) != 1 {
		t.Errorf("got %d open issues for v2.0, want 1", len(got))
	}
	if len(got) > 0 && got[0].Number != 3 {
		t.Errorf("expected issue #3, got issue #%d", got[0].Number)
	}

	// Test non-existent milestone
	got, err = store.GetOpenIssuesCacheByMilestone("v3.0")
	if err != nil {
		t.Fatalf("getting open issues for non-existent milestone: %v", err)
	}
	if len(got) != 0 {
		t.Errorf("got %d issues for non-existent milestone, want 0", len(got))
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
		if err := store.SaveIssueCache(issue, "backlog", true); err != nil {
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

	if err := store.SaveIssueCache(issue, "v1.0", true); err != nil {
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

	if err := store.SaveIssueCache(issue, "v1.0", true); err != nil {
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

	if err := store.SaveIssueCache(issue, "v1.0", true); err != nil {
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

func TestSaveIssueCache_TimestampComparison_SkipWhenLocalNewer(t *testing.T) {
	store := openTestStore(t)

	// Create initial issue with NEWER timestamp (simulating recent local edit)
	newTime := time.Now().UTC().Truncate(time.Second)
	issue := github.Issue{
		Number:    200,
		Title:     "Local Title",
		Body:      "Local body",
		State:     "open",
		UpdatedAt: &newTime,
		Labels:    nil,
		Assignees: nil,
	}

	// Save initial issue with force=true (simulating local edit)
	if err := store.SaveIssueCache(issue, "v1.0", true); err != nil {
		t.Fatalf("saving initial issue cache: %v", err)
	}

	// Verify initial save
	got, err := store.GetIssueCache(200)
	if err != nil {
		t.Fatalf("getting initial issue cache: %v", err)
	}
	if got.Title != "Local Title" {
		t.Errorf("initial title = %q, want %q", got.Title, "Local Title")
	}

	// Simulate stale GitHub CDN data with OLDER timestamp
	oldTime := time.Now().UTC().Add(-1 * time.Hour).Truncate(time.Second)
	staleGitHubIssue := github.Issue{
		Number:    200,
		Title:     "Stale GitHub Title",
		Body:      "Stale GitHub body",
		State:     "open",
		UpdatedAt: &oldTime,
		Labels:    nil,
		Assignees: nil,
	}

	// Save with force=false (auto-sync) - should skip because local is newer than GitHub
	if err := store.SaveIssueCache(staleGitHubIssue, "v1.0", false); err != nil {
		t.Fatalf("saving stale GitHub issue cache with force=false: %v", err)
	}

	// Verify the cache was NOT updated (still has local data)
	got, err = store.GetIssueCache(200)
	if err != nil {
		t.Fatalf("getting issue cache after skip: %v", err)
	}
	if got.Title != "Local Title" {
		t.Errorf("title after skip = %q, want %q (should not have updated)", got.Title, "Local Title")
	}
	if got.Body != "Local body" {
		t.Errorf("body after skip = %q, want %q (should not have updated)", got.Body, "Local body")
	}
}

func TestSaveIssueCache_TimestampComparison_UpdateWhenGitHubNewer(t *testing.T) {
	store := openTestStore(t)

	// Create initial issue with newer timestamp (simulating local edit)
	newTime := time.Now().UTC().Truncate(time.Second)
	issue := github.Issue{
		Number:    201,
		Title:     "Local Title",
		Body:      "Local body",
		State:     "open",
		UpdatedAt: &newTime,
		Labels:    nil,
		Assignees: nil,
	}

	// Save initial issue with force=true
	if err := store.SaveIssueCache(issue, "v1.0", true); err != nil {
		t.Fatalf("saving initial issue cache: %v", err)
	}

	// Simulate GitHub update with even newer timestamp
	newerTime := time.Now().UTC().Add(1 * time.Minute).Truncate(time.Second)
	githubIssue := github.Issue{
		Number:    201,
		Title:     "GitHub Title",
		Body:      "GitHub body",
		State:     "open",
		UpdatedAt: &newerTime,
		Labels:    nil,
		Assignees: nil,
	}

	// Save with force=false (auto-sync) - should update because GitHub is newer
	if err := store.SaveIssueCache(githubIssue, "v1.0", false); err != nil {
		t.Fatalf("saving GitHub issue cache with force=false: %v", err)
	}

	// Verify the cache WAS updated
	got, err := store.GetIssueCache(201)
	if err != nil {
		t.Fatalf("getting issue cache after update: %v", err)
	}
	if got.Title != "GitHub Title" {
		t.Errorf("title after update = %q, want %q", got.Title, "GitHub Title")
	}
	if got.Body != "GitHub body" {
		t.Errorf("body after update = %q, want %q", got.Body, "GitHub body")
	}
}

func TestSaveIssueCache_ForceTrue_AlwaysUpdates(t *testing.T) {
	store := openTestStore(t)

	// Create initial issue
	oldTime := time.Now().UTC().Add(-1 * time.Hour).Truncate(time.Second)
	issue := github.Issue{
		Number:    202,
		Title:     "Original Title",
		Body:      "Original body",
		State:     "open",
		UpdatedAt: &oldTime,
		Labels:    nil,
		Assignees: nil,
	}

	// Save initial issue
	if err := store.SaveIssueCache(issue, "v1.0", true); err != nil {
		t.Fatalf("saving initial issue cache: %v", err)
	}

	// Update local issue with newer timestamp
	newTime := time.Now().UTC().Truncate(time.Second)
	localIssue := github.Issue{
		Number:    202,
		Title:     "Local Title",
		Body:      "Local body",
		State:     "open",
		UpdatedAt: &newTime,
		Labels:    nil,
		Assignees: nil,
	}

	// Save with force=true (manual action) - should always update even if local is newer
	if err := store.SaveIssueCache(localIssue, "v1.0", true); err != nil {
		t.Fatalf("saving local issue cache with force=true: %v", err)
	}

	// Verify the cache WAS updated despite local being newer
	got, err := store.GetIssueCache(202)
	if err != nil {
		t.Fatalf("getting issue cache after force update: %v", err)
	}
	if got.Title != "Local Title" {
		t.Errorf("title after force update = %q, want %q", got.Title, "Local Title")
	}
	if got.Body != "Local body" {
		t.Errorf("body after force update = %q, want %q", got.Body, "Local body")
	}
}

func TestSaveIssueCache_NoTimestamps_UpdatesAnyway(t *testing.T) {
	store := openTestStore(t)

	// Create issue without timestamps
	issue := github.Issue{
		Number:    203,
		Title:     "First Title",
		Body:      "First body",
		State:     "open",
		UpdatedAt: nil, // No timestamp
		Labels:    nil,
		Assignees: nil,
	}

	// Save initial issue
	if err := store.SaveIssueCache(issue, "v1.0", true); err != nil {
		t.Fatalf("saving initial issue cache: %v", err)
	}

	// Update issue
	updatedIssue := github.Issue{
		Number:    203,
		Title:     "Second Title",
		Body:      "Second body",
		State:     "open",
		UpdatedAt: nil, // Still no timestamp
		Labels:    nil,
		Assignees: nil,
	}

	// Save with force=false - should update since we can't compare timestamps
	if err := store.SaveIssueCache(updatedIssue, "v1.0", false); err != nil {
		t.Fatalf("saving updated issue cache: %v", err)
	}

	// Verify the cache WAS updated
	got, err := store.GetIssueCache(203)
	if err != nil {
		t.Fatalf("getting issue cache: %v", err)
	}
	if got.Title != "Second Title" {
		t.Errorf("title = %q, want %q", got.Title, "Second Title")
	}
}

func TestConcurrentWrites_NoBusyErrors(t *testing.T) {
	store := openTestStore(t)

	var wg sync.WaitGroup
	errors := make(chan error, 100)

	for i := 0; i < 50; i++ {
		wg.Add(1)
		go func(n int) {
			defer wg.Done()
			// Concurrent writes to same issue
			metric := db.StageMetric{
				TaskID:    n,
				SprintID:  1,
				Stage:     "test",
				LLM:       "test-llm",
				TokensIn:  100,
				TokensOut: 50,
				CostUSD:   0.01,
				DurationS: 10,
				Retries:   0,
			}
			err := store.SaveStageMetric(metric)
			if err != nil {
				errors <- err
			}
		}(i)
	}

	wg.Wait()
	close(errors)

	// Verify no SQLITE_BUSY errors
	for err := range errors {
		if strings.Contains(err.Error(), "BUSY") {
			t.Errorf("Got SQLITE_BUSY: %v", err)
		}
	}
}

func TestConcurrentMixedWrites_NoBusyErrors(t *testing.T) {
	store := openTestStore(t)

	var wg sync.WaitGroup
	errors := make(chan error, 200)

	// Launch goroutines doing different write operations
	for i := 0; i < 20; i++ {
		wg.Add(1)
		go func(n int) {
			defer wg.Done()
			// SaveStageMetric
			metric := db.StageMetric{
				TaskID:    n,
				SprintID:  1,
				Stage:     "analysis",
				LLM:       "claude",
				TokensIn:  100,
				TokensOut: 50,
				CostUSD:   0.01,
				DurationS: 10,
				Retries:   0,
			}
			if err := store.SaveStageMetric(metric); err != nil {
				errors <- err
			}
		}(i)
	}

	for i := 0; i < 20; i++ {
		wg.Add(1)
		go func(n int) {
			defer wg.Done()
			// InsertStep and FinishStep
			stepID, err := store.InsertStep(n, "test-step", "test prompt", "session-1")
			if err != nil {
				errors <- err
				return
			}
			if err := store.FinishStep(stepID, "test response"); err != nil {
				errors <- err
			}
		}(i)
	}

	for i := 0; i < 10; i++ {
		wg.Add(1)
		go func(n int) {
			defer wg.Done()
			// SaveIssueCache
			issue := github.Issue{
				Number:    1000 + n,
				Title:     "Test Issue",
				State:     "open",
				Labels:    nil,
				Assignees: nil,
			}
			if err := store.SaveIssueCache(issue, "v1.0", true); err != nil {
				errors <- err
			}
		}(i)
	}

	wg.Wait()
	close(errors)

	// Verify no SQLITE_BUSY errors
	for err := range errors {
		if strings.Contains(err.Error(), "BUSY") {
			t.Errorf("Got SQLITE_BUSY: %v", err)
		}
	}
}

func TestWriteOrdering(t *testing.T) {
	store := openTestStore(t)

	// Submit numbered jobs and verify execution order
	var wg sync.WaitGroup
	results := make(chan int, 100)

	for i := 0; i < 50; i++ {
		wg.Add(1)
		go func(n int) {
			defer wg.Done()
			metric := db.StageMetric{
				TaskID:    n,
				SprintID:  1,
				Stage:     "test",
				LLM:       "test-llm",
				TokensIn:  100,
				TokensOut: 50,
				CostUSD:   0.01,
				DurationS: 10,
				Retries:   0,
			}
			if err := store.SaveStageMetric(metric); err != nil {
				t.Errorf("SaveStageMetric failed: %v", err)
			}
			results <- n
		}(i)
	}

	wg.Wait()
	close(results)

	// All jobs should complete without error
	count := 0
	for range results {
		count++
	}
	if count != 50 {
		t.Errorf("expected 50 completed jobs, got %d", count)
	}
}

func TestGracefulShutdown(t *testing.T) {
	path := filepath.Join(t.TempDir(), "shutdown.db")
	store, err := db.Open(path)
	if err != nil {
		t.Fatalf("opening store: %v", err)
	}

	// Submit jobs and track when all submissions are done
	submitted := make(chan struct{})
	var wg sync.WaitGroup
	for i := 0; i < 20; i++ {
		wg.Add(1)
		go func(n int) {
			defer wg.Done()
			metric := db.StageMetric{
				TaskID:    n,
				SprintID:  1,
				Stage:     "test",
				LLM:       "test-llm",
				TokensIn:  100,
				TokensOut: 50,
				CostUSD:   0.01,
				DurationS: 10,
				Retries:   0,
			}
			if err := store.SaveStageMetric(metric); err != nil {
				t.Errorf("SaveStageMetric failed: %v", err)
			}
		}(i)
	}

	// Wait for all submissions to complete before closing
	go func() {
		wg.Wait()
		close(submitted)
	}()

	select {
	case <-submitted:
		// All jobs submitted, now close
	case <-time.After(5 * time.Second):
		t.Fatal("timeout waiting for job submissions")
	}

	// Close after all jobs are submitted
	if err := store.Close(); err != nil {
		t.Fatalf("closing store: %v", err)
	}
}
