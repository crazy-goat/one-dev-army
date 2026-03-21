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
}

type GitHub struct {
	Repo string `yaml:"repo"`
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

type Stage struct {
	Name           string `yaml:"name"`
	LLM            string `yaml:"llm,omitempty"`
	Lint           bool   `yaml:"lint,omitempty"`
	ManualApproval bool   `yaml:"manual_approval,omitempty"`
}

type Pipeline struct {
	Stages     []Stage `yaml:"stages"`
	MaxRetries int     `yaml:"max_retries"`
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

	return &cfg, nil
}
