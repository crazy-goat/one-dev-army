package scheduler

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"sync"
	"time"

	"github.com/crazy-goat/one-dev-army/internal/config"
	"github.com/crazy-goat/one-dev-army/internal/github"
	"github.com/crazy-goat/one-dev-army/internal/llm"
	"github.com/crazy-goat/one-dev-army/internal/opencode"
	"github.com/google/uuid"
)

// IssueCandidate represents an issue for sprint planning
type IssueCandidate struct {
	Number     int      `json:"number"`
	Title      string   `json:"title"`
	Labels     []string `json:"labels"`
	Complexity *int     `json:"complexity,omitempty"`
}

// LinkedIssue represents a linked issue relationship
type LinkedIssue struct {
	Number       int    `json:"number"`
	Relationship string `json:"relationship"`
}

// Branch represents a group of related issues
type Branch struct {
	ID              string `json:"id"`
	Name            string `json:"name"`
	RootIssue       int    `json:"root_issue"`
	Issues          []int  `json:"issues"`
	TotalComplexity int    `json:"total_complexity"`
}

// ProposedIssue extends IssueCandidate with AI reasoning
type ProposedIssue struct {
	IssueCandidate
	Reason       string `json:"reason"`
	Dependencies []int  `json:"dependencies"`
	Branch       string `json:"branch"`
}

// ProposalJob tracks the status of an AI proposal generation
type ProposalJob struct {
	ID        string          `json:"id"`
	Status    string          `json:"status"` // pending, processing, completed, failed
	Proposal  []ProposedIssue `json:"proposal,omitempty"`
	Branches  []Branch        `json:"branches,omitempty"`
	Error     string          `json:"error,omitempty"`
	CreatedAt time.Time       `json:"created_at"`
}

// PlanProposer generates AI-powered sprint proposals
type PlanProposer struct {
	cfg          *config.Config
	router       *llm.Router
	oc           *opencode.Client
	githubClient *github.Client
	jobs         map[string]*ProposalJob
	mu           sync.RWMutex
}

// NewPlanProposer creates a new plan proposer
func NewPlanProposer(cfg *config.Config, router *llm.Router, oc *opencode.Client, ghClient *github.Client) *PlanProposer {
	return &PlanProposer{
		cfg:          cfg,
		router:       router,
		oc:           oc,
		githubClient: ghClient,
		jobs:         make(map[string]*ProposalJob),
	}
}

// CreateProposal starts an async proposal generation job
func (p *PlanProposer) CreateProposal(candidates []IssueCandidate, targetCount int, lastTag string) (string, error) {
	jobID := uuid.New().String()

	job := &ProposalJob{
		ID:        jobID,
		Status:    "pending",
		CreatedAt: time.Now(),
	}

	p.mu.Lock()
	p.jobs[jobID] = job
	p.mu.Unlock()

	// Start async processing
	go p.processProposal(jobID, candidates, targetCount, lastTag)

	return jobID, nil
}

// processProposal runs the AI proposal generation
func (p *PlanProposer) processProposal(jobID string, candidates []IssueCandidate, targetCount int, lastTag string) {
	p.mu.Lock()
	job := p.jobs[jobID]
	job.Status = "processing"
	p.mu.Unlock()

	// Build dependency graph
	ctx := context.Background()
	graph := p.buildDependencyGraph(ctx, candidates)

	// Build prompt for LLM
	prompt := buildProposalPrompt(candidates, targetCount, lastTag, graph)

	// Select model for planning
	llmModel := p.router.SelectModel(config.CategoryPlanning, config.ComplexityMedium, nil)

	// Create session and send message
	session, err := p.oc.CreateSession("sprint-planning-proposal")
	if err != nil {
		p.mu.Lock()
		job.Status = "failed"
		job.Error = fmt.Sprintf("Failed to create session: %v", err)
		p.mu.Unlock()
		return
	}

	msg, err := p.oc.SendMessage(session.ID, prompt, opencode.ParseModelRef(llmModel), os.Stdout)
	if err != nil {
		p.mu.Lock()
		job.Status = "failed"
		job.Error = fmt.Sprintf("Failed to get AI response: %v", err)
		p.mu.Unlock()
		return
	}

	// Parse response - extract text from message parts
	var responseText string
	for _, part := range msg.Parts {
		if part.Type == "text" {
			responseText = part.Text
			break
		}
	}

	// Extract JSON from response (handle markdown code blocks and text around JSON)
	jsonStr := extractJSON(responseText)
	if jsonStr == "" {
		p.mu.Lock()
		job.Status = "failed"
		job.Error = "No JSON found in AI response"
		p.mu.Unlock()
		return
	}

	var result struct {
		Issues   []ProposedIssue `json:"issues"`
		Branches []Branch        `json:"branches"`
	}
	if err := json.Unmarshal([]byte(jsonStr), &result); err != nil {
		p.mu.Lock()
		job.Status = "failed"
		job.Error = fmt.Sprintf("Failed to parse proposal: %v\nResponse: %s", err, jsonStr[:min(len(jsonStr), 200)])
		p.mu.Unlock()
		return
	}

	p.mu.Lock()
	job.Status = "completed"
	job.Proposal = result.Issues
	job.Branches = result.Branches
	p.mu.Unlock()
}

// buildDependencyGraph builds a map of issue dependencies
func (p *PlanProposer) buildDependencyGraph(ctx context.Context, candidates []IssueCandidate) map[int][]github.LinkedIssue {
	graph := make(map[int][]github.LinkedIssue)

	for _, candidate := range candidates {
		linked, err := p.githubClient.GetLinkedIssues(ctx, candidate.Number)
		if err != nil {
			continue // Skip on error
		}
		graph[candidate.Number] = linked
	}

	return graph
}

// GetProposalStatus returns the current status of a proposal job
func (p *PlanProposer) GetProposalStatus(jobID string) (*ProposalJob, error) {
	p.mu.RLock()
	defer p.mu.RUnlock()

	job, exists := p.jobs[jobID]
	if !exists {
		return nil, fmt.Errorf("job not found: %s", jobID)
	}

	return job, nil
}

// buildProposalPrompt creates the LLM prompt for proposal generation
func buildProposalPrompt(candidates []IssueCandidate, targetCount int, lastTag string, graph map[int][]github.LinkedIssue) string {
	candidatesJSON, _ := json.Marshal(candidates)
	graphJSON, _ := json.Marshal(graph)

	return fmt.Sprintf(`Given:
1. List of unassigned GitHub issues (with labels, complexity):
%s

2. Last release tag: %s (for context)

3. Target count: %d (soft limit, can exceed by up to 20%%)

4. Dependency graph (issue number -> linked issues):
%s

Select the best issues to include in the current sprint.

Rules:
- Group issues into branches based on dependencies (transitive closure)
- Select complete branches only (all or nothing)
- Prioritize: priority:high > priority:medium > priority:low
- Consider what was done in last release for context
- Can exceed target by up to 20%% if it completes important branches
- Return 100-120%% of target count

Return JSON with:
1. Selected issues with reasoning and branch assignment
2. Branches structure (root issue + all dependencies in branch)

Format:
{
  "issues": [
    {
      "number": 123,
      "title": "Issue title",
      "labels": ["priority:high", "type:bug"],
      "complexity": 3,
      "reason": "High priority bug",
      "dependencies": [456, 789],
      "branch": "auth-epic"
    }
  ],
  "branches": [
    {
      "id": "auth-epic",
      "name": "Epic: User Authentication",
      "root_issue": 123,
      "issues": [123, 456, 789],
      "total_complexity": 12
    }
  ]
}

Do not include any other text, only the JSON.`, candidatesJSON, lastTag, targetCount, graphJSON)
}

// min returns the minimum of two integers
func min(a, b int) int {
	if a < b {
		return a
	}
	return b
}
