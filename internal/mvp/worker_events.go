package mvp

// WorkerEvent represents a message from worker to orchestrator
type WorkerEvent struct {
	IssueNumber int
	Stage       string      // Stage that was completed (e.g., "coding", "code-review")
	Status      EventStatus // success, failed, blocked
	Output      string      // Result/output from stage
}

type EventStatus string

const (
	EventSuccess EventStatus = "success"
	EventFailed  EventStatus = "failed"
	EventBlocked EventStatus = "blocked"
)
