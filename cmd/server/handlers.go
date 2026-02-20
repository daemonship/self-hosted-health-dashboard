package main

import (
	"html/template"
	"io/fs"
	"log"
	"net/http"

	"health-dashboard/internal/auth"
)

// handleHealth returns a simple JSON status — used by load balancers / Docker HEALTHCHECK.
func (s *server) handleHealth(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "application/json")
	w.Write([]byte(`{"status":"ok"}`))
}

// handleDashboard serves the embedded index.html (placeholder for Task 5 frontend).
func (s *server) handleDashboard(w http.ResponseWriter, r *http.Request) {
	sub, err := fs.Sub(staticFS, "static")
	if err != nil {
		http.Error(w, "internal error", http.StatusInternalServerError)
		return
	}
	http.FileServer(http.FS(sub)).ServeHTTP(w, r)
}

var loginTmpl = template.Must(template.New("login").Parse(`<!DOCTYPE html>
<html lang="en">
<head>
  <meta charset="utf-8">
  <meta name="viewport" content="width=device-width, initial-scale=1">
  <title>Login — Health Dashboard</title>
  <style>
    *, *::before, *::after { box-sizing: border-box; margin: 0; padding: 0; }
    body {
      font-family: -apple-system, BlinkMacSystemFont, "Segoe UI", Roboto, sans-serif;
      background: #0f1117;
      color: #e2e8f0;
      display: flex;
      align-items: center;
      justify-content: center;
      min-height: 100vh;
    }
    .card {
      background: #1a1d27;
      border: 1px solid #2d3148;
      border-radius: 8px;
      padding: 2rem;
      width: 100%;
      max-width: 360px;
    }
    h1 { font-size: 1.25rem; margin-bottom: 1.5rem; color: #f8fafc; }
    label { display: block; font-size: 0.85rem; color: #94a3b8; margin-bottom: 0.35rem; }
    input[type=password] {
      width: 100%;
      padding: 0.6rem 0.75rem;
      background: #0f1117;
      border: 1px solid #2d3148;
      border-radius: 4px;
      color: #e2e8f0;
      font-size: 1rem;
      margin-bottom: 1rem;
    }
    input[type=password]:focus { outline: none; border-color: #6366f1; }
    button {
      width: 100%;
      padding: 0.65rem;
      background: #6366f1;
      color: #fff;
      border: none;
      border-radius: 4px;
      font-size: 1rem;
      cursor: pointer;
    }
    button:hover { background: #4f46e5; }
    .error { color: #f87171; font-size: 0.85rem; margin-bottom: 1rem; }
  </style>
</head>
<body>
  <div class="card">
    <h1>Health Dashboard</h1>
    {{if .Error}}<p class="error">{{.Error}}</p>{{end}}
    <form method="POST" action="/login">
      <label for="password">Password</label>
      <input type="password" id="password" name="password" autofocus autocomplete="current-password">
      <button type="submit">Sign in</button>
    </form>
  </div>
</body>
</html>`))

func (s *server) handleLoginPage(w http.ResponseWriter, r *http.Request) {
	// Already logged in — send to dashboard.
	if s.sessions.Valid(auth.GetSessionToken(r)) {
		http.Redirect(w, r, "/", http.StatusFound)
		return
	}
	loginTmpl.Execute(w, nil)
}

func (s *server) handleLogin(w http.ResponseWriter, r *http.Request) {
	if err := r.ParseForm(); err != nil {
		http.Error(w, "bad request", http.StatusBadRequest)
		return
	}
	password := r.FormValue("password")

	if !auth.CheckPassword(s.cfg.Auth.Password, password) {
		w.WriteHeader(http.StatusUnauthorized)
		loginTmpl.Execute(w, map[string]string{"Error": "Invalid password."})
		return
	}

	token, err := s.sessions.Create()
	if err != nil {
		log.Printf("session create: %v", err)
		http.Error(w, "internal error", http.StatusInternalServerError)
		return
	}
	auth.SetSessionCookie(w, token)
	http.Redirect(w, r, "/", http.StatusFound)
}

func (s *server) handleLogout(w http.ResponseWriter, r *http.Request) {
	token := auth.GetSessionToken(r)
	s.sessions.Delete(token)
	auth.ClearSessionCookie(w)
	http.Redirect(w, r, "/login", http.StatusFound)
}
