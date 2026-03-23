package dashboard

import (
	"testing"
	"time"

	"github.com/crazy-goat/one-dev-army/internal/db"
	"github.com/crazy-goat/one-dev-army/internal/github"
)

// mockGitHubClient is a mock implementation of the GitHub client for testing
type mockGitHubClient struct {
	issues []github.Issue
	err    error
}

func (m *mockGitHubClient) ListIssuesForMilestone(milestone string) ([]github.Issue, error) {
	if m.err != nil {
		return nil, m.err
	}
	return m.issues, nil
}

// mockStore is a mock implementation of the database store for testing
type mockStore struct {
	savedIssues []github.Issue
	err         error
}

func (m *mockStore) SaveIssueCache(issue github.Issue, milestone string) error {
	if m.err != nil {
		return m.err
	}
	m.savedIssues = append(m.savedIssues, issue)
	return nil
}

func TestSyncService_NewSyncService(t *testing.T) {
	gh := &github.Client{}
	store := &db.Store{}
	hub := NewHub()

	service := NewSyncService(gh, store, hub)

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

	if service.running {
		t.Error("Service should not be running initially")
	}
}

func TestSyncService_SetActiveMilestone(t *testing.T) {
	service := NewSyncService(nil, nil, nil)

	service.SetActiveMilestone("Sprint 1")

	if got := service.GetActiveMilestone(); got != "Sprint 1" {
		t.Errorf("GetActiveMilestone() = %q, want %q", got, "Sprint 1")
	}
}

func TestSyncService_StartStop(t *testing.T) {
	service := NewSyncService(nil, nil, nil)

	// Test starting
	service.Start()

	if !service.IsRunning() {
		t.Error("Service should be running after Start()")
	}

	// Test starting again (should be idempotent)
	service.Start()

	if !service.IsRunning() {
		t.Error("Service should still be running after second Start()")
	}

	// Test stopping
	service.Stop()

	// Give it a moment to stop
	time.Sleep(100 * time.Millisecond)

	if service.IsRunning() {
		t.Error("Service should not be running after Stop()")
	}

	// Test stopping again (should be idempotent)
	service.Stop()
}

func TestSyncService_SyncNow_NotRunning(t *testing.T) {
	service := NewSyncService(nil, nil, nil)

	err := service.SyncNow()
	if err == nil {
		t.Error("SyncNow() should return error when service is not running")
	}
}

func TestSyncService_SyncNow_NoMilestone(t *testing.T) {
	service := NewSyncService(nil, nil, nil)
	service.Start()
	defer service.Stop()

	// Should not error even without milestone
	err := service.SyncNow()
	if err != nil {
		t.Errorf("SyncNow() with no milestone should not error, got: %v", err)
	}
}

func TestSyncService_ThreadSafety(t *testing.T) {
	service := NewSyncService(nil, nil, nil)

	// Test concurrent access to milestone
	done := make(chan bool, 3)

	go func() {
		for i := 0; i < 100; i++ {
			service.SetActiveMilestone("Sprint 1")
		}
		done <- true
	}()

	go func() {
		for i := 0; i < 100; i++ {
			_ = service.GetActiveMilestone()
		}
		done <- true
	}()

	go func() {
		for i := 0; i < 100; i++ {
			service.SetActiveMilestone("Sprint 2")
		}
		done <- true
	}()

	// Wait for all goroutines
	for i := 0; i < 3; i++ {
		<-done
	}

	// No race conditions should occur
	t.Log("Thread safety test passed")
}

func TestSyncService_MultipleStartStop(t *testing.T) {
	service := NewSyncService(nil, nil, nil)

	// Multiple start/stop cycles
	for i := 0; i < 5; i++ {
		service.Start()
		if !service.IsRunning() {
			t.Errorf("Cycle %d: Service should be running after Start()", i)
		}

		service.Stop()
		time.Sleep(50 * time.Millisecond)

		if service.IsRunning() {
			t.Errorf("Cycle %d: Service should not be running after Stop()", i)
		}
	}
}
