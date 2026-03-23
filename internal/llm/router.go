package llm

import (
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"sync"
	"time"

	"github.com/crazy-goat/one-dev-army/internal/config"
)

// Router provides intelligent model selection based on task category and complexity
type Router struct {
	cfg      *config.LLMConfig
	mu       sync.RWMutex
	onReload []func()
}

// NewRouter creates a new LLM router with the given configuration
func NewRouter(cfg *config.LLMConfig) *Router {
	return &Router{
		cfg:      cfg,
		onReload: make([]func(), 0),
	}
}

// SelectModel returns the appropriate model reference for a task
func (r *Router) SelectModel(category config.TaskCategory, complexity config.ComplexityLevel, hints map[string]any) string {
	r.mu.RLock()
	defer r.mu.RUnlock()

	// Check for stage override in hints
	if stage, ok := hints["stage"].(string); ok {
		if r.cfg.ShouldUseStrongModel(complexity, stage) {
			return r.getStrongModel(category)
		}
	}

	// Use complexity-based selection
	modelCfg := r.cfg.GetModelForCategory(category, complexity)
	return modelCfg.ToModelRef()
}

// SelectModelForStage returns the appropriate model for a pipeline stage
func (r *Router) SelectModelForStage(stage string, context string) string {
	r.mu.RLock()
	defer r.mu.RUnlock()

	// Detect complexity from context
	complexity := DetectComplexity(context, r.cfg.RoutingRules.ComplexityThresholds)

	// Determine category based on stage
	category := r.categoryForStage(stage)

	// Check if stage should always use strong model
	if r.cfg.ShouldUseStrongModel(complexity, stage) {
		return r.getStrongModel(category)
	}

	modelCfg := r.cfg.GetModelForCategory(category, complexity)
	return modelCfg.ToModelRef()
}

// categoryForStage maps pipeline stages to task categories
func (r *Router) categoryForStage(stage string) config.TaskCategory {
	switch stage {
	case "analysis", "planning", "plan-review":
		return config.CategoryPlanning
	case "coding", "testing":
		return config.CategoryDevelopment
	case "code-review":
		return config.CategoryDevelopment
	default:
		return config.CategoryDevelopment
	}
}

// getStrongModel returns the strong model reference for a category
func (r *Router) getStrongModel(category config.TaskCategory) string {
	var modelCfg config.ModelConfig

	switch category {
	case config.CategoryDevelopment:
		modelCfg = r.cfg.Development.Strong
	case config.CategoryPlanning:
		modelCfg = r.cfg.Planning.Strong
	case config.CategoryOrchestration:
		modelCfg = r.cfg.Orchestration.Strong
	case config.CategorySetup:
		modelCfg = r.cfg.Setup.Strong
	default:
		modelCfg = r.cfg.Development.Strong
	}

	return modelCfg.ToModelRef()
}

// UpdateConfig updates the router configuration dynamically
func (r *Router) UpdateConfig(cfg *config.LLMConfig) {
	r.mu.Lock()
	defer r.mu.Unlock()

	r.cfg = cfg

	// Notify listeners
	for _, fn := range r.onReload {
		go fn()
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

// ComplexityDetector provides utilities for detecting task complexity
type ComplexityDetector struct {
	thresholds config.ComplexityThresholds
}

// NewComplexityDetector creates a new complexity detector
func NewComplexityDetector(thresholds config.ComplexityThresholds) *ComplexityDetector {
	return &ComplexityDetector{
		thresholds: thresholds,
	}
}

// DetectComplexity analyzes context and returns complexity level
func DetectComplexity(context string, thresholds config.ComplexityThresholds) config.ComplexityLevel {
	if thresholds.CodeSizeThreshold == 0 {
		thresholds = config.DefaultLLMConfig().RoutingRules.ComplexityThresholds
	}

	// Count lines of code/context
	lines := countLines(context)

	// Check for high complexity indicators
	highComplexityScore := 0

	// Large code size
	if lines > thresholds.HighComplexityThreshold {
		highComplexityScore += 3
	} else if lines > thresholds.CodeSizeThreshold {
		highComplexityScore += 1
	}

	// Complex patterns
	highComplexityScore += countComplexityIndicators(context)

	// File references
	fileCount := countFileReferences(context)
	if fileCount > thresholds.FileCountThreshold {
		highComplexityScore += 2
	} else if fileCount > 2 {
		highComplexityScore += 1
	}

	// Determine complexity level
	if highComplexityScore >= 4 {
		return config.ComplexityHigh
	} else if highComplexityScore >= 2 {
		return config.ComplexityMedium
	}

	return config.ComplexityLow
}

// countLines counts non-empty lines in text
func countLines(text string) int {
	if text == "" {
		return 0
	}

	lines := strings.Split(text, "\n")
	count := 0
	for _, line := range lines {
		if strings.TrimSpace(line) != "" {
			count++
		}
	}
	return count
}

// complexityIndicators are patterns that suggest high complexity
var complexityIndicators = []*regexp.Regexp{
	regexp.MustCompile(`(?i)\b(refactor|rearchitecture|redesign)\b`),
	regexp.MustCompile(`(?i)\b(algorithm|complex|optimization)\b`),
	regexp.MustCompile(`(?i)\b(concurrency|parallel|async|goroutine|thread)\b`),
	regexp.MustCompile(`(?i)\b(distributed|microservice|service mesh)\b`),
	regexp.MustCompile(`(?i)\b(critical|security|performance|scalability)\b`),
	regexp.MustCompile(`(?i)\b(database migration|schema change)\b`),
	regexp.MustCompile(`(?i)\b(api design|interface design|protocol)\b`),
}

// countComplexityIndicators counts how many high-complexity patterns are found
func countComplexityIndicators(text string) int {
	score := 0
	for _, re := range complexityIndicators {
		if re.MatchString(text) {
			score++
		}
	}
	return score
}

// fileReferencePattern matches file path references
var fileReferencePattern = regexp.MustCompile(`[a-zA-Z0-9_/-]+\.[a-zA-Z0-9]+`)

// countFileReferences counts unique file references in text
func countFileReferences(text string) int {
	matches := fileReferencePattern.FindAllString(text, -1)
	unique := make(map[string]bool)
	for _, m := range matches {
		unique[m] = true
	}
	return len(unique)
}

// RoutingHints provides a builder for routing hints
type RoutingHints struct {
	hints map[string]any
}

// NewRoutingHints creates a new routing hints builder
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
