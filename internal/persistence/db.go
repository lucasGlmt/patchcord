// Package persistence manages the agent's SQLite database: opening it with
// the pragmas Patchcord relies on, and applying versioned migrations.
package persistence

import (
	"database/sql"
	"fmt"
	"os"
	"path/filepath"

	_ "modernc.org/sqlite"
)

const fileName = "patchcord.db"

// Open creates dataDir if needed and opens the agent's SQLite database
// inside it, with foreign keys enforced, WAL journaling, and a busy timeout
// so concurrent access waits instead of failing immediately.
func Open(dataDir string) (*sql.DB, error) {
	if err := os.MkdirAll(dataDir, 0o755); err != nil {
		return nil, fmt.Errorf("create data directory %q: %w", dataDir, err)
	}

	dbPath := filepath.Join(dataDir, fileName)
	dsn := fmt.Sprintf("file:%s?_journal_mode=WAL&_foreign_keys=1&_busy_timeout=5000", dbPath)

	db, err := sql.Open("sqlite", dsn)
	if err != nil {
		return nil, fmt.Errorf("open database %q: %w", dbPath, err)
	}

	// SQLite allows only one writer at a time. Serializing all access
	// through a single connection avoids "database is locked" errors from
	// database/sql's connection pool; revisit once a workload needs more
	// read concurrency than one connection can give.
	db.SetMaxOpenConns(1)

	if err := db.Ping(); err != nil {
		_ = db.Close()
		return nil, fmt.Errorf("ping database %q: %w", dbPath, err)
	}

	return db, nil
}
