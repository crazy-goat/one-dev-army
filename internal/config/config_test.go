package config_test

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/crazy-goat/one-dev-army/internal/config"
)

const validConfig = `github:
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
  max_retries: 5
planning:
  llm: claude-opus-4
epic_analysis:
  llm: claude-sonnet-4
sprint:
  tasks_per_sprint: 10
`

func setupConfigDir(t *testing.T, content string) string {
	t.Helper()
	dir := t.TempDir()
	odaDir := filepath.Join(dir, ".oda")
	if err := os.MkdirAll(odaDir, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(odaDir, "config.yaml"), []byte(content), 0o644); err != nil {
		t.Fatal(err)
	}
	return dir
}

func TestLoad_ValidConfig(t *testing.T) {
	dir := setupConfigDir(t, validConfig)

	cfg, err := config.Load(dir)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if cfg.GitHub.Repo != "owner/repo" {
		t.Errorf("github.repo = %q, want %q", cfg.GitHub.Repo, "owner/repo")
	}
	if cfg.Dashboard.Port != 8080 {
		t.Errorf("dashboard.port = %d, want %d", cfg.Dashboard.Port, 8080)
	}
	if cfg.Workers.Count != 3 {
		t.Errorf("workers.count = %d, want %d", cfg.Workers.Count, 3)
	}
	if cfg.OpenCode.URL != "http://localhost:4096" {
		t.Errorf("opencode.url = %q, want %q", cfg.OpenCode.URL, "http://localhost:4096")
	}
}

func TestLoad_ToolsFields(t *testing.T) {
	dir := setupConfigDir(t, validConfig)

	cfg, err := config.Load(dir)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if cfg.Tools.LintCmd != "make lint" {
		t.Errorf("tools.lint_cmd = %q, want %q", cfg.Tools.LintCmd, "make lint")
	}
	if cfg.Tools.TestCmd != "make test" {
		t.Errorf("tools.test_cmd = %q, want %q", cfg.Tools.TestCmd, "make test")
	}
	if cfg.Tools.E2ECmd != "make e2e" {
		t.Errorf("tools.e2e_cmd = %q, want %q", cfg.Tools.E2ECmd, "make e2e")
	}
}

func TestLoad_PlanningAndEpicAnalysis(t *testing.T) {
	dir := setupConfigDir(t, validConfig)

	cfg, err := config.Load(dir)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if cfg.Planning.LLM != "claude-opus-4" {
		t.Errorf("planning.llm = %q, want %q", cfg.Planning.LLM, "claude-opus-4")
	}
	if cfg.EpicAnalysis.LLM != "claude-sonnet-4" {
		t.Errorf("epic_analysis.llm = %q, want %q", cfg.EpicAnalysis.LLM, "claude-sonnet-4")
	}
}

func TestLoad_Sprint(t *testing.T) {
	dir := setupConfigDir(t, validConfig)

	cfg, err := config.Load(dir)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if cfg.Sprint.TasksPerSprint != 10 {
		t.Errorf("sprint.tasks_per_sprint = %d, want 10", cfg.Sprint.TasksPerSprint)
	}
}

func TestLoad_GitHubUseProjects(t *testing.T) {
	configWithProjects := `github:
  repo: "owner/repo"
  use_projects: true
`
	dir := setupConfigDir(t, configWithProjects)

	cfg, err := config.Load(dir)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if !cfg.GitHub.UseProjects {
		t.Errorf("github.use_projects = %v, want true", cfg.GitHub.UseProjects)
	}
}

func TestLoad_GitHubUseProjectsDefault(t *testing.T) {
	configWithoutProjects := `github:
  repo: "owner/repo"
`
	dir := setupConfigDir(t, configWithoutProjects)

	cfg, err := config.Load(dir)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if cfg.GitHub.UseProjects {
		t.Errorf("github.use_projects = %v, want false (default)", cfg.GitHub.UseProjects)
	}
}

func TestLoad_MissingConfig(t *testing.T) {
	dir := t.TempDir()

	_, err := config.Load(dir)
	if err == nil {
		t.Fatal("expected error for missing config, got nil")
	}
}

func TestLoad_InvalidYAML(t *testing.T) {
	dir := setupConfigDir(t, "{{invalid yaml")

	_, err := config.Load(dir)
	if err == nil {
		t.Fatal("expected error for invalid YAML, got nil")
	}
}
