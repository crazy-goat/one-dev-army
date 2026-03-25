package dashboard

import (
	"context"
	"embed"
	"encoding/json"
	"errors"
	"fmt"
	"html/template"
	"log"
	"net/http"
	"time"

	"github.com/crazy-goat/one-dev-army/internal/config"
	"github.com/crazy-goat/one-dev-army/internal/db"
	"github.com/crazy-goat/one-dev-army/internal/git"
	"github.com/crazy-goat/one-dev-army/internal/github"
	"github.com/crazy-goat/one-dev-army/internal/mvp"
	"github.com/crazy-goat/one-dev-army/internal/opencode"
	"github.com/crazy-goat/one-dev-army/internal/worker"
)

//go:embed templates/*.html
var templateFS embed.FS

type Server struct {
	port             int
	webPort          int
	tmpls            map[string]*template.Template
	store            *db.Store
	pool             func() []worker.WorkerInfo
	gh               *github.Client
	orchestrator     *mvp.Orchestrator
	mux              *http.ServeMux
	httpSrv          *http.Server
	wizardStore      *WizardSessionStore
	oc               *opencode.Client
	wizardLLM        string
	hub              *Hub
	syncService      *SyncService
	rateLimitService *RateLimitService
	rootDir          string
	modelsCache      []opencode.ProviderModel
	brMgr            *git.BranchManager
	configPropagator *config.ConfigPropagator
	yoloOverride     *bool // Runtime YOLO mode override (nil = use config file)
}

func NewServer(port int, webPort int, store *db.Store, pool func() []worker.WorkerInfo, gh *github.Client, orchestrator *mvp.Orchestrator, oc *opencode.Client, wizardLLM string, hub *Hub, syncService *SyncService, rootDir string, brMgr *git.BranchManager, configPropagator *config.ConfigPropagator) (*Server, error) {
	tmpls, err := parseTemplates()
	if err != nil {
		return nil, err
	}

	mux := http.NewServeMux()

	s := &Server{
		port:         port,
		webPort:      webPort,
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
		wizardStore:      NewWizardSessionStore(),
		oc:               oc,
		wizardLLM:        wizardLLM,
		hub:              hub,
		syncService:      syncService,
		rootDir:          rootDir,
		brMgr:            brMgr,
		configPropagator: configPropagator,
	}

	if s.wizardLLM == "" {
		s.wizardLLM = DefaultLLMModel
	}

	// Initialize rate limit service
	token, _ := github.GetToken()
	s.rateLimitService = NewRateLimitService(token)
	s.rateLimitService.Start()

	s.routes()

	// Load available models from opencode API
	if err := s.LoadModels(); err != nil {
		log.Printf("[Dashboard] Warning: failed to load models: %v", err)
	}

	return s, nil
}

// LoadModels fetches and caches available models from the opencode API
func (s *Server) LoadModels() error {
	if s.oc == nil {
		return errors.New("opencode client not configured")
	}

	providers, err := s.oc.ListProviders()
	if err != nil {
		return fmt.Errorf("loading providers: %w", err)
	}

	var models []opencode.ProviderModel
	connectedSet := make(map[string]bool)
	for _, id := range providers.Connected {
		connectedSet[id] = true
	}

	for _, p := range providers.All {
		if !connectedSet[p.ID] {
			continue
		}
		for _, model := range p.Models {
			models = append(models, model)
		}
	}

	s.modelsCache = models
	log.Printf("[Dashboard] Loaded %d models from %d connected providers", len(models), len(providers.Connected))
	return nil
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
		"json": func(v any) string {
			b, err := json.Marshal(v)
			if err != nil {
				return ""
			}
			return string(b)
		},
		"labelIcon":    LabelIcon,
		"labelTooltip": LabelTooltip,
	}

	pages := []string{"board.html", "task.html"}
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
	wizardPartials := []string{"wizard_new.html", "wizard_refine.html", "wizard_create.html", "wizard_error.html", "wizard_logs.html"}
	for _, page := range wizardPartials {
		t, err := template.ParseFS(templateFS, "templates/wizard_steps.html", "templates/"+page)
		if err != nil {
			return nil, fmt.Errorf("parsing %s: %w", page, err)
		}
		tmpls[page] = t
	}

	// Parse settings template
	settingsTmpl, err := template.New("").Funcs(funcMap).ParseFS(templateFS, "templates/layout.html", "templates/llm-config.html")
	if err != nil {
		return nil, fmt.Errorf("parsing llm-config.html: %w", err)
	}
	tmpls["llm-config.html"] = settingsTmpl

	return tmpls, nil
}

func (s *Server) routes() {
	s.mux.HandleFunc("GET /{$}", s.handleBoard)
	s.mux.HandleFunc("GET /api/current-task", s.handleCurrentTask)
	s.mux.HandleFunc("GET /api/sprint/status", s.handleSprintStatus)
	s.mux.HandleFunc("POST /api/sprint/start", s.handleSprintStart)
	s.mux.HandleFunc("POST /api/sprint/pause", s.handleSprintPause)
	s.mux.HandleFunc("POST /api/sprint/close", s.handleSprintClose)
	s.mux.HandleFunc("POST /epic", s.handleAddEpic)
	s.mux.HandleFunc("POST /sync", s.handleSync)
	s.mux.HandleFunc("POST /api/sync", s.handleManualSync)
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
	s.mux.HandleFunc("POST /block/{id}", s.handleBlock)
	s.mux.HandleFunc("POST /unblock/{id}", s.handleUnblock)

	// WebSocket endpoint
	s.mux.HandleFunc("GET /ws", s.handleWebSocket)

	// Wizard routes
	s.mux.HandleFunc("GET /wizard", s.handleWizardPage)
	s.mux.HandleFunc("GET /wizard/new", s.handleWizardNew)
	s.mux.HandleFunc("GET /wizard/modal", s.handleWizardModal)
	s.mux.HandleFunc("POST /wizard/select-type", s.handleWizardSelectType)
	s.mux.HandleFunc("POST /wizard/cancel", s.handleWizardCancel)
	s.mux.HandleFunc("POST /wizard/refine", s.handleWizardRefine)
	// REMOVED: s.mux.HandleFunc("POST /wizard/title", s.handleWizardGenerateTitle) (merged into refine step)
	// REMOVED: s.mux.HandleFunc("POST /wizard/breakdown", s.handleWizardBreakdown)
	s.mux.HandleFunc("POST /wizard/create", s.handleWizardCreate)
	s.mux.HandleFunc("GET /wizard/logs/{sessionId}", s.handleWizardLogs)

	// Rate limit endpoints
	s.mux.HandleFunc("GET /api/rate-limit", s.handleRateLimit)
	s.mux.HandleFunc("POST /api/rate-limit/refresh", s.handleRateLimitRefresh)

	// Worker status endpoint
	s.mux.HandleFunc("GET /api/worker-status", s.handleWorkerStatus)

	// Settings endpoints
	s.mux.HandleFunc("GET /settings", s.handleSettings)
	s.mux.HandleFunc("POST /settings", s.handleSaveSettings)

	// YOLO mode toggle endpoint
	s.mux.HandleFunc("POST /api/yolo/toggle", s.handleYoloToggle)
}

func (s *Server) Start() error {
	return s.httpSrv.ListenAndServe()
}

func (s *Server) Shutdown(ctx context.Context) error {
	// Stop the rate limit service
	if s.rateLimitService != nil {
		s.rateLimitService.Stop()
	}

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

// labelInfo holds the icon and tooltip for a label.
type labelInfo struct {
	Icon    string
	Tooltip string
}

// labelData maps label strings to their icon and tooltip.
// Multiple label strings can map to the same info (e.g. "type:feature", "feature", "enhancement").
var labelData = map[string]labelInfo{
	"type:feature":    {Icon: "✨", Tooltip: "Type: Feature"},
	"feature":         {Icon: "✨", Tooltip: "Type: Feature"},
	"enhancement":     {Icon: "✨", Tooltip: "Type: Feature"},
	"type:bug":        {Icon: "🐛", Tooltip: "Type: Bug"},
	"bug":             {Icon: "🐛", Tooltip: "Type: Bug"},
	"type:docs":       {Icon: "📚", Tooltip: "Documentation"},
	"type:refactor":   {Icon: "🔧", Tooltip: "Refactor"},
	"size:S":          {Icon: "🐜", Tooltip: "Size: Small"},
	"size:M":          {Icon: "🐕", Tooltip: "Size: Medium"},
	"size:L":          {Icon: "🐘", Tooltip: "Size: Large"},
	"size:XL":         {Icon: "🦕", Tooltip: "Size: Extra Large"},
	"priority:high":   {Icon: "🔴", Tooltip: "Priority: High"},
	"priority:medium": {Icon: "🟡", Tooltip: "Priority: Medium"},
	"priority:low":    {Icon: "🟢", Tooltip: "Priority: Low"},
}

// LabelIcon returns the emoji icon for a given label string.
func LabelIcon(label string) string {
	if info, ok := labelData[label]; ok {
		return info.Icon
	}
	return ""
}

// LabelTooltip returns the human-readable tooltip for a given label string.
func LabelTooltip(label string) string {
	if info, ok := labelData[label]; ok {
		return info.Tooltip
	}
	return ""
}

// Hub returns the WebSocket hub for broadcasting messages
func (s *Server) Hub() *Hub {
	return s.hub
}

// GetAvailableModelIDs returns a list of available model IDs from the cache
func (s *Server) GetAvailableModelIDs() []string {
	if len(s.modelsCache) == 0 {
		return nil
	}

	ids := make([]string, len(s.modelsCache))
	for i, m := range s.modelsCache {
		ids[i] = m.ID
	}
	return ids
}

// handleWebSocket handles WebSocket upgrade requests
func (s *Server) handleWebSocket(w http.ResponseWriter, r *http.Request) {
	ServeWs(s.hub, w, r)
}
