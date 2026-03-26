package config

import (
	"fmt"
	"os"
	"path/filepath"

	"gopkg.in/yaml.v3"
)

type Config struct {
	GitHub         GitHub    `yaml:"github"`
	Dashboard      Dashboard `yaml:"dashboard"`
	Workers        Workers   `yaml:"workers"`
	OpenCode       OpenCode  `yaml:"opencode"`
	Tools          Tools     `yaml:"tools"`
	Pipeline       Pipeline  `yaml:"pipeline"`
	Sprint         Sprint    `yaml:"sprint"`
	LLM            LLMConfig `yaml:"llm"`
	YoloMode       bool      `yaml:"yolo_mode"`
	UseNewFrontend bool      `yaml:"use_new_frontend"`
}

type GitHub struct {
	Repo        string `yaml:"repo"`
	UseProjects bool   `yaml:"use_projects,omitempty"`
}

type Dashboard struct {
	Port int `yaml:"port"`
}

type Workers struct {
	Count int `yaml:"count"`
}

type OpenCode struct {
	URL     string `yaml:"url"`
	WebPort int    `yaml:"web_port,omitempty"`
}

type Tools struct {
	LintCmd string `yaml:"lint_cmd"`
	TestCmd string `yaml:"test_cmd"`
	E2ECmd  string `yaml:"e2e_cmd"`
}

type Pipeline struct {
	MaxRetries    int `yaml:"max_retries"`
	CheckInterval int `yaml:"check_interval"`
	CheckTimeout  int `yaml:"check_timeout"`
}

type Sprint struct {
	TasksPerSprint int  `yaml:"tasks_per_sprint"`
	AutoStart      bool `yaml:"auto_start"`
}

func Load(rootDir string, _ ...string) (*Config, error) {
	path := filepath.Join(rootDir, ".oda", "config.yaml")

	data, err := os.ReadFile(path)
	if err != nil {
		return nil, fmt.Errorf("reading config %s: %w", path, err)
	}

	var cfg Config
	if err := yaml.Unmarshal(data, &cfg); err != nil {
		return nil, fmt.Errorf("parsing config %s: %w", path, err)
	}

	// Migrate from legacy top-level planning/epic_analysis fields if present
	cfg.migrateFromLegacyFields(data)

	// Normalize model formats to ensure provider/model format
	cfg.LLM.NormalizeAllModels()

	// Apply default LLM config if not fully specified
	cfg.applyLLMDefaults()

	// Apply pipeline defaults
	cfg.applyPipelineDefaults()

	return &cfg, nil
}

// migrateFromLegacyFields migrates old top-level planning.llm and epic_analysis.llm to new structure
func (cfg *Config) migrateFromLegacyFields(data []byte) {
	// Check if old fields exist in raw YAML
	var raw map[string]any
	if err := yaml.Unmarshal(data, &raw); err != nil {
		return
	}

	// Migrate planning.llm -> llm.planning.model
	if planning, ok := raw["planning"].(map[string]any); ok {
		if llm, ok := planning["llm"].(string); ok && llm != "" {
			if cfg.LLM.Planning.Model == "" {
				cfg.LLM.Planning.Model = llm
			}
		}
	}

	// Migrate epic_analysis.llm -> llm.planning.model (epic analysis uses planning model)
	if epicAnalysis, ok := raw["epic_analysis"].(map[string]any); ok {
		if llm, ok := epicAnalysis["llm"].(string); ok && llm != "" {
			if cfg.LLM.Planning.Model == "" {
				cfg.LLM.Planning.Model = llm
			}
		}
	}
}

// applyLLMDefaults fills in missing LLM configuration with defaults
func (cfg *Config) applyLLMDefaults() {
	defaults := DefaultLLMConfig()

	// If Setup model is empty, use defaults
	if cfg.LLM.Setup.Model == "" {
		cfg.LLM.Setup.Model = defaults.Setup.Model
	}

	// If Planning model is empty, use defaults
	if cfg.LLM.Planning.Model == "" {
		cfg.LLM.Planning.Model = defaults.Planning.Model
	}

	// If Orchestration model is empty, use defaults
	if cfg.LLM.Orchestration.Model == "" {
		cfg.LLM.Orchestration.Model = defaults.Orchestration.Model
	}

	// If Code model is empty, use defaults
	if cfg.LLM.Code.Model == "" {
		cfg.LLM.Code.Model = defaults.Code.Model
	}

	// If CodeHeavy model is empty, use defaults
	if cfg.LLM.CodeHeavy.Model == "" {
		cfg.LLM.CodeHeavy.Model = defaults.CodeHeavy.Model
	}
}

// applyPipelineDefaults fills in missing Pipeline configuration with defaults
func (cfg *Config) applyPipelineDefaults() {
	if cfg.Pipeline.CheckInterval == 0 {
		cfg.Pipeline.CheckInterval = 10
	}
	if cfg.Pipeline.CheckTimeout == 0 {
		cfg.Pipeline.CheckTimeout = 1800
	}
}
