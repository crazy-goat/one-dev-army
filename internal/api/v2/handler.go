package v2

import (
	"net/http"

	"github.com/crazy-goat/one-dev-army/internal/config"
	"github.com/crazy-goat/one-dev-army/internal/db"
	"github.com/crazy-goat/one-dev-army/internal/git"
	"github.com/crazy-goat/one-dev-army/internal/github"
	"github.com/crazy-goat/one-dev-army/internal/mvp"
)

// Handler serves the API v2 endpoints.
type Handler struct {
	store        *db.Store
	gh           *github.Client
	orchestrator *mvp.Orchestrator
	rootDir      string
	brMgr        *git.BranchManager
	configProp   *config.ConfigPropagator
	yoloOverride **bool // pointer to Server's yoloOverride field
}

// NewHandler creates a new API v2 handler.
func NewHandler(store *db.Store, gh *github.Client, orch *mvp.Orchestrator, rootDir string, brMgr *git.BranchManager, configProp *config.ConfigPropagator, yoloOverride **bool) *Handler {
	return &Handler{
		store:        store,
		gh:           gh,
		orchestrator: orch,
		rootDir:      rootDir,
		brMgr:        brMgr,
		configProp:   configProp,
		yoloOverride: yoloOverride,
	}
}

// Register registers all API v2 routes on the given mux.
func (h *Handler) Register(mux *http.ServeMux) {
	mux.HandleFunc("GET /api/v2/board", h.GetBoard)
	mux.HandleFunc("GET /api/v2/tasks/{id}", h.GetTask)
	mux.HandleFunc("POST /api/v2/tasks/{id}/actions/{action}", h.TaskAction)
	mux.HandleFunc("GET /api/v2/sprint", h.GetSprint)
	mux.HandleFunc("POST /api/v2/sprint/{action}", h.SprintAction)
	mux.HandleFunc("GET /api/v2/worker-status", h.GetWorkerStatus)
	mux.HandleFunc("GET /api/v2/rate-limit", h.GetRateLimit)
	mux.HandleFunc("GET /api/v2/settings", h.GetSettings)
	mux.HandleFunc("POST /api/v2/yolo/toggle", h.ToggleYolo)
}
