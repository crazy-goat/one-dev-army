package dashboard

import (
	"context"
	"log"
	"sync"
	"time"

	"github.com/crazy-goat/one-dev-army/internal/db"
	"github.com/crazy-goat/one-dev-army/internal/github"
)

// SyncInterval is the default interval between automatic syncs
const SyncInterval = 30 * time.Second

// SyncService handles background synchronization of GitHub issues to the local cache
type SyncService struct {
	gh        *github.Client
	store     *db.Store
	hub       *Hub
	interval  time.Duration
	ticker    *time.Ticker
	ctx       context.Context
	cancel    context.CancelFunc
	wg        sync.WaitGroup
	mu        sync.RWMutex
	milestone string
	running   bool
}

// NewSyncService creates a new SyncService instance
func NewSyncService(gh *github.Client, store *db.Store, hub *Hub) *SyncService {
	return &SyncService{
		gh:       gh,
		store:    store,
		hub:      hub,
		interval: SyncInterval,
	}
}

// Start begins the background sync loop
func (s *SyncService) Start() {
	s.mu.Lock()
	defer s.mu.Unlock()

	if s.running {
		return
	}

	s.ctx, s.cancel = context.WithCancel(context.Background())
	s.ticker = time.NewTicker(s.interval)
	s.running = true

	s.wg.Add(1)
	go s.run()

	log.Printf("[SyncService] Started with interval %v", s.interval)
}

// Stop gracefully shuts down the sync service
func (s *SyncService) Stop() {
	s.mu.Lock()
	if !s.running {
		s.mu.Unlock()
		return
	}

	s.running = false
	if s.cancel != nil {
		s.cancel()
	}
	if s.ticker != nil {
		s.ticker.Stop()
	}
	s.mu.Unlock()

	// Wait for the goroutine to finish
	s.wg.Wait()

	log.Printf("[SyncService] Stopped")
}

// SyncNow triggers an immediate sync operation
func (s *SyncService) SyncNow() {
	s.mu.RLock()
	if !s.running {
		s.mu.RUnlock()
		log.Printf("[SyncService] Cannot sync: service not running")
		return
	}
	s.mu.RUnlock()

	// Perform sync outside of lock
	s.performSync()
}

// SetActiveMilestone sets the milestone to sync (thread-safe)
func (s *SyncService) SetActiveMilestone(milestone string) {
	s.mu.Lock()
	s.milestone = milestone
	s.mu.Unlock()

	log.Printf("[SyncService] Active milestone set to: %s", milestone)
}

// GetActiveMilestone returns the currently active milestone (thread-safe)
func (s *SyncService) GetActiveMilestone() string {
	s.mu.RLock()
	defer s.mu.RUnlock()
	return s.milestone
}

// IsRunning returns whether the sync service is currently running
func (s *SyncService) IsRunning() bool {
	s.mu.RLock()
	defer s.mu.RUnlock()
	return s.running
}

// run is the main event loop for the sync service
func (s *SyncService) run() {
	defer s.wg.Done()

	// Perform initial sync
	s.performSync()

	for {
		select {
		case <-s.ctx.Done():
			return
		case <-s.ticker.C:
			s.performSync()
		}
	}
}

// performSync executes the actual synchronization operation
func (s *SyncService) performSync() {
	milestone := s.GetActiveMilestone()
	if milestone == "" {
		log.Printf("[SyncService] No active milestone set, skipping sync")
		return
	}

	log.Printf("[SyncService] Starting sync for milestone: %s", milestone)

	// Fetch issues from GitHub
	issues, err := s.gh.ListIssuesForMilestone(milestone)
	if err != nil {
		log.Printf("[SyncService] Error fetching issues from GitHub: %v", err)
		return
	}

	log.Printf("[SyncService] Fetched %d issues from GitHub", len(issues))

	// Save issues to cache
	savedCount := 0
	for _, issue := range issues {
		if err := s.store.SaveIssueCache(issue, milestone); err != nil {
			log.Printf("[SyncService] Error saving issue #%d to cache: %v", issue.Number, err)
			continue
		}
		savedCount++
	}

	log.Printf("[SyncService] Sync complete: %d/%d issues cached", savedCount, len(issues))

	// Broadcast sync completion via WebSocket
	if s.hub != nil {
		s.hub.BroadcastSyncComplete(savedCount)
	}
}
