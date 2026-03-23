package dashboard

import (
	"fmt"
	"log"
	"sync"
	"time"

	"github.com/crazy-goat/one-dev-army/internal/github"
)

// GitHubClient defines the interface for GitHub operations needed by SyncService
type GitHubClient interface {
	ListIssuesForMilestone(milestone string) ([]github.Issue, error)
	GetIssuePRStatus(issueNumber int) (bool, *time.Time, error)
}

// Store defines the interface for database operations needed by SyncService
type Store interface {
	SaveIssueCache(issue github.Issue, milestone string, force bool) error
}

// SyncService handles periodic synchronization of GitHub issues with the local cache
type SyncService struct {
	gh              GitHubClient
	store           Store
	hub             *Hub
	activeMilestone string
	ticker          *time.Ticker
	stopCh          chan struct{}
	mu              sync.RWMutex
	running         bool
	wg              sync.WaitGroup
}

// NewSyncService creates a new SyncService instance
func NewSyncService(gh GitHubClient, store Store, hub *Hub) *SyncService {
	return &SyncService{
		gh:     gh,
		store:  store,
		hub:    hub,
		stopCh: make(chan struct{}),
	}
}

// SetActiveMilestone sets the active milestone for synchronization
func (s *SyncService) SetActiveMilestone(milestone string) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.activeMilestone = milestone
}

// GetActiveMilestone returns the currently active milestone
func (s *SyncService) GetActiveMilestone() string {
	s.mu.RLock()
	defer s.mu.RUnlock()
	return s.activeMilestone
}

// Start begins the periodic synchronization with a 30-second ticker
func (s *SyncService) Start() {
	s.mu.Lock()
	defer s.mu.Unlock()

	if s.running {
		log.Println("[SyncService] Already running, ignoring start request")
		return
	}

	s.running = true
	s.stopCh = make(chan struct{})
	s.ticker = time.NewTicker(30 * time.Second)

	// Perform initial sync immediately
	go s.syncNow()

	// Start the ticker loop
	go s.run()

	log.Println("[SyncService] Started with 30s interval")
}

// Stop gracefully shuts down the sync service
func (s *SyncService) Stop() {
	s.mu.Lock()
	if !s.running {
		s.mu.Unlock()
		return
	}

	s.running = false
	if s.ticker != nil {
		s.ticker.Stop()
	}
	close(s.stopCh)
	s.mu.Unlock()

	// Wait for any ongoing sync operations to complete
	s.wg.Wait()

	log.Println("[SyncService] Stopped")
}

// IsRunning returns whether the sync service is currently running
func (s *SyncService) IsRunning() bool {
	s.mu.RLock()
	defer s.mu.RUnlock()
	return s.running
}

// run is the main event loop for the sync service
func (s *SyncService) run() {
	for {
		select {
		case <-s.ticker.C:
			s.syncNow()
		case <-s.stopCh:
			return
		}
	}
}

// syncNow performs a single synchronization operation
func (s *SyncService) syncNow() {
	s.wg.Add(1)
	defer s.wg.Done()

	milestone := s.GetActiveMilestone()
	if milestone == "" {
		log.Println("[SyncService] No active milestone set, skipping sync")
		return
	}

	if s.gh == nil {
		log.Println("[SyncService] No GitHub client set, skipping sync")
		return
	}

	if s.store == nil {
		log.Println("[SyncService] No store set, skipping sync")
		return
	}

	log.Printf("[SyncService] Starting sync for milestone: %s", milestone)

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
		// For closed issues, fetch PR merge status
		if issue.State == "CLOSED" {
			isMerged, mergedAt, err := s.gh.GetIssuePRStatus(issue.Number)
			if err != nil {
				log.Printf("[SyncService] Error fetching PR status for issue #%d: %v", issue.Number, err)
			} else {
				issue.PRMerged = isMerged
				issue.MergedAt = mergedAt
				log.Printf("[SyncService] Issue #%d PR status: merged=%v", issue.Number, isMerged)
			}
		}

		if err := s.store.SaveIssueCache(issue, milestone, false); err != nil {
			log.Printf("[SyncService] Error caching issue #%d: %v", issue.Number, err)
			continue
		}
		cachedCount++

		// Broadcast update to WebSocket clients
		if s.hub != nil {
			s.hub.BroadcastIssueUpdate(issue)
		}
	}

	log.Printf("[SyncService] Cached %d/%d issues", cachedCount, len(issues))

	// Broadcast sync completion
	if s.hub != nil {
		s.hub.BroadcastSyncComplete(cachedCount)
	}
}

// SyncNow triggers an immediate synchronization (public method)
func (s *SyncService) SyncNow() error {
	if !s.IsRunning() {
		return fmt.Errorf("sync service is not running")
	}

	go s.syncNow()
	return nil
}
