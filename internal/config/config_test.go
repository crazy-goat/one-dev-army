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

func TestLoad_WithModelValidation(t *testing.T) {
	configWithModels := `llm:
  setup:
    model: "nexos-ai/Invalid-Model"
  planning:
    model: "nexos-ai/Kimi K2.5"
  orchestration:
    model: ""
  code:
    model: "nexos-ai/Another-Invalid"
  code-heavy:
    model: "nexos-ai/Kimi K2.5"
`
	dir := setupConfigDir(t, configWithModels)

	availableModels := []string{"nexos-ai/Kimi K2.5", "nexos-ai/Claude-3-Opus"}

	cfg, err := config.Load(dir, availableModels...)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	// Invalid models should fallback to first available
	if cfg.LLM.Setup.Model != "nexos-ai/Kimi K2.5" {
		t.Errorf("setup model should fallback to first available, got %q", cfg.LLM.Setup.Model)
	}

	// Valid model should remain unchanged
	if cfg.LLM.Planning.Model != "nexos-ai/Kimi K2.5" {
		t.Errorf("planning model should remain unchanged, got %q", cfg.LLM.Planning.Model)
	}

	// Empty model should fallback to first available
	if cfg.LLM.Orchestration.Model != "nexos-ai/Kimi K2.5" {
		t.Errorf("orchestration model should fallback to first available, got %q", cfg.LLM.Orchestration.Model)
	}

	// Another invalid model should fallback
	if cfg.LLM.Code.Model != "nexos-ai/Kimi K2.5" {
		t.Errorf("code model should fallback to first available, got %q", cfg.LLM.Code.Model)
	}
}

func TestLoad_WithEmptyAvailableModels(t *testing.T) {
	configWithModels := `llm:
  setup:
    model: "nexos-ai/Some-Model"
  planning:
    model: "nexos-ai/Another-Model"
`
	dir := setupConfigDir(t, configWithModels)

	// Pass empty available models - should skip validation
	cfg, err := config.Load(dir)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	// Models should remain as configured when no validation is performed
	if cfg.LLM.Setup.Model != "nexos-ai/Some-Model" {
		t.Errorf("setup model should remain unchanged, got %q", cfg.LLM.Setup.Model)
	}
}

func TestValidateAndFallbackModels(t *testing.T) {
	tests := []struct {
		name            string
		setupModel      string
		planningModel   string
		availableModels []string
		wantSetup       string
		wantPlanning    string
		wantReplaced    bool
	}{
		{
			name:            "all valid models",
			setupModel:      "provider/model1",
			planningModel:   "provider/model2",
			availableModels: []string{"provider/model1", "provider/model2"},
			wantSetup:       "provider/model1",
			wantPlanning:    "provider/model2",
			wantReplaced:    false,
		},
		{
			name:            "one invalid model",
			setupModel:      "provider/invalid",
			planningModel:   "provider/model2",
			availableModels: []string{"provider/model1", "provider/model2"},
			wantSetup:       "provider/model1",
			wantPlanning:    "provider/model2",
			wantReplaced:    true,
		},
		{
			name:            "empty available models",
			setupModel:      "provider/model1",
			planningModel:   "provider/model2",
			availableModels: []string{},
			wantSetup:       "provider/model1",
			wantPlanning:    "provider/model2",
			wantReplaced:    false,
		},
		{
			name:            "empty model falls back",
			setupModel:      "",
			planningModel:   "provider/model2",
			availableModels: []string{"provider/model1", "provider/model2"},
			wantSetup:       "provider/model1",
			wantPlanning:    "provider/model2",
			wantReplaced:    true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			cfg := config.LLMConfig{
				Setup:         config.CategoryModels{Model: tt.setupModel},
				Planning:      config.CategoryModels{Model: tt.planningModel},
				Orchestration: config.CategoryModels{Model: tt.setupModel},
				Code:          config.CategoryModels{Model: tt.setupModel},
				CodeHeavy:     config.CategoryModels{Model: tt.setupModel},
			}

			result := cfg.ValidateAndFallbackModels(tt.availableModels)

			if cfg.Setup.Model != tt.wantSetup {
				t.Errorf("setup model = %q, want %q", cfg.Setup.Model, tt.wantSetup)
			}
			if cfg.Planning.Model != tt.wantPlanning {
				t.Errorf("planning model = %q, want %q", cfg.Planning.Model, tt.wantPlanning)
			}
			if result.HasReplacements != tt.wantReplaced {
				t.Errorf("HasReplacements = %v, want %v", result.HasReplacements, tt.wantReplaced)
			}
		})
	}
}

func TestGetFirstAvailableModel(t *testing.T) {
	tests := []struct {
		name            string
		availableModels []string
		want            string
	}{
		{
			name:            "has models",
			availableModels: []string{"model1", "model2"},
			want:            "model1",
		},
		{
			name:            "empty list",
			availableModels: []string{},
			want:            "",
		},
		{
			name:            "nil list",
			availableModels: nil,
			want:            "",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := config.GetFirstAvailableModel(tt.availableModels)
			if got != tt.want {
				t.Errorf("GetFirstAvailableModel() = %q, want %q", got, tt.want)
			}
		})
	}
}
