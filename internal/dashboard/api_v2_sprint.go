package dashboard

import (
	"encoding/json"
	"fmt"
	"net/http"
	"strconv"
	"strings"
	"time"

	"github.com/crazy-goat/one-dev-army/internal/github"
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
	if err := json.NewEncoder(w).Encode(tagInfo); err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
	}
}

// IssueCandidate represents an issue without milestone (candidate for sprint planning)
type IssueCandidate struct {
	Number     int      `json:"number"`
	Title      string   `json:"title"`
	Labels     []string `json:"labels"`
	Complexity *int     `json:"complexity,omitempty"`
}

// handleGetUnassignedIssues returns all open issues without a milestone
func (s *Server) handleGetUnassignedIssues(w http.ResponseWriter, _ *http.Request) {
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
	if err := json.NewEncoder(w).Encode(candidates); err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
	}
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
	if err := json.NewEncoder(w).Encode(CreateProposalResponse{JobID: jobID}); err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
	}
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
	if err := json.NewEncoder(w).Encode(job); err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
	}
}

// AssignRequest represents the request to assign issues to sprint
type AssignRequest struct {
	IssueNumbers []int    `json:"issueNumbers"`
	Branches     []string `json:"branches,omitempty"`
}

// AssignProgress represents the progress of assigning issues
type AssignProgress struct {
	Type    string `json:"type"` // progress, completed, error
	Current int    `json:"current"`
	Total   int    `json:"total"`
	Issue   int    `json:"issue,omitempty"`
	Branch  string `json:"branch,omitempty"`
	Error   string `json:"error,omitempty"`
}

// handleAssignIssues assigns selected issues to the current sprint using SSE
func (s *Server) handleAssignIssues(w http.ResponseWriter, r *http.Request) {
	var req AssignRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}

	// Get current sprint
	sprintDetector := github.NewSprintDetector(s.gh)
	currentSprint, err := sprintDetector.GetCurrentSprint()
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}

	if currentSprint == nil {
		http.Error(w, "No active sprint", http.StatusBadRequest)
		return
	}

	// Setup SSE
	w.Header().Set("Content-Type", "text/event-stream")
	w.Header().Set("Cache-Control", "no-cache")
	w.Header().Set("Connection", "keep-alive")
	w.Header().Set("X-Accel-Buffering", "no") // Disable nginx buffering

	flusher, ok := w.(http.Flusher)
	if !ok {
		http.Error(w, "Streaming not supported", http.StatusInternalServerError)
		return
	}

	total := len(req.IssueNumbers)

	// Map issue to branch for progress reporting
	issueToBranch := make(map[int]string)
	// TODO: Build this map from request data if needed

	for i, issueNumber := range req.IssueNumbers {
		// Assign issue to milestone using milestone title
		err := s.gh.SetMilestone(issueNumber, currentSprint.Title)

		progress := AssignProgress{
			Type:    "progress",
			Current: i + 1,
			Total:   total,
			Issue:   issueNumber,
			Branch:  issueToBranch[issueNumber],
		}

		if err != nil {
			progress.Type = "error"
			progress.Error = err.Error()
		}

		// Send SSE event
		data, _ := json.Marshal(progress)
		fmt.Fprintf(w, "data: %s\n\n", data)
		flusher.Flush()

		// Small delay to avoid rate limiting
		time.Sleep(100 * time.Millisecond)
	}

	// Send completion event
	completed := AssignProgress{Type: "completed", Current: total, Total: total}
	data, _ := json.Marshal(completed)
	fmt.Fprintf(w, "data: %s\n\n", data)
	flusher.Flush()
}
