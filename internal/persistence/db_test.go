package persistence

import (
	"os"
	"path/filepath"
	"testing"
)

func TestOpen(t *testing.T) {
	t.Run("creates the data directory and the database file", func(t *testing.T) {
		dataDir := filepath.Join(t.TempDir(), "nested")

		db, err := Open(dataDir)
		if err != nil {
			t.Fatalf("Open() error = %v", err)
		}
		defer db.Close()

		if err := db.Ping(); err != nil {
			t.Fatalf("Ping() error = %v", err)
		}
	})

	t.Run("fails when the data directory cannot be created", func(t *testing.T) {
		// A regular file cannot be used as a parent directory.
		blocker := filepath.Join(t.TempDir(), "blocker")
		if err := os.WriteFile(blocker, []byte("x"), 0o644); err != nil {
			t.Fatalf("write blocker file: %v", err)
		}

		if _, err := Open(filepath.Join(blocker, "data")); err == nil {
			t.Fatal("expected an error, got nil")
		}
	})
}
