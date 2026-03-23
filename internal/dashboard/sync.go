package dashboard

import (
	"log"
	"sync"
	"time"

	"github.com/crazy-goat/one-dev-army/internal/github"
)

// GitHubClient defines the interface for GitHub operations needed by SyncService
type GitHubClient interface {
	ListIssuesForMilestone(milestone string) ([]github.Issue, error)
}

// Store defines the interface for database operations needed by SyncService
type Store interface {
	SaveIssueCache(issue github.Issue, milestone string) error
}

// SyncService handles periodic synchronization of GitHub issues to the local cache.
// It broadcasts updates via WebSocket and can be started/stopped gracefully.
type SyncService struct {
	gh              GitHubClient
	store           Store
	hub             *Hub
	ticker          *time.Ticker
	stopCh          chan struct{}
	wg              sync.WaitGroup
	mu              sync.RWMutex
	activeMilestone string
	running         bool
}

// NewSyncService creates a new SyncService instance.
// The hub parameter is optional and can be nil if WebSocket broadcasting is not needed.
func NewSyncService(gh GitHubClient, store Store, hub *Hub) *SyncService {
	return &SyncService{
		gh:     gh,
		store:  store,
		hub:    hub,
		stopCh: make(chan struct{}),
	}
}

// SetActiveMilestone sets the milestone to sync issues from.
func (s *SyncService) SetActiveMilestone(milestone string) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.activeMilestone = milestone
}

// GetActiveMilestone returns the currently set active milestone.
func (s *SyncService) GetActiveMilestone() string {
	s.mu.RLock()
	defer s.mu.RUnlock()
	return s.activeMilestone
}

// Start begins the periodic sync with a 30-second interval.
// It performs an immediate sync on startup.
func (s *SyncService) Start() {
	s.mu.Lock()
	if s.running {
		s.mu.Unlock()
		log.Printf("[SyncService] Already running, ignoring Start() call")
		return
	}
	s.running = true
	s.ticker = time.NewTicker(30 * time.Second)
	s.mu.Unlock()

	log.Printf("[SyncService] Starting with 30s interval")

	// Perform initial sync immediately
	s.wg.Add(1)
	go func() {
		defer s.wg.Done()
		s.syncNow()
	}()

	// Start the ticker goroutine
	s.wg.Add(1)
	go func() {
		defer s.wg.Done()
		for {
			select {
			case <-s.ticker.C:
				s.syncNow()
			case <-s.stopCh:
				return
			}
		}
	}()
}

// Stop gracefully shuts down the SyncService.
// It waits for any ongoing sync to complete.
func (s *SyncService) Stop() {
	s.mu.Lock()
	if !s.running {
		s.mu.Unlock()
		return
	}
	s.running = false
	s.ticker.Stop()
	close(s.stopCh)
	s.mu.Unlock()

	log.Printf("[SyncService] Stopping...")
	s.wg.Wait()
	log.Printf("[SyncService] Stopped")
}

// IsRunning returns true if the SyncService is currently running.
func (s *SyncService) IsRunning() bool {
	s.mu.RLock()
	defer s.mu.RUnlock()
	return s.running
}

// syncNow performs a single synchronization of issues from GitHub to the cache.
// This method is thread-safe and can be called manually.
func (s *SyncService) syncNow() {
	s.mu.RLock()
	milestone := s.activeMilestone
	s.mu.RUnlock()

	if milestone == "" {
		log.Printf("[SyncService] No active milestone set, skipping sync")
		return
	}

	if s.gh == nil {
		log.Printf("[SyncService] No GitHub client available, skipping sync")
		return
	}

	if s.store == nil {
		log.Printf("[SyncService] No store available, skipping sync")
		return
	}

	log.Printf("[SyncService] Syncing issues for milestone: %s", milestone)

	// Fetch issues from GitHub
	issues, err := s.gh.ListIssuesForMilestone(milestone)
	if err != nil {
		log.Printf("[SyncService] Error fetching issues: %v", err)
		return
	}

	log.Printf("[SyncService] Fetched %d issues from GitHub", len(issues))

	// Cache each issue
	cachedCount := 0
	for _, issue := range issues {
		if err := s.store.SaveIssueCache(issue, milestone); err != nil {
			log.Printf("[SyncService] Error caching issue #%d: %v", issue.Number, err)
			continue
		}
		cachedCount++

		// Broadcast update via WebSocket if hub is available
		if s.hub != nil {
			s.hub.BroadcastIssueUpdate(issue)
		}
	}

	log.Printf("[SyncService] Cached %d/%d issues", cachedCount, len(issues))

	// Broadcast sync completion if hub is available
	if s.hub != nil {
		s.hub.BroadcastSyncComplete(cachedCount)
	}
}

// SyncNow triggers an immediate sync (public method for manual sync).
func (s *SyncService) SyncNow() {
	s.wg.Add(1)
	go func() {
		defer s.wg.Done()
		s.syncNow()
	}()
}
