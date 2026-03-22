package internal_test

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
	"github.com/crazy-goat/one-dev-army/internal/dashboard"
	"github.com/crazy-goat/one-dev-army/internal/db"
	"github.com/crazy-goat/one-dev-army/internal/git"
	"github.com/crazy-goat/one-dev-army/internal/github"
	"github.com/crazy-goat/one-dev-army/internal/opencode"
	"github.com/crazy-goat/one-dev-army/internal/worker"
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

func integrationConfig(opencodeURL string) *config.Config {
	return &config.Config{
		GitHub:   config.GitHub{Repo: "owner/repo"},
		OpenCode: config.OpenCode{URL: opencodeURL},
		Tools: config.Tools{
			LintCmd: "echo lint-ok",
			TestCmd: "echo test-ok",
		},
		Pipeline: config.Pipeline{
			MaxRetries: 3,
			Stages: []config.Stage{
				{Name: "analysis", LLM: "claude-sonnet-4"},
				{Name: "planning", LLM: "claude-opus-4"},
				{Name: "plan-review", LLM: "claude-opus-4"},
				{Name: "coding", LLM: "claude-sonnet-4"},
				{Name: "testing", LLM: "claude-sonnet-4"},
				{Name: "code-review", LLM: "claude-opus-4"},
				{Name: "merge", ManualApproval: false},
			},
		},
		Dashboard: config.Dashboard{Port: 0},
		Workers:   config.Workers{Count: 2},
	}
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

		if r.URL.Path == "/global/health" && r.Method == http.MethodGet {
			json.NewEncoder(w).Encode(map[string]bool{"healthy": true})
			return
		}

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

		if strings.Contains(r.URL.Path, "/session/") && strings.HasSuffix(r.URL.Path, "/prompt_async") && r.Method == http.MethodPost {
			pathParts := strings.Split(strings.TrimPrefix(r.URL.Path, "/session/"), "/")
			sessID := pathParts[0]

			body, _ := io.ReadAll(r.Body)
			var req opencode.SendMessageRequest
			json.Unmarshal(body, &req)

			content := ""
			if len(req.Parts) > 0 {
				content = req.Parts[0].Text
			}

			model := ""
			if req.Model != nil {
				model = req.Model.ModelID
			}

			log.mu.Lock()
			log.messages = append(log.messages, messageLog{
				sessionID: sessID,
				model:     model,
				content:   content,
			})
			log.mu.Unlock()

			responseContent := `{"approved": true, "verdict": "looks good"}`
			if strings.Contains(content, "analyzing") || strings.Contains(content, "Analyze") {
				responseContent = `{"summary": "test analysis", "requirements": [], "complexity": "low"}`
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

		w.WriteHeader(http.StatusNotFound)
	}))

	t.Cleanup(srv.Close)
	return srv, log
}

type requestLog struct {
	mu       sync.Mutex
	sessions []string
	messages []messageLog
}

type messageLog struct {
	sessionID string
	model     string
	content   string
}

type mockQueue struct {
	mu      sync.Mutex
	tasks   []*worker.Task
	done    []int
	blocked map[int]string
	idx     int
}

func newMockQueue(tasks []*worker.Task) *mockQueue {
	return &mockQueue{tasks: tasks, blocked: make(map[int]string)}
}

func (q *mockQueue) Next() (*worker.Task, error) {
	q.mu.Lock()
	defer q.mu.Unlock()
	if q.idx >= len(q.tasks) {
		return nil, nil
	}
	t := q.tasks[q.idx]
	q.idx++
	return t, nil
}

func (q *mockQueue) MarkDone(taskID int) error {
	q.mu.Lock()
	defer q.mu.Unlock()
	q.done = append(q.done, taskID)
	return nil
}

func (q *mockQueue) MarkBlocked(taskID int, reason string) error {
	q.mu.Lock()
	defer q.mu.Unlock()
	q.blocked[taskID] = reason
	return nil
}

func TestFullPipelineWithMockOpencode(t *testing.T) {
	srv, log := startMockOpencode(t)

	oc := opencode.NewClient(srv.URL)
	if err := oc.HealthCheck(); err != nil {
		t.Fatalf("health check failed: %v", err)
	}

	cfg := integrationConfig(srv.URL)

	tmpDir := t.TempDir()
	dbPath := filepath.Join(tmpDir, "oda.db")
	store, err := db.Open(dbPath)
	if err != nil {
		t.Fatalf("opening db: %v", err)
	}
	defer store.Close()

	repoDir := t.TempDir()
	wtDir := t.TempDir()
	initGitRepo(t, repoDir)
	wtMgr := git.NewWorktreeManager(repoDir, wtDir)

	ghClient := &github.Client{Repo: "owner/repo"}
	proc := worker.NewProcessor(cfg, oc, ghClient, store, wtMgr)

	task := &worker.Task{
		ID:          1,
		IssueNumber: 42,
		Title:       "Add integration tests",
		Body:        "Write integration tests for the pipeline",
		Stage:       "",
	}

	w := worker.NewWorker("integration-worker")
	w.SetTask(task)

	err = proc.Process(context.Background(), w, task)
	if err != nil && !strings.Contains(err.Error(), "gh ") && !strings.Contains(err.Error(), "creating PR") {
		t.Fatalf("unexpected error: %v", err)
	}

	metrics, err := store.GetTaskMetrics(1)
	if err != nil {
		t.Fatalf("getting metrics: %v", err)
	}
	if len(metrics) == 0 {
		t.Fatal("expected metrics to be saved to SQLite")
	}

	stagesSeen := make(map[string]bool)
	for _, m := range metrics {
		stagesSeen[m.Stage] = true
	}
	for _, expected := range []string{"analysis", "planning", "plan-review", "coding", "testing", "code-review"} {
		if !stagesSeen[expected] {
			t.Errorf("missing metric for stage %q", expected)
		}
	}

	log.mu.Lock()
	defer log.mu.Unlock()

	expectedStages := []string{"analysis", "planning", "plan-review", "coding", "code-review"}
	for _, stage := range expectedStages {
		found := false
		for _, title := range log.sessions {
			if strings.Contains(title, stage) && strings.Contains(title, "#42") {
				found = true
				break
			}
		}
		if !found {
			t.Errorf("no session created for stage %q with issue #42", stage)
		}
	}

	if len(log.messages) < len(expectedStages) {
		t.Errorf("expected at least %d messages, got %d", len(expectedStages), len(log.messages))
	}
}

func TestWorkerPoolWithPipeline(t *testing.T) {
	tasks := []*worker.Task{
		{ID: 1, IssueNumber: 10, Title: "Task A", Body: "body A"},
		{ID: 2, IssueNumber: 11, Title: "Task B", Body: "body B"},
		{ID: 3, IssueNumber: 12, Title: "Task C", Body: "body C"},
	}
	queue := newMockQueue(tasks)

	var mu sync.Mutex
	processed := make(map[int][]string)

	proc := &stagingProcessor{
		mu:        &mu,
		processed: processed,
	}

	pool := worker.NewPool(2, queue, proc)
	pool.Start(context.Background())

	done := make(chan struct{})
	go func() {
		pool.Wait()
		close(done)
	}()

	select {
	case <-done:
	case <-time.After(10 * time.Second):
		t.Fatal("pool did not finish within timeout")
	}

	mu.Lock()
	defer mu.Unlock()

	if len(processed) != 3 {
		t.Fatalf("expected 3 tasks processed, got %d", len(processed))
	}

	for _, task := range tasks {
		stages, ok := processed[task.ID]
		if !ok {
			t.Errorf("task %d was not processed", task.ID)
			continue
		}
		if len(stages) == 0 {
			t.Errorf("task %d has no recorded stages", task.ID)
		}
	}

	queue.mu.Lock()
	defer queue.mu.Unlock()

	totalHandled := len(queue.done) + len(queue.blocked)
	if totalHandled != 3 {
		t.Errorf("expected 3 tasks handled (done+blocked), got %d", totalHandled)
	}
}

type stagingProcessor struct {
	mu        *sync.Mutex
	processed map[int][]string
}

func (p *stagingProcessor) Process(_ context.Context, w *worker.Worker, task *worker.Task) error {
	stages := []string{"analysis", "planning", "coding", "done"}
	for _, s := range stages {
		w.SetStage(s)
		time.Sleep(2 * time.Millisecond)
	}
	p.mu.Lock()
	p.processed[task.ID] = stages
	p.mu.Unlock()
	return nil
}

func TestConfigToProcessorWiring(t *testing.T) {
	var mu sync.Mutex
	modelsUsed := make(map[string]string)

	var sessionCounter int
	var counterMu sync.Mutex
	wiringHub := newSSEHub()

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
			wiringHub.add(client)
			defer wiringHub.remove(client)

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

			json.NewEncoder(w).Encode(opencode.Session{ID: sessID, Title: req["title"]})
			return
		}

		if strings.Contains(r.URL.Path, "/session/") && strings.HasSuffix(r.URL.Path, "/prompt_async") && r.Method == http.MethodPost {
			pathParts := strings.Split(strings.TrimPrefix(r.URL.Path, "/session/"), "/")
			sessID := pathParts[0]

			body, _ := io.ReadAll(r.Body)
			var req opencode.SendMessageRequest
			json.Unmarshal(body, &req)

			model := ""
			if req.Model != nil {
				model = req.Model.ModelID
			}
			mu.Lock()
			modelsUsed[sessID] = model
			mu.Unlock()

			responseContent := `{"summary": "done", "complexity": "low"}`

			w.WriteHeader(http.StatusNoContent)

			go func() {
				time.Sleep(5 * time.Millisecond)
				msgUpdated := fmt.Sprintf(`{"type":"message.updated","properties":{"info":{"id":"msg-1","sessionID":"%s","role":"assistant"}}}`, sessID)
				wiringHub.broadcast(msgUpdated)

				delta := fmt.Sprintf(`{"type":"message.part.delta","properties":{"sessionID":"%s","messageID":"msg-1","partID":"prt-1","field":"text","delta":"%s"}}`,
					sessID, strings.ReplaceAll(responseContent, `"`, `\"`))
				wiringHub.broadcast(delta)

				idle := fmt.Sprintf(`{"type":"session.status","properties":{"sessionID":"%s","status":{"type":"idle"}}}`, sessID)
				wiringHub.broadcast(idle)
			}()
			return
		}

		w.WriteHeader(http.StatusNotFound)
	}))
	defer srv.Close()

	cfg := &config.Config{
		GitHub:   config.GitHub{Repo: "owner/repo"},
		OpenCode: config.OpenCode{URL: srv.URL},
		Pipeline: config.Pipeline{
			MaxRetries: 3,
			Stages: []config.Stage{
				{Name: "analysis", LLM: "custom-model-for-analysis"},
				{Name: "planning", LLM: "claude-opus-4"},
				{Name: "plan-review", LLM: "claude-opus-4"},
				{Name: "coding", LLM: "claude-sonnet-4"},
				{Name: "testing", LLM: "claude-sonnet-4"},
				{Name: "code-review", LLM: "claude-opus-4"},
			},
		},
	}

	oc := opencode.NewClient(srv.URL)
	store, err := db.Open(filepath.Join(t.TempDir(), "test.db"))
	if err != nil {
		t.Fatalf("opening db: %v", err)
	}
	defer store.Close()

	repoDir := t.TempDir()
	wtDir := t.TempDir()
	initGitRepo(t, repoDir)
	wtMgr := git.NewWorktreeManager(repoDir, wtDir)

	task := &worker.Task{
		ID:          1,
		IssueNumber: 77,
		Title:       "Config wiring test",
		Body:        "Verify model wiring",
	}

	wt, err := wtMgr.Create("wiring-worker", "task/77-config-wiring-test")
	if err != nil {
		t.Fatalf("creating worktree: %v", err)
	}

	executor := worker.NewStageExecutor(cfg, oc, store, task, wt)

	_, err = executor.Execute(1, "analysis", "test context")
	if err != nil {
		t.Fatalf("executing analysis stage: %v", err)
	}

	mu.Lock()
	defer mu.Unlock()

	foundModel := false
	for _, model := range modelsUsed {
		if model == "custom-model-for-analysis" {
			foundModel = true
			break
		}
	}
	if !foundModel {
		t.Errorf("expected model 'custom-model-for-analysis' to be used, got models: %v", modelsUsed)
	}
}

func TestDashboardRendering(t *testing.T) {
	store, err := db.Open(filepath.Join(t.TempDir(), "dash.db"))
	if err != nil {
		t.Fatalf("opening db: %v", err)
	}
	defer store.Close()

	poolFn := func() []worker.WorkerInfo {
		return []worker.WorkerInfo{
			{ID: "worker-1", Status: worker.StatusIdle},
			{ID: "worker-2", Status: worker.StatusWorking, TaskID: 5, TaskTitle: "Test task", Stage: "coding", Elapsed: 30 * time.Second},
		}
	}

	srv, err := dashboard.NewServer(0, store, poolFn, nil, 0, nil, nil, "")
	if err != nil {
		t.Fatalf("creating dashboard server: %v", err)
	}

	tests := []struct {
		path       string
		wantStatus int
		wantBody   string
	}{
		{"/", http.StatusOK, "Sprint Board"},
		{"/backlog", http.StatusOK, "Backlog"},
		{"/costs", http.StatusOK, "Costs"},
		{"/api/workers", http.StatusOK, "worker-1"},
	}

	for _, tt := range tests {
		t.Run(tt.path, func(t *testing.T) {
			req := httptest.NewRequest(http.MethodGet, tt.path, nil)
			rec := httptest.NewRecorder()
			srv.Handler().ServeHTTP(rec, req)

			if rec.Code != tt.wantStatus {
				t.Errorf("GET %s: status = %d, want %d", tt.path, rec.Code, tt.wantStatus)
			}

			body := rec.Body.String()
			if !strings.Contains(body, tt.wantBody) {
				t.Errorf("GET %s: body does not contain %q\nbody (first 500 chars): %s", tt.path, tt.wantBody, truncate(body, 500))
			}
		})
	}
}

func truncate(s string, n int) string {
	if len(s) <= n {
		return s
	}
	return s[:n] + "..."
}
