package github

// SprintDetector provides methods for detecting and working with sprints
// This is a thin wrapper around existing milestone functionality
type SprintDetector struct {
	client *Client
}

// NewSprintDetector creates a new SprintDetector for the given client
func NewSprintDetector(client *Client) *SprintDetector {
	return &SprintDetector{client: client}
}

// GetCurrentSprint returns the oldest open milestone (considered the current/active sprint)
// Returns nil if no open milestones exist
func (s *SprintDetector) GetCurrentSprint() (*Milestone, error) {
	return s.client.GetOldestOpenMilestone()
}

// GetCurrentSprintTitle returns the title of the current sprint or empty string if none exists
func (s *SprintDetector) GetCurrentSprintTitle() (string, error) {
	milestone, err := s.GetCurrentSprint()
	if err != nil {
		return "", err
	}
	if milestone == nil {
		return "", nil
	}
	return milestone.Title, nil
}
