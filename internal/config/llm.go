package config

import (
	"strings"
)

// TaskCategory represents the type of task for LLM routing
type TaskCategory string

const (
	CategoryCode          TaskCategory = "code"          // Complex coding tasks (renamed from development)
	CategoryCodeHeavy     TaskCategory = "code-heavy"    // Heavy/complex coding tasks
	CategoryPlanning      TaskCategory = "planning"      // Technical analysis, sprint planning
	CategoryOrchestration TaskCategory = "orchestration" // Ticket selection, task routing
	CategorySetup         TaskCategory = "setup"         // CI generation, project setup
)

// ComplexityLevel represents the complexity hint for model selection.
// Kept for keyword-based complexity estimation in the llm package.
type ComplexityLevel string

const (
	ComplexityLow    ComplexityLevel = "low"
	ComplexityMedium ComplexityLevel = "medium"
	ComplexityHigh   ComplexityLevel = "high"
)

// ModelConfig represents a single LLM model configuration
type ModelConfig struct {
	Model string `yaml:"model"` // e.g., "nexos-ai/Kimi K2.5" (format: "provider/model")
}

// ParseModel extracts the provider and model name from the Model field
// Expected format: "provider/model" (e.g., "nexos-ai/Kimi K2.5")
func (mc *ModelConfig) ParseModel() (provider, model string) {
	if mc.Model == "" {
		return "", ""
	}

	parts := strings.SplitN(mc.Model, "/", 2)
	if len(parts) == 2 {
		return parts[0], parts[1]
	}

	// If no slash found, return empty provider and full string as model
	return "", mc.Model
}

// GetProvider returns the provider from the Model field
func (mc *ModelConfig) GetProvider() string {
	provider, _ := mc.ParseModel()
	return provider
}

// GetModelName returns the model name from the Model field
func (mc *ModelConfig) GetModelName() string {
	_, model := mc.ParseModel()
	return model
}

// CategoryModels holds the model configuration for a task category
// Now uses a single model per category instead of strong/weak variants
type CategoryModels struct {
	Model string `yaml:"model"`
}

// NormalizeModel ensures the model is in "provider/model" format.
// If the model doesn't contain a "/", it attempts to detect the provider
// based on known model name prefixes.
func (cm *CategoryModels) NormalizeModel() string {
	if cm.Model == "" {
		return ""
	}

	// If already in provider/model format, return as-is
	if strings.Contains(cm.Model, "/") {
		return cm.Model
	}

	// Otherwise, try to detect provider from model name
	provider := detectProvider(cm.Model)
	if provider != "" {
		return provider + "/" + cm.Model
	}

	// If no provider detected, return as-is (will need manual fix)
	return cm.Model
}

// detectProvider attempts to detect the provider from a model name
func detectProvider(modelName string) string {
	// Known provider prefixes
	prefixes := map[string]string{
		"claude":   "anthropic",
		"gpt":      "openai",
		"o3":       "openai",
		"o4":       "openai",
		"gemini":   "google",
		"llama":    "groq",
		"mistral":  "mistral",
		"deepseek": "deepseek",
		"kimi":     "nexos-ai",
	}

	lowerModel := strings.ToLower(modelName)
	for prefix, provider := range prefixes {
		if strings.HasPrefix(lowerModel, prefix) {
			return provider
		}
	}

	return ""
}

// LLMConfig holds the multi-model configuration with task-based routing
type LLMConfig struct {
	// Categories define models for each task type (5 independent modes)
	Setup         CategoryModels `yaml:"setup"`
	Planning      CategoryModels `yaml:"planning"`
	Orchestration CategoryModels `yaml:"orchestration"`
	Code          CategoryModels `yaml:"code"`       // renamed from Development
	CodeHeavy     CategoryModels `yaml:"code-heavy"` // new
}

// NormalizeAllModels ensures all models are in "provider/model" format.
// This should be called before saving the configuration.
func (cfg *LLMConfig) NormalizeAllModels() {
	cfg.Setup.Model = cfg.Setup.NormalizeModel()
	cfg.Planning.Model = cfg.Planning.NormalizeModel()
	cfg.Orchestration.Model = cfg.Orchestration.NormalizeModel()
	cfg.Code.Model = cfg.Code.NormalizeModel()
	cfg.CodeHeavy.Model = cfg.CodeHeavy.NormalizeModel()
}

// DefaultLLMConfig returns a default configuration with sensible defaults
func DefaultLLMConfig() LLMConfig {
	return LLMConfig{
		Setup: CategoryModels{
			Model: "nexos-ai/Kimi K2.5",
		},
		Planning: CategoryModels{
			Model: "nexos-ai/Kimi K2.5",
		},
		Orchestration: CategoryModels{
			Model: "nexos-ai/Kimi K2.5",
		},
		Code: CategoryModels{
			Model: "nexos-ai/Kimi K2.5",
		},
		CodeHeavy: CategoryModels{
			Model: "nexos-ai/Kimi K2.5",
		},
	}
}

// GetModelForCategory returns the model config for a category
// Note: complexity parameter is now ignored - each mode has a single model
func (cfg *LLMConfig) GetModelForCategory(category TaskCategory, _ ComplexityLevel) ModelConfig {
	switch category {
	case CategorySetup:
		return ModelConfig{Model: cfg.Setup.Model}
	case CategoryPlanning:
		return ModelConfig{Model: cfg.Planning.Model}
	case CategoryOrchestration:
		return ModelConfig{Model: cfg.Orchestration.Model}
	case CategoryCode:
		return ModelConfig{Model: cfg.Code.Model}
	case CategoryCodeHeavy:
		return ModelConfig{Model: cfg.CodeHeavy.Model}
	default:
		return ModelConfig{Model: cfg.Code.Model}
	}
}

// ToModelRef converts a ModelConfig to a model reference string
func (mc *ModelConfig) ToModelRef() string {
	return mc.Model
}

// ModelValidationResult tracks which models were replaced during validation
type ModelValidationResult struct {
	ReplacedModels map[string]struct {
		OldModel string
		NewModel string
	}
	HasReplacements bool
	ValidatedConfig LLMConfig // New: returns a copy with fallbacks applied, original unchanged
}

// ValidateAndFallbackModels validates all models against available models and returns a result
// with fallback models applied. The original config is NOT mutated - instead, the result
// contains a validated copy that can be used when needed.
// Returns a result indicating which models would be replaced and a validated config copy
func (cfg *LLMConfig) ValidateAndFallbackModels(availableModels []string) ModelValidationResult {
	result := ModelValidationResult{
		ReplacedModels: make(map[string]struct {
			OldModel string
			NewModel string
		}),
		ValidatedConfig: *cfg, // Copy the original config
	}

	// If no available models, skip validation (API might be unavailable)
	if len(availableModels) == 0 {
		return result
	}

	// Build set of available models for O(1) lookup
	availableSet := make(map[string]bool)
	for _, m := range availableModels {
		availableSet[m] = true
	}

	// Get first available model as fallback
	fallbackModel := availableModels[0]

	// Validate and fallback each model (modifying the copy, not the original)
	modes := []struct {
		name      string
		model     *string
		defaultTo string
	}{
		{"Setup", &result.ValidatedConfig.Setup.Model, fallbackModel},
		{"Planning", &result.ValidatedConfig.Planning.Model, fallbackModel},
		{"Orchestration", &result.ValidatedConfig.Orchestration.Model, fallbackModel},
		{"Code", &result.ValidatedConfig.Code.Model, fallbackModel},
		{"CodeHeavy", &result.ValidatedConfig.CodeHeavy.Model, fallbackModel},
	}

	for _, mode := range modes {
		if *mode.model == "" || !availableSet[*mode.model] {
			oldModel := *mode.model
			if oldModel == "" {
				oldModel = "(empty)"
			}
			*mode.model = mode.defaultTo
			result.ReplacedModels[mode.name] = struct {
				OldModel string
				NewModel string
			}{
				OldModel: oldModel,
				NewModel: mode.defaultTo,
			}
			result.HasReplacements = true
		}
	}

	return result
}

// GetFirstAvailableModel returns the first available model from the list, or empty if none available
func GetFirstAvailableModel(availableModels []string) string {
	if len(availableModels) > 0 {
		return availableModels[0]
	}
	return ""
}
