package main

import (
	"encoding/json"
	"net/http"
)

// dashboardMonitor is the per-monitor payload returned by GET /api/dashboard/monitors.
type dashboardMonitor struct {
	ID             int64    `json:"id"`
	Name           string   `json:"name"`
	URL            string   `json:"url"`
	State          string   `json:"state"`
	LastResponseMs *int64   `json:"last_response_ms"`
	Uptime24h      *float64 `json:"uptime_24h"`
}

// handleDashboardMonitors returns all monitors enriched with last response time
// and 24-hour uptime percentage.
func (s *server) handleDashboardMonitors(w http.ResponseWriter, r *http.Request) {
	rows, err := s.db.QueryContext(r.Context(), `
		SELECT
			m.id, m.name, m.url, m.state,
			(SELECT response_time_ms FROM checks
			 WHERE monitor_id = m.id AND is_up = 1
			 ORDER BY checked_at DESC LIMIT 1),
			(SELECT CAST(SUM(is_up) AS REAL) / COUNT(*) * 100
			 FROM checks
			 WHERE monitor_id = m.id
			   AND checked_at >= datetime('now', '-24 hours'))
		FROM monitors m
		ORDER BY m.id
	`)
	if err != nil {
		jsonErr(w, "database error", http.StatusInternalServerError)
		return
	}
	defer rows.Close()

	result := make([]dashboardMonitor, 0)
	for rows.Next() {
		var m dashboardMonitor
		if err := rows.Scan(&m.ID, &m.Name, &m.URL, &m.State, &m.LastResponseMs, &m.Uptime24h); err != nil {
			jsonErr(w, "scan error", http.StatusInternalServerError)
			return
		}
		result = append(result, m)
	}
	if err := rows.Err(); err != nil {
		jsonErr(w, "database error", http.StatusInternalServerError)
		return
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(result)
}

// metricPoint is a single timeseries entry for the metrics chart.
type metricPoint struct {
	Ts         int64   `json:"ts"`
	CPUPercent float64 `json:"cpu_percent"`
	MemUsed    int64   `json:"mem_used"`
	MemTotal   int64   `json:"mem_total"`
}

// diskInfo is a parsed disk entry from the metrics disk_json column.
type diskInfo struct {
	Mount string `json:"mount"`
	Used  int64  `json:"used"`
	Total int64  `json:"total"`
}

// latestMetrics is the most recent snapshot used for the gauges.
type latestMetrics struct {
	CPUPercent float64    `json:"cpu_percent"`
	MemUsed    int64      `json:"mem_used"`
	MemTotal   int64      `json:"mem_total"`
	Disks      []diskInfo `json:"disks"`
}

// metricsResponse is returned by GET /api/dashboard/metrics.
type metricsResponse struct {
	Latest *latestMetrics `json:"latest"`
	Series []metricPoint  `json:"series"`
}

// handleDashboardMetrics returns the last 24 h of system metrics and the
// most-recent snapshot for gauges.
func (s *server) handleDashboardMetrics(w http.ResponseWriter, r *http.Request) {
	rows, err := s.db.QueryContext(r.Context(), `
		SELECT strftime('%s', recorded_at), cpu_percent, mem_used, mem_total, disk_json
		FROM metrics
		WHERE recorded_at >= datetime('now', '-24 hours')
		ORDER BY recorded_at ASC
	`)
	if err != nil {
		jsonErr(w, "database error", http.StatusInternalServerError)
		return
	}
	defer rows.Close()

	var series []metricPoint
	var lastDiskJSON string
	var latest *latestMetrics

	for rows.Next() {
		var ts int64
		var cpu float64
		var memUsed, memTotal int64
		var diskJSON string
		if err := rows.Scan(&ts, &cpu, &memUsed, &memTotal, &diskJSON); err != nil {
			jsonErr(w, "scan error", http.StatusInternalServerError)
			return
		}
		series = append(series, metricPoint{
			Ts:         ts,
			CPUPercent: cpu,
			MemUsed:    memUsed,
			MemTotal:   memTotal,
		})
		lastDiskJSON = diskJSON
		latest = &latestMetrics{
			CPUPercent: cpu,
			MemUsed:    memUsed,
			MemTotal:   memTotal,
		}
	}
	if err := rows.Err(); err != nil {
		jsonErr(w, "database error", http.StatusInternalServerError)
		return
	}

	if latest != nil {
		var disks []diskInfo
		if err := json.Unmarshal([]byte(lastDiskJSON), &disks); err == nil {
			latest.Disks = disks
		}
		if latest.Disks == nil {
			latest.Disks = []diskInfo{}
		}
	}

	resp := metricsResponse{
		Latest: latest,
		Series: series,
	}
	if resp.Series == nil {
		resp.Series = []metricPoint{}
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(resp)
}

// handleDashboardEvents returns the same event summary as the API-key-gated
// endpoint but accepts a session cookie — used by the dashboard frontend.
func (s *server) handleDashboardEvents(w http.ResponseWriter, r *http.Request) {
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
		jsonErr(w, "database error", http.StatusInternalServerError)
		return
	}
	defer rows.Close()

	summaries := make([]EventSummary, 0)
	for rows.Next() {
		var es EventSummary
		if err := rows.Scan(&es.EventName, &es.Today, &es.Last7Days); err != nil {
			jsonErr(w, "scan error", http.StatusInternalServerError)
			return
		}
		summaries = append(summaries, es)
	}
	if err := rows.Err(); err != nil {
		jsonErr(w, "database error", http.StatusInternalServerError)
		return
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(summaries)
}

// jsonErr writes a JSON error response.
func jsonErr(w http.ResponseWriter, msg string, code int) {
	w.Header().Set("Content-Type", "application/json")
	http.Error(w, `{"error":"`+msg+`"}`, code)
}
