package dashboard

import (
	"errors"
	"log"
	"sync"
	"time"

	"github.com/crazy-goat/one-dev-army/internal/github"
)

func hasStageLabel(issue github.Issue) bool {
	for _, label := range issue.Labels {
		if github.IsStageLabel(label.Name) {
			return true
		}
	}
	return false
}

// GitHubClient defines the interface for GitHub operations needed by SyncService
type GitHubClient interface {
	ListIssuesWithPRStatus(milestone string) ([]github.Issue, error)
	AddLabel(issueNum int, label string) error
	GetOldestOpenMilestone() (*github.Milestone, error)
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
	orchestrator    OrchestratorClient
	activeMilestone string
	ticker          *time.Ticker
	stopCh          chan struct{}
	mu              sync.RWMutex
	running         bool
	wg              sync.WaitGroup
}

// OrchestratorClient defines the interface for orchestrator operations needed by SyncService
type OrchestratorClient interface {
	HandleSyncEvent(issue github.Issue)
}

// NewSyncService creates a new SyncService instance
func NewSyncService(gh GitHubClient, store Store, hub *Hub, orchestrator OrchestratorClient) *SyncService {
	return &SyncService{
		gh:           gh,
		store:        store,
		hub:          hub,
		orchestrator: orchestrator,
		stopCh:       make(chan struct{}),
	}
}

// SetActiveMilestone sets the active milestone for synchronization
func (s *SyncService) SetActiveMilestone(milestone string) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.activeMilestone = milestone
}

// SetOrchestrator sets the orchestrator for sync event handling
func (s *SyncService) SetOrchestrator(orchestrator OrchestratorClient) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.orchestrator = orchestrator
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
	s.wg.Add(1)
	go s.syncNowFromStart()

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

// syncNowFromStart is called from Start() where wg.Add(1) is already done
func (s *SyncService) syncNowFromStart() {
	defer s.wg.Done()
	s.doSync()
}

// syncNow performs a single synchronization operation
func (s *SyncService) syncNow() {
	s.wg.Add(1)
	defer s.wg.Done()
	s.doSync()
}

// doSync performs the actual synchronization work
func (s *SyncService) doSync() {
	if s.gh == nil {
		log.Println("[SyncService] No GitHub client set, skipping sync")
		return
	}

	// Check for milestone change first
	currentMilestone := s.GetActiveMilestone()
	latestMilestone, err := s.gh.GetOldestOpenMilestone()
	switch {
	case err != nil:
		log.Printf("[SyncService] Error checking for new milestone: %v", err)
	case latestMilestone == nil:
		// No open milestones at all
		if currentMilestone != "" {
			log.Printf("[SyncService] No open milestones found (was: %s), stopping sync", currentMilestone)
			s.SetActiveMilestone("")
		}
		return
	case latestMilestone.Title != currentMilestone:
		log.Printf("[SyncService] Milestone change detected: %s (was: %s)",
			latestMilestone.Title, currentMilestone)
		s.SetActiveMilestone(latestMilestone.Title)
	}

	milestone := s.GetActiveMilestone()
	if milestone == "" {
		log.Println("[SyncService] No active milestone set, skipping sync")
		return
	}

	if s.store == nil {
		log.Println("[SyncService] No store set, skipping sync")
		return
	}

	log.Printf("[SyncService] Starting sync for milestone: %s", milestone)

	// Single GraphQL query fetches issues + PR merge status together
	issues, err := s.gh.ListIssuesWithPRStatus(milestone)
	if err != nil {
		log.Printf("[SyncService] Error fetching issues: %v", err)
		return
	}

	log.Printf("[SyncService] Fetched %d issues from GitHub (single query)", len(issues))

	// Cache each issue and notify orchestrator
	cachedCount := 0
	for _, issue := range issues {
		if issue.State == "open" && !hasStageLabel(issue) {
			log.Printf("[SyncService] Auto-assigning stage:backlog to issue #%d", issue.Number)
			if err := s.gh.AddLabel(issue.Number, string(github.StageBacklog)); err != nil {
				log.Printf("[SyncService] Error adding stage:backlog to issue #%d: %v", issue.Number, err)
			} else {
				issue.Labels = append(issue.Labels, struct {
					Name string `json:"name"`
				}{Name: string(github.StageBacklog)})
			}
		}

		if err := s.store.SaveIssueCache(issue, milestone, false); err != nil {
			log.Printf("[SyncService] Error caching issue #%d: %v", issue.Number, err)
			continue
		}
		cachedCount++

		// Notify orchestrator of sync event
		if s.orchestrator != nil {
			s.orchestrator.HandleSyncEvent(issue)
		}
	}

	log.Printf("[SyncService] Cached %d/%d issues", cachedCount, len(issues))

	// Broadcast sync completion - frontend will refresh board
	if s.hub != nil {
		s.hub.BroadcastSyncComplete(cachedCount)
	}
}

// SyncNow triggers an immediate synchronization (public method)
func (s *SyncService) SyncNow() error {
	if !s.IsRunning() {
		return errors.New("sync service is not running")
	}

	s.wg.Add(1)
	go s.syncNowFromStart()
	return nil
}
