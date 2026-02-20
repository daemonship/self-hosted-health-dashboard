package main

import (
	"encoding/json"
	"net/http"
	"strconv"
	"strings"

	"health-dashboard/internal/monitor"
)

// handleMonitorCreate handles POST /api/monitors.
func (s *server) handleMonitorCreate(w http.ResponseWriter, r *http.Request) {
	var req struct {
		Name            string `json:"name"`
		URL             string `json:"url"`
		IntervalSeconds int    `json:"interval_seconds"`
		TimeoutSeconds  int    `json:"timeout_seconds"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		http.Error(w, "invalid JSON", http.StatusBadRequest)
		return
	}
	if strings.TrimSpace(req.Name) == "" || strings.TrimSpace(req.URL) == "" {
		http.Error(w, `{"error":"name and url are required"}`, http.StatusBadRequest)
		return
	}
	if req.IntervalSeconds <= 0 {
		req.IntervalSeconds = 60
	}
	if req.TimeoutSeconds <= 0 {
		req.TimeoutSeconds = 10
	}

	m := &monitor.Monitor{
		Name:            strings.TrimSpace(req.Name),
		URL:             strings.TrimSpace(req.URL),
		IntervalSeconds: req.IntervalSeconds,
		TimeoutSeconds:  req.TimeoutSeconds,
	}
	if err := s.monitors.Create(m); err != nil {
		http.Error(w, `{"error":"database error"}`, http.StatusInternalServerError)
		return
	}
	s.checker.Add(r.Context(), m)

	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusCreated)
	json.NewEncoder(w).Encode(m)
}

// handleMonitorList handles GET /api/monitors.
func (s *server) handleMonitorList(w http.ResponseWriter, r *http.Request) {
	monitors, err := s.monitors.List()
	if err != nil {
		http.Error(w, `{"error":"database error"}`, http.StatusInternalServerError)
		return
	}
	if monitors == nil {
		monitors = []*monitor.Monitor{}
	}
	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(monitors)
}

// handleMonitorGet handles GET /api/monitors/{id}.
func (s *server) handleMonitorGet(w http.ResponseWriter, r *http.Request) {
	id, ok := parseMonitorID(w, r)
	if !ok {
		return
	}
	m, err := s.monitors.Get(id)
	if err != nil {
		http.Error(w, `{"error":"database error"}`, http.StatusInternalServerError)
		return
	}
	if m == nil {
		http.Error(w, `{"error":"not found"}`, http.StatusNotFound)
		return
	}
	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(m)
}

// handleMonitorUpdate handles PUT /api/monitors/{id}.
func (s *server) handleMonitorUpdate(w http.ResponseWriter, r *http.Request) {
	id, ok := parseMonitorID(w, r)
	if !ok {
		return
	}
	existing, err := s.monitors.Get(id)
	if err != nil {
		http.Error(w, `{"error":"database error"}`, http.StatusInternalServerError)
		return
	}
	if existing == nil {
		http.Error(w, `{"error":"not found"}`, http.StatusNotFound)
		return
	}

	var req struct {
		Name            string `json:"name"`
		URL             string `json:"url"`
		IntervalSeconds int    `json:"interval_seconds"`
		TimeoutSeconds  int    `json:"timeout_seconds"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		http.Error(w, "invalid JSON", http.StatusBadRequest)
		return
	}

	// Apply only provided fields.
	if n := strings.TrimSpace(req.Name); n != "" {
		existing.Name = n
	}
	if u := strings.TrimSpace(req.URL); u != "" {
		existing.URL = u
	}
	if req.IntervalSeconds > 0 {
		existing.IntervalSeconds = req.IntervalSeconds
	}
	if req.TimeoutSeconds > 0 {
		existing.TimeoutSeconds = req.TimeoutSeconds
	}

	if err := s.monitors.Update(existing); err != nil {
		http.Error(w, `{"error":"database error"}`, http.StatusInternalServerError)
		return
	}
	s.checker.Restart(r.Context(), existing)

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(existing)
}

// handleMonitorDelete handles DELETE /api/monitors/{id}.
func (s *server) handleMonitorDelete(w http.ResponseWriter, r *http.Request) {
	id, ok := parseMonitorID(w, r)
	if !ok {
		return
	}
	s.checker.Remove(id)
	if err := s.monitors.Delete(id); err != nil {
		http.Error(w, `{"error":"database error"}`, http.StatusInternalServerError)
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

// handleMonitorChecks handles GET /api/monitors/{id}/checks.
func (s *server) handleMonitorChecks(w http.ResponseWriter, r *http.Request) {
	id, ok := parseMonitorID(w, r)
	if !ok {
		return
	}
	m, err := s.monitors.Get(id)
	if err != nil {
		http.Error(w, `{"error":"database error"}`, http.StatusInternalServerError)
		return
	}
	if m == nil {
		http.Error(w, `{"error":"not found"}`, http.StatusNotFound)
		return
	}

	checks, err := s.monitors.RecentChecks(id, 100)
	if err != nil {
		http.Error(w, `{"error":"database error"}`, http.StatusInternalServerError)
		return
	}
	if checks == nil {
		checks = []*monitor.Check{}
	}
	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(checks)
}

// parseMonitorID extracts and validates the {id} path value from r.
func parseMonitorID(w http.ResponseWriter, r *http.Request) (int64, bool) {
	idStr := r.PathValue("id")
	id, err := strconv.ParseInt(idStr, 10, 64)
	if err != nil || id <= 0 {
		http.Error(w, `{"error":"invalid id"}`, http.StatusBadRequest)
		return 0, false
	}
	return id, true
}
