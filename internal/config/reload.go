package config

import (
	"fmt"
	"os"
	"path/filepath"
	"sync"
	"time"

	"gopkg.in/yaml.v3"
)

// ReloadManager handles dynamic configuration reloading
type ReloadManager struct {
	rootDir       string
	configPath    string
	currentConfig *Config
	mu            sync.RWMutex
	lastModified  time.Time
	watchers      []ConfigWatcher
	stopCh        chan struct{}
	wg            sync.WaitGroup
}

// ConfigWatcher is called when configuration changes
type ConfigWatcher func(oldCfg, newCfg *Config)

// NewReloadManager creates a new configuration reload manager
func NewReloadManager(rootDir string) *ReloadManager {
	configPath := filepath.Join(rootDir, ".oda", "config.yaml")
	return &ReloadManager{
		rootDir:    rootDir,
		configPath: configPath,
		watchers:   make([]ConfigWatcher, 0),
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
	// Get initial modification time
	if info, err := os.Stat(rm.configPath); err == nil {
		rm.lastModified = info.ModTime()
	}
	rm.mu.Unlock()

	return cfg, nil
}

// GetConfig returns the current configuration (thread-safe)
func (rm *ReloadManager) GetConfig() *Config {
	rm.mu.RLock()
	defer rm.mu.RUnlock()

	if rm.currentConfig == nil {
		return nil
	}

	// Return a copy to prevent external modification
	cfgCopy := *rm.currentConfig
	return &cfgCopy
}

// GetLLMConfig returns the current LLM configuration (thread-safe)
func (rm *ReloadManager) GetLLMConfig() *LLMConfig {
	rm.mu.RLock()
	defer rm.mu.RUnlock()

	if rm.currentConfig == nil {
		return nil
	}

	// Return a copy
	llmCopy := rm.currentConfig.LLM
	return &llmCopy
}

// Watch registers a callback to be invoked when config changes
func (rm *ReloadManager) Watch(watcher ConfigWatcher) {
	rm.mu.Lock()
	defer rm.mu.Unlock()
	rm.watchers = append(rm.watchers, watcher)
}

// Unwatch removes a watcher (compares function pointers)
func (rm *ReloadManager) Unwatch(watcher ConfigWatcher) {
	rm.mu.Lock()
	defer rm.mu.Unlock()

	for i, w := range rm.watchers {
		// Compare function pointers using fmt.Sprintf
		if fmt.Sprintf("%p", w) == fmt.Sprintf("%p", watcher) {
			rm.watchers = append(rm.watchers[:i], rm.watchers[i+1:]...)
			break
		}
	}
}

// notifyWatchers notifies all registered watchers of a config change
func (rm *ReloadManager) notifyWatchers(oldCfg, newCfg *Config) {
	rm.mu.RLock()
	watchers := make([]ConfigWatcher, len(rm.watchers))
	copy(watchers, rm.watchers)
	rm.mu.RUnlock()

	for _, watcher := range watchers {
		// Run in goroutine to prevent blocking
		go watcher(oldCfg, newCfg)
	}
}

// CheckReload checks if the configuration file has changed and reloads if necessary
func (rm *ReloadManager) CheckReload() (bool, error) {
	info, err := os.Stat(rm.configPath)
	if err != nil {
		return false, fmt.Errorf("stat config file: %w", err)
	}

	rm.mu.RLock()
	lastMod := rm.lastModified
	rm.mu.RUnlock()

	// Check if file has been modified
	if !info.ModTime().After(lastMod) {
		return false, nil
	}

	// File changed, reload configuration
	newCfg, err := Load(rm.rootDir)
	if err != nil {
		return false, fmt.Errorf("reloading config: %w", err)
	}

	rm.mu.Lock()
	oldCfg := rm.currentConfig
	rm.currentConfig = newCfg
	rm.lastModified = info.ModTime()
	rm.mu.Unlock()

	// Notify watchers
	if oldCfg != nil {
		rm.notifyWatchers(oldCfg, newCfg)
	}

	return true, nil
}

// Start begins watching for configuration changes
func (rm *ReloadManager) Start(interval time.Duration) {
	rm.wg.Add(1)
	go rm.watchLoop(interval)
}

// Stop stops the configuration watcher
func (rm *ReloadManager) Stop() {
	close(rm.stopCh)
	rm.wg.Wait()
}

// watchLoop periodically checks for configuration changes
func (rm *ReloadManager) watchLoop(interval time.Duration) {
	defer rm.wg.Done()

	ticker := time.NewTicker(interval)
	defer ticker.Stop()

	for {
		select {
		case <-rm.stopCh:
			return
		case <-ticker.C:
			if reloaded, err := rm.CheckReload(); err != nil {
				// Log error but continue watching
				fmt.Fprintf(os.Stderr, "[Config] Reload error: %v\n", err)
			} else if reloaded {
				fmt.Println("[Config] Configuration reloaded successfully")
			}
		}
	}
}

// ForceReload forces an immediate configuration reload
func (rm *ReloadManager) ForceReload() error {
	reloaded, err := rm.CheckReload()
	if err != nil {
		return err
	}
	if !reloaded {
		return fmt.Errorf("configuration unchanged")
	}
	return nil
}

// SaveConfig saves a new configuration to disk and triggers reload
func (rm *ReloadManager) SaveConfig(cfg *Config) error {
	// Marshal to YAML
	data, err := yaml.Marshal(cfg)
	if err != nil {
		return fmt.Errorf("marshaling config: %w", err)
	}

	// Write to file
	if err := os.WriteFile(rm.configPath, data, 0644); err != nil {
		return fmt.Errorf("writing config file: %w", err)
	}

	// Trigger reload
	_, err = rm.CheckReload()
	return err
}

// LastModified returns the last modification time of the config file
func (rm *ReloadManager) LastModified() time.Time {
	rm.mu.RLock()
	defer rm.mu.RUnlock()
	return rm.lastModified
}

// ConfigAccessor provides thread-safe access to configuration
type ConfigAccessor struct {
	manager *ReloadManager
}

// NewConfigAccessor creates a new config accessor
func NewConfigAccessor(manager *ReloadManager) *ConfigAccessor {
	return &ConfigAccessor{manager: manager}
}

// Get returns the current configuration
func (ca *ConfigAccessor) Get() *Config {
	return ca.manager.GetConfig()
}

// GetLLM returns the current LLM configuration
func (ca *ConfigAccessor) GetLLM() *LLMConfig {
	return ca.manager.GetLLMConfig()
}

// SimpleReload is a convenience function for simple reload scenarios
func SimpleReload(rootDir string) (*Config, error) {
	return Load(rootDir)
}
