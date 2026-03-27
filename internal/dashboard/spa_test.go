package dashboard

import (
	"net/http"
	"net/http/httptest"
	"testing"
	"testing/fstest"
)

func TestServeSPA_StaticFile(t *testing.T) {
	spaFS := fstest.MapFS{
		"index.html":     {Data: []byte("<html>SPA Root</html>")},
		"assets/app.js":  {Data: []byte("console.log('app')")},
		"assets/app.css": {Data: []byte("body{}")},
	}

	srv := &Server{
		mux:   http.NewServeMux(),
		spaFS: spaFS,
	}
	srv.serveSPA()

	tests := []struct {
		name       string
		path       string
		wantStatus int
		wantBody   string
	}{
		{
			name:       "index.html at root",
			path:       "/new/",
			wantStatus: http.StatusOK,
			wantBody:   "SPA Root",
		},
		{
			name:       "JS asset",
			path:       "/new/assets/app.js",
			wantStatus: http.StatusOK,
			wantBody:   "console.log('app')",
		},
		{
			name:       "CSS asset",
			path:       "/new/assets/app.css",
			wantStatus: http.StatusOK,
			wantBody:   "body{}",
		},
		{
			name:       "SPA fallback for unknown route",
			path:       "/new/settings",
			wantStatus: http.StatusOK,
			wantBody:   "SPA Root",
		},
		{
			name:       "SPA fallback for nested route",
			path:       "/new/issues/42",
			wantStatus: http.StatusOK,
			wantBody:   "SPA Root",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			req := httptest.NewRequest(http.MethodGet, tt.path, nil)
			rec := httptest.NewRecorder()
			srv.mux.ServeHTTP(rec, req)

			if rec.Code != tt.wantStatus {
				t.Errorf("GET %s: status = %d, want %d", tt.path, rec.Code, tt.wantStatus)
			}

			body := rec.Body.String()
			if len(body) == 0 || !contains(body, tt.wantBody) {
				t.Errorf("GET %s: body does not contain %q, got %q", tt.path, tt.wantBody, body)
			}
		})
	}
}

func TestServeSPA_NilFS(t *testing.T) {
	srv := &Server{
		mux:   http.NewServeMux(),
		spaFS: nil,
	}
	srv.serveSPA()

	// With nil spaFS, /new/ should not be registered — expect 404
	req := httptest.NewRequest(http.MethodGet, "/new/", nil)
	rec := httptest.NewRecorder()
	srv.mux.ServeHTTP(rec, req)

	if rec.Code != http.StatusNotFound {
		t.Errorf("GET /new/ with nil spaFS: status = %d, want %d", rec.Code, http.StatusNotFound)
	}
}

func TestServeSPA_ContentType(t *testing.T) {
	spaFS := fstest.MapFS{
		"index.html":     {Data: []byte("<!DOCTYPE html><html></html>")},
		"assets/app.css": {Data: []byte("body{}")},
	}

	srv := &Server{
		mux:   http.NewServeMux(),
		spaFS: spaFS,
	}
	srv.serveSPA()

	t.Run("fallback returns text/html", func(t *testing.T) {
		req := httptest.NewRequest(http.MethodGet, "/new/some/route", nil)
		rec := httptest.NewRecorder()
		srv.mux.ServeHTTP(rec, req)

		ct := rec.Header().Get("Content-Type")
		if ct != "text/html; charset=utf-8" {
			t.Errorf("SPA fallback Content-Type = %q, want %q", ct, "text/html; charset=utf-8")
		}
	})
}

func contains(s, substr string) bool {
	return len(s) >= len(substr) && (s == substr || len(s) > 0 && containsSubstring(s, substr))
}

func containsSubstring(s, substr string) bool {
	for i := 0; i <= len(s)-len(substr); i++ {
		if s[i:i+len(substr)] == substr {
			return true
		}
	}
	return false
}
