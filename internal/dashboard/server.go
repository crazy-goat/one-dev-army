package dashboard

import (
	"context"
	"embed"
	"fmt"
	"html/template"
	"net/http"
	"time"

	"github.com/crazy-goat/one-dev-army/internal/db"
	"github.com/crazy-goat/one-dev-army/internal/github"
	"github.com/crazy-goat/one-dev-army/internal/mvp"
	"github.com/crazy-goat/one-dev-army/internal/opencode"
	"github.com/crazy-goat/one-dev-army/internal/worker"
)

//go:embed templates/*.html
var templateFS embed.FS

type Server struct {
	port         int
	tmpls        map[string]*template.Template
	store        *db.Store
	pool         func() []worker.WorkerInfo
	gh           *github.Client
	orchestrator *mvp.Orchestrator
	mux          *http.ServeMux
	httpSrv      *http.Server
	wizardStore  *WizardSessionStore
	oc           *opencode.Client
	wizardLLM    string
	hub          *Hub
	syncService  *SyncService
}

func NewServer(port int, store *db.Store, pool func() []worker.WorkerInfo, gh *github.Client, orchestrator *mvp.Orchestrator, oc *opencode.Client, wizardLLM string, hub *Hub, syncService *SyncService) (*Server, error) {
	tmpls, err := parseTemplates()
	if err != nil {
		return nil, err
	}

	mux := http.NewServeMux()

	s := &Server{
		port:         port,
		tmpls:        tmpls,
		store:        store,
		pool:         pool,
		gh:           gh,
		orchestrator: orchestrator,
		mux:          mux,
		httpSrv: &http.Server{
			Addr:    fmt.Sprintf(":%d", port),
			Handler: mux,
		},
		wizardStore: NewWizardSessionStore(),
		oc:          oc,
		wizardLLM:   wizardLLM,
		hub:         hub,
		syncService: syncService,
	}
	if s.wizardLLM == "" {
		s.wizardLLM = DefaultLLMModel
	}
	s.routes()
	return s, nil
}

func parseTemplates() (map[string]*template.Template, error) {
	tmpls := make(map[string]*template.Template)

	funcMap := template.FuncMap{
		"duration": func(start, end *time.Time) string {
			if start == nil || end == nil {
				return ""
			}
			d := end.Sub(*start).Round(time.Second)
			if d < time.Minute {
				return fmt.Sprintf("%ds", int(d.Seconds()))
			}
			return fmt.Sprintf("%dm%ds", int(d.Minutes()), int(d.Seconds())%60)
		},
		"truncate": func(s string, n int) string {
			if len(s) <= n {
				return s
			}
			return s[:n] + "\n... (truncated)"
		},
	}

	pages := []string{"board.html", "backlog.html", "costs.html", "task.html"}
	for _, page := range pages {
		t, err := template.New("").Funcs(funcMap).ParseFS(templateFS, "templates/layout.html", "templates/"+page)
		if err != nil {
			return nil, fmt.Errorf("parsing %s: %w", page, err)
		}
		tmpls[page] = t
	}

	wizardPageTmpl, err := template.New("").Funcs(funcMap).ParseFS(templateFS, "templates/layout.html", "templates/wizard_steps.html", "templates/wizard_new.html", "templates/wizard_page.html")
	if err != nil {
		return nil, fmt.Errorf("parsing wizard_page.html: %w", err)
	}
	tmpls["wizard_page.html"] = wizardPageTmpl

	wizardModalTmpl, err := template.New("").Funcs(funcMap).ParseFS(templateFS, "templates/layout.html", "templates/wizard_steps.html", "templates/wizard_new.html", "templates/wizard_modal.html")
	if err != nil {
		return nil, fmt.Errorf("parsing wizard_modal.html: %w", err)
	}
	tmpls["wizard_modal.html"] = wizardModalTmpl

	// Parse wizard partial templates (no layout)
	wizardPartials := []string{"wizard_new.html", "wizard_refine.html", "wizard_title.html", "wizard_create.html", "wizard_error.html", "wizard_logs.html"}
	for _, page := range wizardPartials {
		t, err := template.ParseFS(templateFS, "templates/wizard_steps.html", "templates/"+page)
		if err != nil {
			return nil, fmt.Errorf("parsing %s: %w", page, err)
		}
		tmpls[page] = t
	}

	t, err := template.ParseFS(templateFS, "templates/workers.html")
	if err != nil {
		return nil, fmt.Errorf("parsing workers.html: %w", err)
	}
	tmpls["workers.html"] = t

	return tmpls, nil
}

func (s *Server) routes() {
	s.mux.HandleFunc("GET /{$}", s.handleBoard)
	s.mux.HandleFunc("GET /backlog", s.handleBacklog)
	s.mux.HandleFunc("GET /costs", s.handleCosts)
	s.mux.HandleFunc("GET /api/workers", s.handleWorkers)
	s.mux.HandleFunc("GET /api/current-task", s.handleCurrentTask)
	s.mux.HandleFunc("GET /api/sprint/status", s.handleSprintStatus)
	s.mux.HandleFunc("POST /api/sprint/start", s.handleSprintStart)
	s.mux.HandleFunc("POST /api/sprint/pause", s.handleSprintPause)
	s.mux.HandleFunc("POST /epic", s.handleAddEpic)
	s.mux.HandleFunc("POST /sync", s.handleSync)
	s.mux.HandleFunc("GET /api/board-data", s.handleBoardData)
	s.mux.HandleFunc("POST /plan-sprint", s.handlePlanSprint)
	s.mux.HandleFunc("GET /task/{id}", s.handleTaskDetail)
	s.mux.HandleFunc("GET /api/task/{id}/stream", s.handleTaskStream)
	s.mux.HandleFunc("POST /approve/{id}", s.handleApprove)
	s.mux.HandleFunc("POST /reject/{id}", s.handleReject)
	s.mux.HandleFunc("POST /retry/{id}", s.handleRetry)
	s.mux.HandleFunc("POST /retry-fresh/{id}", s.handleRetryFresh)
	s.mux.HandleFunc("POST /approve-merge/{id}", s.handleApproveMerge)
	s.mux.HandleFunc("POST /decline/{id}", s.handleDecline)

	// WebSocket endpoint
	s.mux.HandleFunc("GET /ws", s.handleWebSocket)

	// Wizard routes
	s.mux.HandleFunc("GET /wizard", s.handleWizardPage)
	s.mux.HandleFunc("GET /wizard/new", s.handleWizardNew)
	s.mux.HandleFunc("GET /wizard/modal", s.handleWizardModal)
	s.mux.HandleFunc("POST /wizard/select-type", s.handleWizardSelectType)
	s.mux.HandleFunc("POST /wizard/cancel", s.handleWizardCancel)
	s.mux.HandleFunc("POST /wizard/refine", s.handleWizardRefine)
	s.mux.HandleFunc("POST /wizard/title", s.handleWizardGenerateTitle)
	// REMOVED: s.mux.HandleFunc("POST /wizard/breakdown", s.handleWizardBreakdown)
	s.mux.HandleFunc("POST /wizard/create", s.handleWizardCreate)
	s.mux.HandleFunc("GET /wizard/logs/{sessionId}", s.handleWizardLogs)
}

func (s *Server) Start() error {
	return s.httpSrv.ListenAndServe()
}

func (s *Server) Shutdown(ctx context.Context) error {
	// Stop the sync service
	if s.syncService != nil {
		s.syncService.Stop()
	}

	// Stop the WebSocket hub
	if s.hub != nil {
		s.hub.Stop()
	}
	return s.httpSrv.Shutdown(ctx)
}

func (s *Server) Handler() http.Handler {
	return s.mux
}

// Hub returns the WebSocket hub for broadcasting messages
func (s *Server) Hub() *Hub {
	return s.hub
}

// handleWebSocket handles WebSocket upgrade requests
func (s *Server) handleWebSocket(w http.ResponseWriter, r *http.Request) {
	ServeWs(s.hub, w, r)
}
