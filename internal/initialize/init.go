package initialize

import (
	"bufio"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"gopkg.in/yaml.v3"

	"github.com/crazy-goat/one-dev-army/internal/github"
	"github.com/crazy-goat/one-dev-army/internal/opencode"
)

type Init struct {
	ProjectDir string
	OC         *opencode.Client
}

func New(projectDir string, oc *opencode.Client) *Init {
	return &Init{
		ProjectDir: projectDir,
		OC:         oc,
	}
}

func (i *Init) Run() error {
	configPath := filepath.Join(i.ProjectDir, ".oda", "config.yaml")
	if _, err := os.Stat(configPath); err == nil {
		return fmt.Errorf("already initialized: %s exists", configPath)
	}

	empty, err := isRepoEmpty(i.ProjectDir)
	if err != nil {
		return fmt.Errorf("checking repo contents: %w", err)
	}

	if empty {
		fmt.Print("This repo is empty. Describe your project: ")
		scanner := bufio.NewScanner(os.Stdin)
		if scanner.Scan() {
			desc := strings.TrimSpace(scanner.Text())
			if desc != "" {
				fmt.Printf("Project description: %s\n", desc)
			}
		}
	}

	repo, err := github.DetectRepo()
	if err != nil {
		return fmt.Errorf("detecting GitHub repo: %w", err)
	}
	fmt.Printf("Detected repo: %s\n", repo)

	odaDir := filepath.Join(i.ProjectDir, ".oda")
	if err := os.MkdirAll(odaDir, 0o755); err != nil {
		return fmt.Errorf("creating .oda directory: %w", err)
	}

	cfg := defaultConfig(repo)
	data, err := yaml.Marshal(cfg)
	if err != nil {
		return fmt.Errorf("marshaling config: %w", err)
	}

	if err := os.WriteFile(configPath, data, 0o644); err != nil {
		return fmt.Errorf("writing config: %w", err)
	}

	fmt.Println()
	fmt.Println("Initialized ODA project!")
	fmt.Println()
	fmt.Println("Created .oda/config.yaml with defaults:")
	fmt.Printf("  repo:       %s\n", repo)
	fmt.Printf("  workers:    %d\n", cfg.Workers.Count)
	fmt.Printf("  dashboard:  http://localhost:%d\n", cfg.Dashboard.Port)
	fmt.Println()
	fmt.Println("Next steps:")
	fmt.Println("  1. Review .oda/config.yaml and adjust as needed")
	fmt.Println("  2. Ensure opencode is running: opencode serve")
	fmt.Println("  3. Start ODA: oda")

	return nil
}

type configFile struct {
	GitHub    ghSection       `yaml:"github"`
	Dashboard dashSection     `yaml:"dashboard"`
	Workers   workersSection  `yaml:"workers"`
	OpenCode  ocSection       `yaml:"opencode"`
	Pipeline  pipelineSection `yaml:"pipeline"`
	Sprint    sprintSection   `yaml:"sprint"`
}

type ghSection struct {
	Repo        string `yaml:"repo"`
	UseProjects bool   `yaml:"use_projects,omitempty"`
}

type dashSection struct {
	Port int `yaml:"port"`
}

type workersSection struct {
	Count int `yaml:"count"`
}

type ocSection struct {
	URL string `yaml:"url"`
}

type pipelineSection struct {
	MaxRetries int `yaml:"max_retries"`
}

type sprintSection struct {
	TasksPerSprint int `yaml:"tasks_per_sprint"`
}

func defaultConfig(repo string) configFile {
	return configFile{
		GitHub:    ghSection{Repo: repo, UseProjects: false},
		Dashboard: dashSection{Port: 5000},
		Workers:   workersSection{Count: 3},
		OpenCode:  ocSection{URL: "http://localhost:5002"},
		Pipeline: pipelineSection{
			MaxRetries: 5,
		},
		Sprint: sprintSection{TasksPerSprint: 10},
	}
}

func isRepoEmpty(dir string) (bool, error) {
	entries, err := os.ReadDir(dir)
	if err != nil {
		return false, err
	}
	for _, e := range entries {
		if e.Name() != ".git" {
			return false, nil
		}
	}
	return true, nil
}
