package v2

import (
	"log"
	"net/http"

	"github.com/crazy-goat/one-dev-army/internal/config"
)

// GetWorkerStatus returns the current worker status.
func (h *Handler) GetWorkerStatus(w http.ResponseWriter, _ *http.Request) {
	w.Header().Set("Content-Type", "application/json")

	resp := WorkerStatusResponse{}

	if h.orchestrator == nil {
		resp.Paused = true
		writeJSON(w, resp)
		return
	}

	resp.Active = h.orchestrator.IsProcessing()
	resp.Paused = h.orchestrator.IsPaused()

	if task := h.orchestrator.CurrentTask(); task != nil {
		resp.IssueID = task.Issue.Number
		resp.IssueTitle = task.Issue.Title
		resp.Step = string(task.Status)
	}

	writeJSON(w, resp)
}

// GetRateLimit returns rate limit info as JSON (instead of HTML like the old API).
func (*Handler) GetRateLimit(w http.ResponseWriter, _ *http.Request) {
	w.Header().Set("Content-Type", "application/json")
	// Rate limit service is on the dashboard server, not accessible here.
	// Return empty for now — the React frontend can use the existing /api/rate-limit endpoint.
	writeJSON(w, RateLimitResponse{})
}

// GetSettings returns current settings as JSON.
func (h *Handler) GetSettings(w http.ResponseWriter, _ *http.Request) {
	w.Header().Set("Content-Type", "application/json")

	cfg, err := config.Load(h.rootDir)
	if err != nil {
		log.Printf("[API v2] Error loading config: %v", err)
		cfg = &config.Config{LLM: config.DefaultLLMConfig()}
	}

	resp := SettingsResponse{
		LLM: LLMSettingsResponse{
			Setup:         cfg.LLM.Setup.Model,
			Planning:      cfg.LLM.Planning.Model,
			Orchestration: cfg.LLM.Orchestration.Model,
			Code:          cfg.LLM.Code.Model,
			CodeHeavy:     cfg.LLM.CodeHeavy.Model,
		},
		YoloMode: cfg.YoloMode,
	}

	if h.yoloOverride != nil && *h.yoloOverride != nil {
		resp.YoloMode = **h.yoloOverride
	}

	writeJSON(w, resp)
}

// ToggleYolo toggles YOLO mode and returns the new state as JSON.
func (h *Handler) ToggleYolo(w http.ResponseWriter, _ *http.Request) {
	w.Header().Set("Content-Type", "application/json")

	cfg, err := config.Load(h.rootDir)
	if err != nil {
		w.WriteHeader(http.StatusInternalServerError)
		writeJSON(w, ActionResponse{Success: false, Error: "failed to load config"})
		return
	}

	currentYolo := cfg.YoloMode
	if h.yoloOverride != nil && *h.yoloOverride != nil {
		currentYolo = **h.yoloOverride
	}

	newYolo := !currentYolo
	if h.yoloOverride != nil {
		*h.yoloOverride = &newYolo
	}

	cfg.YoloMode = newYolo
	if h.configProp != nil {
		h.configProp.Propagate(cfg)
	}

	log.Printf("[API v2] YOLO mode toggled to: %v", newYolo)

	writeJSON(w, map[string]any{
		"success":   true,
		"yolo_mode": newYolo,
	})
}
