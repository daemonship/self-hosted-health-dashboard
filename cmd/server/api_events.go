package main

import (
	"encoding/json"
	"net/http"
)

// requireAPIKey is a middleware that checks the X-API-Key header against the
// configured events API key.
func (s *server) requireAPIKey(next http.HandlerFunc) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		key := r.Header.Get("X-API-Key")
		if key == "" || key != s.cfg.Events.APIKey {
			w.Header().Set("Content-Type", "application/json")
			http.Error(w, `{"error":"unauthorized"}`, http.StatusUnauthorized)
			return
		}
		next(w, r)
	}
}

// handleEventPost handles POST /api/events.
// Body: {"event_name": "signup", "value": 1}
// value is optional and defaults to 1.
func (s *server) handleEventPost(w http.ResponseWriter, r *http.Request) {
	var payload struct {
		EventName string   `json:"event_name"`
		Value     *float64 `json:"value"`
	}
	if err := json.NewDecoder(r.Body).Decode(&payload); err != nil {
		w.Header().Set("Content-Type", "application/json")
		http.Error(w, `{"error":"invalid JSON"}`, http.StatusBadRequest)
		return
	}
	if payload.EventName == "" {
		w.Header().Set("Content-Type", "application/json")
		http.Error(w, `{"error":"event_name is required"}`, http.StatusBadRequest)
		return
	}

	value := 1.0
	if payload.Value != nil {
		value = *payload.Value
	}

	_, err := s.db.ExecContext(r.Context(),
		`INSERT INTO events (event_name, value) VALUES (?, ?)`,
		payload.EventName, value,
	)
	if err != nil {
		w.Header().Set("Content-Type", "application/json")
		http.Error(w, `{"error":"database error"}`, http.StatusInternalServerError)
		return
	}

	w.WriteHeader(http.StatusNoContent)
}

// EventSummary is the per-event summary returned by GET /api/events/summary.
type EventSummary struct {
	EventName string  `json:"event_name"`
	Today     float64 `json:"today"`
	Last7Days float64 `json:"last_7_days"`
}

// handleEventSummary handles GET /api/events/summary.
// Returns per-event totals for today and the trailing 7 days.
func (s *server) handleEventSummary(w http.ResponseWriter, r *http.Request) {
	rows, err := s.db.QueryContext(r.Context(), `
		SELECT
			event_name,
			SUM(CASE WHEN created_at >= datetime('now', 'start of day') THEN value ELSE 0 END) AS today,
			SUM(value) AS last_7_days
		FROM events
		WHERE created_at >= datetime('now', '-7 days')
		GROUP BY event_name
		ORDER BY event_name
	`)
	if err != nil {
		w.Header().Set("Content-Type", "application/json")
		http.Error(w, `{"error":"database error"}`, http.StatusInternalServerError)
		return
	}
	defer rows.Close()

	summaries := make([]EventSummary, 0)
	for rows.Next() {
		var es EventSummary
		if err := rows.Scan(&es.EventName, &es.Today, &es.Last7Days); err != nil {
			w.Header().Set("Content-Type", "application/json")
			http.Error(w, `{"error":"scan error"}`, http.StatusInternalServerError)
			return
		}
		summaries = append(summaries, es)
	}
	if err := rows.Err(); err != nil {
		w.Header().Set("Content-Type", "application/json")
		http.Error(w, `{"error":"database error"}`, http.StatusInternalServerError)
		return
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(summaries)
}
