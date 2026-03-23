package dashboard

import (
	"context"
	"log"
	"sync"
	"time"

	"github.com/crazy-goat/one-dev-army/internal/db"
	"github.com/crazy-goat/one-dev-army/internal/github"
)

// SyncService handles background synchronization of GitHub issues with the local cache
type SyncService struct {
	gh              *github.Client
	store           *db.Store
	hub             *Hub
	activeMilestone string
	mu              sync.RWMutex
	ticker          *time.Ticker
	stopCh          chan struct{}
	wg              sync.WaitGroup
	ctx             context.Context
	cancel          context.CancelFunc
}

// NewSyncService creates a new SyncService instance
func NewSyncService(gh *github.Client, store *db.Store, hub *Hub) *SyncService {
	ctx, cancel := context.WithCancel(context.Background())
	return &SyncService{
		gh:     gh,
		store:  store,
		hub:    hub,
		stopCh: make(chan struct{}),
		ctx:    ctx,
		cancel: cancel,
	}
}

// Start begins the background sync loop with a 30-second ticker
func (s *SyncService) Start() {
	s.mu.Lock()
	defer s.mu.Unlock()

	if s.ticker != nil {
		log.Printf("[SyncService] Already running, ignoring Start() call")
		return
	}

	s.ticker = time.NewTicker(30 * time.Second)
	s.wg.Add(1)

	go func() {
		defer s.wg.Done()
		log.Printf("[SyncService] Background sync started (interval: 30s)")

		// Perform initial sync
		s.performSync()

		for {
			select {
			case <-s.ticker.C:
				s.performSync()
			case <-s.stopCh:
				log.Printf("[SyncService] Stop signal received, exiting sync loop")
				return
			case <-s.ctx.Done():
				log.Printf("[SyncService] Context cancelled, exiting sync loop")
				return
			}
		}
	}()
}

// Stop gracefully shuts down the sync service
func (s *SyncService) Stop() {
	s.mu.Lock()
	defer s.mu.Unlock()

	if s.ticker == nil {
		log.Printf("[SyncService] Not running, ignoring Stop() call")
		return
	}

	log.Printf("[SyncService] Stopping background sync...")

	// Signal the goroutine to stop
	close(s.stopCh)

	// Cancel the context
	s.cancel()

	// Stop the ticker
	s.ticker.Stop()
	s.ticker = nil

	// Wait for the goroutine to finish
	s.wg.Wait()

	log.Printf("[SyncService] Background sync stopped")
}

// SyncNow triggers a manual sync immediately
func (s *SyncService) SyncNow() {
	log.Printf("[SyncService] Manual sync triggered")
	s.performSync()
}

// SetActiveMilestone sets the active milestone for synchronization with thread-safe access
func (s *SyncService) SetActiveMilestone(milestone string) {
	s.mu.Lock()
	defer s.mu.Unlock()

	s.activeMilestone = milestone
	log.Printf("[SyncService] Active milestone set to: %s", milestone)
}

// GetActiveMilestone returns the currently active milestone with thread-safe access
func (s *SyncService) GetActiveMilestone() string {
	s.mu.RLock()
	defer s.mu.RUnlock()

	return s.activeMilestone
}

// performSync executes the actual synchronization with GitHub API
func (s *SyncService) performSync() {
	s.mu.RLock()
	milestone := s.activeMilestone
	s.mu.RUnlock()

	if milestone == "" {
		log.Printf("[SyncService] No active milestone set, skipping sync")
		return
	}

	if s.gh == nil {
		log.Printf("[SyncService] GitHub client not configured, skipping sync")
		return
	}

	if s.store == nil {
		log.Printf("[SyncService] Database store not configured, skipping sync")
		return
	}

	log.Printf("[SyncService] Starting sync for milestone: %s", milestone)

	// Fetch issues from GitHub API for the active milestone
	issues, err := s.gh.ListIssuesForMilestone(milestone)
	if err != nil {
		log.Printf("[SyncService] Error fetching issues for milestone %s: %v", milestone, err)
		// Broadcast failure via WebSocket
		if s.hub != nil {
			s.hub.BroadcastSyncComplete(false, milestone, err.Error())
		}
		return
	}

	log.Printf("[SyncService] Fetched %d issues from GitHub for milestone %s", len(issues), milestone)

	// Save each issue to the SQLite cache
	savedCount := 0
	for _, issue := range issues {
		if err := s.store.SaveIssueCache(issue, milestone); err != nil {
			log.Printf("[SyncService] Error saving issue #%d to cache: %v", issue.Number, err)
			// Continue with other issues, don't fail the entire sync
			continue
		}
		savedCount++
	}

	log.Printf("[SyncService] Successfully synced %d/%d issues to cache for milestone %s", savedCount, len(issues), milestone)

	// Broadcast success via WebSocket
	if s.hub != nil {
		s.hub.BroadcastSyncComplete(true, milestone, "")
	}
}
