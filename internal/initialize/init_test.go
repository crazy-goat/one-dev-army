package initialize

import (
	"os"
	"path/filepath"
	"testing"
)

func TestRun_AlreadyInitialized(t *testing.T) {
	dir := t.TempDir()
	odaDir := filepath.Join(dir, ".oda")
	if err := os.MkdirAll(odaDir, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(odaDir, "config.yaml"), []byte("github:\n  repo: test\n"), 0o644); err != nil {
		t.Fatal(err)
	}

	i := New(dir, nil)
	err := i.Run()
	if err == nil {
		t.Fatal("expected error for already initialized project")
	}
	if got := err.Error(); got != "already initialized: "+filepath.Join(dir, ".oda", "config.yaml")+" exists" {
		t.Errorf("unexpected error: %s", got)
	}
}

func TestIsRepoEmpty_WithOnlyGit(t *testing.T) {
	dir := t.TempDir()
	if err := os.MkdirAll(filepath.Join(dir, ".git"), 0o755); err != nil {
		t.Fatal(err)
	}

	empty, err := isRepoEmpty(dir)
	if err != nil {
		t.Fatal(err)
	}
	if !empty {
		t.Error("expected repo with only .git to be empty")
	}
}

func TestIsRepoEmpty_WithFiles(t *testing.T) {
	dir := t.TempDir()
	if err := os.MkdirAll(filepath.Join(dir, ".git"), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dir, "README.md"), []byte("hello"), 0o644); err != nil {
		t.Fatal(err)
	}

	empty, err := isRepoEmpty(dir)
	if err != nil {
		t.Fatal(err)
	}
	if empty {
		t.Error("expected repo with files to not be empty")
	}
}

func TestDefaultConfig(t *testing.T) {
	cfg := defaultConfig("owner/repo")

	if cfg.GitHub.Repo != "owner/repo" {
		t.Errorf("repo = %q, want %q", cfg.GitHub.Repo, "owner/repo")
	}
	if cfg.Dashboard.Port != 8080 {
		t.Errorf("port = %d, want 8080", cfg.Dashboard.Port)
	}
	if cfg.Workers.Count != 3 {
		t.Errorf("workers = %d, want 3", cfg.Workers.Count)
	}
	if cfg.OpenCode.URL != "http://localhost:4096" {
		t.Errorf("opencode url = %q, want %q", cfg.OpenCode.URL, "http://localhost:4096")
	}
	if cfg.Pipeline.MaxRetries != 5 {
		t.Errorf("max_retries = %d, want 5", cfg.Pipeline.MaxRetries)
	}
	if cfg.Sprint.TasksPerSprint != 10 {
		t.Errorf("tasks_per_sprint = %d, want 10", cfg.Sprint.TasksPerSprint)
	}
	if len(cfg.Pipeline.Stages) != 7 {
		t.Fatalf("stages count = %d, want 7", len(cfg.Pipeline.Stages))
	}
	if cfg.Pipeline.Stages[0].Name != "analysis" {
		t.Errorf("first stage = %q, want %q", cfg.Pipeline.Stages[0].Name, "analysis")
	}
	if cfg.Pipeline.Stages[6].Name != "merge" {
		t.Errorf("last stage = %q, want %q", cfg.Pipeline.Stages[6].Name, "merge")
	}
	if !cfg.Pipeline.Stages[6].ManualApproval {
		t.Error("merge stage should have manual_approval=true")
	}
}
