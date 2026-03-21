package dashboard

import (
	"embed"
	"fmt"
	"html/template"
	"net/http"

	"github.com/one-dev-army/oda/internal/db"
	"github.com/one-dev-army/oda/internal/worker"
)

//go:embed templates/*.html
var templateFS embed.FS

type Server struct {
	port  int
	tmpls map[string]*template.Template
	store *db.Store
	pool  func() []worker.WorkerInfo
	mux   *http.ServeMux
}

func NewServer(port int, store *db.Store, pool func() []worker.WorkerInfo) (*Server, error) {
	tmpls, err := parseTemplates()
	if err != nil {
		return nil, err
	}

	s := &Server{
		port:  port,
		tmpls: tmpls,
		store: store,
		pool:  pool,
		mux:   http.NewServeMux(),
	}
	s.routes()
	return s, nil
}

func parseTemplates() (map[string]*template.Template, error) {
	tmpls := make(map[string]*template.Template)

	pages := []string{"board.html", "backlog.html", "costs.html"}
	for _, page := range pages {
		t, err := template.ParseFS(templateFS, "templates/layout.html", "templates/"+page)
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
	s.mux.HandleFunc("POST /epic", s.handleAddEpic)
	s.mux.HandleFunc("POST /sync", s.handleSync)
	s.mux.HandleFunc("POST /plan-sprint", s.handlePlanSprint)
	s.mux.HandleFunc("POST /approve/{id}", s.handleApprove)
	s.mux.HandleFunc("POST /reject/{id}", s.handleReject)
}

func (s *Server) Start() error {
	addr := fmt.Sprintf(":%d", s.port)
	return http.ListenAndServe(addr, s.mux)
}

func (s *Server) Handler() http.Handler {
	return s.mux
}
