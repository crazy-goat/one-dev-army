package dashboard

import (
	"fmt"
	"log"
	"net/http"
	"sync"
	"time"
)

// HealthChecker monitors the health of the opencode web UI.
type HealthChecker struct {
	port      int
	interval  time.Duration
	client    *http.Client
	stopCh    chan struct{}
	mu        sync.RWMutex
	lastCheck time.Time
	healthy   bool
}

// NewHealthChecker creates a new HealthChecker instance.
func NewHealthChecker(port int, interval time.Duration) *HealthChecker {
	return &HealthChecker{
		port:     port,
		interval: interval,
		client: &http.Client{
			Timeout: 5 * time.Second,
		},
		stopCh:  make(chan struct{}),
		healthy: false,
	}
}

// Check performs a health check and returns true if the web server is healthy.
func (h *HealthChecker) Check() bool {
	url := fmt.Sprintf("http://localhost:%d/health", h.port)

	resp, err := h.client.Get(url)
	if err != nil {
		h.setHealth(false)
		log.Printf("[HealthChecker] Health check failed: %v", err)
		return false
	}
	defer resp.Body.Close()

	isHealthy := resp.StatusCode == http.StatusOK
	h.setHealth(isHealthy)

	if isHealthy {
		log.Printf("[HealthChecker] Health check passed (port %d)", h.port)
	} else {
		log.Printf("[HealthChecker] Health check failed with status: %d", resp.StatusCode)
	}

	return isHealthy
}

// Stop stops the health checker.
func (h *HealthChecker) Stop() {
	close(h.stopCh)
}

// IsHealthy returns the last known health status.
func (h *HealthChecker) IsHealthy() bool {
	h.mu.RLock()
	defer h.mu.RUnlock()
	return h.healthy
}

// LastCheck returns the time of the last health check.
func (h *HealthChecker) LastCheck() time.Time {
	h.mu.RLock()
	defer h.mu.RUnlock()
	return h.lastCheck
}

// setHealth updates the health status.
func (h *HealthChecker) setHealth(healthy bool) {
	h.mu.Lock()
	defer h.mu.Unlock()
	h.healthy = healthy
	h.lastCheck = time.Now()
}
