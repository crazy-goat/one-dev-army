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

// SyncService handles background synchronization of GitHub issues with the local cache
type SyncService struct {
	gh              *github.Client
	store           *db.Store
	hub             *Hub
	activeMilestone string
	interval        time.Duration
	mu              sync.RWMutex
	ticker          *time.Ticker
	stopCh          chan struct{}
	wg              sync.WaitGroup
	ctx             context.Context
	cancel          context.CancelFunc
	running         bool
}

// NewSyncService creates a new SyncService instance
func NewSyncService(gh *github.Client, store *db.Store, hub *Hub) *SyncService {
	ctx, cancel := context.WithCancel(context.Background())
	return &SyncService{
		gh:       gh,
		store:    store,
		hub:      hub,
		interval: SyncInterval,
		stopCh:   make(chan struct{}),
		ctx:      ctx,
		cancel:   cancel,
	}
}

// Start begins the background sync loop
func (s *SyncService) Start() {
	s.mu.Lock()
	defer s.mu.Unlock()

	if s.running {
		log.Printf("[SyncService] Already running, ignoring Start() call")
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

// SetActiveMilestone sets the active milestone for synchronization with thread-safe access
func (s *SyncService) SetActiveMilestone(milestone string) {
	s.mu.Lock()
	s.activeMilestone = milestone
	s.mu.Unlock()

	log.Printf("[SyncService] Active milestone set to: %s", milestone)
}

// GetActiveMilestone returns the currently active milestone with thread-safe access
func (s *SyncService) GetActiveMilestone() string {
	s.mu.RLock()
	defer s.mu.RUnlock()
	return s.activeMilestone
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
		s.hub.BroadcastSyncComplete(savedCount)
	}
}
