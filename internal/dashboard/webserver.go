package dashboard

import (
	"context"
	"errors"
	"fmt"
	"log"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"strings"
	"sync"
	"syscall"
	"time"
)

const (
	defaultWebPort      = 8081
	healthCheckInterval = 30 * time.Second
	restartDelay        = 5 * time.Second
	maxRestartAttempts  = 5
)

// WebServer manages the lifecycle of the opencode web UI process.
type WebServer struct {
	port          int
	dir           string
	cmd           *exec.Cmd
	mu            sync.RWMutex
	stopCh        chan struct{}
	stopOnce      sync.Once
	wg            sync.WaitGroup
	restartCount  int
	lastRestart   time.Time
	healthChecker *HealthChecker
	ctx           context.Context
	cancel        context.CancelFunc
}

// NewWebServer creates a new WebServer instance.
func NewWebServer(port int, dir string) (*WebServer, error) {
	if port == 0 {
		port = defaultWebPort
	}
	// Validate port range (1-65535)
	if port < 1 || port > 65535 {
		return nil, fmt.Errorf("invalid port %d: must be between 1 and 65535", port)
	}
	ctx, cancel := context.WithCancel(context.Background())
	return &WebServer{
		port:          port,
		dir:           dir,
		stopCh:        make(chan struct{}),
		healthChecker: NewHealthChecker(port, healthCheckInterval),
		ctx:           ctx,
		cancel:        cancel,
	}, nil
}

// Start starts the opencode web UI process and begins health monitoring.
func (w *WebServer) Start() error {
	w.mu.Lock()
	defer w.mu.Unlock()

	if w.cmd != nil && w.cmd.Process != nil {
		return errors.New("web server already running")
	}

	log.Printf("[WebServer] Starting opencode web on port %d...", w.port)

	if err := w.startProcess(); err != nil {
		return fmt.Errorf("starting opencode web: %w", err)
	}

	// Start health monitoring
	w.wg.Add(1)
	go w.monitor()

	log.Printf("[WebServer] opencode web started successfully on port %d", w.port)
	return nil
}

// Stop gracefully stops the web server and health monitoring.
func (w *WebServer) Stop() error {
	w.mu.Lock()
	defer w.mu.Unlock()

	log.Println("[WebServer] Stopping opencode web...")

	// Signal monitor to stop (only once)
	w.stopOnce.Do(func() {
		close(w.stopCh)
		w.cancel()
	})

	// Stop the process
	err := w.stopProcess()

	// Wait for monitor goroutine to finish
	w.wg.Wait()

	log.Println("[WebServer] opencode web stopped")
	return err
}

// IsRunning returns true if the web server process is running.
func (w *WebServer) IsRunning() bool {
	w.mu.RLock()
	defer w.mu.RUnlock()

	return w.isProcessRunning()
}

// isProcessRunning checks if the process is still alive (internal, must hold read lock).
func (w *WebServer) isProcessRunning() bool {
	if w.cmd == nil || w.cmd.Process == nil {
		return false
	}

	// Check if process is still alive by sending signal 0
	return w.cmd.Process.Signal(syscall.Signal(0)) == nil
}

// Port returns the configured port.
func (w *WebServer) Port() int {
	return w.port
}

// URL returns the base URL for the web UI.
func (w *WebServer) URL() string {
	return fmt.Sprintf("http://localhost:%d", w.port)
}

// browserShimDir returns the path to the directory containing no-op browser shims.
func (*WebServer) browserShimDir() string {
	return filepath.Join(os.TempDir(), "oda-shims")
}

// ensureBrowserShims creates no-op scripts for commands that the `open` npm
// package uses to launch a browser. This covers Linux (xdg-open) and
// macOS (open). On macOS the real `open` is at /usr/bin/open so prepending
// our shim dir to PATH shadows it only for this child process.
func (w *WebServer) ensureBrowserShims() error {
	dir := w.browserShimDir()
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return fmt.Errorf("creating shim dir: %w", err)
	}
	script := []byte("#!/bin/sh\nexit 0\n")
	for _, name := range []string{"xdg-open", "open"} {
		if err := os.WriteFile(filepath.Join(dir, name), script, 0o755); err != nil {
			return fmt.Errorf("creating %s shim: %w", name, err)
		}
	}
	return nil
}

// startProcess starts the opencode web process.
func (w *WebServer) startProcess() error {
	w.cmd = exec.Command("opencode", "web", "--port", strconv.Itoa(w.port))
	w.cmd.Dir = w.dir

	// Prevent opencode from opening a browser window on startup.
	// The bundled `open` npm package spawns platform-specific commands
	// (xdg-open on Linux, open on macOS) as detached subprocesses.
	// We prepend a directory with no-op shims to PATH so the package
	// finds our shims instead of the real commands.
	if err := w.ensureBrowserShims(); err != nil {
		log.Printf("[WebServer] Warning: could not create browser shims: %v", err)
	} else {
		shimDir := w.browserShimDir()
		env := w.cmd.Environ()
		for i, e := range env {
			if strings.HasPrefix(e, "PATH=") {
				env[i] = "PATH=" + shimDir + ":" + e[5:]
				break
			}
		}
		w.cmd.Env = env
	}

	if err := w.cmd.Start(); err != nil {
		return fmt.Errorf("starting process: %w", err)
	}

	// Wait a moment for the process to initialize
	time.Sleep(500 * time.Millisecond)

	// Check if process is still running
	if w.cmd.Process == nil || !w.isProcessRunning() {
		return errors.New("process failed to start")
	}

	return nil
}

// stopProcess stops the opencode web process.
func (w *WebServer) stopProcess() error {
	if w.cmd == nil || w.cmd.Process == nil {
		return nil
	}

	// Try graceful termination first (SIGTERM)
	if err := w.cmd.Process.Signal(syscall.SIGTERM); err != nil {
		log.Printf("[WebServer] Warning: failed to send SIGTERM: %v", err)
	}

	// Wait for process to exit
	done := make(chan error, 1)
	go func() {
		done <- w.cmd.Wait()
	}()

	select {
	case <-done:
		// Process exited gracefully
	case <-time.After(5 * time.Second):
		// Force kill after timeout
		log.Println("[WebServer] Process did not exit gracefully, force killing...")
		if err := w.cmd.Process.Kill(); err != nil {
			return fmt.Errorf("killing process: %w", err)
		}
		<-done
	}

	w.cmd = nil
	return nil
}

// monitor runs the health monitoring and auto-restart loop.
func (w *WebServer) monitor() {
	defer w.wg.Done()

	ticker := time.NewTicker(healthCheckInterval)
	defer ticker.Stop()

	for {
		select {
		case <-w.stopCh:
			return
		case <-ticker.C:
			w.checkAndRestartIfNeeded()
		}
	}
}

// checkAndRestartIfNeeded checks the web server health and restarts if necessary.
func (w *WebServer) checkAndRestartIfNeeded() {
	w.mu.Lock()

	// Check if process is still running
	if w.isProcessRunning() {
		// Process is running, perform HTTP health check
		if w.healthChecker != nil && w.healthChecker.Check() {
			// Reset restart count on successful health check
			w.restartCount = 0
			w.mu.Unlock()
			return
		}
	}

	// Process is not running or health check failed, restart it
	log.Printf("[WebServer] Web server not responding, restarting... (attempt %d/%d)", w.restartCount+1, maxRestartAttempts)

	if w.restartCount >= maxRestartAttempts {
		log.Printf("[WebServer] Max restart attempts (%d) reached, giving up", maxRestartAttempts)
		w.mu.Unlock()
		return
	}

	// Apply exponential backoff for restart delay
	delay := min(restartDelay*time.Duration(1<<w.restartCount), 60*time.Second)

	// Increment restart count before attempting restart
	w.restartCount++
	w.mu.Unlock()

	log.Printf("[WebServer] Waiting %v before restart...", delay)
	time.Sleep(delay)

	// Re-acquire lock for process operations
	w.mu.Lock()
	defer w.mu.Unlock()

	// Check if we should stop (context canceled)
	select {
	case <-w.ctx.Done():
		return
	default:
	}

	// Stop the old process if it exists
	if w.cmd != nil && w.cmd.Process != nil {
		_ = w.stopProcess()
	}

	// Start new process
	if err := w.startProcess(); err != nil {
		log.Printf("[WebServer] Failed to restart web server: %v", err)
		return
	}

	w.lastRestart = time.Now()
	log.Printf("[WebServer] Web server restarted successfully (attempt %d)", w.restartCount)
}
