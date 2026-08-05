package mcpserver

import (
	"context"
	"database/sql"
	"encoding/json"
	"io"
	"log/slog"
	"testing"

	"github.com/lucasglmt/patchcord/internal/persistence"
	"github.com/lucasglmt/patchcord/internal/plugins"
	"github.com/lucasglmt/patchcord/migrations"
)

// openTestDB returns a freshly migrated, empty database, ready for the
// tools under test.
func openTestDB(t *testing.T) *sql.DB {
	t.Helper()

	db, err := persistence.Open(t.TempDir())
	if err != nil {
		t.Fatalf("persistence.Open() error = %v", err)
	}
	t.Cleanup(func() { _ = db.Close() })

	logger := slog.New(slog.NewTextHandler(io.Discard, nil))
	if err := persistence.Migrate(context.Background(), db, migrations.FS, logger); err != nil {
		t.Fatalf("persistence.Migrate() error = %v", err)
	}

	return db
}

// seedPlugin inserts one plugin catalog entry directly — the same on-disk
// shape plugins.Install records after a real handshake — so a test can
// exercise this package's tools without launching an actual plugin
// process. Mirrors internal/api/connectors_test.go's insertTestPlugin, but
// keeps full descriptors (description, schemas) rather than bare ids,
// since that's exactly what this package's tools surface.
func seedPlugin(t *testing.T, db *sql.DB, entry plugins.CatalogEntry) {
	t.Helper()

	actionsJSON, err := json.Marshal(entry.Actions)
	if err != nil {
		t.Fatalf("marshal actions: %v", err)
	}
	connectorsJSON, err := json.Marshal(entry.Connectors)
	if err != nil {
		t.Fatalf("marshal connectors: %v", err)
	}
	permissionsJSON, err := json.Marshal(entry.Permissions)
	if err != nil {
		t.Fatalf("marshal permissions: %v", err)
	}

	protocolVersion := entry.ProtocolVersion
	if protocolVersion == 0 {
		protocolVersion = 1
	}

	_, err = db.ExecContext(context.Background(), `
		INSERT INTO plugins (plugin_id, version, executable_path, protocol_version, connectors, actions, permissions)
		VALUES (?, ?, '/dev/null', ?, ?, ?, ?)
	`, entry.PluginID, entry.Version, protocolVersion, string(connectorsJSON), string(actionsJSON), string(permissionsJSON))
	if err != nil {
		t.Fatalf("seed plugin %q: %v", entry.PluginID, err)
	}
}
