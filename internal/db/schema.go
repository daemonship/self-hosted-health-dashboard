package db

import "database/sql"

const schema = `
CREATE TABLE IF NOT EXISTS monitors (
    id                   INTEGER PRIMARY KEY AUTOINCREMENT,
    name                 TEXT    NOT NULL,
    url                  TEXT    NOT NULL,
    interval_seconds     INTEGER NOT NULL DEFAULT 60,
    timeout_seconds      INTEGER NOT NULL DEFAULT 10,
    state                TEXT    NOT NULL DEFAULT 'unknown',
    consecutive_failures INTEGER NOT NULL DEFAULT 0,
    created_at           DATETIME NOT NULL DEFAULT (datetime('now')),
    updated_at           DATETIME NOT NULL DEFAULT (datetime('now'))
);

CREATE TABLE IF NOT EXISTS checks (
    id               INTEGER PRIMARY KEY AUTOINCREMENT,
    monitor_id       INTEGER NOT NULL REFERENCES monitors(id) ON DELETE CASCADE,
    checked_at       DATETIME NOT NULL DEFAULT (datetime('now')),
    status_code      INTEGER,
    response_time_ms INTEGER,
    is_up            INTEGER NOT NULL DEFAULT 0
);
CREATE INDEX IF NOT EXISTS idx_checks_monitor_checked ON checks(monitor_id, checked_at);

-- Prune checks older than 7 days via a scheduled ticker in the server.
-- Trigger kept as a belt-and-suspenders safety net.
CREATE TRIGGER IF NOT EXISTS prune_old_checks
    AFTER INSERT ON checks
BEGIN
    DELETE FROM checks
    WHERE checked_at < datetime('now', '-7 days');
END;

CREATE TABLE IF NOT EXISTS metrics (
    id          INTEGER PRIMARY KEY AUTOINCREMENT,
    recorded_at DATETIME NOT NULL DEFAULT (datetime('now')),
    cpu_percent REAL    NOT NULL DEFAULT 0,
    mem_used    INTEGER NOT NULL DEFAULT 0,
    mem_total   INTEGER NOT NULL DEFAULT 0,
    disk_json   TEXT    NOT NULL DEFAULT '[]'
);
CREATE INDEX IF NOT EXISTS idx_metrics_recorded_at ON metrics(recorded_at);

CREATE TRIGGER IF NOT EXISTS prune_old_metrics
    AFTER INSERT ON metrics
BEGIN
    DELETE FROM metrics
    WHERE recorded_at < datetime('now', '-7 days');
END;

CREATE TABLE IF NOT EXISTS events (
    id         INTEGER PRIMARY KEY AUTOINCREMENT,
    event_name TEXT    NOT NULL,
    value      REAL    NOT NULL DEFAULT 1,
    created_at DATETIME NOT NULL DEFAULT (datetime('now'))
);
CREATE INDEX IF NOT EXISTS idx_events_name_created ON events(event_name, created_at);

CREATE TRIGGER IF NOT EXISTS prune_old_events
    AFTER INSERT ON events
BEGIN
    DELETE FROM events
    WHERE created_at < datetime('now', '-7 days');
END;
`

func migrate(db *sql.DB) error {
	_, err := db.Exec(schema)
	return err
}
