package config

// TaskCategory represents the type of task for LLM routing
type TaskCategory string

const (
	CategoryDevelopment   TaskCategory = "development"   // Complex coding tasks
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
type ModelVariant string

const (
	VariantStrong ModelVariant = "strong" // High quality, expensive
	VariantWeak   ModelVariant = "weak"   // Fast, cheap
)

// ModelConfig represents a single LLM model configuration
type ModelConfig struct {
	Provider string `yaml:"provider"`           // e.g., "openai", "anthropic", "local"
	Model    string `yaml:"model"`              // e.g., "gpt-4", "claude-3-opus"
	APIKey   string `yaml:"api_key,omitempty"`  // Optional API key (can use env var)
	BaseURL  string `yaml:"base_url,omitempty"` // Optional custom base URL
}

// CategoryModels holds strong and weak model variants for a category
type CategoryModels struct {
	Strong ModelConfig `yaml:"strong"`
	Weak   ModelConfig `yaml:"weak"`
}

// LLMConfig holds the multi-model configuration with task-based routing
type LLMConfig struct {
	// Categories define models for each task type
	Development   CategoryModels `yaml:"development"`
	Planning      CategoryModels `yaml:"planning"`
	Orchestration CategoryModels `yaml:"orchestration"`
	Setup         CategoryModels `yaml:"setup"`

	// DefaultComplexity is used when no complexity hint is provided
	DefaultComplexity ComplexityLevel `yaml:"default_complexity,omitempty"`

	// RoutingRules define when to use strong vs weak models
	RoutingRules RoutingConfig `yaml:"routing_rules,omitempty"`
}

// RoutingConfig defines rules for model selection
type RoutingConfig struct {
	// ComplexityThresholds define when to upgrade to strong model
	ComplexityThresholds ComplexityThresholds `yaml:"complexity_thresholds,omitempty"`

	// ForceStrongForStages lists pipeline stages that always use strong models
	ForceStrongForStages []string `yaml:"force_strong_for_stages,omitempty"`
}

// ComplexityThresholds defines thresholds for complexity detection
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
		Development: CategoryModels{
			Strong: ModelConfig{
				Provider: "nexos-ai",
				Model:    "Kimi K2.5",
			},
			Weak: ModelConfig{
				Provider: "nexos-ai",
				Model:    "Kimi K2.5",
			},
		},
		Planning: CategoryModels{
			Strong: ModelConfig{
				Provider: "nexos-ai",
				Model:    "Kimi K2.5",
			},
			Weak: ModelConfig{
				Provider: "nexos-ai",
				Model:    "Kimi K2.5",
			},
		},
		Orchestration: CategoryModels{
			Strong: ModelConfig{
				Provider: "nexos-ai",
				Model:    "Kimi K2.5",
			},
			Weak: ModelConfig{
				Provider: "nexos-ai",
				Model:    "Kimi K2.5",
			},
		},
		Setup: CategoryModels{
			Strong: ModelConfig{
				Provider: "nexos-ai",
				Model:    "Kimi K2.5",
			},
			Weak: ModelConfig{
				Provider: "nexos-ai",
				Model:    "Kimi K2.5",
			},
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

// GetModelForCategory returns the appropriate model config for a category and complexity
func (cfg *LLMConfig) GetModelForCategory(category TaskCategory, complexity ComplexityLevel) ModelConfig {
	var models CategoryModels

	switch category {
	case CategoryDevelopment:
		models = cfg.Development
	case CategoryPlanning:
		models = cfg.Planning
	case CategoryOrchestration:
		models = cfg.Orchestration
	case CategorySetup:
		models = cfg.Setup
	default:
		models = cfg.Development
	}

	// Use strong model for high complexity or if no weak model configured
	if complexity == ComplexityHigh || models.Weak.Model == "" {
		return models.Strong
	}

	return models.Weak
}

// ShouldUseStrongModel determines if a task should use the strong model based on complexity
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
	if mc.Provider == "" || mc.Model == "" {
		return ""
	}
	return mc.Provider + "/" + mc.Model
}
