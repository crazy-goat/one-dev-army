package llm

import (
	"context"
	"fmt"
	"sync"
	"time"

	"github.com/crazy-goat/one-dev-army/internal/config"
	"github.com/crazy-goat/one-dev-army/internal/opencode"
)

// Router handles model selection based on task category and complexity
type Router struct {
	cfg            *config.LLMConfig
	mu             sync.RWMutex
	opencodeClient *opencode.Client
	lastReload     time.Time
}

// NewRouter creates a new LLM router with the given configuration
func NewRouter(cfg *config.LLMConfig, oc *opencode.Client) *Router {
	if cfg == nil {
		cfg = &config.LLMConfig{}
		cfg.SetDefaults()
	}

	return &Router{
		cfg:            cfg,
		opencodeClient: oc,
		lastReload:     time.Now(),
	}
}

// SelectModel returns the appropriate model configuration for a task
func (r *Router) SelectModel(category config.TaskCategory, complexity config.ComplexityLevel, hints map[string]interface{}) config.ModelConfig {
	r.mu.RLock()
	defer r.mu.RUnlock()

	// Get model from configuration
	model := r.cfg.GetModelForCategory(category, complexity)

	// Apply any hints-based overrides
	if hints != nil {
		if forceStrong, ok := hints["force_strong"].(bool); ok && forceStrong {
			// Force strong variant
			switch category {
			case config.CategoryDevelopment:
				if r.cfg.Development.Strong.ModelID != "" {
					model = r.cfg.Development.Strong
				}
			case config.CategoryPlanning:
				if r.cfg.Planning.Strong.ModelID != "" {
					model = r.cfg.Planning.Strong
				}
			case config.CategoryOrchestration:
				if r.cfg.Orchestration.Strong.ModelID != "" {
					model = r.cfg.Orchestration.Strong
				}
			case config.CategorySetup:
				if r.cfg.Setup.Strong.ModelID != "" {
					model = r.cfg.Setup.Strong
				}
			}
		}
	}

	return model
}

// SelectModelString returns the model reference string for a task
func (r *Router) SelectModelString(category config.TaskCategory, complexity config.ComplexityLevel, hints map[string]interface{}) string {
	model := r.SelectModel(category, complexity, hints)
	return model.String()
}

// UpdateConfig updates the router configuration (hot-reload)
func (r *Router) UpdateConfig(cfg *config.LLMConfig) {
	r.mu.Lock()
	defer r.mu.Unlock()

	if cfg != nil {
		r.cfg = cfg
		r.lastReload = time.Now()
	}
}

// GetConfig returns the current configuration
func (r *Router) GetConfig() *config.LLMConfig {
	r.mu.RLock()
	defer r.mu.RUnlock()

	// Return a copy to prevent external modification
	cfgCopy := *r.cfg
	return &cfgCopy
}

// LastReload returns the timestamp of the last configuration reload
func (r *Router) LastReload() time.Time {
	r.mu.RLock()
	defer r.mu.RUnlock()
	return r.lastReload
}

// ValidateModels validates that all configured models are available
func (r *Router) ValidateModels() error {
	if r.opencodeClient == nil {
		return fmt.Errorf("opencode client not configured")
	}

	r.mu.RLock()
	cfg := r.cfg
	r.mu.RUnlock()

	// Collect all unique models
	models := make(map[string]config.ModelConfig)

	addModel := func(m config.ModelConfig) {
		if m.ModelID != "" {
			key := m.String()
			models[key] = m
		}
	}

	addModel(cfg.Default)
	addModel(cfg.Development.Strong)
	addModel(cfg.Development.Weak)
	addModel(cfg.Planning.Strong)
	addModel(cfg.Planning.Weak)
	addModel(cfg.Orchestration.Strong)
	addModel(cfg.Orchestration.Weak)
	addModel(cfg.Setup.Strong)
	addModel(cfg.Setup.Weak)

	// Convert to ModelRef slice
	modelRefs := make([]opencode.ModelRef, 0, len(models))
	for _, m := range models {
		modelRefs = append(modelRefs, opencode.ModelRef{
			ProviderID: m.ProviderID,
			ModelID:    m.ModelID,
		})
	}

	return r.opencodeClient.ValidateModels(modelRefs)
}

// RouteRequest represents a routing request with all necessary context
type RouteRequest struct {
	Category   config.TaskCategory
	Complexity config.ComplexityLevel
	Hints      map[string]interface{}
	Content    string // Content to analyze for complexity detection
}

// Route executes the routing logic and returns the selected model
func (r *Router) Route(req RouteRequest) config.ModelConfig {
	// Auto-detect complexity if not specified and content is provided
	complexity := req.Complexity
	if complexity == "" && req.Content != "" {
		complexity = DetectComplexity(req.Content)
	}

	return r.SelectModel(req.Category, complexity, req.Hints)
}

// RouteString executes routing and returns the model reference string
func (r *Router) RouteString(req RouteRequest) string {
	model := r.Route(req)
	return model.String()
}

// Context-aware routing helpers

// ForDevelopment returns the appropriate model for development tasks
func (r *Router) ForDevelopment(complexity config.ComplexityLevel, hints map[string]interface{}) config.ModelConfig {
	return r.SelectModel(config.CategoryDevelopment, complexity, hints)
}

// ForPlanning returns the appropriate model for planning tasks
func (r *Router) ForPlanning(complexity config.ComplexityLevel, hints map[string]interface{}) config.ModelConfig {
	return r.SelectModel(config.CategoryPlanning, complexity, hints)
}

// ForOrchestration returns the appropriate model for orchestration tasks
func (r *Router) ForOrchestration(complexity config.ComplexityLevel, hints map[string]interface{}) config.ModelConfig {
	return r.SelectModel(config.CategoryOrchestration, complexity, hints)
}

// ForSetup returns the appropriate model for setup tasks
func (r *Router) ForSetup(complexity config.ComplexityLevel, hints map[string]interface{}) config.ModelConfig {
	return r.SelectModel(config.CategorySetup, complexity, hints)
}

// String helpers for convenience

// ForDevelopmentString returns model string for development
func (r *Router) ForDevelopmentString(complexity config.ComplexityLevel, hints map[string]interface{}) string {
	return r.ForDevelopment(complexity, hints).String()
}

// ForPlanningString returns model string for planning
func (r *Router) ForPlanningString(complexity config.ComplexityLevel, hints map[string]interface{}) string {
	return r.ForPlanning(complexity, hints).String()
}

// ForOrchestrationString returns model string for orchestration
func (r *Router) ForOrchestrationString(complexity config.ComplexityLevel, hints map[string]interface{}) string {
	return r.ForOrchestration(complexity, hints).String()
}

// ForSetupString returns model string for setup
func (r *Router) ForSetupString(complexity config.ComplexityLevel, hints map[string]interface{}) string {
	return r.ForSetup(complexity, hints).String()
}

// ConfigReloader interface for hot-reload support
type ConfigReloader interface {
	UpdateConfig(cfg *config.LLMConfig)
	GetConfig() *config.LLMConfig
}

// Ensure Router implements ConfigReloader
var _ ConfigReloader = (*Router)(nil)

// WaitForReload blocks until a reload occurs or context is cancelled
func (r *Router) WaitForReload(ctx context.Context, since time.Time) error {
	ticker := time.NewTicker(100 * time.Millisecond)
	defer ticker.Stop()

	for {
		select {
		case <-ctx.Done():
			return ctx.Err()
		case <-ticker.C:
			if r.LastReload().After(since) {
				return nil
			}
		}
	}
}
