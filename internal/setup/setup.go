package setup

import (
	"bufio"
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"strings"

	"github.com/crazy-goat/one-dev-army/internal/config"
	"github.com/crazy-goat/one-dev-army/internal/llm"
	"github.com/crazy-goat/one-dev-army/internal/opencode"
)

var codeBlockRe = regexp.MustCompile("(?s)```[^\n]*\n(.*?)```")

type Setup struct {
	projectDir string
	oc         *opencode.Client
	cfg        *config.Config
	router     *llm.Router
}

func New(projectDir string, oc *opencode.Client, cfg *config.Config, router *llm.Router) *Setup {
	return &Setup{
		projectDir: projectDir,
		oc:         oc,
		cfg:        cfg,
		router:     router,
	}
}

func (s *Setup) CheckAndGenerate() error {
	if err := s.checkAgentsMD(); err != nil {
		return fmt.Errorf("AGENTS.md check: %w", err)
	}

	if err := s.checkGitHubActions(); err != nil {
		return fmt.Errorf("GitHub Actions check: %w", err)
	}

	if err := s.checkGitignore(); err != nil {
		return fmt.Errorf(".gitignore check: %w", err)
	}

	return nil
}

func (s *Setup) checkAgentsMD() error {
	path := filepath.Join(s.projectDir, "AGENTS.md")
	if fileExists(path) {
		return nil
	}

	fmt.Println("AGENTS.md not found. Creating with template...")

	projectName := filepath.Base(s.projectDir)
	language := detectLanguage(s.projectDir)
	content := generateAgentsTemplate(projectName, language)

	if err := os.WriteFile(path, []byte(content), 0o644); err != nil {
		return fmt.Errorf("writing AGENTS.md: %w", err)
	}

	fmt.Println("AGENTS.md created.")
	return nil
}

func detectLanguage(projectDir string) string {
	patterns := map[string]string{
		"go.mod":           "Go",
		"Cargo.toml":       "Rust",
		"package.json":     "JavaScript/TypeScript",
		"requirements.txt": "Python",
		"pyproject.toml":   "Python",
		"composer.json":    "PHP",
		"pom.xml":          "Java",
		"build.gradle":     "Java/Kotlin",
		"Gemfile":          "Ruby",
		"Cargo.lock":       "Rust",
	}

	for file, lang := range patterns {
		if fileExists(filepath.Join(projectDir, file)) {
			return lang
		}
	}

	return "Unknown"
}

func generateAgentsTemplate(projectName, language string) string {
	return fmt.Sprintf(`# %s

## Project Overview

- **Language**: %s
- **Project**: %s

## Development Guidelines

- Follow existing code conventions
- Write tests for new functionality
- Update documentation as needed
`, projectName, language, projectName)
}

func (s *Setup) checkGitHubActions() error {
	workflowDir := filepath.Join(s.projectDir, ".github", "workflows")
	if hasWorkflowFiles(workflowDir) {
		return nil
	}

	fmt.Print("No GitHub Actions workflow found. Generate CI using LLM? [Y/n]: ")
	reader := bufio.NewReader(os.Stdin)
	line, _ := reader.ReadString('\n')
	line = strings.TrimSpace(strings.ToLower(line))
	if line != "" && line != "y" && line != "yes" {
		fmt.Println("Skipping CI workflow generation.")
		return nil
	}

	fmt.Println("Generating CI workflow...")

	content, err := s.generateWithLLM("generate-ci-workflow",
		"Analyze this project and create a GitHub Actions CI workflow file appropriate for the detected stack. "+
			"Return ONLY the YAML content in a single code block, nothing else.")
	if err != nil {
		return fmt.Errorf("generating CI workflow: %w", err)
	}

	if err := os.MkdirAll(workflowDir, 0o755); err != nil {
		return fmt.Errorf("creating workflows directory: %w", err)
	}

	path := filepath.Join(workflowDir, "ci.yml")
	if err := os.WriteFile(path, []byte(content), 0o644); err != nil {
		return fmt.Errorf("writing CI workflow: %w", err)
	}

	fmt.Println("CI workflow created at .github/workflows/ci.yml")
	return nil
}

func (s *Setup) checkGitignore() error {
	path := filepath.Join(s.projectDir, ".gitignore")
	entry := ".oda/artifacts/"

	// Check if .gitignore exists and contains the entry
	if fileExists(path) {
		content, err := os.ReadFile(path)
		if err != nil {
			return fmt.Errorf("reading .gitignore: %w", err)
		}

		// Check if entry already exists (handle both with and without trailing slash)
		contentStr := string(content)
		if strings.Contains(contentStr, entry) || strings.Contains(contentStr, ".oda/artifacts") {
			return nil
		}

		// Entry not found, append it
		fmt.Println("Adding .oda/artifacts/ to .gitignore...")

		// Ensure there's a newline before appending
		if !strings.HasSuffix(contentStr, "\n") {
			content = append(content, '\n')
		}

		content = append(content, []byte(entry+"\n")...)
		if err := os.WriteFile(path, content, 0o644); err != nil {
			return fmt.Errorf("writing .gitignore: %w", err)
		}

		fmt.Println(".gitignore updated.")
		return nil
	}

	// .gitignore doesn't exist, create it
	fmt.Println("Creating .gitignore with .oda/artifacts/ entry...")

	if err := os.WriteFile(path, []byte(entry+"\n"), 0o644); err != nil {
		return fmt.Errorf("creating .gitignore: %w", err)
	}

	fmt.Println(".gitignore created.")
	return nil
}

func (s *Setup) generateWithLLM(title, prompt string) (string, error) {
	session, err := s.oc.CreateSession(title)
	if err != nil {
		return "", fmt.Errorf("creating session: %w", err)
	}

	// Use router to select model for setup category
	llmModel := s.cfg.LLM.Setup.Model
	if s.router != nil {
		llmModel = s.router.SelectModel(config.CategorySetup, config.ComplexityMedium, nil)
	}

	msg, err := s.oc.SendMessage(session.ID, prompt, opencode.ParseModelRef(llmModel), os.Stdout)
	if err != nil {
		return "", fmt.Errorf("sending message: %w", err)
	}

	return extractContent(msg), nil
}

func extractContent(msg *opencode.Message) string {
	for _, part := range msg.Parts {
		if part.Type == "text" && part.Text != "" {
			if matches := codeBlockRe.FindStringSubmatch(part.Text); len(matches) > 1 {
				return matches[1]
			}
			return part.Text
		}
	}
	return ""
}

func fileExists(path string) bool {
	info, err := os.Stat(path)
	return err == nil && !info.IsDir()
}

func hasWorkflowFiles(dir string) bool {
	entries, err := os.ReadDir(dir)
	if err != nil {
		return false
	}

	for _, entry := range entries {
		if entry.IsDir() {
			continue
		}
		name := entry.Name()
		if strings.HasSuffix(name, ".yml") || strings.HasSuffix(name, ".yaml") {
			return true
		}
	}
	return false
}
