package mvp

// UserDecision represents a user's approve/decline decision from the dashboard.
type UserDecision struct {
	Action string // "approve" or "decline"
	Reason string // decline reason (empty for approve)
}
