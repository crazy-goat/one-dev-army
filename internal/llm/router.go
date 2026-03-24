package llm

import (
	"os"
	"path/filepath"
	"sync"
	"time"

	"github.com/crazy-goat/one-dev-army/internal/config"
)

// Router provides intelligent model selection based on task category
type Router struct {
	cfg             *config.LLMConfig
	availableModels []string
	mu              sync.RWMutex
	onReload        []func()
}

// NewRouter creates a new LLM router with the given configuration
func NewRouter(cfg *config.LLMConfig) *Router {
	return &Router{
		cfg:             cfg,
		availableModels: make([]string, 0),
		onReload:        make([]func(), 0),
	}
}

// SetAvailableModels updates the list of available models for runtime fallback
func (r *Router) SetAvailableModels(models []string) {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.availableModels = models
}

// GetAvailableModels returns the current list of available models
func (r *Router) GetAvailableModels() []string {
	r.mu.RLock()
	defer r.mu.RUnlock()
	return r.availableModels
}

// selectModelWithFallback returns the model if available, otherwise returns the first available model
func (r *Router) selectModelWithFallback(model string) string {
	// If no available models tracked, return the configured model as-is
	if len(r.availableModels) == 0 {
		return model
	}

	// Build set for O(1) lookup
	availableSet := make(map[string]bool)
	for _, m := range r.availableModels {
		availableSet[m] = true
	}

	// If model is available, use it
	if availableSet[model] {
		return model
	}

	// Otherwise fallback to first available
	return r.availableModels[0]
}

// SelectModel returns the appropriate model reference for a task
// The complexity parameter is kept for backward compatibility but is now ignored
// Each task category has a single dedicated model
func (r *Router) SelectModel(category config.TaskCategory, _ config.ComplexityLevel, _ map[string]any) string {
	r.mu.RLock()
	defer r.mu.RUnlock()

	// Get model for category (each mode has a single model now)
	modelCfg := r.cfg.GetModelForCategory(category, config.ComplexityMedium)
	configuredModel := modelCfg.ToModelRef()

	// Apply runtime fallback if model is not available
	return r.selectModelWithFallback(configuredModel)
}

// SelectModelForStage returns the appropriate model for a pipeline stage
// Uses the new per-mode model selection
func (r *Router) SelectModelForStage(stage string, _ string) string {
	category := categoryForStage(stage)
	return r.SelectModel(category, config.ComplexityMedium, nil)
}

// categoryForStage maps pipeline stages to task categories
func categoryForStage(stage string) config.TaskCategory {
	switch stage {
	case "analysis", "planning", "plan-review":
		return config.CategoryPlanning
	case "coding", "testing", "code-review":
		return config.CategoryCode
	case "orchestration", "ticket-selection":
		return config.CategoryOrchestration
	case "setup", "ci-generation", "project-setup":
		return config.CategorySetup
	default:
		return config.CategoryCode
	}
}

func (r *Router) UpdateConfig(cfg *config.LLMConfig) {
	r.mu.Lock()
	defer r.mu.Unlock()

	r.cfg = cfg

	// Notify listeners synchronously
	// Callbacks should be lightweight; callers needing async behavior
	// can spawn their own goroutines inside the callback.
	for _, fn := range r.onReload {
		fn()
	}
}

// OnReload registers a callback to be called when config is reloaded
func (r *Router) OnReload(fn func()) {
	r.mu.Lock()
	defer r.mu.Unlock()

	r.onReload = append(r.onReload, fn)
}

// GetConfig returns the current configuration (for inspection)
func (r *Router) GetConfig() config.LLMConfig {
	r.mu.RLock()
	defer r.mu.RUnlock()

	return *r.cfg
}

// RoutingHints provides a builder for routing hints
//
// Deprecated: Hints are no longer used for model selection
type RoutingHints struct {
	hints map[string]any
}

// NewRoutingHints creates a new routing hints builder
//
// Deprecated: Hints are no longer used for model selection
func NewRoutingHints() *RoutingHints {
	return &RoutingHints{
		hints: make(map[string]any),
	}
}

// WithStage adds stage information to hints
func (h *RoutingHints) WithStage(stage string) *RoutingHints {
	h.hints["stage"] = stage
	return h
}

// WithFileCount adds file count to hints
func (h *RoutingHints) WithFileCount(count int) *RoutingHints {
	h.hints["file_count"] = count
	return h
}

// WithCodeSize adds code size to hints
func (h *RoutingHints) WithCodeSize(lines int) *RoutingHints {
	h.hints["code_size"] = lines
	return h
}

// WithPriority adds priority to hints
func (h *RoutingHints) WithPriority(priority string) *RoutingHints {
	h.hints["priority"] = priority
	return h
}

// Build returns the hints map
func (h *RoutingHints) Build() map[string]any {
	return h.hints
}

// ConfigReloader handles dynamic configuration reloading
type ConfigReloader struct {
	router      *Router
	configPath  string
	lastModTime time.Time
	mu          sync.RWMutex
	stopCh      chan struct{}
}

// NewConfigReloader creates a new config reloader
func NewConfigReloader(router *Router, configPath string) *ConfigReloader {
	return &ConfigReloader{
		router:     router,
		configPath: configPath,
		stopCh:     make(chan struct{}),
	}
}

// Start begins watching for config changes
func (cr *ConfigReloader) Start(interval time.Duration) {
	if interval == 0 {
		interval = 5 * time.Second
	}

	go cr.watch(interval)
}

// Stop stops watching for config changes
func (cr *ConfigReloader) Stop() {
	close(cr.stopCh)
}

// watch periodically checks for config changes
func (cr *ConfigReloader) watch(interval time.Duration) {
	ticker := time.NewTicker(interval)
	defer ticker.Stop()

	for {
		select {
		case <-ticker.C:
			cr.checkAndReload()
		case <-cr.stopCh:
			return
		}
	}
}

// checkAndReload checks if config has changed and reloads if necessary
func (cr *ConfigReloader) checkAndReload() {
	info, err := os.Stat(cr.configPath)
	if err != nil {
		return
	}

	cr.mu.RLock()
	lastMod := cr.lastModTime
	cr.mu.RUnlock()

	if info.ModTime().After(lastMod) {
		// Config has changed, reload it
		cfg, err := config.Load(filepath.Dir(cr.configPath))
		if err != nil {
			return
		}

		cr.mu.Lock()
		cr.lastModTime = info.ModTime()
		cr.mu.Unlock()

		cr.router.UpdateConfig(&cfg.LLM)
	}
}
