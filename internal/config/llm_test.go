package config_test

import (
	"testing"

	"github.com/crazy-goat/one-dev-army/internal/config"
)

const llmConfig = `github:
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
pipeline:
  stages:
    - name: analysis
      llm: claude-sonnet-4
    - name: coding
      llm: claude-sonnet-4
  max_retries: 5
llm:
  default:
    provider_id: anthropic
    model_id: claude-sonnet-4
  development:
    strong:
      provider_id: anthropic
      model_id: claude-opus-4
    weak:
      provider_id: anthropic
      model_id: claude-haiku-4
  planning:
    strong:
      provider_id: openai
      model_id: gpt-4
    weak:
      provider_id: openai
      model_id: gpt-3.5-turbo
  orchestration:
    strong:
      provider_id: anthropic
      model_id: claude-sonnet-4
    weak:
      provider_id: anthropic
      model_id: claude-haiku-4
  setup:
    strong:
      provider_id: openai
      model_id: gpt-4
    weak:
      provider_id: openai
      model_id: gpt-3.5-turbo
  routing:
    thresholds:
      development: medium
      planning: low
      orchestration: low
      setup: medium
`

func TestLoad_LLMConfig(t *testing.T) {
	dir := setupConfigDir(t, llmConfig)

	cfg, err := config.Load(dir)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	// Test default model
	if cfg.LLM.Default.ProviderID != "anthropic" {
		t.Errorf("llm.default.provider_id = %q, want %q", cfg.LLM.Default.ProviderID, "anthropic")
	}
	if cfg.LLM.Default.ModelID != "claude-sonnet-4" {
		t.Errorf("llm.default.model_id = %q, want %q", cfg.LLM.Default.ModelID, "claude-sonnet-4")
	}

	// Test development category
	if cfg.LLM.Development.Strong.ProviderID != "anthropic" {
		t.Errorf("llm.development.strong.provider_id = %q, want %q", cfg.LLM.Development.Strong.ProviderID, "anthropic")
	}
	if cfg.LLM.Development.Strong.ModelID != "claude-opus-4" {
		t.Errorf("llm.development.strong.model_id = %q, want %q", cfg.LLM.Development.Strong.ModelID, "claude-opus-4")
	}
	if cfg.LLM.Development.Weak.ProviderID != "anthropic" {
		t.Errorf("llm.development.weak.provider_id = %q, want %q", cfg.LLM.Development.Weak.ProviderID, "anthropic")
	}
	if cfg.LLM.Development.Weak.ModelID != "claude-haiku-4" {
		t.Errorf("llm.development.weak.model_id = %q, want %q", cfg.LLM.Development.Weak.ModelID, "claude-haiku-4")
	}

	// Test planning category
	if cfg.LLM.Planning.Strong.ProviderID != "openai" {
		t.Errorf("llm.planning.strong.provider_id = %q, want %q", cfg.LLM.Planning.Strong.ProviderID, "openai")
	}
	if cfg.LLM.Planning.Weak.ProviderID != "openai" {
		t.Errorf("llm.planning.weak.provider_id = %q, want %q", cfg.LLM.Planning.Weak.ProviderID, "openai")
	}

	// Test routing thresholds
	if cfg.LLM.Routing.Thresholds[config.CategoryDevelopment] != config.ComplexityMedium {
		t.Errorf("routing.thresholds.development = %q, want %q", cfg.LLM.Routing.Thresholds[config.CategoryDevelopment], config.ComplexityMedium)
	}
	if cfg.LLM.Routing.Thresholds[config.CategoryPlanning] != config.ComplexityLow {
		t.Errorf("routing.thresholds.planning = %q, want %q", cfg.LLM.Routing.Thresholds[config.CategoryPlanning], config.ComplexityLow)
	}
}

func TestLLMConfig_GetModelForCategory(t *testing.T) {
	cfg := &config.LLMConfig{
		Default: config.ModelConfig{
			ProviderID: "anthropic",
			ModelID:    "claude-sonnet-4",
		},
		Development: config.CategoryModels{
			Strong: config.ModelConfig{
				ProviderID: "anthropic",
				ModelID:    "claude-opus-4",
			},
			Weak: config.ModelConfig{
				ProviderID: "anthropic",
				ModelID:    "claude-haiku-4",
			},
		},
		Planning: config.CategoryModels{
			Strong: config.ModelConfig{
				ProviderID: "openai",
				ModelID:    "gpt-4",
			},
			Weak: config.ModelConfig{
				ProviderID: "openai",
				ModelID:    "gpt-3.5-turbo",
			},
		},
		Routing: config.RoutingConfig{
			Thresholds: map[config.TaskCategory]config.ComplexityLevel{
				config.CategoryDevelopment: config.ComplexityMedium,
				config.CategoryPlanning:    config.ComplexityLow,
			},
		},
	}

	// Test development with low complexity (should use weak)
	model := cfg.GetModelForCategory(config.CategoryDevelopment, config.ComplexityLow)
	if model.ModelID != "claude-haiku-4" {
		t.Errorf("development + low complexity = %q, want %q", model.ModelID, "claude-haiku-4")
	}

	// Test development with high complexity (should use strong)
	model = cfg.GetModelForCategory(config.CategoryDevelopment, config.ComplexityHigh)
	if model.ModelID != "claude-opus-4" {
		t.Errorf("development + high complexity = %q, want %q", model.ModelID, "claude-opus-4")
	}

	// Test planning with low complexity (should use strong due to threshold)
	model = cfg.GetModelForCategory(config.CategoryPlanning, config.ComplexityLow)
	if model.ModelID != "gpt-4" {
		t.Errorf("planning + low complexity = %q, want %q", model.ModelID, "gpt-4")
	}
}

func TestLLMConfig_SetDefaults(t *testing.T) {
	cfg := &config.LLMConfig{
		Default: config.ModelConfig{
			ProviderID: "anthropic",
			ModelID:    "claude-sonnet-4",
		},
	}

	cfg.SetDefaults()

	// Check that thresholds are set
	if cfg.Routing.Thresholds[config.CategoryDevelopment] == "" {
		t.Error("routing.thresholds.development not set")
	}
	if cfg.Routing.Thresholds[config.CategoryPlanning] == "" {
		t.Error("routing.thresholds.planning not set")
	}

	// Check that categories inherit defaults
	if cfg.Development.Strong.ModelID == "" {
		t.Error("development.strong not populated from default")
	}
	if cfg.Development.Weak.ModelID == "" {
		t.Error("development.weak not populated from strong")
	}
}

func TestLLMConfig_Validate(t *testing.T) {
	// Valid config
	validCfg := &config.LLMConfig{
		Default: config.ModelConfig{
			ProviderID: "anthropic",
			ModelID:    "claude-sonnet-4",
		},
	}
	if err := validCfg.Validate(); err != nil {
		t.Errorf("valid config should not error: %v", err)
	}

	// Invalid config - missing default
	invalidCfg := &config.LLMConfig{}
	if err := invalidCfg.Validate(); err == nil {
		t.Error("config without default should error")
	}

	// Invalid config - missing provider_id
	invalidCfg2 := &config.LLMConfig{
		Default: config.ModelConfig{
			ModelID: "claude-sonnet-4",
		},
		Development: config.CategoryModels{
			Strong: config.ModelConfig{
				ModelID: "claude-opus-4", // Missing provider_id
			},
		},
	}
	if err := invalidCfg2.Validate(); err == nil {
		t.Error("config with missing provider_id should error")
	}
}

func TestModelConfig_String(t *testing.T) {
	// Test with provider
	m1 := config.ModelConfig{
		ProviderID: "anthropic",
		ModelID:    "claude-sonnet-4",
	}
	if s := m1.String(); s != "anthropic/claude-sonnet-4" {
		t.Errorf("model string = %q, want %q", s, "anthropic/claude-sonnet-4")
	}

	// Test without provider
	m2 := config.ModelConfig{
		ModelID: "claude-sonnet-4",
	}
	if s := m2.String(); s != "claude-sonnet-4" {
		t.Errorf("model string without provider = %q, want %q", s, "claude-sonnet-4")
	}
}

func TestLoad_LLMConfigDefaults(t *testing.T) {
	// Config without LLM section - should use defaults
	minimalConfig := `github:
  repo: "owner/repo"
`
	dir := setupConfigDir(t, minimalConfig)

	cfg, err := config.Load(dir)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	// Check that defaults are set
	if cfg.LLM.Default.ModelID == "" {
		t.Error("llm.default not set automatically")
	}

	// Check that routing thresholds are set
	if cfg.LLM.Routing.Thresholds == nil {
		t.Error("llm.routing.thresholds not initialized")
	}
}

func TestLoad_BackwardCompatibility(t *testing.T) {
	// Old-style config without llm section
	oldStyleConfig := `github:
  repo: "owner/repo"
planning:
  llm: "claude-opus-4"
epic_analysis:
  llm: "claude-sonnet-4"
`
	dir := setupConfigDir(t, oldStyleConfig)

	cfg, err := config.Load(dir)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	// Old fields should still work
	if cfg.Planning.LLM != "claude-opus-4" {
		t.Errorf("planning.llm = %q, want %q", cfg.Planning.LLM, "claude-opus-4")
	}
	if cfg.EpicAnalysis.LLM != "claude-sonnet-4" {
		t.Errorf("epic_analysis.llm = %q, want %q", cfg.EpicAnalysis.LLM, "claude-sonnet-4")
	}

	// New LLM config should have defaults
	if cfg.LLM.Default.ModelID == "" {
		t.Error("llm.default should be set even with old config")
	}
}
