package db

import (
	"database/sql"
	"fmt"

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
