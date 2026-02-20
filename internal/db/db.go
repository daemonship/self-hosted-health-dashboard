package db

import (
	"database/sql"
	"fmt"

	_ "modernc.org/sqlite"
)

// Open opens (or creates) the SQLite database with WAL mode enabled.
// MaxOpenConns is set to 1 to serialize writes and avoid SQLITE_BUSY.
func Open(dataDir string) (*sql.DB, error) {
	path := fmt.Sprintf("%s/health.db", dataDir)
	dsn := fmt.Sprintf("file:%s?_journal_mode=WAL&_busy_timeout=5000&_foreign_keys=on", path)
	db, err := sql.Open("sqlite", dsn)
	if err != nil {
		return nil, err
	}
	db.SetMaxOpenConns(1)
	if err := migrate(db); err != nil {
		db.Close()
		return nil, err
	}
	return db, nil
}
