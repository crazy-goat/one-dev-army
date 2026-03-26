package dashboard

import (
	"io/fs"
	"log"
	"net/http"
	"strings"

	"github.com/crazy-goat/one-dev-army/web"
)

// serveReactApp serves the React SPA index.html for all /new/* routes.
func (*Server) serveReactApp(w http.ResponseWriter, _ *http.Request) {
	data, err := web.DistFS.ReadFile("dist/index.html")
	if err != nil {
		log.Printf("[Dashboard] Failed to read React index.html: %v", err)
		http.Error(w, "New dashboard not built. Run: cd web && npm run build", http.StatusServiceUnavailable)
		return
	}

	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	w.Header().Set("Cache-Control", "no-cache")
	_, _ = w.Write(data)
}

// registerReactRoutes registers the routes for serving the React SPA.
func (s *Server) registerReactRoutes() {
	webDist, err := fs.Sub(web.DistFS, "dist")
	if err != nil {
		log.Printf("[Dashboard] Failed to create sub-filesystem for React assets: %v", err)
		return
	}

	// Serve /new/assets/* as static files with long cache
	fileServer := http.FileServer(http.FS(webDist))
	s.mux.HandleFunc("GET /new/assets/{file...}", func(w http.ResponseWriter, r *http.Request) {
		// Strip /new/ prefix so the file server finds files correctly
		r.URL.Path = strings.TrimPrefix(r.URL.Path, "/new")
		w.Header().Set("Cache-Control", "public, max-age=31536000, immutable")
		fileServer.ServeHTTP(w, r)
	})

	// SPA fallback: serve index.html for all /new/* routes
	s.mux.HandleFunc("GET /new/{$}", s.serveReactApp)
	s.mux.HandleFunc("GET /new/{path...}", s.serveReactApp)
}
