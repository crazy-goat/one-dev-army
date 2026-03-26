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
  port: 5000
workers:
  count: 3
opencode:
  url: "http://localhost:5002"
tools:
  lint_cmd: "make lint"
  test_cmd: "make test"
  e2e_cmd: "make e2e"
pipeline:
  max_retries: 5
sprint:
  tasks_per_sprint: 10
llm:
  setup:
    model: "nexos-ai/Kimi K2.5"
  planning:
    model: "nexos-ai/Kimi K2.5"
  orchestration:
    model: "nexos-ai/Kimi K2.5"
  code:
    model: "nexos-ai/Kimi K2.5"
  code-heavy:
    model: "nexos-ai/Kimi K2.5"
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
	if cfg.Dashboard.Port != 5000 {
		t.Errorf("dashboard.port = %d, want %d", cfg.Dashboard.Port, 5000)
	}
	if cfg.Workers.Count != 3 {
		t.Errorf("workers.count = %d, want %d", cfg.Workers.Count, 3)
	}
	if cfg.OpenCode.URL != "http://localhost:5002" {
		t.Errorf("opencode.url = %q, want %q", cfg.OpenCode.URL, "http://localhost:5002")
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

func TestLoad_Sprint(t *testing.T) {
	dir := setupConfigDir(t, validConfig)

	cfg, err := config.Load(dir)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if cfg.Sprint.TasksPerSprint != 10 {
		t.Errorf("sprint.tasks_per_sprint = %d, want 10", cfg.Sprint.TasksPerSprint)
	}
	if cfg.Sprint.AutoStart {
		t.Errorf("sprint.auto_start = %v, want false (default)", cfg.Sprint.AutoStart)
	}
}

func TestLoad_SprintAutoStart(t *testing.T) {
	configWithAutoStart := `github:
  repo: "owner/repo"
sprint:
  tasks_per_sprint: 5
  auto_start: true
`
	dir := setupConfigDir(t, configWithAutoStart)

	cfg, err := config.Load(dir)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if cfg.Sprint.TasksPerSprint != 5 {
		t.Errorf("sprint.tasks_per_sprint = %d, want 5", cfg.Sprint.TasksPerSprint)
	}
	if !cfg.Sprint.AutoStart {
		t.Errorf("sprint.auto_start = %v, want true", cfg.Sprint.AutoStart)
	}
}

func TestLoad_SprintAutoStartDefault(t *testing.T) {
	configWithoutAutoStart := `github:
  repo: "owner/repo"
sprint:
  tasks_per_sprint: 8
`
	dir := setupConfigDir(t, configWithoutAutoStart)

	cfg, err := config.Load(dir)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if cfg.Sprint.TasksPerSprint != 8 {
		t.Errorf("sprint.tasks_per_sprint = %d, want 8", cfg.Sprint.TasksPerSprint)
	}
	if cfg.Sprint.AutoStart {
		t.Errorf("sprint.auto_start = %v, want false (default)", cfg.Sprint.AutoStart)
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

func TestLoad_DoesNotValidateModels(t *testing.T) {
	configWithModels := `llm:
  setup:
    model: "nexos-ai/Invalid-Model"
  planning:
    model: "nexos-ai/Kimi K2.5"
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

	// Config should NOT be mutated during load - exact values from file should be preserved
	// (except empty models which get defaults via applyLLMDefaults)
	if cfg.LLM.Setup.Model != "nexos-ai/Invalid-Model" {
		t.Errorf("setup model should remain as configured, got %q", cfg.LLM.Setup.Model)
	}

	if cfg.LLM.Planning.Model != "nexos-ai/Kimi K2.5" {
		t.Errorf("planning model should remain unchanged, got %q", cfg.LLM.Planning.Model)
	}

	// Empty orchestration model gets default via applyLLMDefaults
	if cfg.LLM.Orchestration.Model != "nexos-ai/Kimi K2.5" {
		t.Errorf("orchestration model should have default applied, got %q", cfg.LLM.Orchestration.Model)
	}

	if cfg.LLM.Code.Model != "nexos-ai/Another-Invalid" {
		t.Errorf("code model should remain as configured, got %q", cfg.LLM.Code.Model)
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

			// Original config should NOT be mutated
			if cfg.Setup.Model != tt.setupModel {
				t.Errorf("original setup model was mutated = %q, want %q", cfg.Setup.Model, tt.setupModel)
			}
			if cfg.Planning.Model != tt.planningModel {
				t.Errorf("original planning model was mutated = %q, want %q", cfg.Planning.Model, tt.planningModel)
			}

			// Validated config copy should have fallbacks applied
			if result.ValidatedConfig.Setup.Model != tt.wantSetup {
				t.Errorf("validated setup model = %q, want %q", result.ValidatedConfig.Setup.Model, tt.wantSetup)
			}
			if result.ValidatedConfig.Planning.Model != tt.wantPlanning {
				t.Errorf("validated planning model = %q, want %q", result.ValidatedConfig.Planning.Model, tt.wantPlanning)
			}
			if result.HasReplacements != tt.wantReplaced {
				t.Errorf("HasReplacements = %v, want %v", result.HasReplacements, tt.wantReplaced)
			}
		})
	}
}

func TestLoad_UseNewFrontend(t *testing.T) {
	configWithNewFE := `github:
  repo: "owner/repo"
use_new_frontend: true
`
	configWithoutNewFE := `github:
  repo: "owner/repo"
`

	tests := []struct {
		name string
		cfg  string
		want bool
	}{
		{"explicitly true", configWithNewFE, true},
		{"default false", configWithoutNewFE, false},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			dir := setupConfigDir(t, tt.cfg)
			cfg, err := config.Load(dir)
			if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}
			if cfg.UseNewFrontend != tt.want {
				t.Errorf("use_new_frontend = %v, want %v", cfg.UseNewFrontend, tt.want)
			}
		})
	}
}

func TestLoad_YoloMode(t *testing.T) {
	configWithYoloTrue := `github:
  repo: "owner/repo"
yolo_mode: true
`
	configWithYoloFalse := `github:
  repo: "owner/repo"
yolo_mode: false
`
	configWithoutYolo := `github:
  repo: "owner/repo"
`

	tests := []struct {
		name         string
		config       string
		wantYoloMode bool
	}{
		{
			name:         "yolo_mode explicitly true",
			config:       configWithYoloTrue,
			wantYoloMode: true,
		},
		{
			name:         "yolo_mode explicitly false",
			config:       configWithYoloFalse,
			wantYoloMode: false,
		},
		{
			name:         "yolo_mode not specified (default false)",
			config:       configWithoutYolo,
			wantYoloMode: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			dir := setupConfigDir(t, tt.config)
			cfg, err := config.Load(dir)
			if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}
			if cfg.YoloMode != tt.wantYoloMode {
				t.Errorf("yolo_mode = %v, want %v", cfg.YoloMode, tt.wantYoloMode)
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

func TestCategoryModels_NormalizeModel(t *testing.T) {
	tests := []struct {
		name  string
		input string
		want  string
	}{
		{
			name:  "already normalized",
			input: "nexos-ai/Kimi K2.5",
			want:  "nexos-ai/Kimi K2.5",
		},
		{
			name:  "kimi without provider",
			input: "Kimi K2.5",
			want:  "nexos-ai/Kimi K2.5",
		},
		{
			name:  "claude without provider",
			input: "claude-sonnet-4",
			want:  "anthropic/claude-sonnet-4",
		},
		{
			name:  "gpt without provider",
			input: "gpt-4o",
			want:  "openai/gpt-4o",
		},
		{
			name:  "gemini without provider",
			input: "gemini-2.5-pro",
			want:  "google/gemini-2.5-pro",
		},
		{
			name:  "empty model",
			input: "",
			want:  "",
		},
		{
			name:  "unknown model without provider",
			input: "unknown-model",
			want:  "unknown-model",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			cm := config.CategoryModels{Model: tt.input}
			got := cm.NormalizeModel()
			if got != tt.want {
				t.Errorf("NormalizeModel() = %q, want %q", got, tt.want)
			}
		})
	}
}

func TestLLMConfig_NormalizeAllModels(t *testing.T) {
	cfg := config.LLMConfig{
		Setup:         config.CategoryModels{Model: "Kimi K2.5"},
		Planning:      config.CategoryModels{Model: "claude-sonnet-4"},
		Orchestration: config.CategoryModels{Model: "gpt-4o"},
		Code:          config.CategoryModels{Model: "gemini-2.5-pro"},
		CodeHeavy:     config.CategoryModels{Model: "deepseek-r1"},
	}

	cfg.NormalizeAllModels()

	if cfg.Setup.Model != "nexos-ai/Kimi K2.5" {
		t.Errorf("Setup.Model = %q, want %q", cfg.Setup.Model, "nexos-ai/Kimi K2.5")
	}
	if cfg.Planning.Model != "anthropic/claude-sonnet-4" {
		t.Errorf("Planning.Model = %q, want %q", cfg.Planning.Model, "anthropic/claude-sonnet-4")
	}
	if cfg.Orchestration.Model != "openai/gpt-4o" {
		t.Errorf("Orchestration.Model = %q, want %q", cfg.Orchestration.Model, "openai/gpt-4o")
	}
	if cfg.Code.Model != "google/gemini-2.5-pro" {
		t.Errorf("Code.Model = %q, want %q", cfg.Code.Model, "google/gemini-2.5-pro")
	}
	if cfg.CodeHeavy.Model != "deepseek/deepseek-r1" {
		t.Errorf("CodeHeavy.Model = %q, want %q", cfg.CodeHeavy.Model, "deepseek/deepseek-r1")
	}
}
