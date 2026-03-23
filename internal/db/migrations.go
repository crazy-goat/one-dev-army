package db

import (
	"database/sql"
	"strings"
)

var migrations = []string{
	`CREATE TABLE IF NOT EXISTS stage_metrics (
		id         INTEGER PRIMARY KEY AUTOINCREMENT,
		task_id    INTEGER NOT NULL,
		sprint_id  INTEGER NOT NULL,
		stage      TEXT    NOT NULL,
		llm        TEXT    NOT NULL,
		tokens_in  INTEGER NOT NULL,
		tokens_out INTEGER NOT NULL,
		cost_usd   REAL    NOT NULL,
		duration_s INTEGER NOT NULL,
		retries    INTEGER NOT NULL
	)`,
	`CREATE INDEX IF NOT EXISTS idx_stage_metrics_task_id ON stage_metrics(task_id)`,
	`CREATE INDEX IF NOT EXISTS idx_stage_metrics_sprint_id ON stage_metrics(sprint_id)`,
	`CREATE TABLE IF NOT EXISTS task_steps (
		id           INTEGER PRIMARY KEY AUTOINCREMENT,
		issue_number INTEGER NOT NULL,
		step_name    TEXT    NOT NULL,
		status       TEXT    NOT NULL DEFAULT 'pending',
		prompt       TEXT    NOT NULL DEFAULT '',
		response     TEXT    NOT NULL DEFAULT '',
		error_msg    TEXT    NOT NULL DEFAULT '',
		session_id   TEXT    NOT NULL DEFAULT '',
		started_at   DATETIME,
		finished_at  DATETIME
	)`,
	`CREATE INDEX IF NOT EXISTS idx_task_steps_issue ON task_steps(issue_number)`,
	`ALTER TABLE task_steps ADD COLUMN plan_attachment_url TEXT NOT NULL DEFAULT ''`,
	`CREATE TABLE IF NOT EXISTS issue_cache (
		issue_number INTEGER PRIMARY KEY,
		title TEXT NOT NULL,
		body TEXT,
		state TEXT NOT NULL,
		labels TEXT,
		assignee TEXT,
		milestone TEXT,
		updated_at DATETIME,
		cached_at DATETIME DEFAULT CURRENT_TIMESTAMP
	)`,
	`CREATE INDEX IF NOT EXISTS idx_issue_cache_state ON issue_cache(state)`,
	`CREATE INDEX IF NOT EXISTS idx_issue_cache_cached_at ON issue_cache(cached_at)`,
	`CREATE INDEX IF NOT EXISTS idx_issue_cache_milestone ON issue_cache(milestone)`,
}

// columnExists checks if a column exists in a table
func columnExists(db *sql.DB, table, column string) bool {
	var count int
	err := db.QueryRow(
		`SELECT COUNT(*) FROM pragma_table_info(?) WHERE name = ?`,
		table, column,
	).Scan(&count)
	if err != nil {
		return false
	}
	return count > 0
}

func migrate(db *sql.DB) error {
	for _, m := range migrations {
		// Special handling for the plan_attachment_url migration
		if strings.Contains(m, "plan_attachment_url") {
			if columnExists(db, "task_steps", "plan_attachment_url") {
				continue // Skip if column already exists
			}
		}
		if _, err := db.Exec(m); err != nil {
			return err
		}
	}
	return nil
}
