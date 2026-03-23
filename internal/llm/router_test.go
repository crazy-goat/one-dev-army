package llm_test

import (
	"testing"

	"github.com/crazy-goat/one-dev-army/internal/config"
	"github.com/crazy-goat/one-dev-army/internal/llm"
	"github.com/crazy-goat/one-dev-army/internal/opencode"
)

func TestNewRouter(t *testing.T) {
	// Test with nil config - should create with defaults
	router := llm.NewRouter(nil, nil)
	if router == nil {
		t.Fatal("NewRouter with nil config should not return nil")
	}

	cfg := router.GetConfig()
	if cfg == nil {
		t.Fatal("router should have a config")
	}

	// Test with provided config
	customCfg := &config.LLMConfig{
		Default: config.ModelConfig{
			ProviderID: "anthropic",
			ModelID:    "claude-sonnet-4",
		},
	}
	router2 := llm.NewRouter(customCfg, nil)
	cfg2 := router2.GetConfig()
	if cfg2.Default.ModelID != "claude-sonnet-4" {
		t.Errorf("router config default model = %q, want %q", cfg2.Default.ModelID, "claude-sonnet-4")
	}
}

func TestRouter_SelectModel(t *testing.T) {
	cfg := &config.LLMConfig{
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

	router := llm.NewRouter(cfg, nil)

	// Test development with low complexity
	model := router.SelectModel(config.CategoryDevelopment, config.ComplexityLow, nil)
	if model.ModelID != "claude-haiku-4" {
		t.Errorf("development + low = %q, want %q", model.ModelID, "claude-haiku-4")
	}

	// Test development with high complexity
	model = router.SelectModel(config.CategoryDevelopment, config.ComplexityHigh, nil)
	if model.ModelID != "claude-opus-4" {
		t.Errorf("development + high = %q, want %q", model.ModelID, "claude-opus-4")
	}

	// Test planning (always strong due to threshold)
	model = router.SelectModel(config.CategoryPlanning, config.ComplexityLow, nil)
	if model.ModelID != "gpt-4" {
		t.Errorf("planning + low = %q, want %q", model.ModelID, "gpt-4")
	}
}

func TestRouter_SelectModelString(t *testing.T) {
	cfg := &config.LLMConfig{
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
		Routing: config.RoutingConfig{
			Thresholds: map[config.TaskCategory]config.ComplexityLevel{
				config.CategoryDevelopment: config.ComplexityMedium,
			},
		},
	}

	router := llm.NewRouter(cfg, nil)

	modelStr := router.SelectModelString(config.CategoryDevelopment, config.ComplexityHigh, nil)
	if modelStr != "anthropic/claude-opus-4" {
		t.Errorf("model string = %q, want %q", modelStr, "anthropic/claude-opus-4")
	}
}

func TestRouter_SelectModelWithHints(t *testing.T) {
	cfg := &config.LLMConfig{
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
		Routing: config.RoutingConfig{
			Thresholds: map[config.TaskCategory]config.ComplexityLevel{
				config.CategoryDevelopment: config.ComplexityMedium,
			},
		},
	}

	router := llm.NewRouter(cfg, nil)

	// Test with force_strong hint
	hints := map[string]interface{}{
		"force_strong": true,
	}
	model := router.SelectModel(config.CategoryDevelopment, config.ComplexityLow, hints)
	if model.ModelID != "claude-opus-4" {
		t.Errorf("with force_strong hint = %q, want %q", model.ModelID, "claude-opus-4")
	}
}

func TestRouter_UpdateConfig(t *testing.T) {
	cfg1 := &config.LLMConfig{
		Default: config.ModelConfig{
			ProviderID: "anthropic",
			ModelID:    "claude-sonnet-4",
		},
	}

	router := llm.NewRouter(cfg1, nil)

	// Update config
	cfg2 := &config.LLMConfig{
		Default: config.ModelConfig{
			ProviderID: "openai",
			ModelID:    "gpt-4",
		},
	}

	router.UpdateConfig(cfg2)

	// Verify update
	updatedCfg := router.GetConfig()
	if updatedCfg.Default.ModelID != "gpt-4" {
		t.Errorf("updated config default model = %q, want %q", updatedCfg.Default.ModelID, "gpt-4")
	}

	// Verify last reload time was updated
	if router.LastReload().IsZero() {
		t.Error("last reload time should not be zero after update")
	}
}

func TestRouter_ContextHelpers(t *testing.T) {
	cfg := &config.LLMConfig{
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
		Orchestration: config.CategoryModels{
			Strong: config.ModelConfig{
				ProviderID: "anthropic",
				ModelID:    "claude-sonnet-4",
			},
			Weak: config.ModelConfig{
				ProviderID: "anthropic",
				ModelID:    "claude-haiku-4",
			},
		},
		Setup: config.CategoryModels{
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
				config.CategoryDevelopment:   config.ComplexityMedium,
				config.CategoryPlanning:      config.ComplexityLow,
				config.CategoryOrchestration: config.ComplexityLow,
				config.CategorySetup:         config.ComplexityMedium,
			},
		},
	}

	router := llm.NewRouter(cfg, nil)

	// Test ForDevelopment
	model := router.ForDevelopment(config.ComplexityHigh, nil)
	if model.ModelID != "claude-opus-4" {
		t.Errorf("ForDevelopment = %q, want %q", model.ModelID, "claude-opus-4")
	}

	// Test ForPlanning
	model = router.ForPlanning(config.ComplexityLow, nil)
	if model.ModelID != "gpt-4" {
		t.Errorf("ForPlanning = %q, want %q", model.ModelID, "gpt-4")
	}

	// Test ForOrchestration
	// With threshold "low", even low complexity uses strong model
	model = router.ForOrchestration(config.ComplexityLow, nil)
	if model.ModelID != "claude-sonnet-4" {
		t.Errorf("ForOrchestration = %q, want %q", model.ModelID, "claude-sonnet-4")
	}

	// Test ForSetup
	model = router.ForSetup(config.ComplexityHigh, nil)
	if model.ModelID != "gpt-4" {
		t.Errorf("ForSetup = %q, want %q", model.ModelID, "gpt-4")
	}

	// Test string helpers
	modelStr := router.ForDevelopmentString(config.ComplexityHigh, nil)
	if modelStr != "anthropic/claude-opus-4" {
		t.Errorf("ForDevelopmentString = %q, want %q", modelStr, "anthropic/claude-opus-4")
	}
}

func TestRouter_Route(t *testing.T) {
	cfg := &config.LLMConfig{
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
		Routing: config.RoutingConfig{
			Thresholds: map[config.TaskCategory]config.ComplexityLevel{
				config.CategoryDevelopment: config.ComplexityMedium,
			},
		},
	}

	router := llm.NewRouter(cfg, nil)

	// Test Route with content for auto-detection
	req := llm.RouteRequest{
		Category: config.CategoryDevelopment,
		Content:  "This is a simple task with minimal code changes",
	}
	model := router.Route(req)
	// Should use weak model due to low complexity content
	if model.ModelID != "claude-haiku-4" {
		t.Errorf("Route with simple content = %q, want %q", model.ModelID, "claude-haiku-4")
	}

	// Test RouteString
	req2 := llm.RouteRequest{
		Category:   config.CategoryDevelopment,
		Complexity: config.ComplexityHigh,
	}
	modelStr := router.RouteString(req2)
	if modelStr != "anthropic/claude-opus-4" {
		t.Errorf("RouteString = %q, want %q", modelStr, "anthropic/claude-opus-4")
	}
}

func TestRouter_ValidateModels(t *testing.T) {
	// Test with nil opencode client
	router := llm.NewRouter(nil, nil)
	err := router.ValidateModels()
	if err == nil {
		t.Error("ValidateModels with nil client should error")
	}

	// Note: Testing with actual client would require mocking
	// This is covered by integration tests
}

func TestRouter_GetConfigThreadSafety(t *testing.T) {
	cfg := &config.LLMConfig{
		Default: config.ModelConfig{
			ProviderID: "anthropic",
			ModelID:    "claude-sonnet-4",
		},
	}

	router := llm.NewRouter(cfg, nil)

	// Get config should return a copy
	cfg1 := router.GetConfig()
	cfg2 := router.GetConfig()

	// Modify cfg1
	cfg1.Default.ModelID = "modified"

	// cfg2 should be unchanged
	if cfg2.Default.ModelID != "claude-sonnet-4" {
		t.Error("GetConfig should return a copy, not a reference")
	}
}

// Mock opencode client for testing validation
type mockOpencodeClient struct {
	validateCalled bool
	models         []opencode.ModelRef
}

func (m *mockOpencodeClient) ValidateModels(models []opencode.ModelRef) error {
	m.validateCalled = true
	m.models = models
	return nil
}
