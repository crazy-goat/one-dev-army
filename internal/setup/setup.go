package setup

import (
	"bufio"
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"strings"

	"github.com/crazy-goat/one-dev-army/internal/config"
	"github.com/crazy-goat/one-dev-army/internal/opencode"
)

var codeBlockRe = regexp.MustCompile("(?s)```[^\n]*\n(.*?)```")

type Setup struct {
	projectDir string
	oc         *opencode.Client
	cfg        *config.Config
	reader     *bufio.Reader
}

func New(projectDir string, oc *opencode.Client, cfg *config.Config) *Setup {
	return &Setup{
		projectDir: projectDir,
		oc:         oc,
		cfg:        cfg,
		reader:     bufio.NewReader(os.Stdin),
	}
}

func (s *Setup) CheckAndGenerate() error {
	if err := s.checkAgentsMD(); err != nil {
		return fmt.Errorf("AGENTS.md check: %w", err)
	}

	if err := s.checkGitHubActions(); err != nil {
		return fmt.Errorf("GitHub Actions check: %w", err)
	}

	return nil
}

func (s *Setup) checkAgentsMD() error {
	path := filepath.Join(s.projectDir, "AGENTS.md")
	if fileExists(path) {
		return nil
	}

	fmt.Print("AGENTS.md not found. Generate using LLM? [Y/n]: ")
	if !promptYesNo(s.reader) {
		fmt.Println("Skipping AGENTS.md generation. You can generate it later.")
		return nil
	}

	fmt.Println("Generating AGENTS.md...")

	prompt := "Analyze this project and create an AGENTS.md file. " +
		"Describe the project structure, build commands, test commands, and coding conventions. " +
		"Return only the file content in a single markdown code block."

	content, err := s.generateWithLLM("generate-agents-md", prompt)
	if err != nil {
		return fmt.Errorf("generating AGENTS.md: %w", err)
	}

	if err := os.WriteFile(path, []byte(content), 0o644); err != nil {
		return fmt.Errorf("writing AGENTS.md: %w", err)
	}

	fmt.Println("AGENTS.md created.")
	return nil
}

func (s *Setup) checkGitHubActions() error {
	workflowDir := filepath.Join(s.projectDir, ".github", "workflows")
	if hasWorkflowFiles(workflowDir) {
		return nil
	}

	fmt.Print("No GitHub Actions workflow found. Generate CI using LLM? [Y/n]: ")
	if !promptYesNo(s.reader) {
		fmt.Println("Skipping CI workflow generation. You can generate it later.")
		return nil
	}

	fmt.Println("Generating CI workflow...")

	prompt := "Analyze this project and create a GitHub Actions CI workflow file appropriate for the detected stack. " +
		"Return only the YAML content in a single code block."

	content, err := s.generateWithLLM("generate-ci-workflow", prompt)
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

func (s *Setup) generateWithLLM(title, prompt string) (string, error) {
	session, err := s.oc.CreateSession(title)
	if err != nil {
		return "", fmt.Errorf("creating session: %w", err)
	}

	msg, err := s.oc.SendMessage(session.ID, prompt, s.cfg.Planning.LLM)
	if err != nil {
		return "", fmt.Errorf("sending message: %w", err)
	}

	return extractContent(msg), nil
}

func extractContent(msg *opencode.Message) string {
	for _, part := range msg.Parts {
		if part.Type == "text" && part.Content != "" {
			if matches := codeBlockRe.FindStringSubmatch(part.Content); len(matches) > 1 {
				return matches[1]
			}
			return part.Content
		}
	}
	return ""
}

func promptYesNo(reader *bufio.Reader) bool {
	line, _ := reader.ReadString('\n')
	line = strings.TrimSpace(line)

	switch strings.ToLower(line) {
	case "", "y", "yes":
		return true
	default:
		return false
	}
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
