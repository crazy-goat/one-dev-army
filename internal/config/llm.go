package config

import (
	"fmt"
	"strings"
)

// TaskCategory represents the type of task being performed
type TaskCategory string

const (
	CategoryDevelopment   TaskCategory = "development"   // Complex coding tasks
	CategoryPlanning      TaskCategory = "planning"      // Technical analysis, sprint planning
	CategoryOrchestration TaskCategory = "orchestration" // Ticket selection, task routing
	CategorySetup         TaskCategory = "setup"         // CI generation, project setup
)

// ComplexityLevel represents the complexity hint for task routing
type ComplexityLevel string

const (
	ComplexityLow    ComplexityLevel = "low"
	ComplexityMedium ComplexityLevel = "medium"
	ComplexityHigh   ComplexityLevel = "high"
)

// ModelVariant represents the strength variant of a model
type ModelVariant string

const (
	VariantStrong ModelVariant = "strong" // High quality, expensive
	VariantWeak   ModelVariant = "weak"   // Fast, cheap
)

// ModelConfig holds configuration for a specific model
type ModelConfig struct {
	ProviderID string `yaml:"provider_id" json:"provider_id"`
	ModelID    string `yaml:"model_id" json:"model_id"`
	APIKey     string `yaml:"api_key,omitempty" json:"api_key,omitempty"`
}

// String returns the model reference string (provider/model)
func (m ModelConfig) String() string {
	if m.ProviderID == "" {
		return m.ModelID
	}
	return fmt.Sprintf("%s/%s", m.ProviderID, m.ModelID)
}

// CategoryModels holds strong and weak model variants for a task category
type CategoryModels struct {
	Strong ModelConfig `yaml:"strong" json:"strong"`
	Weak   ModelConfig `yaml:"weak" json:"weak"`
}

// LLMConfig holds the multi-model configuration with task-based routing
type LLMConfig struct {
	// Categories define models for each task type
	Development   CategoryModels `yaml:"development" json:"development"`
	Planning      CategoryModels `yaml:"planning" json:"planning"`
	Orchestration CategoryModels `yaml:"orchestration" json:"orchestration"`
	Setup         CategoryModels `yaml:"setup" json:"setup"`

	// Default models for backward compatibility
	Default ModelConfig `yaml:"default" json:"default"`

	// Routing configuration
	Routing RoutingConfig `yaml:"routing" json:"routing"`
}

// RoutingConfig holds complexity-based routing rules
type RoutingConfig struct {
	// Thresholds define when to use strong vs weak models
	// For each category, if complexity >= threshold, use strong model
	Thresholds map[TaskCategory]ComplexityLevel `yaml:"thresholds" json:"thresholds"`
}

// GetModelForCategory returns the appropriate model for a category and complexity
func (l *LLMConfig) GetModelForCategory(category TaskCategory, complexity ComplexityLevel) ModelConfig {
	var models CategoryModels

	switch category {
	case CategoryDevelopment:
		models = l.Development
	case CategoryPlanning:
		models = l.Planning
	case CategoryOrchestration:
		models = l.Orchestration
	case CategorySetup:
		models = l.Setup
	default:
		// Fall back to default if category not found
		return l.Default
	}

	// Check if we should use strong model based on complexity
	if shouldUseStrong(l.Routing.Thresholds[category], complexity) {
		if models.Strong.ModelID != "" {
			return models.Strong
		}
		// Fall back to default if strong not configured
		return l.Default
	}

	// Use weak model
	if models.Weak.ModelID != "" {
		return models.Weak
	}
	// Fall back to strong if weak not configured, then default
	if models.Strong.ModelID != "" {
		return models.Strong
	}
	return l.Default
}

// shouldUseStrong determines if strong model should be used based on threshold
func shouldUseStrong(threshold, complexity ComplexityLevel) bool {
	// If no threshold set, default to using strong for medium and high
	if threshold == "" {
		threshold = ComplexityMedium
	}

	levels := map[ComplexityLevel]int{
		ComplexityLow:    1,
		ComplexityMedium: 2,
		ComplexityHigh:   3,
	}

	return levels[complexity] >= levels[threshold]
}

// SetDefaults populates default values for missing configuration
func (l *LLMConfig) SetDefaults() {
	// Set default routing thresholds
	if l.Routing.Thresholds == nil {
		l.Routing.Thresholds = make(map[TaskCategory]ComplexityLevel)
	}

	// Default thresholds by category
	defaults := map[TaskCategory]ComplexityLevel{
		CategoryDevelopment:   ComplexityMedium,
		CategoryPlanning:      ComplexityLow, // Planning usually needs strong model
		CategoryOrchestration: ComplexityLow, // Orchestration can use weak
		CategorySetup:         ComplexityMedium,
	}

	for cat, level := range defaults {
		if l.Routing.Thresholds[cat] == "" {
			l.Routing.Thresholds[cat] = level
		}
	}

	// If no default model set, use a sensible default
	if l.Default.ModelID == "" {
		l.Default = ModelConfig{
			ProviderID: "anthropic",
			ModelID:    "claude-sonnet-4",
		}
	}

	// Propagate defaults to categories if not set
	l.propagateDefaults()
}

// propagateDefaults fills in missing category models from defaults
func (l *LLMConfig) propagateDefaults() {
	// Helper to set defaults for a category
	setCategoryDefaults := func(models *CategoryModels) {
		if models.Strong.ModelID == "" {
			models.Strong = l.Default
		}
		if models.Weak.ModelID == "" {
			// Weak defaults to strong if not specified
			models.Weak = models.Strong
		}
	}

	setCategoryDefaults(&l.Development)
	setCategoryDefaults(&l.Planning)
	setCategoryDefaults(&l.Orchestration)
	setCategoryDefaults(&l.Setup)
}

// Validate checks the configuration for errors
func (l *LLMConfig) Validate() error {
	var errors []string

	// Check that at least default is configured
	if l.Default.ModelID == "" {
		errors = append(errors, "default model is required")
	}

	// Validate all category models
	categories := map[string]CategoryModels{
		"development":   l.Development,
		"planning":      l.Planning,
		"orchestration": l.Orchestration,
		"setup":         l.Setup,
	}

	for name, models := range categories {
		if models.Strong.ModelID != "" && models.Strong.ProviderID == "" {
			errors = append(errors, fmt.Sprintf("%s.strong model missing provider_id", name))
		}
		if models.Weak.ModelID != "" && models.Weak.ProviderID == "" {
			errors = append(errors, fmt.Sprintf("%s.weak model missing provider_id", name))
		}
	}

	if len(errors) > 0 {
		return fmt.Errorf("LLM config validation failed: %s", strings.Join(errors, "; "))
	}

	return nil
}

// ToMap converts the config to a map for serialization
func (l *LLMConfig) ToMap() map[string]interface{} {
	return map[string]interface{}{
		"development": map[string]interface{}{
			"strong": l.Development.Strong,
			"weak":   l.Development.Weak,
		},
		"planning": map[string]interface{}{
			"strong": l.Planning.Strong,
			"weak":   l.Planning.Weak,
		},
		"orchestration": map[string]interface{}{
			"strong": l.Orchestration.Strong,
			"weak":   l.Orchestration.Weak,
		},
		"setup": map[string]interface{}{
			"strong": l.Setup.Strong,
			"weak":   l.Setup.Weak,
		},
		"default": l.Default,
		"routing": map[string]interface{}{
			"thresholds": l.Routing.Thresholds,
		},
	}
}
