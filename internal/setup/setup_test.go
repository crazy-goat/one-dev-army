package setup

import (
	"os"
	"path/filepath"
	"strings"
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

	// Verify existing file was not overwritten
	content, err := os.ReadFile(filepath.Join(dir, "AGENTS.md"))
	if err != nil {
		t.Fatal(err)
	}
	if string(content) != "# Agents" {
		t.Errorf("existing AGENTS.md was overwritten: got %q, want %q", string(content), "# Agents")
	}
}

func TestCheckAgentsMD_CreatesTemplate(t *testing.T) {
	dir := t.TempDir()

	s := &Setup{
		projectDir: dir,
		oc:         opencode.NewClient("http://localhost:0"),
		cfg:        &config.Config{},
	}

	if err := s.checkAgentsMD(); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	// Verify file was created
	path := filepath.Join(dir, "AGENTS.md")
	if !fileExists(path) {
		t.Fatal("AGENTS.md was not created")
	}

	// Verify content contains project name
	content, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	contentStr := string(content)
	if !strings.Contains(contentStr, filepath.Base(dir)) {
		t.Errorf("AGENTS.md does not contain project name: %q", contentStr)
	}

	// Verify file permissions
	info, err := os.Stat(path)
	if err != nil {
		t.Fatal(err)
	}
	mode := info.Mode().Perm()
	if mode != 0o644 {
		t.Errorf("AGENTS.md has wrong permissions: got %o, want %o", mode, 0o644)
	}
}

func TestDetectLanguage(t *testing.T) {
	tests := []struct {
		name     string
		files    map[string]string
		expected string
	}{
		{
			name:     "Go project",
			files:    map[string]string{"go.mod": "module test"},
			expected: "Go",
		},
		{
			name:     "Rust project",
			files:    map[string]string{"Cargo.toml": "[package]"},
			expected: "Rust",
		},
		{
			name:     "Node.js project",
			files:    map[string]string{"package.json": "{}"},
			expected: "JavaScript/TypeScript",
		},
		{
			name:     "Python project (requirements.txt)",
			files:    map[string]string{"requirements.txt": "requests"},
			expected: "Python",
		},
		{
			name:     "Python project (pyproject.toml)",
			files:    map[string]string{"pyproject.toml": "[project]"},
			expected: "Python",
		},
		{
			name:     "PHP project",
			files:    map[string]string{"composer.json": "{}"},
			expected: "PHP",
		},
		{
			name:     "Java project (pom.xml)",
			files:    map[string]string{"pom.xml": "<project>"},
			expected: "Java",
		},
		{
			name:     "Java/Kotlin project (build.gradle)",
			files:    map[string]string{"build.gradle": "plugins"},
			expected: "Java/Kotlin",
		},
		{
			name:     "Ruby project",
			files:    map[string]string{"Gemfile": "source"},
			expected: "Ruby",
		},
		{
			name:     "Unknown project",
			files:    map[string]string{},
			expected: "Unknown",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			dir := t.TempDir()
			for filename, content := range tt.files {
				if err := os.WriteFile(filepath.Join(dir, filename), []byte(content), 0o644); err != nil {
					t.Fatal(err)
				}
			}

			got := detectLanguage(dir)
			if got != tt.expected {
				t.Errorf("detectLanguage() = %q, want %q", got, tt.expected)
			}
		})
	}
}

func TestGenerateAgentsTemplate(t *testing.T) {
	projectName := "my-awesome-project"
	language := "Go"

	got := generateAgentsTemplate(projectName, language)

	if !strings.Contains(got, projectName) {
		t.Errorf("template does not contain project name: %q", got)
	}
	if !strings.Contains(got, language) {
		t.Errorf("template does not contain language: %q", got)
	}
	if !strings.Contains(got, "# my-awesome-project") {
		t.Errorf("template does not contain header: %q", got)
	}
	if !strings.Contains(got, "Project Overview") {
		t.Errorf("template does not contain 'Project Overview' section: %q", got)
	}
	if !strings.Contains(got, "Development Guidelines") {
		t.Errorf("template does not contain 'Development Guidelines' section: %q", got)
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
