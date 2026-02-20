package main

import (
	"database/sql"
	"net/http"

	"health-dashboard/internal/auth"
	"health-dashboard/internal/config"
	"health-dashboard/internal/monitor"
)

type server struct {
	cfg      *config.Config
	db       *sql.DB
	sessions *auth.Store
	monitors *monitor.Store
	checker  *monitor.Checker
}

func (s *server) routes() *http.ServeMux {
	mux := http.NewServeMux()

	// Public endpoints
	mux.HandleFunc("GET /health", s.handleHealth)
	mux.HandleFunc("GET /login", s.handleLoginPage)
	mux.HandleFunc("POST /login", s.handleLogin)
	mux.HandleFunc("POST /logout", s.handleLogout)

	// Metrics ingestion (agent token auth — no session required)
	mux.HandleFunc("POST /api/metrics", s.handleMetricsPost)

	// Monitor CRUD API (session auth)
	mux.HandleFunc("POST /api/monitors", s.requireAuthAPI(s.handleMonitorCreate))
	mux.HandleFunc("GET /api/monitors", s.requireAuthAPI(s.handleMonitorList))
	mux.HandleFunc("GET /api/monitors/{id}", s.requireAuthAPI(s.handleMonitorGet))
	mux.HandleFunc("PUT /api/monitors/{id}", s.requireAuthAPI(s.handleMonitorUpdate))
	mux.HandleFunc("DELETE /api/monitors/{id}", s.requireAuthAPI(s.handleMonitorDelete))
	mux.HandleFunc("GET /api/monitors/{id}/checks", s.requireAuthAPI(s.handleMonitorChecks))

	// Protected dashboard (must be last — it's the catch-all)
	mux.HandleFunc("GET /", s.requireAuth(s.handleDashboard))

	return mux
}

// requireAuth wraps a handler to redirect unauthenticated requests to /login.
func (s *server) requireAuth(next http.HandlerFunc) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		token := auth.GetSessionToken(r)
		if !s.sessions.Valid(token) {
			http.Redirect(w, r, "/login", http.StatusFound)
			return
		}
		next(w, r)
	}
}

// requireAuthAPI wraps a handler to return 401 JSON for unauthenticated API requests.
func (s *server) requireAuthAPI(next http.HandlerFunc) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		token := auth.GetSessionToken(r)
		if !s.sessions.Valid(token) {
			w.Header().Set("Content-Type", "application/json")
			http.Error(w, `{"error":"unauthorized"}`, http.StatusUnauthorized)
			return
		}
		next(w, r)
	}
}
