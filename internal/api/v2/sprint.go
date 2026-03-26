package v2

import (
	"errors"
	"fmt"
	"log"
	"net/http"
)

// GetSprint returns the current sprint status.
func (h *Handler) GetSprint(w http.ResponseWriter, _ *http.Request) {
	w.Header().Set("Content-Type", "application/json")

	resp := SprintResponse{Paused: true}

	if h.orchestrator != nil {
		resp.Paused = h.orchestrator.IsPaused()
		resp.Processing = h.orchestrator.IsProcessing()
	}

	if h.gh != nil && h.gh.GetActiveMilestone() != nil {
		resp.SprintName = h.gh.GetActiveMilestone().Title
	}

	// Reuse board data for CanCloseSprint calculation
	board := h.buildBoardResponse()
	resp.CanCloseSprint = board.CanCloseSprint

	writeJSON(w, resp)
}

// SprintAction handles POST /api/v2/sprint/{action}.
func (h *Handler) SprintAction(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "application/json")

	action := r.PathValue("action")

	if h.orchestrator == nil {
		w.WriteHeader(http.StatusServiceUnavailable)
		writeJSON(w, ActionResponse{Success: false, Error: "orchestrator not configured"})
		return
	}

	switch action {
	case "start":
		h.orchestrator.Start()
		writeJSON(w, ActionResponse{Success: true, Message: "Sprint started"})
	case "pause":
		h.orchestrator.Pause()
		writeJSON(w, ActionResponse{Success: true, Message: "Sprint paused"})
	case "close":
		if err := h.handleSprintClose(); err != nil {
			w.WriteHeader(http.StatusConflict)
			writeJSON(w, ActionResponse{Success: false, Error: err.Error()})
			return
		}
		writeJSON(w, ActionResponse{Success: true, Message: "Sprint closed"})
	default:
		w.WriteHeader(http.StatusBadRequest)
		writeJSON(w, ActionResponse{Success: false, Error: "unknown action: " + action})
	}
}

func (h *Handler) handleSprintClose() error {
	if h.orchestrator.IsProcessing() {
		return errors.New("cannot close sprint while processing tasks")
	}

	if h.gh == nil || h.gh.GetActiveMilestone() == nil {
		return errors.New("no active milestone")
	}

	milestone := h.gh.GetActiveMilestone()

	if err := h.gh.CloseMilestone(milestone.Number); err != nil {
		return fmt.Errorf("closing milestone: %w", err)
	}

	newSprintTitle, err := h.gh.CreateNextSprint(milestone.Title)
	if err != nil {
		return fmt.Errorf("creating next sprint: %w", err)
	}
	log.Printf("[API v2] Created new sprint: %s", newSprintTitle)

	newMilestone, err := h.gh.GetOldestOpenMilestone()
	if err != nil {
		return fmt.Errorf("reloading milestones: %w", err)
	}

	if newMilestone == nil {
		return errors.New("no open milestone found after closing")
	}

	h.gh.SetActiveMilestone(newMilestone)
	return nil
}
