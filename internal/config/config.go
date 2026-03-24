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
	URL     string `yaml:"url"`
	WebPort int    `yaml:"web_port,omitempty"`
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

	// Apply default complexity if not set
	if cfg.LLM.DefaultComplexity == "" {
		cfg.LLM.DefaultComplexity = defaults.DefaultComplexity
	}

	// Apply default routing rules if not set
	if cfg.LLM.RoutingRules.ComplexityThresholds.CodeSizeThreshold == 0 {
		cfg.LLM.RoutingRules.ComplexityThresholds = defaults.RoutingRules.ComplexityThresholds
	}
}
