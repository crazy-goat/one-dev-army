package dashboard

import (
	"encoding/json"
	"fmt"
	"net/http"
	"sync"
	"time"
)

// RateLimitInfo holds the rate limit data from GitHub API
// Deprecated: Use APILimit for individual limits or RateLimitSummary for all limits
type RateLimitInfo struct {
	Limit     int       `json:"limit"`
	Remaining int       `json:"remaining"`
	Reset     int64     `json:"reset"`
	UpdatedAt time.Time `json:"updated_at"`
	Error     string    `json:"error,omitempty"`
}

// APILimit represents an individual API rate limit type (core, graphql, search)
type APILimit struct {
	Name      string    `json:"name"`
	Limit     int       `json:"limit"`
	Remaining int       `json:"remaining"`
	Reset     int64     `json:"reset"`
	UpdatedAt time.Time `json:"updated_at"`
}

// GetUsagePercentage returns the percentage of API quota used
func (a *APILimit) GetUsagePercentage() float64 {
	if a.Limit == 0 {
		return 0
	}
	return float64(a.Limit-a.Remaining) / float64(a.Limit) * 100
}

// GetResetTimeFormatted returns the reset time in a human-readable format
func (a *APILimit) GetResetTimeFormatted() string {
	if a.Reset == 0 {
		return "Unknown"
	}

	resetTime := time.Unix(a.Reset, 0)
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

// RateLimitSummary holds all three API rate limit types
type RateLimitSummary struct {
	Core      *APILimit `json:"core"`
	GraphQL   *APILimit `json:"graphql"`
	Search    *APILimit `json:"search"`
	UpdatedAt time.Time `json:"updated_at"`
	Error     string    `json:"error,omitempty"`
}

// GetWorstLimit returns the API limit with the highest usage percentage
func (r *RateLimitSummary) GetWorstLimit() *APILimit {
	if r.Core == nil && r.GraphQL == nil && r.Search == nil {
		return nil
	}

	var worst *APILimit
	maxPercentage := -1.0

	limits := []*APILimit{r.Core, r.GraphQL, r.Search}
	for _, limit := range limits {
		if limit != nil && limit.Limit > 0 {
			percentage := limit.GetUsagePercentage()
			if percentage > maxPercentage {
				maxPercentage = percentage
				worst = limit
			}
		}
	}

	return worst
}

// GetWorstPercentage returns the highest usage percentage across all API types
func (r *RateLimitSummary) GetWorstPercentage() float64 {
	worst := r.GetWorstLimit()
	if worst == nil {
		return 0
	}
	return worst.GetUsagePercentage()
}

// GetColorByPercentage returns the color code based on usage percentage
// Green (<50%), Yellow (50-80%), Red (>80%)
func GetColorByPercentage(percentage float64) string {
	if percentage > 80 {
		return "red"
	}
	if percentage > 50 {
		return "yellow"
	}
	return "green"
}

// GetColorCSSByPercentage returns the CSS color variable based on usage percentage
func GetColorCSSByPercentage(percentage float64) string {
	color := GetColorByPercentage(percentage)
	switch color {
	case "red":
		return "var(--red)"
	case "yellow":
		return "var(--orange)"
	default:
		return "var(--green)"
	}
}

// GetWorstColor returns the color code for the worst limit
func (r *RateLimitSummary) GetWorstColor() string {
	return GetColorByPercentage(r.GetWorstPercentage())
}

// GetWorstColorCSS returns the CSS color variable for the worst limit
func (r *RateLimitSummary) GetWorstColorCSS() string {
	return GetColorCSSByPercentage(r.GetWorstPercentage())
}

// GetColor returns the color code based on remaining requests
// Deprecated: Use GetColorByPercentage for percentage-based coloring
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
// Deprecated: Use GetColorCSSByPercentage for percentage-based coloring
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
	summary  *RateLimitSummary
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
		summary: &RateLimitSummary{
			Core:      nil,
			GraphQL:   nil,
			Search:    nil,
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
	defer func() { _ = resp.Body.Close() }()

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
			GraphQL struct {
				Limit     int   `json:"limit"`
				Remaining int   `json:"remaining"`
				Reset     int64 `json:"reset"`
			} `json:"graphql"`
			Search struct {
				Limit     int   `json:"limit"`
				Remaining int   `json:"remaining"`
				Reset     int64 `json:"reset"`
			} `json:"search"`
		} `json:"resources"`
	}

	if err := json.NewDecoder(resp.Body).Decode(&result); err != nil {
		s.updateWithError(fmt.Sprintf("Parse error: %v", err))
		return
	}

	now := time.Now()

	s.mu.Lock()
	// Update legacy data for backward compatibility
	s.data = &RateLimitInfo{
		Limit:     result.Resources.Core.Limit,
		Remaining: result.Resources.Core.Remaining,
		Reset:     result.Resources.Core.Reset,
		UpdatedAt: now,
		Error:     "",
	}

	// Update new summary data
	s.summary = &RateLimitSummary{
		Core: &APILimit{
			Name:      "REST API",
			Limit:     result.Resources.Core.Limit,
			Remaining: result.Resources.Core.Remaining,
			Reset:     result.Resources.Core.Reset,
			UpdatedAt: now,
		},
		GraphQL: &APILimit{
			Name:      "GraphQL",
			Limit:     result.Resources.GraphQL.Limit,
			Remaining: result.Resources.GraphQL.Remaining,
			Reset:     result.Resources.GraphQL.Reset,
			UpdatedAt: now,
		},
		Search: &APILimit{
			Name:      "Search",
			Limit:     result.Resources.Search.Limit,
			Remaining: result.Resources.Search.Remaining,
			Reset:     result.Resources.Search.Reset,
			UpdatedAt: now,
		},
		UpdatedAt: now,
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
	// Also update summary error
	if s.summary.UpdatedAt.IsZero() {
		s.summary.Error = errMsg
	} else {
		s.summary.Error = errMsg
	}
	s.mu.Unlock()
}

// GetData returns the current rate limit data (thread-safe)
// Deprecated: Use GetSummary() for multi-type rate limit data
func (s *RateLimitService) GetData() *RateLimitInfo {
	s.mu.RLock()
	defer s.mu.RUnlock()

	// Return a copy to prevent external modification
	data := *s.data
	return &data
}

// GetSummary returns the current rate limit summary for all API types (thread-safe)
func (s *RateLimitService) GetSummary() *RateLimitSummary {
	s.mu.RLock()
	defer s.mu.RUnlock()

	// Return a copy to prevent external modification
	if s.summary == nil {
		return nil
	}

	summary := *s.summary
	// Deep copy the limit pointers
	if s.summary.Core != nil {
		core := *s.summary.Core
		summary.Core = &core
	}
	if s.summary.GraphQL != nil {
		graphql := *s.summary.GraphQL
		summary.GraphQL = &graphql
	}
	if s.summary.Search != nil {
		search := *s.summary.Search
		summary.Search = &search
	}
	return &summary
}

// Refresh triggers an immediate refresh of the rate limit data
func (s *RateLimitService) Refresh() {
	go s.fetch()
}
