package scheduler

import (
	"testing"

	"github.com/crazy-goat/one-dev-army/internal/config"
	"github.com/crazy-goat/one-dev-army/internal/github"
	"github.com/crazy-goat/one-dev-army/internal/llm"
	"github.com/crazy-goat/one-dev-army/internal/opencode"
)

func TestPlanProposer_CreateProposal(t *testing.T) {
	// Create minimal config
	cfg := &config.Config{
		LLM: config.LLMConfig{
			Planning: config.CategoryModels{Model: "nexos-ai/Kimi K2.5"},
		},
	}

	// Create mock router
	router := llm.NewRouter(&cfg.LLM)

	// Create mock opencode client (will fail, but that's ok for structure test)
	oc := opencode.NewClient("http://localhost:0")

	// Create mock github client
	ghClient := github.NewClient("test/repo")

	proposer := NewPlanProposer(cfg, router, oc, ghClient)

	candidates := []IssueCandidate{
		{Number: 1, Title: "Bug fix", Labels: []string{"priority:high", "type:bug"}},
		{Number: 2, Title: "Feature", Labels: []string{"priority:medium", "type:feature"}},
	}

	jobID, err := proposer.CreateProposal(candidates, 1, "v1.0.0")
	if err != nil {
		t.Fatalf("CreateProposal() error = %v", err)
	}
	if jobID == "" {
		t.Error("CreateProposal() returned empty jobID")
	}

	// Check job exists
	status, err := proposer.GetProposalStatus(jobID)
	if err != nil {
		t.Errorf("GetProposalStatus() error = %v", err)
	}
	if status == nil {
		t.Error("GetProposalStatus() returned nil")
	}
	if status.ID != jobID {
		t.Errorf("GetProposalStatus() returned wrong job ID, got %s want %s", status.ID, jobID)
	}
}
