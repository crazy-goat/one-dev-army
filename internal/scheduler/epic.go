package scheduler

import (
	"encoding/json"
	"fmt"
	"os"
	"strings"

	"github.com/crazy-goat/one-dev-army/internal/config"
	"github.com/crazy-goat/one-dev-army/internal/github"
	"github.com/crazy-goat/one-dev-army/internal/opencode"
)

type TaskSpec struct {
	Title              string   `json:"title"`
	TechnicalDesc      string   `json:"technical_description"`
	AcceptanceCriteria []string `json:"acceptance_criteria"`
	Size               string   `json:"size"`
	Dependencies       []int    `json:"dependencies"`
	Labels             []string `json:"labels"`
}

type EpicAnalyzer struct {
	cfg *config.Config
	oc  *opencode.Client
	gh  *github.Client
}

func NewEpicAnalyzer(cfg *config.Config, oc *opencode.Client, gh *github.Client) *EpicAnalyzer {
	return &EpicAnalyzer{cfg: cfg, oc: oc, gh: gh}
}

func (ea *EpicAnalyzer) Analyze(description string) ([]TaskSpec, error) {
	session, err := ea.oc.CreateSession("epic-analysis")
	if err != nil {
		return nil, fmt.Errorf("creating epic analysis session: %w", err)
	}

	prompt := buildEpicPrompt(description)
	msg, err := ea.oc.SendMessage(session.ID, prompt, opencode.ParseModelRef(ea.cfg.EpicAnalysis.LLM), os.Stdout)
	if err != nil {
		return nil, fmt.Errorf("sending epic analysis prompt: %w", err)
	}

	content := extractTextContent(msg)
	tasks, err := parseTaskSpecs(content)
	if err != nil {
		return nil, fmt.Errorf("parsing epic analysis response: %w", err)
	}

	return tasks, nil
}

func (ea *EpicAnalyzer) CreateIssues(tasks []TaskSpec) ([]int, error) {
	issueNumbers := make([]int, 0, len(tasks))

	for _, task := range tasks {
		body := formatTaskBody(task)
		labels := buildTaskLabels(task)

		num, err := ea.gh.CreateIssue(task.Title, body, labels)
		if err != nil {
			return issueNumbers, fmt.Errorf("creating issue %q: %w", task.Title, err)
		}
		issueNumbers = append(issueNumbers, num)
	}

	for i, task := range tasks {
		for _, dep := range task.Dependencies {
			depIdx := dep - 1
			if depIdx < 0 || depIdx >= len(issueNumbers) {
				continue
			}
			depComment := fmt.Sprintf("Depends on #%d", issueNumbers[depIdx])
			if err := ea.gh.AddComment(issueNumbers[i], depComment); err != nil {
				return issueNumbers, fmt.Errorf("adding dependency link on #%d: %w", issueNumbers[i], err)
			}
		}
	}

	return issueNumbers, nil
}

func buildEpicPrompt(description string) string {
	return fmt.Sprintf(`Break down the following epic into concrete implementation tasks.

Epic description:
%s

Respond with a JSON array of tasks. Each task should have:
- "title": concise task title
- "technical_description": detailed technical description of what needs to be done
- "acceptance_criteria": array of acceptance criteria strings
- "size": one of "S", "M", "L", "XL"
- "dependencies": array of 1-based task indices this task depends on (empty if none)
- "labels": array of relevant labels

Do NOT ask any questions - just produce the output.
Respond ONLY with the JSON array, no other text.`, description)
}

func parseTaskSpecs(content string) ([]TaskSpec, error) {
	cleaned := extractJSON(content)
	var tasks []TaskSpec
	if err := json.Unmarshal([]byte(cleaned), &tasks); err != nil {
		return nil, fmt.Errorf("unmarshaling task specs: %w", err)
	}
	return tasks, nil
}

func formatTaskBody(task TaskSpec) string {
	var b strings.Builder

	b.WriteString("## Technical Description\n\n")
	b.WriteString(task.TechnicalDesc)
	b.WriteString("\n\n## Acceptance Criteria\n\n")
	for _, ac := range task.AcceptanceCriteria {
		b.WriteString("- [ ] ")
		b.WriteString(ac)
		b.WriteString("\n")
	}

	if len(task.Dependencies) > 0 {
		b.WriteString("\n## Dependencies\n\n")
		for _, dep := range task.Dependencies {
			b.WriteString(fmt.Sprintf("- Task %d\n", dep))
		}
	}

	return b.String()
}

func buildTaskLabels(task TaskSpec) []string {
	labels := make([]string, 0, len(task.Labels)+1)
	if task.Size != "" {
		labels = append(labels, "size:"+task.Size)
	}
	labels = append(labels, task.Labels...)
	return labels
}

func extractTextContent(msg *opencode.Message) string {
	for _, part := range msg.Parts {
		if part.Type == "text" && part.Text != "" {
			return part.Text
		}
	}
	return ""
}

func extractJSON(content string) string {
	arrStart := strings.Index(content, "[")
	arrEnd := strings.LastIndex(content, "]")

	objStart := strings.Index(content, "{")
	objEnd := strings.LastIndex(content, "}")

	arrValid := arrStart >= 0 && arrEnd > arrStart
	objValid := objStart >= 0 && objEnd > objStart

	if objValid && arrValid {
		if objStart < arrStart {
			return content[objStart : objEnd+1]
		}
		return content[arrStart : arrEnd+1]
	}
	if objValid {
		return content[objStart : objEnd+1]
	}
	if arrValid {
		return content[arrStart : arrEnd+1]
	}

	return content
}
