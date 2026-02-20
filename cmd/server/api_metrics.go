package main

import (
	"encoding/json"
	"net/http"
)

// handleMetricsPost handles POST /api/metrics.
// Authenticated via the X-Agent-Token header (shared secret from config.yaml).
func (s *server) handleMetricsPost(w http.ResponseWriter, r *http.Request) {
	token := r.Header.Get("X-Agent-Token")
	if token == "" || token != s.cfg.Agent.Token {
		w.Header().Set("Content-Type", "application/json")
		http.Error(w, `{"error":"unauthorized"}`, http.StatusUnauthorized)
		return
	}

	var payload struct {
		CPUPercent float64 `json:"cpu_percent"`
		MemUsed    int64   `json:"mem_used"`
		MemTotal   int64   `json:"mem_total"`
		Disks      []struct {
			Mount string `json:"mount"`
			Used  int64  `json:"used"`
			Total int64  `json:"total"`
		} `json:"disks"`
	}
	if err := json.NewDecoder(r.Body).Decode(&payload); err != nil {
		w.Header().Set("Content-Type", "application/json")
		http.Error(w, `{"error":"invalid JSON"}`, http.StatusBadRequest)
		return
	}

	diskJSON, err := json.Marshal(payload.Disks)
	if err != nil {
		w.Header().Set("Content-Type", "application/json")
		http.Error(w, `{"error":"internal error"}`, http.StatusInternalServerError)
		return
	}

	_, err = s.db.ExecContext(r.Context(),
		`INSERT INTO metrics (cpu_percent, mem_used, mem_total, disk_json) VALUES (?, ?, ?, ?)`,
		payload.CPUPercent, payload.MemUsed, payload.MemTotal, string(diskJSON),
	)
	if err != nil {
		w.Header().Set("Content-Type", "application/json")
		http.Error(w, `{"error":"database error"}`, http.StatusInternalServerError)
		return
	}

	w.WriteHeader(http.StatusNoContent)
}
