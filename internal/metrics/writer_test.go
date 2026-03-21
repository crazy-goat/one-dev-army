package metrics_test

import (
	"path/filepath"
	"strings"
	"testing"

	"github.com/crazy-goat/one-dev-army/internal/db"
	"github.com/crazy-goat/one-dev-army/internal/metrics"
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

func TestFormatMetricsYAML(t *testing.T) {
	input := []db.StageMetric{
		{
			TaskID:    123,
			SprintID:  1,
			Stage:     "analysis",
			LLM:       "claude-sonnet-4",
			TokensIn:  1200,
			TokensOut: 800,
			CostUSD:   0.012,
			DurationS: 15,
			Retries:   0,
		},
		{
			TaskID:    123,
			SprintID:  1,
			Stage:     "planning",
			LLM:       "claude-opus-4",
			TokensIn:  2500,
			TokensOut: 1500,
			CostUSD:   0.045,
			DurationS: 32,
			Retries:   0,
		},
	}

	got := metrics.FormatMetricsYAML(123, input)

	want := strings.Join([]string{
		"# ODA Metrics",
		"task_id: 123",
		"stage_metrics:",
		"  analysis:",
		"    llm: claude-sonnet-4",
		"    tokens_in: 1200",
		"    tokens_out: 800",
		"    cost_usd: 0.012",
		"    duration_s: 15",
		"  planning:",
		"    llm: claude-opus-4",
		"    tokens_in: 2500",
		"    tokens_out: 1500",
		"    cost_usd: 0.045",
		"    duration_s: 32",
		"total:",
		"  tokens: 6000",
		"  cost_usd: 0.057",
		"  duration_s: 47",
		"  retries: 0",
	}, "\n")

	if got != want {
		t.Errorf("FormatMetricsYAML mismatch\ngot:\n%s\n\nwant:\n%s", got, want)
	}
}

func TestFormatMetricsYAML_Empty(t *testing.T) {
	got := metrics.FormatMetricsYAML(1, nil)

	want := strings.Join([]string{
		"# ODA Metrics",
		"task_id: 1",
		"stage_metrics:",
		"total:",
		"  tokens: 0",
		"  cost_usd: 0",
		"  duration_s: 0",
		"  retries: 0",
	}, "\n")

	if got != want {
		t.Errorf("FormatMetricsYAML empty mismatch\ngot:\n%s\n\nwant:\n%s", got, want)
	}
}

func TestFormatMetricsYAML_Retries(t *testing.T) {
	input := []db.StageMetric{
		{TaskID: 5, Stage: "coding", LLM: "claude-sonnet-4", TokensIn: 100, TokensOut: 50, CostUSD: 0.01, DurationS: 10, Retries: 2},
		{TaskID: 5, Stage: "testing", LLM: "claude-sonnet-4", TokensIn: 200, TokensOut: 100, CostUSD: 0.02, DurationS: 20, Retries: 1},
	}

	got := metrics.FormatMetricsYAML(5, input)

	if !strings.Contains(got, "retries: 3") {
		t.Errorf("expected total retries: 3 in output:\n%s", got)
	}
}

func TestWriteStageMetric(t *testing.T) {
	store := openTestStore(t)
	w := metrics.NewWriter(store, nil)

	m := db.StageMetric{
		TaskID:    42,
		SprintID:  1,
		Stage:     "analysis",
		LLM:       "claude-sonnet-4",
		TokensIn:  1000,
		TokensOut: 500,
		CostUSD:   0.015,
		DurationS: 30,
		Retries:   0,
	}

	if err := w.WriteStageMetric(m); err != nil {
		t.Fatalf("WriteStageMetric: %v", err)
	}

	got, err := store.GetTaskMetrics(42)
	if err != nil {
		t.Fatalf("GetTaskMetrics: %v", err)
	}
	if len(got) != 1 {
		t.Fatalf("got %d metrics, want 1", len(got))
	}
	if got[0].Stage != "analysis" {
		t.Errorf("stage = %q, want %q", got[0].Stage, "analysis")
	}
	if got[0].TokensIn != 1000 {
		t.Errorf("tokens_in = %d, want 1000", got[0].TokensIn)
	}
}

func TestWriteTaskSummary(t *testing.T) {
	store := openTestStore(t)

	input := []db.StageMetric{
		{TaskID: 10, SprintID: 1, Stage: "analysis", LLM: "claude-sonnet-4", TokensIn: 500, TokensOut: 300, CostUSD: 0.008, DurationS: 12, Retries: 0},
		{TaskID: 10, SprintID: 1, Stage: "coding", LLM: "claude-sonnet-4", TokensIn: 1500, TokensOut: 1000, CostUSD: 0.025, DurationS: 45, Retries: 1},
	}
	for _, m := range input {
		if err := store.SaveStageMetric(m); err != nil {
			t.Fatalf("saving metric: %v", err)
		}
	}

	got, err := store.GetTaskMetrics(10)
	if err != nil {
		t.Fatalf("GetTaskMetrics: %v", err)
	}
	if len(got) != 2 {
		t.Fatalf("got %d metrics, want 2", len(got))
	}

	yaml := metrics.FormatMetricsYAML(10, got)

	if !strings.Contains(yaml, "task_id: 10") {
		t.Error("missing task_id in YAML")
	}
	if !strings.Contains(yaml, "analysis:") {
		t.Error("missing analysis stage in YAML")
	}
	if !strings.Contains(yaml, "coding:") {
		t.Error("missing coding stage in YAML")
	}
	if !strings.Contains(yaml, "tokens: 3300") {
		t.Errorf("expected total tokens: 3300 in output:\n%s", yaml)
	}
	if !strings.Contains(yaml, "retries: 1") {
		t.Errorf("expected total retries: 1 in output:\n%s", yaml)
	}
}
