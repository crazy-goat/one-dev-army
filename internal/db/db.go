package db

import (
	"database/sql"
	"fmt"
	"time"

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
	return stepName.String, nil
}

func (s *Store) GetStepResponse(issueNumber int, stepName string) (string, error) {
	var response sql.NullString
	err := s.db.QueryRow(
		`SELECT response FROM task_steps WHERE issue_number = ? AND step_name = ? AND status = 'done' ORDER BY id DESC LIMIT 1`,
		issueNumber, stepName,
	).Scan(&response)
	if err == sql.ErrNoRows || !response.Valid {
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
