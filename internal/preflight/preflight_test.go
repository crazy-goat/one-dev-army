package preflight_test

import (
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"testing"

	"github.com/one-dev-army/oda/internal/preflight"
)

func TestCheckGitRepo_NoGitDir(t *testing.T) {
	dir := t.TempDir()
	err := preflight.CheckGitRepo(dir)
	if err == nil {
		t.Fatal("expected error for non-git directory, got nil")
	}
}

func TestCheckGitRepo_WithGitDir(t *testing.T) {
	dir := t.TempDir()
	if err := os.Mkdir(filepath.Join(dir, ".git"), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := preflight.CheckGitRepo(dir); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
}

func TestCheckGitRepo_GitIsFile(t *testing.T) {
	dir := t.TempDir()
	if err := os.WriteFile(filepath.Join(dir, ".git"), []byte("not a dir"), 0o644); err != nil {
		t.Fatal(err)
	}
	err := preflight.CheckGitRepo(dir)
	if err == nil {
		t.Fatal("expected error when .git is a file, got nil")
	}
}

func TestDetectPlatform_NonEmpty(t *testing.T) {
	p := preflight.DetectPlatform()
	if p == "" {
		t.Fatal("DetectPlatform returned empty string")
	}
}

func TestCheckOpencode_Healthy(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/global/health" {
			t.Errorf("unexpected path: %s", r.URL.Path)
			w.WriteHeader(http.StatusNotFound)
			return
		}
		w.WriteHeader(http.StatusOK)
	}))
	defer srv.Close()

	if err := preflight.CheckOpencode(srv.URL); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
}

func TestCheckOpencode_Unhealthy(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusServiceUnavailable)
	}))
	defer srv.Close()

	err := preflight.CheckOpencode(srv.URL)
	if err == nil {
		t.Fatal("expected error for unhealthy server, got nil")
	}
}

func TestCheckOpencode_Unreachable(t *testing.T) {
	err := preflight.CheckOpencode("http://127.0.0.1:1")
	if err == nil {
		t.Fatal("expected error for unreachable server, got nil")
	}
}

func TestCheckConfig_Missing(t *testing.T) {
	dir := t.TempDir()
	err := preflight.CheckConfig(dir)
	if err == nil {
		t.Fatal("expected error for missing config, got nil")
	}
}

func TestCheckConfig_Exists(t *testing.T) {
	dir := t.TempDir()
	odaDir := filepath.Join(dir, ".oda")
	if err := os.MkdirAll(odaDir, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(odaDir, "config.yaml"), []byte("github:\n  repo: test"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := preflight.CheckConfig(dir); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
}

func TestRunAll_CollectsAllResults(t *testing.T) {
	dir := t.TempDir()
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusOK)
	}))
	defer srv.Close()

	results := preflight.RunAll(dir, srv.URL)
	if len(results) != 5 {
		t.Fatalf("expected 5 results, got %d", len(results))
	}

	names := map[string]bool{}
	for _, r := range results {
		names[r.Name] = true
		if r.Name == "" {
			t.Error("result has empty name")
		}
		if r.Message == "" {
			t.Error("result has empty message")
		}
	}

	expected := []string{"git-repo", "gh-cli", "gh-auth", "opencode", "config"}
	for _, name := range expected {
		if !names[name] {
			t.Errorf("missing check result for %q", name)
		}
	}
}
