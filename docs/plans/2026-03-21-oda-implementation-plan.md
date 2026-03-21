# ODA (One Dev Army) Implementation Plan

> **For Claude:** REQUIRED SUB-SKILL: Use superpowers:executing-plans to implement this plan task-by-task.

**Goal:** Build a Go binary that orchestrates opencode workers to implement GitHub issues through a scrum-like pipeline, with an HTMX dashboard.

**Architecture:** Single Go binary with goroutine-based worker pool. Each worker creates opencode sessions via HTTP API, operates on git worktrees. GitHub (via gh CLI) is source of truth. SQLite stores metrics. HTMX + Go templates for dashboard.

**Tech Stack:** Go 1.22+, HTMX 2.x, Go html/template, SQLite (modernc.org/sqlite), gh CLI, git CLI, net/http

---

## Phase 1: Project Skeleton & Core Infrastructure

### Task 1: Go Module & Project Structure

**Files:**
- Create: `go.mod`
- Create: `main.go`
- Create: `internal/config/config.go`
- Create: `internal/config/config_test.go`

**Step 1: Initialize Go module**

Run: `go mod init github.com/one-dev-army/oda`

**Step 2: Create project directory structure**

```bash
mkdir -p internal/{config,preflight,github,opencode,worker,pipeline,db,dashboard,scheduler}
mkdir -p internal/dashboard/{templates,static}
mkdir -p cmd
```

**Step 3: Write config struct and loader test**

```go
// internal/config/config_test.go
package config

import (
	"os"
	"path/filepath"
	"testing"
)

func TestLoadConfig(t *testing.T) {
	dir := t.TempDir()
	odaDir := filepath.Join(dir, ".oda")
	os.MkdirAll(odaDir, 0755)

	yaml := `github:
  repo: "owner/repo"
dashboard:
  port: 8080
workers:
  count: 3
opencode:
  url: "http://localhost:4096"
tools:
  lint_cmd: "make lint"
  test_cmd: "make test"
  e2e_cmd: "make e2e"
pipeline:
  stages:
    - name: analysis
      llm: claude-sonnet-4
    - name: planning
      llm: claude-opus-4
    - name: plan-review
      llm: claude-opus-4
    - name: coding
      llm: claude-sonnet-4
    - name: testing
      llm: claude-sonnet-4
    - name: code-review
      llm: claude-opus-4
    - name: merge
      manual_approval: true
  max_retries: 5
planning:
  llm: claude-opus-4
epic_analysis:
  llm: claude-sonnet-4
sprint:
  tasks_per_sprint: 10
`
	os.WriteFile(filepath.Join(odaDir, "config.yaml"), []byte(yaml), 0644)

	cfg, err := Load(dir)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if cfg.Workers.Count != 3 {
		t.Errorf("expected 3 workers, got %d", cfg.Workers.Count)
	}
	if cfg.Dashboard.Port != 8080 {
		t.Errorf("expected port 8080, got %d", cfg.Dashboard.Port)
	}
	if len(cfg.Pipeline.Stages) != 7 {
		t.Errorf("expected 7 stages, got %d", len(cfg.Pipeline.Stages))
	}
}

func TestLoadConfigMissing(t *testing.T) {
	dir := t.TempDir()
	_, err := Load(dir)
	if err == nil {
		t.Fatal("expected error for missing config")
	}
}
```

**Step 4: Run test to verify it fails**

Run: `go test ./internal/config/ -v`
Expected: FAIL - package/types not defined

**Step 5: Implement config loader**

```go
// internal/config/config.go
package config

import (
	"fmt"
	"os"
	"path/filepath"

	"gopkg.in/yaml.v3"
)

type Config struct {
	GitHub      GitHubConfig      `yaml:"github"`
	Dashboard   DashboardConfig   `yaml:"dashboard"`
	Workers     WorkersConfig     `yaml:"workers"`
	Opencode    OpencodeConfig    `yaml:"opencode"`
	Tools       ToolsConfig       `yaml:"tools"`
	Pipeline    PipelineConfig    `yaml:"pipeline"`
	Planning    LLMConfig         `yaml:"planning"`
	EpicAnalysis LLMConfig        `yaml:"epic_analysis"`
	Sprint      SprintConfig      `yaml:"sprint"`
}

type GitHubConfig struct {
	Repo string `yaml:"repo"`
}

type DashboardConfig struct {
	Port int `yaml:"port"`
}

type WorkersConfig struct {
	Count int `yaml:"count"`
}

type OpencodeConfig struct {
	URL string `yaml:"url"`
}

type ToolsConfig struct {
	LintCmd string `yaml:"lint_cmd"`
	TestCmd string `yaml:"test_cmd"`
	E2ECmd  string `yaml:"e2e_cmd"`
}

type PipelineConfig struct {
	Stages     []StageConfig `yaml:"stages"`
	MaxRetries int           `yaml:"max_retries"`
}

type StageConfig struct {
	Name           string `yaml:"name"`
	LLM            string `yaml:"llm"`
	Lint           bool   `yaml:"lint,omitempty"`
	ManualApproval bool   `yaml:"manual_approval,omitempty"`
}

type LLMConfig struct {
	LLM string `yaml:"llm"`
}

type SprintConfig struct {
	TasksPerSprint int `yaml:"tasks_per_sprint"`
}

const configDir = ".oda"
const configFile = "config.yaml"

func Load(projectDir string) (*Config, error) {
	path := filepath.Join(projectDir, configDir, configFile)
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, fmt.Errorf("config not found at %s: run 'oda init' first: %w", path, err)
	}

	var cfg Config
	if err := yaml.Unmarshal(data, &cfg); err != nil {
		return nil, fmt.Errorf("invalid config: %w", err)
	}

	return &cfg, nil
}
```

**Step 6: Run test to verify it passes**

Run: `go mod tidy && go test ./internal/config/ -v`
Expected: PASS

**Step 7: Commit**

```bash
git add -A && git commit -m "feat: project skeleton with config loader"
```

---

### Task 2: Preflight Checks

**Files:**
- Create: `internal/preflight/preflight.go`
- Create: `internal/preflight/preflight_test.go`

**Step 1: Write preflight test**

```go
// internal/preflight/preflight_test.go
package preflight

import (
	"testing"
)

func TestDetectGitRepo(t *testing.T) {
	dir := t.TempDir()
	err := CheckGitRepo(dir)
	if err == nil {
		t.Fatal("expected error for non-git directory")
	}
}

func TestDetectPlatform(t *testing.T) {
	p := DetectPlatform()
	if p == "" {
		t.Fatal("expected non-empty platform")
	}
}
```

**Step 2: Run test to verify it fails**

Run: `go test ./internal/preflight/ -v`
Expected: FAIL

**Step 3: Implement preflight checks**

```go
// internal/preflight/preflight.go
package preflight

import (
	"fmt"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
	"time"
)

type CheckResult struct {
	Name    string
	OK      bool
	Message string
}

func RunAll(projectDir string, opencodeURL string) []CheckResult {
	var results []CheckResult

	results = append(results, check("git repo", CheckGitRepo(projectDir)))
	results = append(results, check("gh CLI", CheckGhCLI()))
	results = append(results, check("gh auth", CheckGhAuth()))
	results = append(results, check("opencode serve", CheckOpencode(opencodeURL)))
	results = append(results, check("oda config", CheckConfig(projectDir)))

	return results
}

func check(name string, err error) CheckResult {
	if err != nil {
		return CheckResult{Name: name, OK: false, Message: err.Error()}
	}
	return CheckResult{Name: name, OK: true}
}

func CheckGitRepo(dir string) error {
	gitDir := filepath.Join(dir, ".git")
	if _, err := os.Stat(gitDir); os.IsNotExist(err) {
		return fmt.Errorf("not a git repository. Run:\n  git init && git remote add origin <url>\n  or: git clone <url>")
	}
	return nil
}

func CheckGhCLI() error {
	_, err := exec.LookPath("gh")
	if err != nil {
		platform := DetectPlatform()
		return fmt.Errorf("gh CLI not found. Install it:\n%s", ghInstallInstructions(platform))
	}
	return nil
}

func CheckGhAuth() error {
	cmd := exec.Command("gh", "auth", "status")
	output, err := cmd.CombinedOutput()
	if err != nil {
		return fmt.Errorf("gh not authenticated. Run:\n  gh auth login\n\nOutput: %s", string(output))
	}
	return nil
}

func CheckOpencode(url string) error {
	client := &http.Client{Timeout: 5 * time.Second}
	resp, err := client.Get(url + "/global/health")
	if err != nil {
		platform := DetectPlatform()
		return fmt.Errorf("opencode serve not reachable at %s.\n\nInstall:\n%s\n\nThen run: opencode serve", url, opencodeInstallInstructions(platform))
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return fmt.Errorf("opencode serve unhealthy (status %d)", resp.StatusCode)
	}
	return nil
}

func CheckConfig(dir string) error {
	path := filepath.Join(dir, ".oda", "config.yaml")
	if _, err := os.Stat(path); os.IsNotExist(err) {
		return fmt.Errorf("config not found. Run: oda init")
	}
	return nil
}

func DetectPlatform() string {
	switch runtime.GOOS {
	case "darwin":
		return "macos"
	case "linux":
		return detectLinuxDistro()
	case "windows":
		return "windows"
	default:
		return runtime.GOOS
	}
}

func detectLinuxDistro() string {
	data, err := os.ReadFile("/etc/os-release")
	if err != nil {
		return "linux"
	}
	content := string(data)
	lower := strings.ToLower(content)
	switch {
	case strings.Contains(lower, "ubuntu"), strings.Contains(lower, "debian"):
		return "linux-apt"
	case strings.Contains(lower, "fedora"), strings.Contains(lower, "rhel"), strings.Contains(lower, "centos"):
		return "linux-dnf"
	case strings.Contains(lower, "arch"):
		return "linux-pacman"
	default:
		return "linux"
	}
}

func ghInstallInstructions(platform string) string {
	switch platform {
	case "macos":
		return "  brew install gh"
	case "linux-apt":
		return "  sudo apt install gh"
	case "linux-dnf":
		return "  sudo dnf install gh"
	case "linux-pacman":
		return "  sudo pacman -S github-cli"
	case "windows":
		return "  scoop install gh\n  or: winget install GitHub.cli"
	default:
		return "  See: https://cli.github.com/manual/installation"
	}
}

func opencodeInstallInstructions(platform string) string {
	switch platform {
	case "macos":
		return "  brew install opencode"
	case "windows":
		return "  scoop install opencode"
	default:
		return "  curl -fsSL https://opencode.ai/install | sh"
	}
}
```

**Step 4: Run tests**

Run: `go test ./internal/preflight/ -v`
Expected: PASS

**Step 5: Commit**

```bash
git add -A && git commit -m "feat: preflight checks for git, gh, opencode"
```

---

### Task 3: SQLite Database Layer

**Files:**
- Create: `internal/db/db.go`
- Create: `internal/db/db_test.go`
- Create: `internal/db/migrations.go`

**Step 1: Write DB test**

```go
// internal/db/db_test.go
package db

import (
	"path/filepath"
	"testing"
)

func TestOpenAndMigrate(t *testing.T) {
	dir := t.TempDir()
	dbPath := filepath.Join(dir, "metrics.db")

	store, err := Open(dbPath)
	if err != nil {
		t.Fatalf("failed to open db: %v", err)
	}
	defer store.Close()

	err = store.SaveStageMetric(StageMetric{
		TaskID:    123,
		Stage:     "analysis",
		LLM:       "claude-sonnet-4",
		TokensIn:  1200,
		TokensOut: 800,
		CostUSD:   0.012,
		DurationS: 15,
	})
	if err != nil {
		t.Fatalf("failed to save metric: %v", err)
	}

	metrics, err := store.GetTaskMetrics(123)
	if err != nil {
		t.Fatalf("failed to get metrics: %v", err)
	}
	if len(metrics) != 1 {
		t.Fatalf("expected 1 metric, got %d", len(metrics))
	}
	if metrics[0].TokensIn != 1200 {
		t.Errorf("expected 1200 tokens_in, got %d", metrics[0].TokensIn)
	}
}

func TestGetSprintCosts(t *testing.T) {
	dir := t.TempDir()
	dbPath := filepath.Join(dir, "metrics.db")

	store, err := Open(dbPath)
	if err != nil {
		t.Fatalf("failed to open db: %v", err)
	}
	defer store.Close()

	store.SaveStageMetric(StageMetric{TaskID: 1, Stage: "analysis", CostUSD: 0.01, SprintID: 1})
	store.SaveStageMetric(StageMetric{TaskID: 1, Stage: "coding", CostUSD: 0.05, SprintID: 1})
	store.SaveStageMetric(StageMetric{TaskID: 2, Stage: "analysis", CostUSD: 0.02, SprintID: 1})

	total, err := store.GetSprintCost(1)
	if err != nil {
		t.Fatalf("failed to get sprint cost: %v", err)
	}
	if total != 0.08 {
		t.Errorf("expected 0.08, got %f", total)
	}
}
```

**Step 2: Run test to verify it fails**

Run: `go test ./internal/db/ -v`
Expected: FAIL

**Step 3: Implement DB layer**

```go
// internal/db/db.go
package db

import (
	"database/sql"
	"fmt"

	_ "modernc.org/sqlite"
)

type Store struct {
	db *sql.DB
}

type StageMetric struct {
	ID        int64
	TaskID    int
	SprintID  int
	Stage     string
	LLM       string
	TokensIn  int
	TokensOut int
	CostUSD   float64
	DurationS int
	Retries   int
}

func Open(path string) (*Store, error) {
	db, err := sql.Open("sqlite", path)
	if err != nil {
		return nil, fmt.Errorf("open db: %w", err)
	}

	db.Exec("PRAGMA journal_mode=WAL")
	db.Exec("PRAGMA foreign_keys=ON")

	store := &Store{db: db}
	if err := store.migrate(); err != nil {
		return nil, fmt.Errorf("migrate: %w", err)
	}

	return store, nil
}

func (s *Store) Close() error {
	return s.db.Close()
}

func (s *Store) SaveStageMetric(m StageMetric) error {
	_, err := s.db.Exec(
		`INSERT INTO stage_metrics (task_id, sprint_id, stage, llm, tokens_in, tokens_out, cost_usd, duration_s, retries)
		 VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?)`,
		m.TaskID, m.SprintID, m.Stage, m.LLM, m.TokensIn, m.TokensOut, m.CostUSD, m.DurationS, m.Retries,
	)
	return err
}

func (s *Store) GetTaskMetrics(taskID int) ([]StageMetric, error) {
	rows, err := s.db.Query(
		`SELECT id, task_id, sprint_id, stage, llm, tokens_in, tokens_out, cost_usd, duration_s, retries
		 FROM stage_metrics WHERE task_id = ? ORDER BY id`, taskID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var metrics []StageMetric
	for rows.Next() {
		var m StageMetric
		err := rows.Scan(&m.ID, &m.TaskID, &m.SprintID, &m.Stage, &m.LLM, &m.TokensIn, &m.TokensOut, &m.CostUSD, &m.DurationS, &m.Retries)
		if err != nil {
			return nil, err
		}
		metrics = append(metrics, m)
	}
	return metrics, nil
}

func (s *Store) GetSprintCost(sprintID int) (float64, error) {
	var total float64
	err := s.db.QueryRow(
		`SELECT COALESCE(SUM(cost_usd), 0) FROM stage_metrics WHERE sprint_id = ?`, sprintID,
	).Scan(&total)
	return total, err
}
```

```go
// internal/db/migrations.go
package db

func (s *Store) migrate() error {
	_, err := s.db.Exec(`
		CREATE TABLE IF NOT EXISTS stage_metrics (
			id INTEGER PRIMARY KEY AUTOINCREMENT,
			task_id INTEGER NOT NULL,
			sprint_id INTEGER NOT NULL DEFAULT 0,
			stage TEXT NOT NULL,
			llm TEXT NOT NULL DEFAULT '',
			tokens_in INTEGER NOT NULL DEFAULT 0,
			tokens_out INTEGER NOT NULL DEFAULT 0,
			cost_usd REAL NOT NULL DEFAULT 0,
			duration_s INTEGER NOT NULL DEFAULT 0,
			retries INTEGER NOT NULL DEFAULT 0,
			created_at DATETIME DEFAULT CURRENT_TIMESTAMP
		);

		CREATE INDEX IF NOT EXISTS idx_metrics_task ON stage_metrics(task_id);
		CREATE INDEX IF NOT EXISTS idx_metrics_sprint ON stage_metrics(sprint_id);
	`)
	return err
}
```

**Step 4: Run tests**

Run: `go mod tidy && go test ./internal/db/ -v`
Expected: PASS

**Step 5: Commit**

```bash
git add -A && git commit -m "feat: SQLite metrics storage layer"
```

---

### Task 4: GitHub Client (gh CLI wrapper)

**Files:**
- Create: `internal/github/client.go`
- Create: `internal/github/labels.go`
- Create: `internal/github/project.go`
- Create: `internal/github/issues.go`

**Step 1: Implement gh CLI wrapper**

```go
// internal/github/client.go
package github

import (
	"encoding/json"
	"fmt"
	"os/exec"
	"strings"
)

type Client struct {
	Repo string
}

func NewClient(repo string) *Client {
	return &Client{Repo: repo}
}

func (c *Client) gh(args ...string) ([]byte, error) {
	cmd := exec.Command("gh", args...)
	output, err := cmd.CombinedOutput()
	if err != nil {
		return nil, fmt.Errorf("gh %s: %s: %w", strings.Join(args, " "), string(output), err)
	}
	return output, nil
}

func (c *Client) ghJSON(result interface{}, args ...string) error {
	output, err := c.gh(args...)
	if err != nil {
		return err
	}
	return json.Unmarshal(output, result)
}

func (c *Client) RepoArg() string {
	return c.Repo
}

func DetectRepo() (string, error) {
	cmd := exec.Command("gh", "repo", "view", "--json", "nameWithOwner", "-q", ".nameWithOwner")
	output, err := cmd.CombinedOutput()
	if err != nil {
		return "", fmt.Errorf("failed to detect repo: %s", string(output))
	}
	return strings.TrimSpace(string(output)), nil
}
```

```go
// internal/github/labels.go
package github

var RequiredLabels = []struct {
	Name  string
	Color string
}{
	{"sprint", "0E8A16"},
	{"insight", "FBCA04"},
	{"size:S", "C2E0C6"},
	{"size:M", "BFD4F2"},
	{"size:L", "D4C5F9"},
	{"size:XL", "F9D0C4"},
	{"stage:analysis", "1D76DB"},
	{"stage:planning", "1D76DB"},
	{"stage:plan-review", "5319E7"},
	{"stage:coding", "1D76DB"},
	{"stage:testing", "1D76DB"},
	{"stage:code-review", "5319E7"},
	{"stage:needs-user", "B60205"},
	{"stage:cancelled", "CCCCCC"},
}

func (c *Client) EnsureLabels() error {
	for _, label := range RequiredLabels {
		c.gh("label", "create", label.Name,
			"--color", label.Color,
			"--force",
			"-R", c.Repo,
		)
	}
	return nil
}
```

```go
// internal/github/project.go
package github

var ProjectColumns = []string{
	"Backlog",
	"In Progress",
	"Review",
	"Merging",
	"Done",
	"Blocked",
}

func (c *Client) EnsureProject(name string) (string, error) {
	output, err := c.gh("project", "list", "--owner", repoOwner(c.Repo), "--format", "json", "-q", fmt.Sprintf(".projects[] | select(.title == \"%s\") | .number", name))
	if err == nil && strings.TrimSpace(string(output)) != "" {
		return strings.TrimSpace(string(output)), nil
	}

	output, err = c.gh("project", "create", "--owner", repoOwner(c.Repo), "--title", name, "--format", "json")
	if err != nil {
		return "", fmt.Errorf("create project: %w", err)
	}
	return strings.TrimSpace(string(output)), nil
}

func repoOwner(repo string) string {
	parts := strings.SplitN(repo, "/", 2)
	if len(parts) == 2 {
		return parts[0]
	}
	return repo
}
```

```go
// internal/github/issues.go
package github

import "fmt"

type Issue struct {
	Number int    `json:"number"`
	Title  string `json:"title"`
	Body   string `json:"body"`
	State  string `json:"state"`
	Labels []struct {
		Name string `json:"name"`
	} `json:"labels"`
}

func (c *Client) CreateIssue(title, body string, labels []string) (int, error) {
	args := []string{"issue", "create", "--title", title, "--body", body, "-R", c.Repo}
	for _, l := range labels {
		args = append(args, "--label", l)
	}
	args = append(args, "--json", "number", "-q", ".number")

	output, err := c.gh(args...)
	if err != nil {
		return 0, err
	}

	var num int
	fmt.Sscanf(strings.TrimSpace(string(output)), "%d", &num)
	return num, nil
}

func (c *Client) ListIssues(milestone string) ([]Issue, error) {
	args := []string{"issue", "list", "-R", c.Repo, "--json", "number,title,body,state,labels", "--limit", "200"}
	if milestone != "" {
		args = append(args, "--milestone", milestone)
	}

	var issues []Issue
	err := c.ghJSON(&issues, args...)
	return issues, err
}

func (c *Client) AddComment(issueNum int, body string) error {
	_, err := c.gh("issue", "comment", fmt.Sprintf("%d", issueNum), "--body", body, "-R", c.Repo)
	return err
}

func (c *Client) CreateMilestone(title string) error {
	_, err := c.gh("api", "-X", "POST",
		fmt.Sprintf("/repos/%s/milestones", c.Repo),
		"-f", fmt.Sprintf("title=%s", title),
	)
	return err
}
```

**Step 2: Commit**

```bash
git add -A && git commit -m "feat: GitHub client wrapping gh CLI"
```

---

### Task 5: Opencode Client

**Files:**
- Create: `internal/opencode/client.go`
- Create: `internal/opencode/client_test.go`

**Step 1: Write opencode client test**

```go
// internal/opencode/client_test.go
package opencode

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
)

func TestHealthCheck(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/global/health" {
			json.NewEncoder(w).Encode(map[string]interface{}{"healthy": true, "version": "1.0.0"})
			return
		}
		w.WriteHeader(404)
	}))
	defer srv.Close()

	client := NewClient(srv.URL)
	err := client.HealthCheck()
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
}

func TestCreateSession(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/session" && r.Method == "POST" {
			json.NewEncoder(w).Encode(map[string]interface{}{"id": "sess-123", "title": "test"})
			return
		}
		w.WriteHeader(404)
	}))
	defer srv.Close()

	client := NewClient(srv.URL)
	sess, err := client.CreateSession("test session")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if sess.ID != "sess-123" {
		t.Errorf("expected sess-123, got %s", sess.ID)
	}
}
```

**Step 2: Run test to verify it fails**

Run: `go test ./internal/opencode/ -v`
Expected: FAIL

**Step 3: Implement opencode client**

```go
// internal/opencode/client.go
package opencode

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"time"
)

type Client struct {
	baseURL    string
	httpClient *http.Client
}

type Session struct {
	ID    string `json:"id"`
	Title string `json:"title"`
}

type Message struct {
	Info  MessageInfo `json:"info"`
	Parts []Part      `json:"parts"`
}

type MessageInfo struct {
	ID        string `json:"id"`
	SessionID string `json:"sessionID"`
	Role      string `json:"role"`
}

type Part struct {
	Type    string `json:"type"`
	Content string `json:"content,omitempty"`
}

type SendMessageRequest struct {
	Parts []Part `json:"parts"`
	Model string `json:"model,omitempty"`
}

func NewClient(baseURL string) *Client {
	return &Client{
		baseURL: baseURL,
		httpClient: &http.Client{
			Timeout: 10 * time.Minute,
		},
	}
}

func (c *Client) HealthCheck() error {
	resp, err := c.httpClient.Get(c.baseURL + "/global/health")
	if err != nil {
		return fmt.Errorf("opencode not reachable: %w", err)
	}
	defer resp.Body.Close()

	var result struct {
		Healthy bool   `json:"healthy"`
		Version string `json:"version"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&result); err != nil {
		return fmt.Errorf("invalid health response: %w", err)
	}
	if !result.Healthy {
		return fmt.Errorf("opencode unhealthy")
	}
	return nil
}

func (c *Client) CreateSession(title string) (*Session, error) {
	body, _ := json.Marshal(map[string]string{"title": title})
	resp, err := c.httpClient.Post(c.baseURL+"/session", "application/json", bytes.NewReader(body))
	if err != nil {
		return nil, fmt.Errorf("create session: %w", err)
	}
	defer resp.Body.Close()

	var sess Session
	if err := json.NewDecoder(resp.Body).Decode(&sess); err != nil {
		return nil, fmt.Errorf("decode session: %w", err)
	}
	return &sess, nil
}

func (c *Client) SendMessage(sessionID string, prompt string, model string) (*Message, error) {
	req := SendMessageRequest{
		Parts: []Part{{Type: "text", Content: prompt}},
		Model: model,
	}
	body, _ := json.Marshal(req)

	resp, err := c.httpClient.Post(
		fmt.Sprintf("%s/session/%s/message", c.baseURL, sessionID),
		"application/json",
		bytes.NewReader(body),
	)
	if err != nil {
		return nil, fmt.Errorf("send message: %w", err)
	}
	defer resp.Body.Close()

	data, _ := io.ReadAll(resp.Body)
	var msg Message
	if err := json.Unmarshal(data, &msg); err != nil {
		return nil, fmt.Errorf("decode message: %w", err)
	}
	return &msg, nil
}

func (c *Client) AbortSession(sessionID string) error {
	resp, err := c.httpClient.Post(
		fmt.Sprintf("%s/session/%s/abort", c.baseURL, sessionID),
		"application/json",
		nil,
	)
	if err != nil {
		return err
	}
	defer resp.Body.Close()
	return nil
}

func (c *Client) DeleteSession(sessionID string) error {
	req, _ := http.NewRequest("DELETE", fmt.Sprintf("%s/session/%s", c.baseURL, sessionID), nil)
	resp, err := c.httpClient.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()
	return nil
}
```

**Step 4: Run tests**

Run: `go test ./internal/opencode/ -v`
Expected: PASS

**Step 5: Commit**

```bash
git add -A && git commit -m "feat: opencode serve HTTP client"
```

---

### Task 6: Git Worktree Manager

**Files:**
- Create: `internal/git/worktree.go`
- Create: `internal/git/worktree_test.go`

**Step 1: Write worktree test**

```go
// internal/git/worktree_test.go
package git

import (
	"os"
	"os/exec"
	"path/filepath"
	"testing"
)

func setupGitRepo(t *testing.T) string {
	t.Helper()
	dir := t.TempDir()
	run(t, dir, "git", "init")
	run(t, dir, "git", "commit", "--allow-empty", "-m", "init")
	return dir
}

func run(t *testing.T, dir string, name string, args ...string) {
	t.Helper()
	cmd := exec.Command(name, args...)
	cmd.Dir = dir
	cmd.Env = append(os.Environ(), "GIT_AUTHOR_NAME=test", "GIT_AUTHOR_EMAIL=test@test.com", "GIT_COMMITTER_NAME=test", "GIT_COMMITTER_EMAIL=test@test.com")
	out, err := cmd.CombinedOutput()
	if err != nil {
		t.Fatalf("%s %v failed: %s: %v", name, args, string(out), err)
	}
}

func TestCreateAndRemoveWorktree(t *testing.T) {
	repoDir := setupGitRepo(t)
	mgr := NewWorktreeManager(repoDir, filepath.Join(repoDir, ".oda", "worktrees"))

	wt, err := mgr.Create("worker-1", "task/1-test-feature")
	if err != nil {
		t.Fatalf("create worktree: %v", err)
	}

	if _, err := os.Stat(wt.Path); os.IsNotExist(err) {
		t.Fatal("worktree directory not created")
	}

	err = mgr.Remove("worker-1")
	if err != nil {
		t.Fatalf("remove worktree: %v", err)
	}
}
```

**Step 2: Run test to verify it fails**

Run: `go test ./internal/git/ -v`
Expected: FAIL

**Step 3: Implement worktree manager**

```go
// internal/git/worktree.go
package git

import (
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
)

type WorktreeManager struct {
	repoDir      string
	worktreesDir string
}

type Worktree struct {
	Name   string
	Path   string
	Branch string
}

func NewWorktreeManager(repoDir, worktreesDir string) *WorktreeManager {
	return &WorktreeManager{
		repoDir:      repoDir,
		worktreesDir: worktreesDir,
	}
}

func (m *WorktreeManager) Create(workerName, branch string) (*Worktree, error) {
	wtPath := filepath.Join(m.worktreesDir, workerName)

	os.MkdirAll(m.worktreesDir, 0755)

	if _, err := os.Stat(wtPath); err == nil {
		m.Remove(workerName)
	}

	cmd := exec.Command("git", "worktree", "add", "-b", branch, wtPath)
	cmd.Dir = m.repoDir
	if out, err := cmd.CombinedOutput(); err != nil {
		cmd2 := exec.Command("git", "worktree", "add", wtPath, branch)
		cmd2.Dir = m.repoDir
		if out2, err2 := cmd2.CombinedOutput(); err2 != nil {
			return nil, fmt.Errorf("create worktree: %s / %s: %w", string(out), string(out2), err)
		}
	}

	return &Worktree{
		Name:   workerName,
		Path:   wtPath,
		Branch: branch,
	}, nil
}

func (m *WorktreeManager) Remove(workerName string) error {
	wtPath := filepath.Join(m.worktreesDir, workerName)

	cmd := exec.Command("git", "worktree", "remove", wtPath, "--force")
	cmd.Dir = m.repoDir
	cmd.CombinedOutput()

	branchCmd := exec.Command("git", "branch", "-D", workerName)
	branchCmd.Dir = m.repoDir
	branchCmd.CombinedOutput()

	return nil
}

func (m *WorktreeManager) List() ([]Worktree, error) {
	cmd := exec.Command("git", "worktree", "list", "--porcelain")
	cmd.Dir = m.repoDir
	output, err := cmd.CombinedOutput()
	if err != nil {
		return nil, fmt.Errorf("list worktrees: %w", err)
	}

	var worktrees []Worktree
	var current Worktree
	for _, line := range strings.Split(string(output), "\n") {
		if strings.HasPrefix(line, "worktree ") {
			if current.Path != "" {
				worktrees = append(worktrees, current)
			}
			current = Worktree{Path: strings.TrimPrefix(line, "worktree ")}
		}
		if strings.HasPrefix(line, "branch ") {
			current.Branch = strings.TrimPrefix(line, "branch refs/heads/")
		}
	}
	if current.Path != "" {
		worktrees = append(worktrees, current)
	}

	return worktrees, nil
}

func RunInWorktree(wtPath string, name string, args ...string) ([]byte, error) {
	cmd := exec.Command(name, args...)
	cmd.Dir = wtPath
	return cmd.CombinedOutput()
}
```

**Step 4: Run tests**

Run: `go test ./internal/git/ -v`
Expected: PASS

**Step 5: Commit**

```bash
git add -A && git commit -m "feat: git worktree manager"
```

---

## Phase 2: Pipeline Engine & Workers

### Task 7: Pipeline State Machine

**Files:**
- Create: `internal/pipeline/stage.go`
- Create: `internal/pipeline/pipeline.go`
- Create: `internal/pipeline/pipeline_test.go`

**Step 1: Define pipeline stages and state machine**

```go
// internal/pipeline/stage.go
package pipeline

type Stage string

const (
	StageQueued     Stage = "queued"
	StageAnalysis   Stage = "analysis"
	StagePlanning   Stage = "planning"
	StagePlanReview Stage = "plan-review"
	StageCoding     Stage = "coding"
	StageTesting    Stage = "testing"
	StageCodeReview Stage = "code-review"
	StageMerging    Stage = "merging"
	StageDone       Stage = "done"
	StageBlocked    Stage = "blocked"
)

type Column string

const (
	ColumnBacklog    Column = "Backlog"
	ColumnInProgress Column = "In Progress"
	ColumnReview     Column = "Review"
	ColumnMerging    Column = "Merging"
	ColumnDone       Column = "Done"
	ColumnBlocked    Column = "Blocked"
)

func (s Stage) Column() Column {
	switch s {
	case StageQueued:
		return ColumnBacklog
	case StageAnalysis, StagePlanning, StageCoding, StageTesting:
		return ColumnInProgress
	case StagePlanReview, StageCodeReview:
		return ColumnReview
	case StageMerging:
		return ColumnMerging
	case StageDone:
		return ColumnDone
	case StageBlocked:
		return ColumnBlocked
	default:
		return ColumnBacklog
	}
}

func (s Stage) Label() string {
	switch s {
	case StageAnalysis, StagePlanning, StagePlanReview, StageCoding, StageTesting, StageCodeReview:
		return "stage:" + string(s)
	default:
		return ""
	}
}

func (s Stage) Next() Stage {
	switch s {
	case StageQueued:
		return StageAnalysis
	case StageAnalysis:
		return StagePlanning
	case StagePlanning:
		return StagePlanReview
	case StagePlanReview:
		return StageCoding
	case StageCoding:
		return StageTesting
	case StageTesting:
		return StageCodeReview
	case StageCodeReview:
		return StageMerging
	case StageMerging:
		return StageDone
	default:
		return StageDone
	}
}

func (s Stage) RetryTarget() Stage {
	switch s {
	case StagePlanReview:
		return StagePlanning
	case StageTesting, StageCodeReview:
		return StageCoding
	default:
		return s
	}
}
```

**Step 2: Write pipeline test**

```go
// internal/pipeline/pipeline_test.go
package pipeline

import "testing"

func TestStageProgression(t *testing.T) {
	stages := []Stage{
		StageQueued, StageAnalysis, StagePlanning, StagePlanReview,
		StageCoding, StageTesting, StageCodeReview, StageMerging, StageDone,
	}

	for i := 0; i < len(stages)-1; i++ {
		next := stages[i].Next()
		if next != stages[i+1] {
			t.Errorf("stage %s: expected next %s, got %s", stages[i], stages[i+1], next)
		}
	}
}

func TestStageColumns(t *testing.T) {
	tests := []struct {
		stage  Stage
		column Column
	}{
		{StageQueued, ColumnBacklog},
		{StageAnalysis, ColumnInProgress},
		{StageCoding, ColumnInProgress},
		{StagePlanReview, ColumnReview},
		{StageCodeReview, ColumnReview},
		{StageMerging, ColumnMerging},
		{StageDone, ColumnDone},
		{StageBlocked, ColumnBlocked},
	}

	for _, tt := range tests {
		col := tt.stage.Column()
		if col != tt.column {
			t.Errorf("stage %s: expected column %s, got %s", tt.stage, tt.column, col)
		}
	}
}

func TestRetryTarget(t *testing.T) {
	if StagePlanReview.RetryTarget() != StagePlanning {
		t.Error("plan-review should retry to planning")
	}
	if StageTesting.RetryTarget() != StageCoding {
		t.Error("testing should retry to coding")
	}
	if StageCodeReview.RetryTarget() != StageCoding {
		t.Error("code-review should retry to coding")
	}
}
```

**Step 3: Run tests**

Run: `go test ./internal/pipeline/ -v`
Expected: PASS

**Step 4: Implement pipeline executor**

```go
// internal/pipeline/pipeline.go
package pipeline

import (
	"fmt"
)

type StageResult struct {
	Stage   Stage
	Success bool
	Output  string
	Error   error
}

type StageExecutor interface {
	Execute(taskID int, stage Stage, context string) (*StageResult, error)
}

type Pipeline struct {
	maxRetries int
	executor   StageExecutor
	onStageChange func(taskID int, stage Stage)
}

func New(maxRetries int, executor StageExecutor, onStageChange func(int, Stage)) *Pipeline {
	return &Pipeline{
		maxRetries:    maxRetries,
		executor:      executor,
		onStageChange: onStageChange,
	}
}

func (p *Pipeline) Run(taskID int, startStage Stage, context string) (*StageResult, error) {
	stage := startStage
	retries := 0

	for stage != StageDone && stage != StageBlocked {
		if p.onStageChange != nil {
			p.onStageChange(taskID, stage)
		}

		result, err := p.executor.Execute(taskID, stage, context)
		if err != nil {
			return nil, fmt.Errorf("stage %s: %w", stage, err)
		}

		if result.Success {
			context = result.Output
			stage = stage.Next()
			retries = 0
		} else {
			retries++
			if retries >= p.maxRetries {
				stage = StageBlocked
			} else {
				stage = stage.RetryTarget()
			}
		}
	}

	return &StageResult{Stage: stage, Success: stage == StageDone}, nil
}
```

**Step 5: Commit**

```bash
git add -A && git commit -m "feat: pipeline state machine and executor"
```

---

### Task 8: Worker Pool

**Files:**
- Create: `internal/worker/pool.go`
- Create: `internal/worker/worker.go`
- Create: `internal/worker/pool_test.go`

**Step 1: Implement worker and pool**

```go
// internal/worker/worker.go
package worker

import (
	"fmt"
	"sync"
	"time"
)

type Status string

const (
	StatusIdle    Status = "idle"
	StatusWorking Status = "working"
	StatusDone    Status = "done"
)

type Task struct {
	ID           int
	IssueNumber  int
	Title        string
	Body         string
	Stage        string
	Dependencies []int
}

type Worker struct {
	ID          string
	Status      Status
	CurrentTask *Task
	Stage       string
	StartedAt   time.Time
	mu          sync.RWMutex
}

func NewWorker(id string) *Worker {
	return &Worker{
		ID:     id,
		Status: StatusIdle,
	}
}

func (w *Worker) SetTask(task *Task) {
	w.mu.Lock()
	defer w.mu.Unlock()
	w.CurrentTask = task
	w.Status = StatusWorking
	w.StartedAt = time.Now()
}

func (w *Worker) SetStage(stage string) {
	w.mu.Lock()
	defer w.mu.Unlock()
	w.Stage = stage
}

func (w *Worker) SetIdle() {
	w.mu.Lock()
	defer w.mu.Unlock()
	w.CurrentTask = nil
	w.Status = StatusIdle
	w.Stage = ""
}

func (w *Worker) GetInfo() WorkerInfo {
	w.mu.RLock()
	defer w.mu.RUnlock()
	info := WorkerInfo{
		ID:     w.ID,
		Status: w.Status,
		Stage:  w.Stage,
	}
	if w.CurrentTask != nil {
		info.TaskTitle = w.CurrentTask.Title
		info.TaskID = w.CurrentTask.IssueNumber
		info.Elapsed = time.Since(w.StartedAt)
	}
	return info
}

type WorkerInfo struct {
	ID        string
	Status    Status
	TaskID    int
	TaskTitle string
	Stage     string
	Elapsed   time.Duration
}

func (w WorkerInfo) String() string {
	if w.Status == StatusIdle {
		return fmt.Sprintf("[%s] idle", w.ID)
	}
	return fmt.Sprintf("[%s] %s #%d: %s (%s)", w.ID, w.Stage, w.TaskID, w.TaskTitle, w.Elapsed.Round(time.Second))
}
```

```go
// internal/worker/pool.go
package worker

import (
	"context"
	"fmt"
	"sync"
)

type TaskQueue interface {
	Next() (*Task, error)
	MarkDone(taskID int) error
	MarkBlocked(taskID int, reason string) error
}

type TaskProcessor interface {
	Process(ctx context.Context, worker *Worker, task *Task) error
}

type Pool struct {
	workers   []*Worker
	queue     TaskQueue
	processor TaskProcessor
	wg        sync.WaitGroup
}

func NewPool(count int, queue TaskQueue, processor TaskProcessor) *Pool {
	workers := make([]*Worker, count)
	for i := 0; i < count; i++ {
		workers[i] = NewWorker(fmt.Sprintf("worker-%d", i+1))
	}
	return &Pool{
		workers:   workers,
		queue:     queue,
		processor: processor,
	}
}

func (p *Pool) Start(ctx context.Context) {
	for _, w := range p.workers {
		p.wg.Add(1)
		go p.runWorker(ctx, w)
	}
}

func (p *Pool) Wait() {
	p.wg.Wait()
}

func (p *Pool) Workers() []WorkerInfo {
	infos := make([]WorkerInfo, len(p.workers))
	for i, w := range p.workers {
		infos[i] = w.GetInfo()
	}
	return infos
}

func (p *Pool) runWorker(ctx context.Context, w *Worker) {
	defer p.wg.Done()

	for {
		select {
		case <-ctx.Done():
			return
		default:
		}

		task, err := p.queue.Next()
		if err != nil || task == nil {
			return
		}

		w.SetTask(task)
		err = p.processor.Process(ctx, w, task)
		if err != nil {
			p.queue.MarkBlocked(task.ID, err.Error())
		} else {
			p.queue.MarkDone(task.ID)
		}
		w.SetIdle()
	}
}
```

**Step 2: Write pool test**

```go
// internal/worker/pool_test.go
package worker

import (
	"context"
	"sync"
	"sync/atomic"
	"testing"
	"time"
)

type mockQueue struct {
	tasks []*Task
	mu    sync.Mutex
	idx   int
	done  []int
}

func (q *mockQueue) Next() (*Task, error) {
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
	return nil
}

type mockProcessor struct {
	processed atomic.Int32
}

func (p *mockProcessor) Process(ctx context.Context, w *Worker, task *Task) error {
	p.processed.Add(1)
	time.Sleep(10 * time.Millisecond)
	return nil
}

func TestPoolProcessesTasks(t *testing.T) {
	queue := &mockQueue{
		tasks: []*Task{
			{ID: 1, IssueNumber: 1, Title: "Task 1"},
			{ID: 2, IssueNumber: 2, Title: "Task 2"},
			{ID: 3, IssueNumber: 3, Title: "Task 3"},
		},
	}
	proc := &mockProcessor{}

	pool := NewPool(2, queue, proc)
	ctx := context.Background()
	pool.Start(ctx)
	pool.Wait()

	if proc.processed.Load() != 3 {
		t.Errorf("expected 3 processed, got %d", proc.processed.Load())
	}
	if len(queue.done) != 3 {
		t.Errorf("expected 3 done, got %d", len(queue.done))
	}
}

func TestPoolWorkerInfo(t *testing.T) {
	queue := &mockQueue{}
	proc := &mockProcessor{}
	pool := NewPool(3, queue, proc)

	infos := pool.Workers()
	if len(infos) != 3 {
		t.Fatalf("expected 3 workers, got %d", len(infos))
	}
	for _, info := range infos {
		if info.Status != StatusIdle {
			t.Errorf("expected idle, got %s", info.Status)
		}
	}
}
```

**Step 3: Run tests**

Run: `go test ./internal/worker/ -v`
Expected: PASS

**Step 4: Commit**

```bash
git add -A && git commit -m "feat: worker pool with task queue"
```

---

## Phase 3: Dashboard

### Task 9: Dashboard HTTP Server & Sprint Board

**Files:**
- Create: `internal/dashboard/server.go`
- Create: `internal/dashboard/handlers.go`
- Create: `internal/dashboard/templates/layout.html`
- Create: `internal/dashboard/templates/board.html`
- Create: `internal/dashboard/templates/backlog.html`
- Create: `internal/dashboard/templates/costs.html`
- Create: `internal/dashboard/static/htmx.min.js`

**Step 1: Download HTMX**

Run: `curl -o internal/dashboard/static/htmx.min.js https://unpkg.com/htmx.org@2.0.4/dist/htmx.min.js`

**Step 2: Implement dashboard server**

```go
// internal/dashboard/server.go
package dashboard

import (
	"embed"
	"fmt"
	"html/template"
	"net/http"

	"github.com/one-dev-army/oda/internal/db"
	"github.com/one-dev-army/oda/internal/worker"
)

//go:embed templates/*.html
var templateFS embed.FS

//go:embed static/*
var staticFS embed.FS

type Server struct {
	port    int
	tmpl    *template.Template
	store   *db.Store
	pool    func() []worker.WorkerInfo
	mux     *http.ServeMux
}

func NewServer(port int, store *db.Store, pool func() []worker.WorkerInfo) (*Server, error) {
	tmpl, err := template.ParseFS(templateFS, "templates/*.html")
	if err != nil {
		return nil, fmt.Errorf("parse templates: %w", err)
	}

	s := &Server{
		port:  port,
		tmpl:  tmpl,
		store: store,
		pool:  pool,
		mux:   http.NewServeMux(),
	}

	s.routes()
	return s, nil
}

func (s *Server) routes() {
	s.mux.Handle("/static/", http.FileServerFS(staticFS))
	s.mux.HandleFunc("GET /", s.handleBoard)
	s.mux.HandleFunc("GET /backlog", s.handleBacklog)
	s.mux.HandleFunc("GET /costs", s.handleCosts)
	s.mux.HandleFunc("GET /workers", s.handleWorkers)
	s.mux.HandleFunc("POST /epic", s.handleAddEpic)
	s.mux.HandleFunc("POST /sync", s.handleSync)
	s.mux.HandleFunc("POST /plan-sprint", s.handlePlanSprint)
	s.mux.HandleFunc("POST /approve/{id}", s.handleApprove)
	s.mux.HandleFunc("POST /reject/{id}", s.handleReject)
}

func (s *Server) Start() error {
	addr := fmt.Sprintf(":%d", s.port)
	fmt.Printf("Dashboard: http://localhost%s\n", addr)
	return http.ListenAndServe(addr, s.mux)
}
```

```go
// internal/dashboard/handlers.go
package dashboard

import (
	"net/http"
)

type BoardData struct {
	Workers []worker.WorkerInfo
	// Tasks grouped by column will be added when GitHub integration is wired
}

func (s *Server) handleBoard(w http.ResponseWriter, r *http.Request) {
	data := BoardData{
		Workers: s.pool(),
	}
	s.tmpl.ExecuteTemplate(w, "layout.html", map[string]interface{}{
		"Page":    "board",
		"Content": data,
	})
}

func (s *Server) handleBacklog(w http.ResponseWriter, r *http.Request) {
	s.tmpl.ExecuteTemplate(w, "layout.html", map[string]interface{}{
		"Page": "backlog",
	})
}

func (s *Server) handleCosts(w http.ResponseWriter, r *http.Request) {
	s.tmpl.ExecuteTemplate(w, "layout.html", map[string]interface{}{
		"Page": "costs",
	})
}

func (s *Server) handleWorkers(w http.ResponseWriter, r *http.Request) {
	workers := s.pool()
	s.tmpl.ExecuteTemplate(w, "workers-partial", workers)
}

func (s *Server) handleAddEpic(w http.ResponseWriter, r *http.Request) {
	r.ParseForm()
	// TODO: wire to epic analysis LLM
	http.Redirect(w, r, "/backlog", http.StatusSeeOther)
}

func (s *Server) handleSync(w http.ResponseWriter, r *http.Request) {
	// TODO: wire to GitHub sync
	http.Redirect(w, r, "/", http.StatusSeeOther)
}

func (s *Server) handlePlanSprint(w http.ResponseWriter, r *http.Request) {
	// TODO: wire to sprint planner
	http.Redirect(w, r, "/", http.StatusSeeOther)
}

func (s *Server) handleApprove(w http.ResponseWriter, r *http.Request) {
	// TODO: wire to approval flow
	http.Redirect(w, r, "/", http.StatusSeeOther)
}

func (s *Server) handleReject(w http.ResponseWriter, r *http.Request) {
	// TODO: wire to rejection flow
	http.Redirect(w, r, "/", http.StatusSeeOther)
}
```

**Step 3: Create HTML templates**

Create `internal/dashboard/templates/layout.html`, `board.html`, `backlog.html`, `costs.html` with HTMX-powered sprint board, worker status panel, and navigation. Templates use `hx-get="/workers" hx-trigger="every 2s"` for live worker updates.

**Step 4: Commit**

```bash
git add -A && git commit -m "feat: HTMX dashboard with sprint board"
```

---

## Phase 4: Main Entry Point & Init

### Task 10: CLI Entry Point (main.go)

**Files:**
- Create: `main.go`

**Step 1: Implement main with subcommands**

Wire together: preflight -> config -> GitHub verification -> sync -> worker pool -> dashboard.

Two subcommands:
- `oda` (default) - start agent
- `oda init` - initialize project

**Step 2: Commit**

```bash
git add -A && git commit -m "feat: CLI entry point with init and serve"
```

---

### Task 11: Init Command

**Files:**
- Create: `internal/init/init.go`

**Step 1: Implement init flow**

- Detect empty vs existing repo
- Run opencode session to scan codebase
- Generate config.yaml
- Setup GitHub (labels, project, milestone)
- Generate GitHub Actions CI via LLM
- For empty repo: create first sprint from project description

**Step 2: Commit**

```bash
git add -A && git commit -m "feat: oda init command with LLM-driven setup"
```

---

## Phase 5: Integration & End-to-End

### Task 12: Task Processor (wires pipeline + opencode + git)

**Files:**
- Create: `internal/worker/processor.go`

**Step 1: Implement TaskProcessor**

Connects pipeline stages to opencode sessions:
- analysis: create session, send analysis prompt, parse JSON response
- planning: create session, send planning prompt with analysis context
- plan-review: create session, send review prompt
- coding: create session in worktree, send coding prompt with plan + tool commands
- testing: run lint/test/e2e commands in worktree
- code-review: create session, send review prompt with diff
- merge: create PR via gh, handle manual approval

Each stage creates a new opencode session, sends prompt with model from config, parses JSON response.

**Step 2: Commit**

```bash
git add -A && git commit -m "feat: task processor connecting pipeline to opencode"
```

---

### Task 13: Metrics & Insights Writer

**Files:**
- Create: `internal/metrics/writer.go`

**Step 1: Implement dual write**

- After each stage: write to SQLite
- After task completion: write YAML comment to GitHub issue/PR
- After task completion: write insights comment to GitHub issue/PR

**Step 2: Commit**

```bash
git add -A && git commit -m "feat: metrics dual write to SQLite and GitHub"
```

---

### Task 14: Sprint Planner

**Files:**
- Create: `internal/scheduler/planner.go`

**Step 1: Implement sprint planning**

- Sync issues from GitHub
- LLM selects tasks for sprint based on backlog, priorities, sizes
- Create milestone
- Assign issues to milestone
- Move cards on project board
- Post-sprint: analyze insights, create new issues

**Step 2: Commit**

```bash
git add -A && git commit -m "feat: LLM-driven sprint planner"
```

---

### Task 15: Epic Analyzer

**Files:**
- Create: `internal/scheduler/epic.go`

**Step 1: Implement epic breakdown**

- Receive epic description from dashboard
- Create opencode session with epic_analysis LLM
- Parse JSON response into task list
- Create GitHub issues with proper format (title, technical description, acceptance criteria, size, dependencies, labels)

**Step 2: Commit**

```bash
git add -A && git commit -m "feat: epic analyzer breaking ideas into tasks"
```

---

## Phase 6: Polish & Testing

### Task 16: Integration Tests

**Files:**
- Create: `tests/integration_test.go`

**Step 1: Write integration tests**

- Test full pipeline with mock opencode server
- Test worker pool with mock task queue
- Test GitHub client with mock gh CLI
- Test graceful shutdown

**Step 2: Commit**

```bash
git add -A && git commit -m "test: integration tests for pipeline and workers"
```

---

### Task 17: Graceful Shutdown

**Files:**
- Modify: `main.go`

**Step 1: Implement signal handling**

- Catch SIGINT/SIGTERM
- Prompt user: "Workers are running. Force quit or wait?"
- Wait: context cancel with grace period
- Force: abort all opencode sessions, save state

**Step 2: Commit**

```bash
git add -A && git commit -m "feat: graceful shutdown with user prompt"
```

---

## Summary

| Phase | Tasks | Description |
|---|---|---|
| 1 | 1-6 | Project skeleton, config, preflight, SQLite, GitHub client, opencode client, git worktrees |
| 2 | 7-8 | Pipeline state machine, worker pool |
| 3 | 9 | HTMX dashboard |
| 4 | 10-11 | CLI entry point, init command |
| 5 | 12-15 | Task processor, metrics, sprint planner, epic analyzer |
| 6 | 16-17 | Integration tests, graceful shutdown |

**Total: 17 tasks across 6 phases.**

Each phase builds on the previous one. Phase 1 can be fully parallelized (tasks 1-6 are independent). Phases 2-6 are sequential.
