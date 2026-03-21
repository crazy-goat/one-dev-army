package worker_test

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

	"github.com/one-dev-army/oda/internal/config"
	"github.com/one-dev-army/oda/internal/db"
	"github.com/one-dev-army/oda/internal/git"
	"github.com/one-dev-army/oda/internal/github"
	"github.com/one-dev-army/oda/internal/opencode"
	"github.com/one-dev-army/oda/internal/pipeline"
	"github.com/one-dev-army/oda/internal/worker"
)

func TestSlugify(t *testing.T) {
	tests := []struct {
		input string
		want  string
	}{
		{"Add user authentication", "add-user-authentication"},
		{"Fix bug #123", "fix-bug-123"},
		{"  spaces  around  ", "spaces-around"},
		{"UPPERCASE Title", "uppercase-title"},
		{"special!@#$%chars", "specialchars"},
		{"multiple---dashes", "multiple-dashes"},
		{"", ""},
		{"a", "a"},
		{"This is a very long title that should be truncated to fifty characters maximum length", "this-is-a-very-long-title-that-should-be-truncated"},
		{"trailing-dash-at-cutoff-xxxxxxxxxxxxxxxxxxxxxxxxxx-", "trailing-dash-at-cutoff-xxxxxxxxxxxxxxxxxxxxxxxxxx"},
		{"hello world  foo", "hello-world-foo"},
		{"-leading-dash", "leading-dash"},
		{"dots.and.periods", "dotsandperiods"},
	}

	for _, tt := range tests {
		t.Run(tt.input, func(t *testing.T) {
			got := worker.Slugify(tt.input)
			if got != tt.want {
				t.Errorf("Slugify(%q) = %q, want %q", tt.input, got, tt.want)
			}
		})
	}
}

func TestBranchName(t *testing.T) {
	got := worker.BranchName(42, "Add user auth")
	want := "task/42-add-user-auth"
	if got != want {
		t.Errorf("BranchName(42, \"Add user auth\") = %q, want %q", got, want)
	}
}

func testConfig() *config.Config {
	return &config.Config{
		GitHub:   config.GitHub{Repo: "owner/repo"},
		OpenCode: config.OpenCode{URL: "http://localhost:4096"},
		Tools: config.Tools{
			LintCmd: "echo lint-ok",
			TestCmd: "echo test-ok",
			E2ECmd:  "echo e2e-ok",
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
	}
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

func mockOpenCodeServer(t *testing.T, log *requestLog) *httptest.Server {
	t.Helper()

	var sessionCounter int
	var counterMu sync.Mutex

	return httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
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

			json.NewEncoder(w).Encode(opencode.Session{
				ID:    sessID,
				Title: req["title"],
			})
			return
		}

		if strings.Contains(r.URL.Path, "/session/") && strings.HasSuffix(r.URL.Path, "/message") && r.Method == http.MethodPost {
			pathParts := strings.Split(strings.TrimPrefix(r.URL.Path, "/session/"), "/")
			sessID := pathParts[0]

			body, _ := io.ReadAll(r.Body)
			var req opencode.SendMessageRequest
			json.Unmarshal(body, &req)

			log.mu.Lock()
			content := ""
			if len(req.Parts) > 0 {
				content = req.Parts[0].Content
			}
			log.messages = append(log.messages, messageLog{
				sessionID: sessID,
				model:     req.Model,
				content:   content,
			})
			log.mu.Unlock()

			responseContent := `{"approved": true, "verdict": "looks good"}`
			if strings.Contains(content, "Analyze") || strings.Contains(content, "analyzing") {
				responseContent = `{"summary": "test analysis", "requirements": [], "complexity": "low"}`
			}

			json.NewEncoder(w).Encode(opencode.Message{
				Info: opencode.MessageInfo{
					ID:        "msg-1",
					SessionID: sessID,
					Role:      "assistant",
				},
				Parts: []opencode.Part{
					{Type: "text", Content: responseContent},
				},
			})
			return
		}

		w.WriteHeader(http.StatusNotFound)
	}))
}

func TestStageExecutor_Analysis(t *testing.T) {
	log := &requestLog{}
	srv := mockOpenCodeServer(t, log)
	defer srv.Close()

	cfg := testConfig()
	cfg.OpenCode.URL = srv.URL

	oc := opencode.NewClient(srv.URL)

	tmpDir := t.TempDir()
	dbPath := filepath.Join(tmpDir, "test.db")
	store, err := db.Open(dbPath)
	if err != nil {
		t.Fatalf("opening db: %v", err)
	}
	defer store.Close()

	task := &worker.Task{
		ID:          1,
		IssueNumber: 42,
		Title:       "Add user auth",
		Body:        "Implement JWT authentication",
		Stage:       "",
	}

	repoDir := t.TempDir()
	wtDir := t.TempDir()
	initGitRepo(t, repoDir)
	wtMgr := git.NewWorktreeManager(repoDir, wtDir)

	wt, err := wtMgr.Create("test-worker", "task/42-add-user-auth")
	if err != nil {
		t.Fatalf("creating worktree: %v", err)
	}

	executor := worker.NewStageExecutor(cfg, oc, store, task, wt)

	result, err := executor.Execute(1, pipeline.StageAnalysis, "test context")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if !result.Success {
		t.Error("expected success=true")
	}
	if result.Output == "" {
		t.Error("expected non-empty output")
	}

	log.mu.Lock()
	defer log.mu.Unlock()

	if len(log.sessions) != 1 {
		t.Fatalf("expected 1 session created, got %d", len(log.sessions))
	}
	if !strings.Contains(log.sessions[0], "analysis") {
		t.Errorf("session title %q should contain 'analysis'", log.sessions[0])
	}
	if !strings.Contains(log.sessions[0], "#42") {
		t.Errorf("session title %q should contain '#42'", log.sessions[0])
	}

	if len(log.messages) != 1 {
		t.Fatalf("expected 1 message sent, got %d", len(log.messages))
	}
	if log.messages[0].model != "claude-sonnet-4" {
		t.Errorf("model = %q, want %q", log.messages[0].model, "claude-sonnet-4")
	}
	if !strings.Contains(log.messages[0].content, "Add user auth") {
		t.Error("message should contain issue title")
	}
	if !strings.Contains(log.messages[0].content, "JWT authentication") {
		t.Error("message should contain issue body")
	}

	metrics, err := store.GetTaskMetrics(1)
	if err != nil {
		t.Fatalf("getting metrics: %v", err)
	}
	if len(metrics) != 1 {
		t.Fatalf("expected 1 metric, got %d", len(metrics))
	}
	if metrics[0].Stage != "analysis" {
		t.Errorf("metric stage = %q, want %q", metrics[0].Stage, "analysis")
	}
}

func TestStageExecutor_PlanReview_Approved(t *testing.T) {
	log := &requestLog{}
	srv := mockOpenCodeServer(t, log)
	defer srv.Close()

	cfg := testConfig()
	oc := opencode.NewClient(srv.URL)

	task := &worker.Task{
		ID:          1,
		IssueNumber: 10,
		Title:       "Test task",
		Body:        "Test body",
	}

	repoDir := t.TempDir()
	wtDir := t.TempDir()
	initGitRepo(t, repoDir)
	wtMgr := git.NewWorktreeManager(repoDir, wtDir)
	wt, err := wtMgr.Create("test-worker-pr", "task/10-test-task")
	if err != nil {
		t.Fatalf("creating worktree: %v", err)
	}

	executor := worker.NewStageExecutor(cfg, oc, nil, task, wt)

	result, err := executor.Execute(1, pipeline.StagePlanReview, `{"steps": []}`)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if !result.Success {
		t.Error("expected plan review to be approved")
	}
}

func TestStageExecutor_Testing_Success(t *testing.T) {
	cfg := testConfig()
	cfg.Tools.LintCmd = "echo ok"
	cfg.Tools.TestCmd = "echo ok"
	cfg.Tools.E2ECmd = ""

	task := &worker.Task{
		ID:          1,
		IssueNumber: 10,
		Title:       "Test task",
		Body:        "Test body",
	}

	repoDir := t.TempDir()
	wtDir := t.TempDir()
	initGitRepo(t, repoDir)
	wtMgr := git.NewWorktreeManager(repoDir, wtDir)
	wt, err := wtMgr.Create("test-worker-ts", "task/10-test-task-success")
	if err != nil {
		t.Fatalf("creating worktree: %v", err)
	}

	executor := worker.NewStageExecutor(cfg, nil, nil, task, wt)

	result, err := executor.Execute(1, pipeline.StageTesting, "")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if !result.Success {
		t.Errorf("expected success, got failure: %s", result.Output)
	}
}

func TestStageExecutor_Testing_Failure(t *testing.T) {
	cfg := testConfig()
	cfg.Tools.LintCmd = "echo ok"
	cfg.Tools.TestCmd = "false"
	cfg.Tools.E2ECmd = ""

	task := &worker.Task{
		ID:          1,
		IssueNumber: 10,
		Title:       "Test task",
		Body:        "Test body",
	}

	repoDir := t.TempDir()
	wtDir := t.TempDir()
	initGitRepo(t, repoDir)
	wtMgr := git.NewWorktreeManager(repoDir, wtDir)
	wt, err := wtMgr.Create("test-worker-tf", "task/10-test-task-fail")
	if err != nil {
		t.Fatalf("creating worktree: %v", err)
	}

	executor := worker.NewStageExecutor(cfg, nil, nil, task, wt)

	result, err := executor.Execute(1, pipeline.StageTesting, "")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if result.Success {
		t.Error("expected failure when test command fails")
	}
	if !strings.Contains(result.Output, "test:") {
		t.Errorf("output should mention test failure, got: %s", result.Output)
	}
}

func TestStageExecutor_Merging(t *testing.T) {
	cfg := testConfig()
	task := &worker.Task{
		ID:          1,
		IssueNumber: 10,
		Title:       "Test task",
		Body:        "Test body",
	}

	executor := worker.NewStageExecutor(cfg, nil, nil, task, nil)

	result, err := executor.Execute(1, pipeline.StageMerging, "")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if !result.Success {
		t.Error("merging stage should always succeed")
	}
}

func TestProcess_FullPipeline(t *testing.T) {
	log := &requestLog{}
	srv := mockOpenCodeServer(t, log)
	defer srv.Close()

	cfg := testConfig()
	cfg.OpenCode.URL = srv.URL
	cfg.Tools.LintCmd = "echo lint-ok"
	cfg.Tools.TestCmd = "echo test-ok"
	cfg.Tools.E2ECmd = ""

	oc := opencode.NewClient(srv.URL)

	tmpDir := t.TempDir()
	dbPath := filepath.Join(tmpDir, "test.db")
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
		IssueNumber: 99,
		Title:       "Implement feature X",
		Body:        "We need feature X for the product",
		Stage:       "",
	}

	w := worker.NewWorker("test-worker-fp")
	w.SetTask(task)

	err = proc.Process(context.Background(), w, task)

	if err != nil {
		if strings.Contains(err.Error(), "creating PR") || strings.Contains(err.Error(), "gh ") {
			t.Logf("expected gh CLI error in test environment: %v", err)
		} else {
			t.Fatalf("unexpected error: %v", err)
		}
	}

	log.mu.Lock()
	defer log.mu.Unlock()

	expectedStages := []string{"analysis", "planning", "plan-review", "coding", "code-review"}
	if len(log.sessions) < len(expectedStages) {
		t.Fatalf("expected at least %d sessions, got %d: %v", len(expectedStages), len(log.sessions), log.sessions)
	}

	for _, stage := range expectedStages {
		found := false
		for _, title := range log.sessions {
			if strings.Contains(title, stage) && strings.Contains(title, "#99") {
				found = true
				break
			}
		}
		if !found {
			t.Errorf("no session found for stage %q with issue #99, sessions: %v", stage, log.sessions)
		}
	}

	metrics, mErr := store.GetTaskMetrics(1)
	if mErr != nil {
		t.Fatalf("getting metrics: %v", mErr)
	}
	if len(metrics) == 0 {
		t.Error("expected metrics to be recorded")
	}
}

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
