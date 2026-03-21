package setup

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/crazy-goat/one-dev-army/internal/config"
	"github.com/crazy-goat/one-dev-army/internal/opencode"
)

func TestCheckAgentsMD_Exists(t *testing.T) {
	dir := t.TempDir()
	if err := os.WriteFile(filepath.Join(dir, "AGENTS.md"), []byte("# Agents"), 0o644); err != nil {
		t.Fatal(err)
	}

	s := &Setup{
		projectDir: dir,
		oc:         opencode.NewClient("http://localhost:0"),
		cfg:        &config.Config{},
	}

	if err := s.checkAgentsMD(); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
}

func TestCheckGitHubActions_Exists(t *testing.T) {
	dir := t.TempDir()
	workflowDir := filepath.Join(dir, ".github", "workflows")
	if err := os.MkdirAll(workflowDir, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(workflowDir, "ci.yml"), []byte("name: CI"), 0o644); err != nil {
		t.Fatal(err)
	}

	s := &Setup{
		projectDir: dir,
		oc:         opencode.NewClient("http://localhost:0"),
		cfg:        &config.Config{},
	}

	if err := s.checkGitHubActions(); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
}

func TestExtractContent_PlainText(t *testing.T) {
	msg := &opencode.Message{
		Parts: []opencode.Part{
			{Type: "text", Text: "Hello world"},
		},
	}

	got := extractContent(msg)
	if got != "Hello world" {
		t.Errorf("extractContent() = %q, want %q", got, "Hello world")
	}
}

func TestExtractContent_CodeBlock(t *testing.T) {
	msg := &opencode.Message{
		Parts: []opencode.Part{
			{Type: "text", Text: "Here is the file:\n```markdown\n# My Project\nSome content\n```\nDone."},
		},
	}

	got := extractContent(msg)
	want := "# My Project\nSome content\n"
	if got != want {
		t.Errorf("extractContent() = %q, want %q", got, want)
	}
}

func TestExtractContent_EmptyParts(t *testing.T) {
	msg := &opencode.Message{
		Parts: []opencode.Part{},
	}

	got := extractContent(msg)
	if got != "" {
		t.Errorf("extractContent() = %q, want empty string", got)
	}
}

func TestExtractContent_NonTextPart(t *testing.T) {
	msg := &opencode.Message{
		Parts: []opencode.Part{
			{Type: "tool_use", Text: "something"},
		},
	}

	got := extractContent(msg)
	if got != "" {
		t.Errorf("extractContent() = %q, want empty string", got)
	}
}

func TestFileExists(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "test.txt")

	if fileExists(path) {
		t.Error("fileExists() = true for non-existent file")
	}

	if err := os.WriteFile(path, []byte("data"), 0o644); err != nil {
		t.Fatal(err)
	}

	if !fileExists(path) {
		t.Error("fileExists() = false for existing file")
	}

	if fileExists(dir) {
		t.Error("fileExists() = true for directory")
	}
}

func TestHasWorkflowFiles(t *testing.T) {
	dir := t.TempDir()

	if hasWorkflowFiles(dir) {
		t.Error("hasWorkflowFiles() = true for empty directory")
	}

	if hasWorkflowFiles(filepath.Join(dir, "nonexistent")) {
		t.Error("hasWorkflowFiles() = true for non-existent directory")
	}

	if err := os.WriteFile(filepath.Join(dir, "readme.txt"), []byte("hi"), 0o644); err != nil {
		t.Fatal(err)
	}
	if hasWorkflowFiles(dir) {
		t.Error("hasWorkflowFiles() = true for directory with only .txt file")
	}

	if err := os.WriteFile(filepath.Join(dir, "ci.yml"), []byte("name: CI"), 0o644); err != nil {
		t.Fatal(err)
	}
	if !hasWorkflowFiles(dir) {
		t.Error("hasWorkflowFiles() = false for directory with .yml file")
	}
}

func TestHasWorkflowFiles_Yaml(t *testing.T) {
	dir := t.TempDir()

	if err := os.WriteFile(filepath.Join(dir, "deploy.yaml"), []byte("name: Deploy"), 0o644); err != nil {
		t.Fatal(err)
	}
	if !hasWorkflowFiles(dir) {
		t.Error("hasWorkflowFiles() = false for directory with .yaml file")
	}
}
