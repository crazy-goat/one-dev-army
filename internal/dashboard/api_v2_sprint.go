package dashboard

import (
	"encoding/json"
	"net/http"
	"strconv"
	"strings"
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
