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
}

func migrate(db *sql.DB) error {
	for _, m := range migrations {
		if _, err := db.Exec(m); err != nil {
			return err
		}
	}
	return nil
}
