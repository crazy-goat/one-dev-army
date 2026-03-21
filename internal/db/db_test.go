package db_test

import (
	"math"
	"path/filepath"
	"testing"

	"github.com/crazy-goat/one-dev-army/internal/db"
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
