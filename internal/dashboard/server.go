package dashboard

import (
	"context"
	"crypto/rand"
	"crypto/subtle"
	"embed"
	"encoding/base64"
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

// CSRFTokenLength is the length of CSRF tokens in bytes
const CSRFTokenLength = 32

// RateLimitRequests is the maximum number of requests per window
const RateLimitRequests = 10

// RateLimitWindow is the time window for rate limiting
const RateLimitWindow = time.Minute

type Server struct {
	port          int
	tmpls         map[string]*template.Template
	store         *db.Store
	pool          func() []worker.WorkerInfo
	gh            *github.Client
	projectNumber int
	orchestrator  *mvp.Orchestrator
	mux           *http.ServeMux
	httpSrv       *http.Server
	wizardStore   *WizardSessionStore
	oc            *opencode.Client
	csrfKey       []byte
}

// generateCSRFToken generates a new random CSRF token
func generateCSRFToken() (string, error) {
	bytes := make([]byte, CSRFTokenLength)
	if _, err := rand.Read(bytes); err != nil {
		return "", err
	}
	return base64.URLEncoding.EncodeToString(bytes), nil
}

// validateCSRFToken validates a CSRF token against the expected value
func validateCSRFToken(token, expected string) bool {
	if token == "" || expected == "" {
		return false
	}
	return subtle.ConstantTimeCompare([]byte(token), []byte(expected)) == 1
}

// csrfMiddleware adds CSRF protection to POST requests
func (s *Server) csrfMiddleware(next http.HandlerFunc) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		// Only validate POST, PUT, DELETE requests
		if r.Method != http.MethodGet && r.Method != http.MethodHead {
			token := r.Header.Get("X-CSRF-Token")
			if token == "" {
				token = r.FormValue("csrf_token")
			}

			// CSRF token is required for all state-changing requests
			if token == "" {
				http.Error(w, "CSRF token required", http.StatusForbidden)
				return
			}

			if !validateCSRFToken(token, string(s.csrfKey)) {
				http.Error(w, "Invalid CSRF token", http.StatusForbidden)
				return
			}
		}
		next(w, r)
	}
}

// chainMiddleware chains multiple middleware functions
func chainMiddleware(handler http.HandlerFunc, middlewares ...func(http.HandlerFunc) http.HandlerFunc) http.HandlerFunc {
	for i := len(middlewares) - 1; i >= 0; i-- {
		handler = middlewares[i](handler)
	}
	return handler
}

func NewServer(port int, store *db.Store, pool func() []worker.WorkerInfo, gh *github.Client, projectNumber int, orchestrator *mvp.Orchestrator, oc *opencode.Client) (*Server, error) {
	tmpls, err := parseTemplates()
	if err != nil {
		return nil, err
	}

	// Generate CSRF key
	csrfKey := make([]byte, CSRFTokenLength)
	if _, err := rand.Read(csrfKey); err != nil {
		return nil, fmt.Errorf("generating CSRF key: %w", err)
	}

	mux := http.NewServeMux()
	s := &Server{
		port:          port,
		tmpls:         tmpls,
		store:         store,
		pool:          pool,
		gh:            gh,
		projectNumber: projectNumber,
		orchestrator:  orchestrator,
		mux:           mux,
		httpSrv: &http.Server{
			Addr:    fmt.Sprintf(":%d", port),
			Handler: mux,
		},
		wizardStore: NewWizardSessionStore(),
		oc:          oc,
		csrfKey:     csrfKey,
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

	pages := []string{"board.html", "backlog.html", "costs.html", "task.html", "wizard_new.html"}
	for _, page := range pages {
		t, err := template.New("").Funcs(funcMap).ParseFS(templateFS, "templates/layout.html", "templates/"+page)
		if err != nil {
			return nil, fmt.Errorf("parsing %s: %w", page, err)
		}
		tmpls[page] = t
	}

	wizardPageTmpl, err := template.New("").Funcs(funcMap).ParseFS(templateFS, "templates/layout.html", "templates/wizard_page.html", "templates/wizard_new.html")
	if err != nil {
		return nil, fmt.Errorf("parsing wizard_page.html: %w", err)
	}
	tmpls["wizard_page.html"] = wizardPageTmpl

	wizardModalTmpl, err := template.New("").Funcs(funcMap).ParseFS(templateFS, "templates/layout.html", "templates/wizard_modal.html", "templates/wizard_new.html")
	if err != nil {
		return nil, fmt.Errorf("parsing wizard_modal.html: %w", err)
	}
	tmpls["wizard_modal.html"] = wizardModalTmpl

	// Parse wizard partial templates (no layout)
	wizardPartials := []string{"wizard_refine.html", "wizard_breakdown.html", "wizard_create.html", "wizard_logs.html"}
	for _, page := range wizardPartials {
		t, err := template.ParseFS(templateFS, "templates/"+page)
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

	// Wizard routes - with CSRF protection
	s.mux.HandleFunc("GET /wizard", s.handleWizardPage)
	s.mux.HandleFunc("GET /wizard/new", s.handleWizardNew)
	s.mux.HandleFunc("GET /wizard/modal", s.handleWizardModal)
	s.mux.HandleFunc("POST /wizard/cancel", s.csrfMiddleware(s.handleWizardCancel))
	s.mux.HandleFunc("POST /wizard/refine", s.csrfMiddleware(s.handleWizardRefine))
	s.mux.HandleFunc("POST /wizard/breakdown", s.csrfMiddleware(s.handleWizardBreakdown))
	s.mux.HandleFunc("POST /wizard/create", s.csrfMiddleware(s.handleWizardCreate))
	s.mux.HandleFunc("GET /wizard/logs/{sessionId}", s.handleWizardLogs)
}

func (s *Server) Start() error {
	return s.httpSrv.ListenAndServe()
}

func (s *Server) Shutdown(ctx context.Context) error {
	return s.httpSrv.Shutdown(ctx)
}

func (s *Server) Handler() http.Handler {
	return s.mux
}
