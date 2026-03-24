package config

import (
	"fmt"
	"os"
	"path/filepath"
	"sync"
	"time"

	"gopkg.in/yaml.v3"
)

// ReloadManager handles dynamic configuration reloading with hot-swap capability
type ReloadManager struct {
	configPath    string
	rootDir       string
	currentConfig *Config
	onReload      []func(*Config)
	stopCh        chan struct{}
	mu            sync.RWMutex
	lastModTime   time.Time
	watcher       *FileWatcher
}

// NewReloadManager creates a new reload manager
func NewReloadManager(rootDir string) *ReloadManager {
	configPath := filepath.Join(rootDir, ".oda", "config.yaml")
	return &ReloadManager{
		configPath: configPath,
		rootDir:    rootDir,
		onReload:   make([]func(*Config), 0),
		stopCh:     make(chan struct{}),
	}
}

// LoadInitial loads the initial configuration
func (rm *ReloadManager) LoadInitial() (*Config, error) {
	cfg, err := Load(rm.rootDir)
	if err != nil {
		return nil, fmt.Errorf("loading initial config: %w", err)
	}

	rm.mu.Lock()
	rm.currentConfig = cfg
	info, err := os.Stat(rm.configPath)
	if err == nil {
		rm.lastModTime = info.ModTime()
	}
	rm.mu.Unlock()

	return cfg, nil
}

// Start begins watching for configuration changes
func (rm *ReloadManager) Start(interval time.Duration) {
	if interval == 0 {
		interval = 5 * time.Second
	}

	rm.watcher = NewFileWatcher(rm.configPath, interval)
	rm.watcher.OnChange(func() {
		rm.reload()
	})

	go rm.watcher.Start()
}

// Stop stops watching for configuration changes
func (rm *ReloadManager) Stop() {
	if rm.watcher != nil {
		rm.watcher.Stop()
	}
	close(rm.stopCh)
}

// reload reloads the configuration and notifies listeners
func (rm *ReloadManager) reload() {
	cfg, err := Load(rm.rootDir)
	if err != nil {
		return
	}

	rm.mu.Lock()
	rm.currentConfig = cfg
	info, err := os.Stat(rm.configPath)
	if err == nil {
		rm.lastModTime = info.ModTime()
	}
	onReload := make([]func(*Config), len(rm.onReload))
	copy(onReload, rm.onReload)
	rm.mu.Unlock()

	// Notify listeners
	for _, fn := range onReload {
		go fn(cfg)
	}
}

// OnReload registers a callback to be called when config is reloaded
func (rm *ReloadManager) OnReload(fn func(*Config)) {
	rm.mu.Lock()
	defer rm.mu.Unlock()

	rm.onReload = append(rm.onReload, fn)
}

// GetConfig returns the current configuration
func (rm *ReloadManager) GetConfig() *Config {
	rm.mu.RLock()
	defer rm.mu.RUnlock()

	return rm.currentConfig
}

// GetLastModTime returns the last modification time
func (rm *ReloadManager) GetLastModTime() time.Time {
	rm.mu.RLock()
	defer rm.mu.RUnlock()

	return rm.lastModTime
}

// FileWatcher watches a file for changes
type FileWatcher struct {
	path     string
	interval time.Duration
	onChange func()
	stopCh   chan struct{}
	lastMod  time.Time
	mu       sync.Mutex
}

// NewFileWatcher creates a new file watcher
func NewFileWatcher(path string, interval time.Duration) *FileWatcher {
	return &FileWatcher{
		path:     path,
		interval: interval,
		stopCh:   make(chan struct{}),
	}
}

// OnChange sets the callback for file changes
func (fw *FileWatcher) OnChange(fn func()) {
	fw.mu.Lock()
	defer fw.mu.Unlock()

	fw.onChange = fn
}

// Start begins watching the file
func (fw *FileWatcher) Start() {
	ticker := time.NewTicker(fw.interval)
	defer ticker.Stop()

	// Get initial modification time
	info, err := os.Stat(fw.path)
	if err == nil {
		fw.mu.Lock()
		fw.lastMod = info.ModTime()
		fw.mu.Unlock()
	}

	for {
		select {
		case <-ticker.C:
			fw.check()
		case <-fw.stopCh:
			return
		}
	}
}

// Stop stops watching the file
func (fw *FileWatcher) Stop() {
	close(fw.stopCh)
}

// check checks if the file has been modified
func (fw *FileWatcher) check() {
	info, err := os.Stat(fw.path)
	if err != nil {
		return
	}

	fw.mu.Lock()
	lastMod := fw.lastMod
	fw.mu.Unlock()

	if info.ModTime().After(lastMod) {
		fw.mu.Lock()
		fw.lastMod = info.ModTime()
		onChange := fw.onChange
		fw.mu.Unlock()

		if onChange != nil {
			onChange()
		}
	}
}

// SaveConfig saves a configuration to the config file
func SaveConfig(rootDir string, cfg *Config) error {
	path := filepath.Join(rootDir, ".oda", "config.yaml")

	// Normalize all model formats before saving to ensure provider/model format
	cfg.LLM.NormalizeAllModels()

	data, err := yaml.Marshal(cfg)
	if err != nil {
		return fmt.Errorf("marshaling config: %w", err)
	}

	if err := os.WriteFile(path, data, 0o644); err != nil {
		return fmt.Errorf("writing config: %w", err)
	}

	return nil
}

// ConfigPropagator propagates config changes to workers
type ConfigPropagator struct {
	mu       sync.RWMutex
	workers  []ConfigAwareWorker
	interval time.Duration
	stopCh   chan struct{}
}

// ConfigAwareWorker is an interface for workers that can receive config updates
type ConfigAwareWorker interface {
	UpdateConfig(cfg *Config)
}

// NewConfigPropagator creates a new config propagator
func NewConfigPropagator(interval time.Duration) *ConfigPropagator {
	if interval == 0 {
		interval = 5 * time.Second
	}

	return &ConfigPropagator{
		workers:  make([]ConfigAwareWorker, 0),
		interval: interval,
		stopCh:   make(chan struct{}),
	}
}

// RegisterWorker registers a worker to receive config updates
func (cp *ConfigPropagator) RegisterWorker(worker ConfigAwareWorker) {
	cp.mu.Lock()
	defer cp.mu.Unlock()

	cp.workers = append(cp.workers, worker)
}

// UnregisterWorker removes a worker from receiving config updates
func (cp *ConfigPropagator) UnregisterWorker(worker ConfigAwareWorker) {
	cp.mu.Lock()
	defer cp.mu.Unlock()

	for i, w := range cp.workers {
		if w == worker {
			cp.workers = append(cp.workers[:i], cp.workers[i+1:]...)
			break
		}
	}
}

// Propagate sends a config update to all registered workers
func (cp *ConfigPropagator) Propagate(cfg *Config) {
	cp.mu.RLock()
	workers := make([]ConfigAwareWorker, len(cp.workers))
	copy(workers, cp.workers)
	cp.mu.RUnlock()

	for _, worker := range workers {
		go worker.UpdateConfig(cfg)
	}
}

// Start begins periodic config propagation
// If interval is 0, it only listens for stop signal (use with ReloadManager)
func (cp *ConfigPropagator) Start(cfgProvider func() *Config) {
	if cp.interval == 0 {
		// No polling - just wait for stop signal (used with ReloadManager)
		<-cp.stopCh
		return
	}

	ticker := time.NewTicker(cp.interval)
	defer ticker.Stop()

	for {
		select {
		case <-ticker.C:
			if cfg := cfgProvider(); cfg != nil {
				cp.Propagate(cfg)
			}
		case <-cp.stopCh:
			return
		}
	}
}

// Stop stops the propagator
func (cp *ConfigPropagator) Stop() {
	close(cp.stopCh)
}
