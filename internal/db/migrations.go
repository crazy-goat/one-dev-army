package db

import "database/sql"

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
}

func migrate(db *sql.DB) error {
	for _, m := range migrations {
		if _, err := db.Exec(m); err != nil {
			return err
		}
	}
	return nil
}
