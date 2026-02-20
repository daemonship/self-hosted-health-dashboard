package main

import (
	"database/sql"
	"net/http"

	"health-dashboard/internal/auth"
	"health-dashboard/internal/config"
)

type server struct {
	cfg      *config.Config
	db       *sql.DB
	sessions *auth.Store
}

func (s *server) routes() *http.ServeMux {
	mux := http.NewServeMux()

	// Public endpoints
	mux.HandleFunc("GET /health", s.handleHealth)
	mux.HandleFunc("GET /login", s.handleLoginPage)
	mux.HandleFunc("POST /login", s.handleLogin)
	mux.HandleFunc("POST /logout", s.handleLogout)

	// Protected dashboard
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
