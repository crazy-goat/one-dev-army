package dashboard

import (
	"sync"
	"testing"
	"time"
)

func TestNewSyncService(t *testing.T) {
	service := NewSyncService(nil, nil, nil)

	if service == nil {
		t.Fatal("NewSyncService() returned nil")
	}
	if service.gh != nil {
		t.Error("GitHub client should be nil")
	}
	if service.store != nil {
		t.Error("Store should be nil")
	}
	if service.hub != nil {
		t.Error("Hub should be nil")
	}
	if service.activeMilestone != "" {
		t.Error("Active milestone should be empty initially")
	}
	if service.ticker != nil {
		t.Error("Ticker should be nil initially")
	}
}

func TestSyncServiceSetActiveMilestone(t *testing.T) {
	service := NewSyncService(nil, nil, nil)

	service.SetActiveMilestone("Sprint 1")

	if service.GetActiveMilestone() != "Sprint 1" {
		t.Errorf("Expected milestone 'Sprint 1', got '%s'", service.GetActiveMilestone())
	}

	// Test thread safety with concurrent access
	var wg sync.WaitGroup
	for i := 0; i < 100; i++ {
		wg.Add(2)
		go func() {
			defer wg.Done()
			service.SetActiveMilestone("Concurrent Sprint")
		}()
		go func() {
			defer wg.Done()
			_ = service.GetActiveMilestone()
		}()
	}
	wg.Wait()
}

func TestSyncServiceStartStop(t *testing.T) {
	service := NewSyncService(nil, nil, nil)

	// Start the service
	service.Start()

	// Give it a moment to start
	time.Sleep(50 * time.Millisecond)

	// Verify ticker is running
	service.mu.RLock()
	if service.ticker == nil {
		service.mu.RUnlock()
		t.Error("Ticker should be running after Start()")
	} else {
		service.mu.RUnlock()
	}

	// Stop the service
	service.Stop()

	// Verify ticker is stopped
	service.mu.RLock()
	if service.ticker != nil {
		service.mu.RUnlock()
		t.Error("Ticker should be nil after Stop()")
	} else {
		service.mu.RUnlock()
	}
}

func TestSyncServiceMultipleStartStop(t *testing.T) {
	service := NewSyncService(nil, nil, nil)

	// Multiple starts should be safe (subsequent ones ignored)
	service.Start()
	service.Start()
	service.Start()

	time.Sleep(50 * time.Millisecond)

	// Multiple stops should be safe (subsequent ones ignored)
	service.Stop()
	service.Stop()
	service.Stop()

	// Should complete without panic
}

func TestSyncServiceSyncNowWithoutMilestone(t *testing.T) {
	service := NewSyncService(nil, nil, nil)

	// Sync without setting milestone should not panic
	service.SyncNow()

	// Should complete without error
}

func TestSyncServiceSyncNowWithNilDependencies(t *testing.T) {
	service := NewSyncService(nil, nil, nil)

	// Set milestone but with nil dependencies
	service.SetActiveMilestone("Sprint 1")
	service.SyncNow()

	// Should complete without panic
}

func TestSyncServiceEmptyMilestone(t *testing.T) {
	service := NewSyncService(nil, nil, nil)

	// Set empty milestone
	service.SetActiveMilestone("")
	service.SyncNow()

	// Should complete without error
}

func TestSyncServiceConcurrentSync(t *testing.T) {
	service := NewSyncService(nil, nil, nil)

	service.SetActiveMilestone("Sprint 1")

	// Trigger multiple concurrent syncs
	var wg sync.WaitGroup
	for i := 0; i < 10; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			service.SyncNow()
		}()
	}
	wg.Wait()

	// All syncs should complete without panic
}

func TestSyncServiceStopWhileRunning(t *testing.T) {
	service := NewSyncService(nil, nil, nil)

	service.SetActiveMilestone("Sprint 1")
	service.Start()

	// Let it start
	time.Sleep(50 * time.Millisecond)

	// Stop while potentially running
	service.Stop()

	// Should complete without panic or deadlock
}

func TestSyncServiceContextCancellation(t *testing.T) {
	service := NewSyncService(nil, nil, nil)

	// Start and immediately stop to trigger context cancellation
	service.Start()
	time.Sleep(10 * time.Millisecond)
	service.Stop()

	// Should handle context cancellation gracefully
	// If we get here without panic, the test passes
}

func TestSyncServicePeriodicSync(t *testing.T) {
	service := NewSyncService(nil, nil, nil)

	service.SetActiveMilestone("Sprint 1")
	service.Start()

	// Wait briefly to verify service started
	time.Sleep(100 * time.Millisecond)

	// Stop the service
	service.Stop()

	// Should start and stop without issues
	// Note: We can't easily test the 30s periodic sync in unit tests
}

func TestSyncServiceGetActiveMilestoneConcurrent(t *testing.T) {
	service := NewSyncService(nil, nil, nil)

	var wg sync.WaitGroup
	for i := 0; i < 100; i++ {
		wg.Add(1)
		go func(idx int) {
			defer wg.Done()
			// Alternate between reads and writes
			if idx%2 == 0 {
				service.SetActiveMilestone("Sprint " + string(rune('0'+idx%10)))
			} else {
				_ = service.GetActiveMilestone()
			}
		}(i)
	}
	wg.Wait()

	// Should complete without race conditions
}

func TestSyncServiceStopNotRunning(t *testing.T) {
	service := NewSyncService(nil, nil, nil)

	// Stop without starting should not panic
	service.Stop()
	service.Stop()
	service.Stop()
}

func TestSyncServiceWithHub(t *testing.T) {
	hub := NewHub()
	go hub.Run()
	defer hub.Stop()

	service := NewSyncService(nil, nil, hub)

	if service.hub != hub {
		t.Error("Hub not set correctly")
	}

	// Sync with nil dependencies but valid hub
	service.SetActiveMilestone("Sprint 1")
	service.SyncNow()

	// Should complete without panic
}
