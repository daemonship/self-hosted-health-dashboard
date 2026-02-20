package monitor

import (
	"database/sql"
	"time"
)

// Monitor represents a configured uptime check target.
type Monitor struct {
	ID                  int64     `json:"id"`
	Name                string    `json:"name"`
	URL                 string    `json:"url"`
	IntervalSeconds     int       `json:"interval_seconds"`
	TimeoutSeconds      int       `json:"timeout_seconds"`
	State               string    `json:"state"`
	ConsecutiveFailures int       `json:"consecutive_failures"`
	CreatedAt           time.Time `json:"created_at"`
	UpdatedAt           time.Time `json:"updated_at"`
}

// Check is a single HTTP probe result.
type Check struct {
	ID             int64     `json:"id"`
	MonitorID      int64     `json:"monitor_id"`
	CheckedAt      time.Time `json:"checked_at"`
	StatusCode     *int      `json:"status_code"`
	ResponseTimeMs *int      `json:"response_time_ms"`
	IsUp           bool      `json:"is_up"`
}

// Store provides monitor and check DB operations.
type Store struct {
	db *sql.DB
}

// NewStore creates a Store backed by db.
func NewStore(db *sql.DB) *Store {
	return &Store{db: db}
}

const monitorCols = `id, name, url, interval_seconds, timeout_seconds, state, consecutive_failures, created_at, updated_at`

func scanMonitor(row interface{ Scan(...any) error }) (*Monitor, error) {
	m := &Monitor{}
	err := row.Scan(&m.ID, &m.Name, &m.URL, &m.IntervalSeconds, &m.TimeoutSeconds,
		&m.State, &m.ConsecutiveFailures, &m.CreatedAt, &m.UpdatedAt)
	return m, err
}

// Create inserts m into the DB and populates its ID, State, and timestamps.
func (s *Store) Create(m *Monitor) error {
	const q = `
		INSERT INTO monitors (name, url, interval_seconds, timeout_seconds)
		VALUES (?, ?, ?, ?)
		RETURNING ` + monitorCols
	row := s.db.QueryRow(q, m.Name, m.URL, m.IntervalSeconds, m.TimeoutSeconds)
	result, err := scanMonitor(row)
	if err != nil {
		return err
	}
	*m = *result
	return nil
}

// List returns all monitors ordered by ID.
func (s *Store) List() ([]*Monitor, error) {
	rows, err := s.db.Query(`SELECT ` + monitorCols + ` FROM monitors ORDER BY id`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var monitors []*Monitor
	for rows.Next() {
		m, err := scanMonitor(rows)
		if err != nil {
			return nil, err
		}
		monitors = append(monitors, m)
	}
	return monitors, rows.Err()
}

// Get returns the monitor with the given ID, or nil if not found.
func (s *Store) Get(id int64) (*Monitor, error) {
	row := s.db.QueryRow(`SELECT `+monitorCols+` FROM monitors WHERE id = ?`, id)
	m, err := scanMonitor(row)
	if err == sql.ErrNoRows {
		return nil, nil
	}
	return m, err
}

// Update writes m's mutable fields back to the DB.
// Returns sql.ErrNoRows if the ID does not exist.
func (s *Store) Update(m *Monitor) error {
	res, err := s.db.Exec(`
		UPDATE monitors
		SET name = ?, url = ?, interval_seconds = ?, timeout_seconds = ?, updated_at = datetime('now')
		WHERE id = ?`,
		m.Name, m.URL, m.IntervalSeconds, m.TimeoutSeconds, m.ID)
	if err != nil {
		return err
	}
	n, _ := res.RowsAffected()
	if n == 0 {
		return sql.ErrNoRows
	}
	return nil
}

// Delete removes the monitor and its checks (cascade) from the DB.
func (s *Store) Delete(id int64) error {
	_, err := s.db.Exec(`DELETE FROM monitors WHERE id = ?`, id)
	return err
}

// UpdateState sets the monitor's state and consecutive_failures counter.
func (s *Store) UpdateState(monitorID int64, state string, consecutiveFailures int) error {
	_, err := s.db.Exec(`
		UPDATE monitors
		SET state = ?, consecutive_failures = ?, updated_at = datetime('now')
		WHERE id = ?`,
		state, consecutiveFailures, monitorID)
	return err
}

// RecordCheck inserts a probe result into the checks table.
func (s *Store) RecordCheck(c *Check) error {
	_, err := s.db.Exec(`
		INSERT INTO checks (monitor_id, status_code, response_time_ms, is_up)
		VALUES (?, ?, ?, ?)`,
		c.MonitorID, c.StatusCode, c.ResponseTimeMs, boolToInt(c.IsUp))
	return err
}

// RecentChecks returns the most recent limit checks for monitorID, newest first.
func (s *Store) RecentChecks(monitorID int64, limit int) ([]*Check, error) {
	rows, err := s.db.Query(`
		SELECT id, monitor_id, checked_at, status_code, response_time_ms, is_up
		FROM checks
		WHERE monitor_id = ?
		ORDER BY checked_at DESC
		LIMIT ?`, monitorID, limit)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var checks []*Check
	for rows.Next() {
		c := &Check{}
		var isUp int
		if err := rows.Scan(&c.ID, &c.MonitorID, &c.CheckedAt, &c.StatusCode, &c.ResponseTimeMs, &isUp); err != nil {
			return nil, err
		}
		c.IsUp = isUp == 1
		checks = append(checks, c)
	}
	return checks, rows.Err()
}

// PruneOldChecks deletes checks older than 7 days.
func (s *Store) PruneOldChecks() error {
	_, err := s.db.Exec(`DELETE FROM checks WHERE checked_at < datetime('now', '-7 days')`)
	return err
}

func boolToInt(b bool) int {
	if b {
		return 1
	}
	return 0
}
