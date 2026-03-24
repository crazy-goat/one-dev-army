package opencode

import (
	"fmt"
	"net/http"
	"os/exec"
	"time"
)

// Server manages a spawned opencode serve process.
type Server struct {
	cmd     *exec.Cmd
	baseURL string
}

// StartServer starts "opencode serve" as a background process in the given directory.
// It waits up to timeout for the health check to pass.
// Returns a *Server that must be stopped via Stop() when done.
func StartServer(baseURL, dir string, timeout time.Duration) (*Server, error) {
	cmd := exec.Command("opencode", "serve")
	cmd.Dir = dir

	if err := cmd.Start(); err != nil {
		return nil, fmt.Errorf("starting opencode serve: %w", err)
	}

	s := &Server{cmd: cmd, baseURL: baseURL}

	if err := s.waitHealthy(timeout); err != nil {
		_ = cmd.Process.Kill()
		return nil, fmt.Errorf("opencode serve did not become healthy: %w", err)
	}

	return s, nil
}

// Stop terminates the spawned opencode serve process.
func (s *Server) Stop() error {
	if s.cmd == nil || s.cmd.Process == nil {
		return nil
	}
	if err := s.cmd.Process.Kill(); err != nil {
		return fmt.Errorf("stopping opencode serve: %w", err)
	}
	_ = s.cmd.Wait()
	return nil
}

func (s *Server) waitHealthy(timeout time.Duration) error {
	client := &http.Client{Timeout: 2 * time.Second}
	deadline := time.Now().Add(timeout)

	for time.Now().Before(deadline) {
		resp, err := client.Get(s.baseURL + "/global/health")
		if err == nil {
			_ = resp.Body.Close()
			if resp.StatusCode == http.StatusOK {
				return nil
			}
		}
		time.Sleep(500 * time.Millisecond)
	}

	return fmt.Errorf("timed out after %s", timeout)
}
