package metrics

import (
	"fmt"
	"strings"

	"github.com/one-dev-army/oda/internal/db"
	"github.com/one-dev-army/oda/internal/github"
)

type Writer struct {
	store *db.Store
	gh    *github.Client
}

func NewWriter(store *db.Store, gh *github.Client) *Writer {
	return &Writer{store: store, gh: gh}
}

func (w *Writer) WriteStageMetric(m db.StageMetric) error {
	return w.store.SaveStageMetric(m)
}

func (w *Writer) WriteTaskSummary(taskID, issueNumber, sprintID int) error {
	metrics, err := w.store.GetTaskMetrics(taskID)
	if err != nil {
		return fmt.Errorf("reading task metrics: %w", err)
	}
	body := FormatMetricsYAML(taskID, metrics)
	if err := w.gh.AddComment(issueNumber, "```yaml\n"+body+"\n```"); err != nil {
		return fmt.Errorf("posting task summary: %w", err)
	}
	return nil
}

func (w *Writer) WriteInsights(issueNumber int, insights string) error {
	return w.gh.AddComment(issueNumber, insights)
}

func FormatMetricsYAML(taskID int, metrics []db.StageMetric) string {
	var b strings.Builder

	b.WriteString("# ODA Metrics\n")
	fmt.Fprintf(&b, "task_id: %d\n", taskID)
	b.WriteString("stage_metrics:\n")

	var totalTokens int
	var totalCost float64
	var totalDuration int
	var totalRetries int

	for _, m := range metrics {
		fmt.Fprintf(&b, "  %s:\n", m.Stage)
		fmt.Fprintf(&b, "    llm: %s\n", m.LLM)
		fmt.Fprintf(&b, "    tokens_in: %d\n", m.TokensIn)
		fmt.Fprintf(&b, "    tokens_out: %d\n", m.TokensOut)
		fmt.Fprintf(&b, "    cost_usd: %s\n", formatCost(m.CostUSD))
		fmt.Fprintf(&b, "    duration_s: %d\n", m.DurationS)

		totalTokens += m.TokensIn + m.TokensOut
		totalCost += m.CostUSD
		totalDuration += m.DurationS
		totalRetries += m.Retries
	}

	b.WriteString("total:\n")
	fmt.Fprintf(&b, "  tokens: %d\n", totalTokens)
	fmt.Fprintf(&b, "  cost_usd: %s\n", formatCost(totalCost))
	fmt.Fprintf(&b, "  duration_s: %d\n", totalDuration)
	fmt.Fprintf(&b, "  retries: %d", totalRetries)

	return b.String()
}

func formatCost(v float64) string {
	s := fmt.Sprintf("%.6f", v)
	s = strings.TrimRight(s, "0")
	s = strings.TrimRight(s, ".")
	return s
}
