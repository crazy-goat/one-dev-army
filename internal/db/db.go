package db

import (
	"database/sql"
	"encoding/json"
	"fmt"
	"log"
	"time"

	"github.com/crazy-goat/one-dev-army/internal/github"
	_ "modernc.org/sqlite"
)

type StageMetric struct {
	ID        int64
	TaskID    int
	SprintID  int
	Stage     string
	LLM       string
	TokensIn  int
	TokensOut int
	CostUSD   float64
	DurationS int
	Retries   int
}

type Store struct {
	db *sql.DB
}

func Open(path string) (*Store, error) {
	db, err := sql.Open("sqlite", path)
	if err != nil {
		return nil, fmt.Errorf("opening database: %w", err)
	}

	pragmas := []string{
		"PRAGMA journal_mode=WAL",
		"PRAGMA foreign_keys=ON",
	}
	for _, p := range pragmas {
		if _, err := db.Exec(p); err != nil {
			db.Close()
			return nil, fmt.Errorf("setting pragma %q: %w", p, err)
		}
	}

	if err := migrate(db); err != nil {
		db.Close()
		return nil, fmt.Errorf("running migrations: %w", err)
	}

	return &Store{db: db}, nil
}

func (s *Store) Close() error {
	return s.db.Close()
}

func (s *Store) SaveStageMetric(m StageMetric) error {
	_, err := s.db.Exec(
		`INSERT INTO stage_metrics (task_id, sprint_id, stage, llm, tokens_in, tokens_out, cost_usd, duration_s, retries)
		 VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?)`,
		m.TaskID, m.SprintID, m.Stage, m.LLM, m.TokensIn, m.TokensOut, m.CostUSD, m.DurationS, m.Retries,
	)
	if err != nil {
		return fmt.Errorf("inserting stage metric: %w", err)
	}
	return nil
}

func (s *Store) GetTaskMetrics(taskID int) ([]StageMetric, error) {
	rows, err := s.db.Query(
		`SELECT id, task_id, sprint_id, stage, llm, tokens_in, tokens_out, cost_usd, duration_s, retries
		 FROM stage_metrics WHERE task_id = ? ORDER BY id`, taskID,
	)
	if err != nil {
		return nil, fmt.Errorf("querying task metrics: %w", err)
	}
	defer rows.Close()

	var metrics []StageMetric
	for rows.Next() {
		var m StageMetric
		if err := rows.Scan(&m.ID, &m.TaskID, &m.SprintID, &m.Stage, &m.LLM, &m.TokensIn, &m.TokensOut, &m.CostUSD, &m.DurationS, &m.Retries); err != nil {
			return nil, fmt.Errorf("scanning stage metric: %w", err)
		}
		metrics = append(metrics, m)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("iterating task metrics: %w", err)
	}
	return metrics, nil
}

type TaskStep struct {
	ID                int64
	IssueNumber       int
	StepName          string
	Status            string
	Prompt            string
	Response          string
	ErrorMsg          string
	SessionID         string
	PlanAttachmentURL string
	StartedAt         *time.Time
	FinishedAt        *time.Time
}

type IssueCache struct {
	IssueNumber int
	Title       string
	Body        string
	State       string
	Labels      string // JSON array
	Assignee    string
	Milestone   string
	UpdatedAt   *time.Time
	CachedAt    time.Time
	PRMerged    bool
	MergedAt    *time.Time
}

func (s *Store) InsertStep(issueNumber int, stepName, prompt, sessionID string) (int64, error) {
	now := time.Now()
	res, err := s.db.Exec(
		`INSERT INTO task_steps (issue_number, step_name, status, prompt, session_id, started_at)
		 VALUES (?, ?, 'running', ?, ?, ?)`,
		issueNumber, stepName, prompt, sessionID, now,
	)
	if err != nil {
		return 0, fmt.Errorf("inserting task step: %w", err)
	}
	return res.LastInsertId()
}

func (s *Store) FinishStep(id int64, response string) error {
	now := time.Now()
	_, err := s.db.Exec(
		`UPDATE task_steps SET status = 'done', response = ?, finished_at = ? WHERE id = ?`,
		response, now, id,
	)
	if err != nil {
		return fmt.Errorf("finishing task step: %w", err)
	}
	return nil
}

func (s *Store) FailStep(id int64, errMsg string) error {
	now := time.Now()
	_, err := s.db.Exec(
		`UPDATE task_steps SET status = 'failed', error_msg = ?, finished_at = ? WHERE id = ?`,
		errMsg, now, id,
	)
	if err != nil {
		return fmt.Errorf("failing task step: %w", err)
	}
	return nil
}

func (s *Store) GetSteps(issueNumber int) ([]TaskStep, error) {
	rows, err := s.db.Query(
		`SELECT id, issue_number, step_name, status, prompt, response, error_msg, session_id, plan_attachment_url, started_at, finished_at
		 FROM task_steps WHERE issue_number = ? ORDER BY id`, issueNumber,
	)
	if err != nil {
		return nil, fmt.Errorf("querying task steps: %w", err)
	}
	defer rows.Close()

	var steps []TaskStep
	for rows.Next() {
		var st TaskStep
		if err := rows.Scan(&st.ID, &st.IssueNumber, &st.StepName, &st.Status, &st.Prompt, &st.Response, &st.ErrorMsg, &st.SessionID, &st.PlanAttachmentURL, &st.StartedAt, &st.FinishedAt); err != nil {
			return nil, fmt.Errorf("scanning task step: %w", err)
		}
		steps = append(steps, st)
	}
	return steps, rows.Err()
}

func (s *Store) GetLastCompletedStep(issueNumber int) (string, error) {
	var stepName sql.NullString
	err := s.db.QueryRow(
		`SELECT step_name FROM task_steps WHERE issue_number = ? AND status = 'done' ORDER BY id DESC LIMIT 1`,
		issueNumber,
	).Scan(&stepName)
	if err == sql.ErrNoRows || !stepName.Valid {
		return "", nil
	}
	if err != nil {
		return "", fmt.Errorf("querying last completed step: %w", err)
	}

	// Migration: map old step names to new "technical-planning" step
	if stepName.String == "analyze" || stepName.String == "plan" {
		return "technical-planning", nil
	}

	return stepName.String, nil
}

func (s *Store) GetStepResponse(issueNumber int, stepName string) (string, error) {
	var response sql.NullString
	err := s.db.QueryRow(
		`SELECT response FROM task_steps WHERE issue_number = ? AND step_name = ? AND status = 'done' ORDER BY id DESC LIMIT 1`,
		issueNumber, stepName,
	).Scan(&response)
	if err == sql.ErrNoRows || !response.Valid {
		// Migration: if "technical-planning" not found, try old "plan" step
		if stepName == "technical-planning" {
			return s.GetStepResponse(issueNumber, "plan")
		}
		return "", nil
	}
	if err != nil {
		return "", fmt.Errorf("querying step response: %w", err)
	}
	return response.String, nil
}

func (s *Store) DeleteSteps(issueNumber int) error {
	_, err := s.db.Exec(`DELETE FROM task_steps WHERE issue_number = ?`, issueNumber)
	if err != nil {
		return fmt.Errorf("deleting task steps: %w", err)
	}
	return nil
}

func (s *Store) GetSprintCost(sprintID int) (float64, error) {
	var cost sql.NullFloat64
	err := s.db.QueryRow(
		`SELECT SUM(cost_usd) FROM stage_metrics WHERE sprint_id = ?`, sprintID,
	).Scan(&cost)
	if err != nil {
		return 0, fmt.Errorf("querying sprint cost: %w", err)
	}
	if !cost.Valid {
		return 0, nil
	}
	return cost.Float64, nil
}

// UpdateStepPlanURL updates the plan_attachment_url for a specific step
func (s *Store) UpdateStepPlanURL(issueNumber int, stepName, planURL string) error {
	_, err := s.db.Exec(
		`UPDATE task_steps SET plan_attachment_url = ? WHERE issue_number = ? AND step_name = ? AND status = 'done'`,
		planURL, issueNumber, stepName,
	)
	if err != nil {
		return fmt.Errorf("updating step plan URL: %w", err)
	}
	return nil
}

// GetPlanAttachmentURL retrieves the plan_attachment_url for the most recent completed step
func (s *Store) GetPlanAttachmentURL(issueNumber int) (string, error) {
	var url sql.NullString
	err := s.db.QueryRow(
		`SELECT plan_attachment_url FROM task_steps WHERE issue_number = ? AND status = 'done' AND plan_attachment_url != '' ORDER BY id DESC LIMIT 1`,
		issueNumber,
	).Scan(&url)
	if err == sql.ErrNoRows || !url.Valid {
		return "", nil
	}
	if err != nil {
		return "", fmt.Errorf("querying plan attachment URL: %w", err)
	}
	return url.String, nil
}

// SaveIssueCache stores an issue in the cache
// When force=false (auto-sync), it compares timestamps and skips if local data is newer
// When force=true (manual actions), it always updates the cache
func (s *Store) SaveIssueCache(issue github.Issue, milestone string, force bool) error {
	// If not forcing, check if we should skip due to stale CDN data
	if !force {
		existing, err := s.GetIssueCache(issue.Number)
		if err == nil && existing.UpdatedAt != nil && issue.UpdatedAt != nil {
			// If local data is newer than GitHub data, skip the update
			if existing.UpdatedAt.After(*issue.UpdatedAt) {
				log.Printf("[DB] Skipping cache update for issue #%d: local data is newer (local: %v, GitHub: %v)",
					issue.Number, existing.UpdatedAt, issue.UpdatedAt)
				return nil
			}
		}
		// If error getting existing cache (not found), continue with save
	}

	labelsJSON, err := json.Marshal(issue.GetLabelNames())
	if err != nil {
		return fmt.Errorf("marshaling labels: %w", err)
	}

	_, err = s.db.Exec(
		`INSERT OR REPLACE INTO issue_cache (issue_number, title, body, state, labels, assignee, milestone, updated_at, cached_at, pr_merged, merged_at)
		 VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)`,
		issue.Number, issue.Title, issue.Body, issue.State, string(labelsJSON), issue.GetAssignee(), milestone, issue.UpdatedAt, time.Now(),
		issue.PRMerged, issue.MergedAt,
	)
	if err != nil {
		return fmt.Errorf("saving issue cache: %w", err)
	}
	return nil
}

// GetIssueCache retrieves a cached issue by number
func (s *Store) GetIssueCache(issueNumber int) (github.Issue, error) {
	var cache IssueCache
	var labelsJSON string
	var prMergedInt int
	err := s.db.QueryRow(
		`SELECT issue_number, title, body, state, labels, assignee, milestone, updated_at, cached_at, pr_merged, merged_at
		 FROM issue_cache WHERE issue_number = ?`,
		issueNumber,
	).Scan(&cache.IssueNumber, &cache.Title, &cache.Body, &cache.State, &labelsJSON, &cache.Assignee, &cache.Milestone, &cache.UpdatedAt, &cache.CachedAt, &prMergedInt, &cache.MergedAt)
	if err == sql.ErrNoRows {
		return github.Issue{}, fmt.Errorf("issue not found in cache: %d", issueNumber)
	}
	if err != nil {
		return github.Issue{}, fmt.Errorf("getting issue cache: %w", err)
	}

	cache.PRMerged = prMergedInt != 0

	var labelNames []string
	if labelsJSON != "" {
		if err := json.Unmarshal([]byte(labelsJSON), &labelNames); err != nil {
			return github.Issue{}, fmt.Errorf("unmarshaling labels: %w", err)
		}
	}

	labels := make([]struct {
		Name string `json:"name"`
	}, len(labelNames))
	for i, name := range labelNames {
		labels[i].Name = name
	}

	var assignees []struct {
		Login string `json:"login"`
	}
	if cache.Assignee != "" {
		assignees = append(assignees, struct {
			Login string `json:"login"`
		}{Login: cache.Assignee})
	}

	return github.Issue{
		Number:    cache.IssueNumber,
		Title:     cache.Title,
		Body:      cache.Body,
		State:     cache.State,
		Labels:    labels,
		Assignees: assignees,
		PRMerged:  cache.PRMerged,
		MergedAt:  cache.MergedAt,
		UpdatedAt: cache.UpdatedAt,
	}, nil
}

// GetIssuesCacheByMilestone retrieves all cached issues for a specific milestone
func (s *Store) GetIssuesCacheByMilestone(milestone string) ([]github.Issue, error) {
	rows, err := s.db.Query(
		`SELECT issue_number, title, body, state, labels, assignee, milestone, updated_at, cached_at, pr_merged, merged_at
		 FROM issue_cache WHERE milestone = ? ORDER BY issue_number`,
		milestone,
	)
	if err != nil {
		return nil, fmt.Errorf("querying issues by milestone: %w", err)
	}
	defer rows.Close()

	return s.scanIssues(rows)
}

// GetAllCachedIssues retrieves all cached issues
func (s *Store) GetAllCachedIssues() ([]github.Issue, error) {
	rows, err := s.db.Query(
		`SELECT issue_number, title, body, state, labels, assignee, milestone, updated_at, cached_at, pr_merged, merged_at
		 FROM issue_cache ORDER BY issue_number`,
	)
	if err != nil {
		return nil, fmt.Errorf("querying all cached issues: %w", err)
	}
	defer rows.Close()

	return s.scanIssues(rows)
}

// ClearIssueCache deletes all cached issues
func (s *Store) ClearIssueCache() error {
	_, err := s.db.Exec(`DELETE FROM issue_cache`)
	if err != nil {
		return fmt.Errorf("clearing issue cache: %w", err)
	}
	return nil
}

// scanIssues scans rows and converts them to github.Issue slice
func (s *Store) scanIssues(rows *sql.Rows) ([]github.Issue, error) {
	var issues []github.Issue
	for rows.Next() {
		var cache IssueCache
		var labelsJSON string
		var prMergedInt int
		if err := rows.Scan(&cache.IssueNumber, &cache.Title, &cache.Body, &cache.State, &labelsJSON, &cache.Assignee, &cache.Milestone, &cache.UpdatedAt, &cache.CachedAt, &prMergedInt, &cache.MergedAt); err != nil {
			return nil, fmt.Errorf("scanning issue cache: %w", err)
		}

		cache.PRMerged = prMergedInt != 0

		var labelNames []string
		if labelsJSON != "" {
			if err := json.Unmarshal([]byte(labelsJSON), &labelNames); err != nil {
				return nil, fmt.Errorf("unmarshaling labels: %w", err)
			}
		}

		labels := make([]struct {
			Name string `json:"name"`
		}, len(labelNames))
		for i, name := range labelNames {
			labels[i].Name = name
		}

		var assignees []struct {
			Login string `json:"login"`
		}
		if cache.Assignee != "" {
			assignees = append(assignees, struct {
				Login string `json:"login"`
			}{Login: cache.Assignee})
		}

		issues = append(issues, github.Issue{
			Number:    cache.IssueNumber,
			Title:     cache.Title,
			Body:      cache.Body,
			State:     cache.State,
			Labels:    labels,
			Assignees: assignees,
			PRMerged:  cache.PRMerged,
			MergedAt:  cache.MergedAt,
			UpdatedAt: cache.UpdatedAt,
		})
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("iterating issues: %w", err)
	}
	return issues, nil
}
