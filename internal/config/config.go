package config

import (
	"fmt"
	"os"
	"path/filepath"

	"gopkg.in/yaml.v3"
)

type Config struct {
	GitHub       GitHub       `yaml:"github"`
	Dashboard    Dashboard    `yaml:"dashboard"`
	Workers      Workers      `yaml:"workers"`
	OpenCode     OpenCode     `yaml:"opencode"`
	Tools        Tools        `yaml:"tools"`
	Pipeline     Pipeline     `yaml:"pipeline"`
	Planning     Planning     `yaml:"planning"`
	EpicAnalysis EpicAnalysis `yaml:"epic_analysis"`
	Sprint       Sprint       `yaml:"sprint"`
	LLM          LLMConfig    `yaml:"llm"`
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
	URL string `yaml:"url"`
}

type Tools struct {
	LintCmd string `yaml:"lint_cmd"`
	TestCmd string `yaml:"test_cmd"`
	E2ECmd  string `yaml:"e2e_cmd"`
}

type Pipeline struct {
	MaxRetries int `yaml:"max_retries"`
}

type Planning struct {
	LLM string `yaml:"llm"`
}

type EpicAnalysis struct {
	LLM string `yaml:"llm"`
}

type Sprint struct {
	TasksPerSprint int `yaml:"tasks_per_sprint"`
}

func Load(rootDir string) (*Config, error) {
	path := filepath.Join(rootDir, ".oda", "config.yaml")

	data, err := os.ReadFile(path)
	if err != nil {
		return nil, fmt.Errorf("reading config %s: %w", path, err)
	}

	var cfg Config
	if err := yaml.Unmarshal(data, &cfg); err != nil {
		return nil, fmt.Errorf("parsing config %s: %w", path, err)
	}

	// Apply default LLM config if not fully specified
	cfg.applyLLMDefaults()

	return &cfg, nil
}

// applyLLMDefaults fills in missing LLM configuration with defaults
func (cfg *Config) applyLLMDefaults() {
	defaults := DefaultLLMConfig()

	// If Development strong model is empty, use defaults
	if cfg.LLM.Development.Strong.Model == "" {
		cfg.LLM.Development.Strong = defaults.Development.Strong
	}
	if cfg.LLM.Development.Weak.Model == "" {
		cfg.LLM.Development.Weak = defaults.Development.Weak
	}

	// If Planning strong model is empty, use defaults
	if cfg.LLM.Planning.Strong.Model == "" {
		cfg.LLM.Planning.Strong = defaults.Planning.Strong
	}
	if cfg.LLM.Planning.Weak.Model == "" {
		cfg.LLM.Planning.Weak = defaults.Planning.Weak
	}

	// If Orchestration strong model is empty, use defaults
	if cfg.LLM.Orchestration.Strong.Model == "" {
		cfg.LLM.Orchestration.Strong = defaults.Orchestration.Strong
	}
	if cfg.LLM.Orchestration.Weak.Model == "" {
		cfg.LLM.Orchestration.Weak = defaults.Orchestration.Weak
	}

	// If Setup strong model is empty, use defaults
	if cfg.LLM.Setup.Strong.Model == "" {
		cfg.LLM.Setup.Strong = defaults.Setup.Strong
	}
	if cfg.LLM.Setup.Weak.Model == "" {
		cfg.LLM.Setup.Weak = defaults.Setup.Weak
	}

	// Apply default complexity if not set
	if cfg.LLM.DefaultComplexity == "" {
		cfg.LLM.DefaultComplexity = defaults.DefaultComplexity
	}

	// Apply default routing rules if not set
	if cfg.LLM.RoutingRules.ComplexityThresholds.CodeSizeThreshold == 0 {
		cfg.LLM.RoutingRules.ComplexityThresholds = defaults.RoutingRules.ComplexityThresholds
	}
}
