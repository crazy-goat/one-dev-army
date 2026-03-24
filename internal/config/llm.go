package config

import "strings"

// TaskCategory represents the type of task for LLM routing
type TaskCategory string

const (
	CategoryCode          TaskCategory = "code"          // Complex coding tasks (renamed from development)
	CategoryCodeHeavy     TaskCategory = "code-heavy"    // Heavy/complex coding tasks
	CategoryPlanning      TaskCategory = "planning"      // Technical analysis, sprint planning
	CategoryOrchestration TaskCategory = "orchestration" // Ticket selection, task routing
	CategorySetup         TaskCategory = "setup"         // CI generation, project setup
)

// ComplexityLevel represents the complexity hint for model selection
type ComplexityLevel string

const (
	ComplexityLow    ComplexityLevel = "low"
	ComplexityMedium ComplexityLevel = "medium"
	ComplexityHigh   ComplexityLevel = "high"
)

// ModelVariant represents the strength variant for a model
// Deprecated: No longer used with per-mode model selection
type ModelVariant string

const (
	VariantStrong ModelVariant = "strong" // High quality, expensive
	VariantWeak   ModelVariant = "weak"   // Fast, cheap
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

// LLMConfig holds the multi-model configuration with task-based routing
type LLMConfig struct {
	// Categories define models for each task type (5 independent modes)
	Setup         CategoryModels `yaml:"setup"`
	Planning      CategoryModels `yaml:"planning"`
	Orchestration CategoryModels `yaml:"orchestration"`
	Code          CategoryModels `yaml:"code"`       // renamed from Development
	CodeHeavy     CategoryModels `yaml:"code-heavy"` // new

	// DefaultComplexity is used when no complexity hint is provided
	DefaultComplexity ComplexityLevel `yaml:"default_complexity,omitempty"`

	// RoutingRules define when to use strong vs weak models
	// Deprecated: Complexity-based routing is being phased out in favor of explicit mode selection
	RoutingRules RoutingConfig `yaml:"routing_rules,omitempty"`
}

// RoutingConfig defines rules for model selection
// Deprecated: Being phased out in favor of explicit per-mode model selection
type RoutingConfig struct {
	// ComplexityThresholds define when to upgrade to strong model
	ComplexityThresholds ComplexityThresholds `yaml:"complexity_thresholds,omitempty"`

	// ForceStrongForStages lists pipeline stages that always use strong models
	ForceStrongForStages []string `yaml:"force_strong_for_stages,omitempty"`
}

// ComplexityThresholds defines thresholds for complexity detection
// Deprecated: Being phased out in favor of explicit per-mode model selection
type ComplexityThresholds struct {
	// CodeSizeThreshold is the number of lines that triggers medium complexity
	CodeSizeThreshold int `yaml:"code_size_threshold,omitempty"`

	// HighComplexityThreshold is the number of lines that triggers high complexity
	HighComplexityThreshold int `yaml:"high_complexity_threshold,omitempty"`

	// FileCountThreshold is the number of files that triggers higher complexity
	FileCountThreshold int `yaml:"file_count_threshold,omitempty"`
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
		DefaultComplexity: ComplexityMedium,
		RoutingRules: RoutingConfig{
			ComplexityThresholds: ComplexityThresholds{
				CodeSizeThreshold:       100,
				HighComplexityThreshold: 500,
				FileCountThreshold:      5,
			},
			ForceStrongForStages: []string{"plan-review", "code-review", "merge"},
		},
	}
}

// GetModelForCategory returns the model config for a category
// Note: complexity parameter is now ignored - each mode has a single model
func (cfg *LLMConfig) GetModelForCategory(category TaskCategory, complexity ComplexityLevel) ModelConfig {
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

// ShouldUseStrongModel determines if a task should use the strong model based on complexity
// Deprecated: No longer relevant with per-mode model selection, kept for backward compatibility
func (cfg *LLMConfig) ShouldUseStrongModel(complexity ComplexityLevel, stage string) bool {
	// Check if stage is in force-strong list
	for _, s := range cfg.RoutingRules.ForceStrongForStages {
		if s == stage {
			return true
		}
	}

	// Otherwise use complexity-based routing
	return complexity == ComplexityHigh
}

// ToModelRef converts a ModelConfig to a model reference string
func (mc *ModelConfig) ToModelRef() string {
	return mc.Model
}

// LegacyCategoryModels holds strong and weak model variants for a category
// Used for migration from old config format
type LegacyCategoryModels struct {
	Strong ModelConfig `yaml:"strong"`
	Weak   ModelConfig `yaml:"weak"`
}

// LegacyLLMConfig represents the old config format with strong/weak variants
type LegacyLLMConfig struct {
	Development       LegacyCategoryModels `yaml:"development"`
	Planning          LegacyCategoryModels `yaml:"planning"`
	Orchestration     LegacyCategoryModels `yaml:"orchestration"`
	Setup             LegacyCategoryModels `yaml:"setup"`
	DefaultComplexity ComplexityLevel      `yaml:"default_complexity,omitempty"`
	RoutingRules      RoutingConfig        `yaml:"routing_rules,omitempty"`
}

// MigrateFromLegacy converts old strong/weak format to new single-model format
// Returns true if migration was performed
func (cfg *LLMConfig) MigrateFromLegacy(legacy *LegacyLLMConfig) bool {
	migrated := false

	// Migrate Development -> Code (use strong model)
	if legacy.Development.Strong.Model != "" {
		cfg.Code.Model = legacy.Development.Strong.Model
		migrated = true
	}

	// Migrate Planning
	if legacy.Planning.Strong.Model != "" {
		cfg.Planning.Model = legacy.Planning.Strong.Model
		migrated = true
	}

	// Migrate Orchestration
	if legacy.Orchestration.Strong.Model != "" {
		cfg.Orchestration.Model = legacy.Orchestration.Strong.Model
		migrated = true
	}

	// Migrate Setup
	if legacy.Setup.Strong.Model != "" {
		cfg.Setup.Model = legacy.Setup.Strong.Model
		migrated = true
	}

	// Copy other settings
	cfg.DefaultComplexity = legacy.DefaultComplexity
	cfg.RoutingRules = legacy.RoutingRules

	return migrated
}
