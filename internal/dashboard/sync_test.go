package dashboard

import (
	"errors"
	"sync"
	"testing"
	"time"

	"github.com/crazy-goat/one-dev-army/internal/github"
)

// mockGitHubClient is a test double for GitHubClient interface
type mockGitHubClient struct {
	mu        sync.Mutex
	issues    []github.Issue
	listErr   error
	milestone string
}

func (m *mockGitHubClient) ListIssuesWithPRStatus(milestone string) ([]github.Issue, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.milestone = milestone
	if m.listErr != nil {
		return nil, m.listErr
	}
	// Return a copy to avoid races on the slice
	result := make([]github.Issue, len(m.issues))
	copy(result, m.issues)
	return result, nil
}

func (m *mockGitHubClient) AddLabel(issueNum int, label string) error {
	return nil
}

// mockStore is a test double for Store interface
type mockStore struct {
	mu           sync.Mutex
	cachedIssues []github.Issue
	saveErr      error
}

func (m *mockStore) SaveIssueCache(issue github.Issue, milestone string, force bool) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	if m.saveErr != nil {
		return m.saveErr
	}
	m.cachedIssues = append(m.cachedIssues, issue)
	return nil
}

// getCachedIssues returns a thread-safe copy of cached issues
func (m *mockStore) getCachedIssues() []github.Issue {
	m.mu.Lock()
	defer m.mu.Unlock()
	result := make([]github.Issue, len(m.cachedIssues))
	copy(result, m.cachedIssues)
	return result
}

// clearCachedIssues clears cached issues in a thread-safe manner
func (m *mockStore) clearCachedIssues() {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.cachedIssues = nil
}

func TestNewSyncService(t *testing.T) {
	gh := &mockGitHubClient{}
	store := &mockStore{}
	hub := NewHub(false)

	service := NewSyncService(gh, store, hub, nil)

	if service == nil {
		t.Fatal("NewSyncService returned nil")
	}
	if service.gh != gh {
		t.Error("GitHub client not set correctly")
	}
	if service.store != store {
		t.Error("Store not set correctly")
	}
	if service.hub != hub {
		t.Error("Hub not set correctly")
	}
	if service.IsRunning() {
		t.Error("Service should not be running initially")
	}
}

func TestSyncService_SetActiveMilestone(t *testing.T) {
	service := NewSyncService(nil, nil, nil, nil)

	service.SetActiveMilestone("Sprint 1")
	if got := service.GetActiveMilestone(); got != "Sprint 1" {
		t.Errorf("GetActiveMilestone() = %q, want %q", got, "Sprint 1")
	}

	service.SetActiveMilestone("Sprint 2")
	if got := service.GetActiveMilestone(); got != "Sprint 2" {
		t.Errorf("GetActiveMilestone() = %q, want %q", got, "Sprint 2")
	}
}

func TestSyncService_StartStop(t *testing.T) {
	gh := &mockGitHubClient{
		issues: []github.Issue{
			{Number: 1, Title: "Issue 1", State: "open"},
		},
	}
	store := &mockStore{}
	service := NewSyncService(gh, store, nil, nil)
	service.SetActiveMilestone("Sprint 1")

	// Start the service
	service.Start()
	if !service.IsRunning() {
		t.Error("Service should be running after Start()")
	}

	// Stop the service (waits for ongoing sync to complete via wg.Wait)
	service.Stop()
	if service.IsRunning() {
		t.Error("Service should not be running after Stop()")
	}

	// Verify issues were cached during initial sync (safe after Stop returns)
	cached := store.getCachedIssues()
	if len(cached) != 1 {
		t.Errorf("Expected 1 cached issue, got %d", len(cached))
	}
}

func TestSyncService_Start_AlreadyRunning(t *testing.T) {
	gh := &mockGitHubClient{}
	store := &mockStore{}
	service := NewSyncService(gh, store, nil, nil)
	service.SetActiveMilestone("Sprint 1")

	service.Start()
	defer service.Stop()

	// Try to start again - should not panic or create duplicate goroutines
	service.Start()

	if !service.IsRunning() {
		t.Error("Service should still be running")
	}
}

func TestSyncService_Stop_NotRunning(t *testing.T) {
	service := NewSyncService(nil, nil, nil, nil)

	// Should not panic when stopping a non-running service
	service.Stop()

	if service.IsRunning() {
		t.Error("Service should not be running")
	}
}

func TestSyncService_syncNow_NoMilestone(t *testing.T) {
	gh := &mockGitHubClient{}
	store := &mockStore{}
	service := NewSyncService(gh, store, nil, nil)
	// No milestone set

	service.syncNow()

	if len(store.cachedIssues) != 0 {
		t.Error("Should not cache any issues when no milestone is set")
	}
}

func TestSyncService_syncNow_NoGitHubClient(t *testing.T) {
	store := &mockStore{}
	service := NewSyncService(nil, store, nil, nil)
	service.SetActiveMilestone("Sprint 1")

	service.syncNow()

	if len(store.cachedIssues) != 0 {
		t.Error("Should not cache any issues when GitHub client is nil")
	}
}

func TestSyncService_syncNow_NoStore(t *testing.T) {
	gh := &mockGitHubClient{
		issues: []github.Issue{{Number: 1, Title: "Issue 1"}},
	}
	service := NewSyncService(gh, nil, nil, nil)
	service.SetActiveMilestone("Sprint 1")

	service.syncNow()
	// Should not panic
}

func TestSyncService_syncNow_GitHubError(t *testing.T) {
	gh := &mockGitHubClient{
		listErr: errors.New("github error"),
	}
	store := &mockStore{}
	service := NewSyncService(gh, store, nil, nil)
	service.SetActiveMilestone("Sprint 1")

	service.syncNow()

	if len(store.cachedIssues) != 0 {
		t.Error("Should not cache any issues when GitHub returns an error")
	}
}

func TestSyncService_syncNow_SaveError(t *testing.T) {
	gh := &mockGitHubClient{
		issues: []github.Issue{
			{Number: 1, Title: "Issue 1"},
			{Number: 2, Title: "Issue 2"},
		},
	}
	store := &mockStore{
		saveErr: errors.New("save error"),
	}
	service := NewSyncService(gh, store, nil, nil)
	service.SetActiveMilestone("Sprint 1")

	service.syncNow()

	// Should continue even if individual saves fail
	if len(store.cachedIssues) != 0 {
		t.Error("Should not have cached issues when save fails")
	}
}

func TestSyncService_syncNow_Success(t *testing.T) {
	gh := &mockGitHubClient{
		issues: []github.Issue{
			{Number: 1, Title: "Issue 1", State: "open"},
			{Number: 2, Title: "Issue 2", State: "closed"},
			{Number: 3, Title: "Issue 3", State: "open"},
		},
	}
	store := &mockStore{}
	service := NewSyncService(gh, store, nil, nil)
	service.SetActiveMilestone("Sprint 1")

	service.syncNow()

	if len(store.cachedIssues) != 3 {
		t.Errorf("Expected 3 cached issues, got %d", len(store.cachedIssues))
	}

	// Verify the milestone was passed correctly
	if gh.milestone != "Sprint 1" {
		t.Errorf("Expected milestone 'Sprint 1', got %q", gh.milestone)
	}
}

func TestSyncService_SyncNow_ManualTrigger(t *testing.T) {
	gh := &mockGitHubClient{
		issues: []github.Issue{
			{Number: 1, Title: "Issue 1"},
		},
	}
	store := &mockStore{}
	service := NewSyncService(gh, store, nil, nil)
	service.SetActiveMilestone("Sprint 1")

	// Start the service first (this triggers initial sync)
	service.Start()
	defer service.Stop()

	// Wait for initial sync to complete using WaitGroup
	service.wg.Wait()

	// Clear the store to test manual sync specifically (thread-safe)
	store.clearCachedIssues()

	// Trigger manual sync
	err := service.SyncNow()
	if err != nil {
		t.Errorf("SyncNow() should not error when service is running, got: %v", err)
	}

	// Wait for the manual sync goroutine to complete
	service.wg.Wait()

	cached := store.getCachedIssues()
	if len(cached) != 1 {
		t.Errorf("Expected 1 cached issue after manual sync, got %d", len(cached))
	}
}

func TestSyncService_ThreadSafety(t *testing.T) {
	gh := &mockGitHubClient{
		issues: []github.Issue{
			{Number: 1, Title: "Issue 1"},
		},
	}
	store := &mockStore{}
	service := NewSyncService(gh, store, nil, nil)

	// Concurrent operations
	done := make(chan bool, 3)

	go func() {
		service.SetActiveMilestone("Sprint 1")
		done <- true
	}()

	go func() {
		service.GetActiveMilestone()
		done <- true
	}()

	go func() {
		service.IsRunning()
		done <- true
	}()

	// Wait for all goroutines
	for i := 0; i < 3; i++ {
		select {
		case <-done:
		case <-time.After(time.Second):
			t.Fatal("Timeout waiting for concurrent operations")
		}
	}
}

func TestSyncService_syncNow_FetchesPRStatus(t *testing.T) {
	mergedAt := time.Now().UTC().Truncate(time.Second)
	gh := &mockGitHubClient{
		issues: []github.Issue{
			{Number: 1, Title: "Open Issue", State: "open"},
			{Number: 2, Title: "Merged Issue", State: "CLOSED", PRMerged: true, MergedAt: &mergedAt},
			{Number: 3, Title: "Closed Issue", State: "CLOSED"},
		},
	}
	store := &mockStore{}
	service := NewSyncService(gh, store, nil, nil)
	service.SetActiveMilestone("Sprint 1")

	service.syncNow()

	// Verify all 3 issues were cached
	if len(store.cachedIssues) != 3 {
		t.Errorf("Expected 3 cached issues, got %d", len(store.cachedIssues))
	}

	// Find the merged issue
	var mergedIssue *github.Issue
	var closedIssue *github.Issue
	for i := range store.cachedIssues {
		if store.cachedIssues[i].Number == 2 {
			mergedIssue = &store.cachedIssues[i]
		}
		if store.cachedIssues[i].Number == 3 {
			closedIssue = &store.cachedIssues[i]
		}
	}

	if mergedIssue == nil {
		t.Fatal("Merged issue not found in cached issues")
	}
	if !mergedIssue.PRMerged {
		t.Errorf("Expected issue #2 to have PRMerged=true, got false")
	}
	if mergedIssue.MergedAt == nil {
		t.Error("Expected issue #2 to have MergedAt set")
	}

	if closedIssue == nil {
		t.Fatal("Closed issue not found in cached issues")
	}
	if closedIssue.PRMerged {
		t.Errorf("Expected issue #3 to have PRMerged=false, got true")
	}
	if closedIssue.MergedAt != nil {
		t.Error("Expected issue #3 to have MergedAt=nil")
	}
}
