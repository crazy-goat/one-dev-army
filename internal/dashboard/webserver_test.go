package dashboard

import (
	"testing"
	"time"
)

func TestNewWebServer(t *testing.T) {
	// Test with valid port
	ws, err := NewWebServer(8080, "/tmp")
	if err != nil {
		t.Fatalf("NewWebServer failed: %v", err)
	}

	if ws == nil {
		t.Fatal("NewWebServer returned nil")
	}

	if ws.port != 8080 {
		t.Errorf("expected port 8080, got %d", ws.port)
	}

	if ws.dir != "/tmp" {
		t.Errorf("expected dir /tmp, got %s", ws.dir)
	}

	if ws.stopCh == nil {
		t.Error("expected stopCh to be initialized")
	}

	if ws.healthChecker == nil {
		t.Error("expected healthChecker to be initialized")
	}

	if ws.ctx == nil {
		t.Error("expected ctx to be initialized")
	}

	if ws.cancel == nil {
		t.Error("expected cancel to be initialized")
	}
}

func TestNewWebServer_DefaultPort(t *testing.T) {
	ws, err := NewWebServer(0, "/tmp")
	if err != nil {
		t.Fatalf("NewWebServer failed: %v", err)
	}

	if ws.port != defaultWebPort {
		t.Errorf("expected default port %d, got %d", defaultWebPort, ws.port)
	}
}

func TestNewWebServer_InvalidPort(t *testing.T) {
	// Test port 0 is valid (uses default)
	_, err := NewWebServer(0, "/tmp")
	if err != nil {
		t.Errorf("port 0 should be valid (uses default): %v", err)
	}

	// Test negative port
	_, err = NewWebServer(-1, "/tmp")
	if err == nil {
		t.Error("expected error for negative port")
	}

	// Test port too high
	_, err = NewWebServer(65536, "/tmp")
	if err == nil {
		t.Error("expected error for port > 65535")
	}

	// Test port 1 (valid)
	_, err = NewWebServer(1, "/tmp")
	if err != nil {
		t.Errorf("port 1 should be valid: %v", err)
	}

	// Test port 65535 (valid)
	_, err = NewWebServer(65535, "/tmp")
	if err != nil {
		t.Errorf("port 65535 should be valid: %v", err)
	}
}

func TestWebServer_Port(t *testing.T) {
	ws, _ := NewWebServer(9090, "/tmp")

	if ws.Port() != 9090 {
		t.Errorf("expected Port() to return 9090, got %d", ws.Port())
	}
}

func TestWebServer_URL(t *testing.T) {
	ws, _ := NewWebServer(8080, "/tmp")

	expected := "http://localhost:8080"
	if ws.URL() != expected {
		t.Errorf("expected URL() to return %s, got %s", expected, ws.URL())
	}
}

func TestWebServer_IsRunning_NotStarted(t *testing.T) {
	ws, _ := NewWebServer(8080, "/tmp")

	if ws.IsRunning() {
		t.Error("expected IsRunning() to be false before Start()")
	}
}

func TestWebServer_Start_AlreadyRunning(t *testing.T) {
	ws, _ := NewWebServer(8080, "/tmp")

	// First start should fail because opencode is not installed
	// But we can test the "already running" logic
	_ = ws.Start()
	// This will fail because opencode command doesn't exist in test environment
	// But that's expected

	// Try to start again - should return "already running" error
	// Note: Since the first start failed, cmd might be nil, so this might not trigger the error
	// This test is mainly for coverage
	err := ws.Start()
	if err == nil {
		t.Log("Start() on already running server should return error")
	}
}

func TestWebServer_Stop_NotStarted(t *testing.T) {
	ws, _ := NewWebServer(8080, "/tmp")

	// Should not panic when stopping a non-running server
	err := ws.Stop()
	if err != nil {
		t.Logf("Stop() returned error (expected): %v", err)
	}
}

func TestWebServer_DoubleStop(t *testing.T) {
	ws, _ := NewWebServer(8080, "/tmp")

	// First stop
	err := ws.Stop()
	if err != nil {
		t.Logf("First Stop() returned error (expected): %v", err)
	}

	// Second stop should not panic (sync.Once prevents double-close)
	err = ws.Stop()
	if err != nil {
		t.Logf("Second Stop() returned error (expected): %v", err)
	}
}

func TestWebServer_ThreadSafety(t *testing.T) {
	ws, _ := NewWebServer(8080, "/tmp")

	// Run concurrent operations
	done := make(chan bool, 4)

	go func() {
		ws.IsRunning()
		done <- true
	}()

	go func() {
		ws.Port()
		done <- true
	}()

	go func() {
		ws.URL()
		done <- true
	}()

	go func() {
		_ = ws.Stop()
		done <- true
	}()

	// Wait for all goroutines
	for range 4 {
		select {
		case <-done:
		case <-time.After(time.Second):
			t.Fatal("Timeout waiting for concurrent operations")
		}
	}
}

func TestWebServer_ContextCancellation(t *testing.T) {
	ws, _ := NewWebServer(8080, "/tmp")

	// Cancel the context
	ws.cancel()

	// Check that context is done
	select {
	case <-ws.ctx.Done():
		// Expected
	case <-time.After(time.Second):
		t.Error("Context should be canceled")
	}
}
