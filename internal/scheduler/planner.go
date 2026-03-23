package scheduler

import (
	"encoding/json"
	"fmt"
	"os"
	"strings"

	"github.com/crazy-goat/one-dev-army/internal/config"
	"github.com/crazy-goat/one-dev-army/internal/db"
	"github.com/crazy-goat/one-dev-army/internal/github"
	"github.com/crazy-goat/one-dev-army/internal/llm"
	"github.com/crazy-goat/one-dev-army/internal/opencode"
)

const automatedPipelineNotice = "CRITICAL: You are running in a fully automated pipeline with NO human operator. " +
	"NEVER ask questions, request clarification, or wait for input - nobody will answer and the pipeline will hang forever. " +
	"Make your best judgment and produce output immediately.\n\n"

type Sprint struct {
	ID        int
	Name      string
	Milestone string
	TaskIDs   []int
}

type Planner struct {
	cfg    *config.Config
	oc     *opencode.Client
	gh     *github.Client
	store  *db.Store
	router *llm.Router
}

func NewPlanner(cfg *config.Config, oc *opencode.Client, gh *github.Client, store *db.Store, router *llm.Router) *Planner {
	return &Planner{cfg: cfg, oc: oc, gh: gh, store: store, router: router}
}

func (p *Planner) PlanSprint() (*Sprint, error) {
	issues, err := p.gh.ListOpenIssues()
	if err != nil {
		return nil, fmt.Errorf("fetching open issues: %w", err)
	}

	if len(issues) == 0 {
		return nil, fmt.Errorf("no open issues in backlog")
	}

	session, err := p.oc.CreateSession("sprint-planning")
	if err != nil {
		return nil, fmt.Errorf("creating planning session: %w", err)
	}

	prompt := automatedPipelineNotice + buildSprintPrompt(issues, p.cfg.Sprint.TasksPerSprint)

	// Use router for model selection
	llmModel := p.cfg.Planning.LLM
	if p.router != nil {
		llmModel = p.router.ForPlanningString(config.ComplexityMedium, nil)
	}

	msg, err := p.oc.SendMessage(session.ID, prompt, opencode.ParseModelRef(llmModel), os.Stdout)
	if err != nil {
		return nil, fmt.Errorf("sending planning prompt: %w", err)
	}

	content := extractTextContent(msg)
	selectedIDs, err := parseSprintPlan(content)
	if err != nil {
		return nil, fmt.Errorf("parsing sprint plan: %w", err)
	}

	sprintID, err := p.nextSprintID()
	if err != nil {
		return nil, fmt.Errorf("determining sprint number: %w", err)
	}

	milestoneName := fmt.Sprintf("Sprint %d", sprintID)
	if err := p.gh.CreateMilestone(milestoneName); err != nil {
		return nil, fmt.Errorf("creating milestone: %w", err)
	}

	for _, id := range selectedIDs {
		if err := p.gh.SetMilestone(id, milestoneName); err != nil {
			return nil, fmt.Errorf("assigning issue #%d to milestone: %w", id, err)
		}
	}

	return &Sprint{
		ID:        sprintID,
		Name:      milestoneName,
		Milestone: milestoneName,
		TaskIDs:   selectedIDs,
	}, nil
}

func (p *Planner) AnalyzeInsights(sprintID int) error {
	milestoneName := fmt.Sprintf("Sprint %d", sprintID)
	issues, err := p.gh.ListIssues(milestoneName)
	if err != nil {
		return fmt.Errorf("listing sprint issues: %w", err)
	}

	var allInsights []string
	var metaIssueNum int

	for _, issue := range issues {
		if strings.HasPrefix(issue.Title, "Sprint "+fmt.Sprintf("%d", sprintID)) {
			metaIssueNum = issue.Number
		}

		comments, err := p.gh.ListComments(issue.Number)
		if err != nil {
			return fmt.Errorf("listing comments for #%d: %w", issue.Number, err)
		}

		for _, comment := range comments {
			if strings.Contains(strings.ToLower(comment.Body), "insight") ||
				strings.Contains(strings.ToLower(comment.Body), "observation") ||
				strings.Contains(strings.ToLower(comment.Body), "idea") {
				allInsights = append(allInsights, fmt.Sprintf("Issue #%d (%s): %s", issue.Number, issue.Title, comment.Body))
			}
		}
	}

	if len(allInsights) == 0 {
		return nil
	}

	session, err := p.oc.CreateSession("insight-analysis")
	if err != nil {
		return fmt.Errorf("creating insight session: %w", err)
	}

	prompt := buildInsightPrompt(allInsights)

	// Use router for model selection
	llmModel := p.cfg.Planning.LLM
	if p.router != nil {
		llmModel = p.router.ForPlanningString(config.ComplexityMedium, nil)
	}

	msg, err := p.oc.SendMessage(session.ID, prompt, opencode.ParseModelRef(llmModel), os.Stdout)
	if err != nil {
		return fmt.Errorf("sending insight prompt: %w", err)
	}

	content := extractTextContent(msg)
	analysis, err := parseInsightAnalysis(content)
	if err != nil {
		return fmt.Errorf("parsing insight analysis: %w", err)
	}

	for _, idea := range analysis.ConcreteIdeas {
		_, err := p.gh.CreateIssue(idea.Title, idea.Description, []string{"insight"})
		if err != nil {
			return fmt.Errorf("creating insight issue %q: %w", idea.Title, err)
		}
	}

	if metaIssueNum > 0 && len(analysis.Observations) > 0 {
		var b strings.Builder
		b.WriteString("## Sprint Insights Analysis\n\n")
		for _, obs := range analysis.Observations {
			b.WriteString("- ")
			b.WriteString(obs)
			b.WriteString("\n")
		}
		if err := p.gh.AddComment(metaIssueNum, b.String()); err != nil {
			return fmt.Errorf("adding observations comment: %w", err)
		}
	}

	return nil
}

func (p *Planner) nextSprintID() (int, error) {
	milestones, err := p.gh.ListMilestones()
	if err != nil {
		return 1, nil
	}

	maxID := 0
	for _, m := range milestones {
		var id int
		if _, err := fmt.Sscanf(m.Title, "Sprint %d", &id); err == nil && id > maxID {
			maxID = id
		}
	}
	return maxID + 1, nil
}

type sprintPlanResponse struct {
	TaskIDs []int `json:"task_ids"`
}

func buildSprintPrompt(issues []github.Issue, maxTasks int) string {
	var b strings.Builder
	b.WriteString("You are a sprint planner. Select tasks from the backlog for the next sprint.\n\n")
	b.WriteString("## Backlog\n\n")

	for _, issue := range issues {
		sizeLabel := ""
		for _, l := range issue.Labels {
			if strings.HasPrefix(l.Name, "size:") {
				sizeLabel = l.Name
				break
			}
		}
		b.WriteString(fmt.Sprintf("- #%d: %s [%s]\n", issue.Number, issue.Title, sizeLabel))
	}

	if maxTasks > 0 {
		b.WriteString(fmt.Sprintf("\nSelect up to %d tasks for this sprint.\n", maxTasks))
	}

	b.WriteString("\nConsider task dependencies and sizes when selecting.\n")
	b.WriteString("Do NOT ask any questions - just produce the output.\n")
	b.WriteString("Respond with JSON: {\"task_ids\": [1, 2, 3]}\n")
	b.WriteString("Respond ONLY with the JSON object, no other text.")

	return b.String()
}

func parseSprintPlan(content string) ([]int, error) {
	cleaned := extractJSON(content)
	var plan sprintPlanResponse
	if err := json.Unmarshal([]byte(cleaned), &plan); err != nil {
		return nil, fmt.Errorf("unmarshaling sprint plan: %w", err)
	}
	if len(plan.TaskIDs) == 0 {
		return nil, fmt.Errorf("sprint plan contains no tasks")
	}
	return plan.TaskIDs, nil
}

type insightIdea struct {
	Title       string `json:"title"`
	Description string `json:"description"`
}

type insightAnalysis struct {
	ConcreteIdeas []insightIdea `json:"concrete_ideas"`
	Observations  []string      `json:"observations"`
}

func buildInsightPrompt(insights []string) string {
	var b strings.Builder
	b.WriteString("Analyze the following insights collected during a sprint.\n\n")
	b.WriteString("Categorize each into either:\n")
	b.WriteString("1. Concrete ideas that should become new GitHub issues\n")
	b.WriteString("2. General observations worth noting\n\n")
	b.WriteString("## Insights\n\n")
	for _, insight := range insights {
		b.WriteString("- ")
		b.WriteString(insight)
		b.WriteString("\n")
	}
	b.WriteString("\nDo NOT ask any questions - just produce the output.\n")
	b.WriteString("Respond with JSON:\n")
	b.WriteString(`{"concrete_ideas": [{"title": "...", "description": "..."}], "observations": ["..."]}`)
	b.WriteString("\nRespond ONLY with the JSON object, no other text.")
	return b.String()
}

func parseInsightAnalysis(content string) (*insightAnalysis, error) {
	cleaned := extractJSON(content)
	var analysis insightAnalysis
	if err := json.Unmarshal([]byte(cleaned), &analysis); err != nil {
		return nil, fmt.Errorf("unmarshaling insight analysis: %w", err)
	}
	return &analysis, nil
}
