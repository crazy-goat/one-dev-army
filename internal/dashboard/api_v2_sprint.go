package dashboard

import (
	"encoding/json"
	"net/http"
	"strconv"
	"strings"

	"github.com/crazy-goat/one-dev-army/internal/scheduler"
)

// handleGetLastTag returns the most recent tag/release for AI context
func (s *Server) handleGetLastTag(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()

	tagInfo, err := s.gh.GetLastTag(ctx)
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}

	if tagInfo == nil {
		http.Error(w, "No tags found", http.StatusNotFound)
		return
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(tagInfo)
}

// IssueCandidate represents an issue without milestone (candidate for sprint planning)
type IssueCandidate struct {
	Number     int      `json:"number"`
	Title      string   `json:"title"`
	Labels     []string `json:"labels"`
	Complexity *int     `json:"complexity,omitempty"`
}

// handleGetUnassignedIssues returns all open issues without a milestone
func (s *Server) handleGetUnassignedIssues(w http.ResponseWriter, r *http.Request) {
	// Get all open issues without milestone
	issues, err := s.gh.ListIssuesWithoutMilestone()
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}

	var candidates []IssueCandidate
	for _, issue := range issues {
		candidate := IssueCandidate{
			Number: issue.Number,
			Title:  issue.Title,
			Labels: issue.GetLabelNames(),
		}

		// Extract complexity from labels if present (e.g., "complexity:3")
		for _, label := range candidate.Labels {
			if strings.HasPrefix(label, "complexity:") {
				if val, err := strconv.Atoi(strings.TrimPrefix(label, "complexity:")); err == nil {
					candidate.Complexity = &val
				}
			}
		}

		candidates = append(candidates, candidate)
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(candidates)
}

// CreateProposalRequest represents the request to create a proposal
type CreateProposalRequest struct {
	TargetCount int `json:"targetCount"`
}

// CreateProposalResponse represents the response with job ID
type CreateProposalResponse struct {
	JobID string `json:"jobId"`
}

// handleCreateProposal starts AI proposal generation
func (s *Server) handleCreateProposal(w http.ResponseWriter, r *http.Request) {
	var req CreateProposalRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}

	// Get unassigned issues
	issues, err := s.gh.ListIssuesWithoutMilestone()
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}

	var candidates []scheduler.IssueCandidate
	for _, issue := range issues {
		candidate := scheduler.IssueCandidate{
			Number: issue.Number,
			Title:  issue.Title,
			Labels: issue.GetLabelNames(),
		}
		candidates = append(candidates, candidate)
	}

	// Get last tag for context
	lastTag, _ := s.gh.GetLastTag(r.Context())
	lastTagName := ""
	if lastTag != nil {
		lastTagName = lastTag.Tag
	}

	// Create proposal job
	jobID, err := s.planProposer.CreateProposal(candidates, req.TargetCount, lastTagName)
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(CreateProposalResponse{JobID: jobID})
}

// handleGetProposal returns the status and result of a proposal job
func (s *Server) handleGetProposal(w http.ResponseWriter, r *http.Request) {
	jobID := r.PathValue("jobId")
	if jobID == "" {
		http.Error(w, "jobId required", http.StatusBadRequest)
		return
	}

	job, err := s.planProposer.GetProposalStatus(jobID)
	if err != nil {
		http.Error(w, err.Error(), http.StatusNotFound)
		return
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(job)
}
