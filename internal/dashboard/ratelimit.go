package dashboard

import (
	"encoding/json"
	"fmt"
	"net/http"
	"sync"
	"time"
)

// RateLimitInfo holds the rate limit data from GitHub API
type RateLimitInfo struct {
	Limit     int       `json:"limit"`
	Remaining int       `json:"remaining"`
	Reset     int64     `json:"reset"`
	UpdatedAt time.Time `json:"updated_at"`
	Error     string    `json:"error,omitempty"`
}

// GetColor returns the color code based on remaining requests
// Green (>1000), Yellow (500-1000), Red (<500)
func (r *RateLimitInfo) GetColor() string {
	if r.Remaining < 500 {
		return "red"
	}
	if r.Remaining < 1000 {
		return "yellow"
	}
	return "green"
}

// GetColorCSS returns the CSS color variable based on remaining requests
func (r *RateLimitInfo) GetColorCSS() string {
	color := r.GetColor()
	switch color {
	case "red":
		return "var(--red)"
	case "yellow":
		return "var(--orange)"
	default:
		return "var(--green)"
	}
}

// GetResetTimeFormatted returns the reset time in a human-readable format
func (r *RateLimitInfo) GetResetTimeFormatted() string {
	if r.Reset == 0 {
		return "Unknown"
	}

	resetTime := time.Unix(r.Reset, 0)
	duration := time.Until(resetTime)

	if duration <= 0 {
		return "Resets soon"
	}

	minutes := int(duration.Minutes())
	if minutes < 1 {
		return "Resets in <1 min"
	}
	if minutes < 60 {
		return fmt.Sprintf("Resets in %d min", minutes)
	}

	hours := minutes / 60
	remainingMinutes := minutes % 60
	if remainingMinutes == 0 {
		return fmt.Sprintf("Resets in %d hr", hours)
	}
	return fmt.Sprintf("Resets in %d hr %d min", hours, remainingMinutes)
}

// RateLimitService manages fetching and caching GitHub API rate limit data
type RateLimitService struct {
	mu       sync.RWMutex
	data     *RateLimitInfo
	client   *http.Client
	token    string
	interval time.Duration
	stopCh   chan struct{}
	stopped  bool
}

// NewRateLimitService creates a new rate limit service
func NewRateLimitService(token string) *RateLimitService {
	return &RateLimitService{
		client:   &http.Client{Timeout: 10 * time.Second},
		token:    token,
		interval: 30 * time.Second,
		stopCh:   make(chan struct{}),
		data: &RateLimitInfo{
			Limit:     0,
			Remaining: 0,
			Reset:     0,
			UpdatedAt: time.Time{},
			Error:     "Initializing...",
		},
	}
}

// Start begins the background polling goroutine
func (s *RateLimitService) Start() {
	// Do an initial fetch immediately
	s.fetch()

	// Start background polling
	go s.poll()
}

// Stop halts the background polling
func (s *RateLimitService) Stop() {
	s.mu.Lock()
	if s.stopped {
		s.mu.Unlock()
		return
	}
	s.stopped = true
	s.mu.Unlock()

	close(s.stopCh)
}

// poll runs the background polling loop
func (s *RateLimitService) poll() {
	ticker := time.NewTicker(s.interval)
	defer ticker.Stop()

	for {
		select {
		case <-ticker.C:
			s.fetch()
		case <-s.stopCh:
			return
		}
	}
}

// fetch retrieves the current rate limit from GitHub API
func (s *RateLimitService) fetch() {
	if s.token == "" {
		s.updateWithError("No GitHub token configured")
		return
	}

	req, err := http.NewRequest("GET", "https://api.github.com/rate_limit", nil)
	if err != nil {
		s.updateWithError(fmt.Sprintf("Request error: %v", err))
		return
	}

	req.Header.Set("Authorization", "Bearer "+s.token)
	req.Header.Set("Accept", "application/vnd.github.v3+json")

	resp, err := s.client.Do(req)
	if err != nil {
		s.updateWithError(fmt.Sprintf("API error: %v", err))
		return
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		s.updateWithError(fmt.Sprintf("HTTP %d: %s", resp.StatusCode, resp.Status))
		return
	}

	var result struct {
		Resources struct {
			Core struct {
				Limit     int   `json:"limit"`
				Remaining int   `json:"remaining"`
				Reset     int64 `json:"reset"`
			} `json:"core"`
		} `json:"resources"`
	}

	if err := json.NewDecoder(resp.Body).Decode(&result); err != nil {
		s.updateWithError(fmt.Sprintf("Parse error: %v", err))
		return
	}

	s.mu.Lock()
	s.data = &RateLimitInfo{
		Limit:     result.Resources.Core.Limit,
		Remaining: result.Resources.Core.Remaining,
		Reset:     result.Resources.Core.Reset,
		UpdatedAt: time.Now(),
		Error:     "",
	}
	s.mu.Unlock()
}

// updateWithError updates the data with an error message while preserving last known values
func (s *RateLimitService) updateWithError(errMsg string) {
	s.mu.Lock()
	// Keep the last known values but mark the error
	// If we have no valid data yet, show the error
	if s.data.UpdatedAt.IsZero() {
		s.data.Error = errMsg
	} else {
		// We have cached data, just update the error field
		// The UI will show cached values with a warning indicator
		s.data.Error = errMsg
	}
	s.mu.Unlock()
}

// GetData returns the current rate limit data (thread-safe)
func (s *RateLimitService) GetData() *RateLimitInfo {
	s.mu.RLock()
	defer s.mu.RUnlock()

	// Return a copy to prevent external modification
	data := *s.data
	return &data
}

// Refresh triggers an immediate refresh of the rate limit data
func (s *RateLimitService) Refresh() {
	go s.fetch()
}
