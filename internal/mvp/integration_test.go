package mvp_test

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/crazy-goat/one-dev-army/internal/config"
	"github.com/crazy-goat/one-dev-army/internal/git"
	"github.com/crazy-goat/one-dev-army/internal/github"
	"github.com/crazy-goat/one-dev-army/internal/mvp"
	"github.com/crazy-goat/one-dev-army/internal/opencode"
)

func initGitRepo(t *testing.T, dir string) {
	t.Helper()
	run := func(args ...string) {
		t.Helper()
		cmd := exec.Command(args[0], args[1:]...)
		cmd.Dir = dir
		if out, err := cmd.CombinedOutput(); err != nil {
			t.Fatalf("running %v: %v\n%s", args, err, out)
		}
	}
	run("git", "init")
	run("git", "config", "user.email", "test@test.com")
	run("git", "config", "user.name", "Test")
	if err := os.WriteFile(filepath.Join(dir, "README.md"), []byte("# test\n"), 0o644); err != nil {
		t.Fatalf("writing README: %v", err)
	}
	run("git", "add", ".")
	run("git", "commit", "-m", "initial")
}

type sseClient struct {
	w       http.ResponseWriter
	flusher http.Flusher
}

type sseHub struct {
	mu      sync.Mutex
	clients map[*sseClient]bool
}

func newSSEHub() *sseHub {
	return &sseHub{clients: make(map[*sseClient]bool)}
}

func (h *sseHub) add(c *sseClient) {
	h.mu.Lock()
	h.clients[c] = true
	h.mu.Unlock()
}

func (h *sseHub) remove(c *sseClient) {
	h.mu.Lock()
	delete(h.clients, c)
	h.mu.Unlock()
}

func (h *sseHub) broadcast(data string) {
	h.mu.Lock()
	defer h.mu.Unlock()
	for c := range h.clients {
		fmt.Fprintf(c.w, "data: %s\n\n", data)
		c.flusher.Flush()
	}
}

type requestLog struct {
	mu       sync.Mutex
	sessions []string
	messages []string
}

func startMockOpencode(t *testing.T) (*httptest.Server, *requestLog) {
	t.Helper()
	log := &requestLog{}
	var sessionCounter int
	var counterMu sync.Mutex
	hub := newSSEHub()

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/event" && r.Method == http.MethodGet {
			flusher, ok := w.(http.Flusher)
			if !ok {
				http.Error(w, "streaming not supported", http.StatusInternalServerError)
				return
			}
			w.Header().Set("Content-Type", "text/event-stream")
			w.Header().Set("Cache-Control", "no-cache")
			w.Header().Set("Connection", "keep-alive")
			w.WriteHeader(http.StatusOK)

			fmt.Fprintf(w, "data: %s\n\n", `{"type":"server.connected","properties":{}}`)
			flusher.Flush()

			client := &sseClient{w: w, flusher: flusher}
			hub.add(client)
			defer hub.remove(client)

			<-r.Context().Done()
			return
		}

		w.Header().Set("Content-Type", "application/json")

		if r.URL.Path == "/session" && r.Method == http.MethodPost {
			body, _ := io.ReadAll(r.Body)
			var req map[string]string
			json.Unmarshal(body, &req)

			counterMu.Lock()
			sessionCounter++
			sessID := fmt.Sprintf("sess-%d", sessionCounter)
			counterMu.Unlock()

			log.mu.Lock()
			log.sessions = append(log.sessions, req["title"])
			log.mu.Unlock()

			json.NewEncoder(w).Encode(opencode.Session{ID: sessID, Title: req["title"]})
			return
		}

		if strings.HasPrefix(r.URL.Path, "/session/") && strings.HasSuffix(r.URL.Path, "/prompt_async") && r.Method == http.MethodPost {
			pathParts := strings.Split(strings.TrimPrefix(r.URL.Path, "/session/"), "/")
			sessID := pathParts[0]

			body, _ := io.ReadAll(r.Body)
			var req opencode.SendMessageRequest
			json.Unmarshal(body, &req)

			content := ""
			if len(req.Parts) > 0 {
				content = req.Parts[0].Text
			}

			log.mu.Lock()
			log.messages = append(log.messages, content)
			log.mu.Unlock()

			responseContent := "mock analysis result"
			if strings.Contains(content, "plan") || strings.Contains(content, "Plan") {
				responseContent = "mock plan result"
			} else if strings.Contains(content, "Implement") {
				responseContent = "mock implementation done"
			}

			w.WriteHeader(http.StatusNoContent)

			go func() {
				time.Sleep(5 * time.Millisecond)
				msgUpdated := fmt.Sprintf(`{"type":"message.updated","properties":{"info":{"id":"msg-1","sessionID":"%s","role":"assistant"}}}`, sessID)
				hub.broadcast(msgUpdated)

				delta := fmt.Sprintf(`{"type":"message.part.delta","properties":{"sessionID":"%s","messageID":"msg-1","partID":"prt-1","field":"text","delta":"%s"}}`,
					sessID, strings.ReplaceAll(responseContent, `"`, `\"`))
				hub.broadcast(delta)

				idle := fmt.Sprintf(`{"type":"session.status","properties":{"sessionID":"%s","status":{"type":"idle"}}}`, sessID)
				hub.broadcast(idle)
			}()
			return
		}

		if strings.HasPrefix(r.URL.Path, "/session/") && r.Method == http.MethodDelete {
			w.WriteHeader(http.StatusNoContent)
			return
		}

		w.WriteHeader(http.StatusNotFound)
	}))

	t.Cleanup(srv.Close)
	return srv, log
}

func TestWorkerProcessEndToEnd(t *testing.T) {
	ocSrv, log := startMockOpencode(t)

	repoDir := t.TempDir()
	wtDir := t.TempDir()
	initGitRepo(t, repoDir)

	wtMgr := git.NewWorktreeManager(repoDir, wtDir)
	oc := opencode.NewClient(ocSrv.URL)
	gh := &github.Client{Repo: "owner/repo"}

	cfg := &config.Config{
		Planning:     config.Planning{LLM: "claude-sonnet-4"},
		EpicAnalysis: config.EpicAnalysis{LLM: "claude-sonnet-4"},
		Tools:        config.Tools{TestCmd: "echo test-ok"},
	}

	w := mvp.NewWorker(1, cfg, oc, gh, wtMgr)

	task := &mvp.Task{
		Issue: github.Issue{
			Number: 99,
			Title:  "Add integration test",
			Body:   "Write an integration test for the MVP worker",
		},
		Milestone: "Sprint 1",
		Status:    mvp.StatusPending,
	}

	err := w.Process(context.Background(), task)

	if err != nil && !strings.Contains(err.Error(), "creating PR") && !strings.Contains(err.Error(), "pushing branch") {
		t.Fatalf("unexpected error: %v", err)
	}

	if task.Branch == "" {
		t.Error("expected Branch to be set")
	}
	if !strings.HasPrefix(task.Branch, "oda-99-") {
		t.Errorf("Branch = %q, want prefix 'oda-99-'", task.Branch)
	}

	if task.Worktree == "" {
		t.Error("expected Worktree to be set")
	}

	log.mu.Lock()
	defer log.mu.Unlock()

	expectedSessions := []string{"analyze-99", "plan-99", "implement-99"}
	for _, expected := range expectedSessions {
		found := false
		for _, s := range log.sessions {
			if s == expected {
				found = true
				break
			}
		}
		if !found {
			t.Errorf("expected session %q to be created, got sessions: %v", expected, log.sessions)
		}
	}

	if len(log.messages) < 3 {
		t.Errorf("expected at least 3 messages sent, got %d", len(log.messages))
	}
}

func TestWorkerProcessStatusTransitions(t *testing.T) {
	ocSrv, _ := startMockOpencode(t)

	repoDir := t.TempDir()
	wtDir := t.TempDir()
	initGitRepo(t, repoDir)

	wtMgr := git.NewWorktreeManager(repoDir, wtDir)
	oc := opencode.NewClient(ocSrv.URL)
	gh := &github.Client{Repo: "owner/repo"}

	cfg := &config.Config{
		Planning:     config.Planning{LLM: "claude-sonnet-4"},
		EpicAnalysis: config.EpicAnalysis{LLM: "claude-sonnet-4"},
		Tools:        config.Tools{TestCmd: "echo ok"},
	}

	w := mvp.NewWorker(1, cfg, oc, gh, wtMgr)

	task := &mvp.Task{
		Issue: github.Issue{
			Number: 100,
			Title:  "Status test",
			Body:   "Test status transitions",
		},
		Status: mvp.StatusPending,
	}

	_ = w.Process(context.Background(), task)

	if task.Status != mvp.StatusFailed && task.Status != mvp.StatusDone {
		t.Errorf("final Status = %q, want 'done' or 'failed'", task.Status)
	}
}

func TestWorkerProcessTestFailure(t *testing.T) {
	ocSrv, _ := startMockOpencode(t)

	repoDir := t.TempDir()
	wtDir := t.TempDir()
	initGitRepo(t, repoDir)

	wtMgr := git.NewWorktreeManager(repoDir, wtDir)
	oc := opencode.NewClient(ocSrv.URL)
	gh := &github.Client{Repo: "owner/repo"}

	cfg := &config.Config{
		Planning:     config.Planning{LLM: "claude-sonnet-4"},
		EpicAnalysis: config.EpicAnalysis{LLM: "claude-sonnet-4"},
		Tools:        config.Tools{TestCmd: "exit 1"},
	}

	w := mvp.NewWorker(1, cfg, oc, gh, wtMgr)

	task := &mvp.Task{
		Issue: github.Issue{
			Number: 101,
			Title:  "Failing test",
			Body:   "This should fail at testing stage",
		},
		Status: mvp.StatusPending,
	}

	err := w.Process(context.Background(), task)

	if err == nil {
		t.Fatal("expected error from failing tests")
	}
	if !strings.Contains(err.Error(), "testing") {
		t.Errorf("error = %q, want to contain 'testing'", err.Error())
	}
	if task.Status != mvp.StatusFailed {
		t.Errorf("Status = %q, want 'failed'", task.Status)
	}
	if task.Result == nil || task.Result.Error == nil {
		t.Error("expected Result.Error to be set")
	}
}
