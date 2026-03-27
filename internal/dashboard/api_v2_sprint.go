package dashboard

import (
	"encoding/json"
	"net/http"
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
