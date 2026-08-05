package plugins

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"log/slog"
	"os"
	"path/filepath"

	"github.com/lucasglmt/patchcord/internal/plugins/embedded"
)

// embeddedSeededKey is the agent_meta row (migrations/0011_agent_meta.sql)
// SeedEmbedded checks and sets to act as a one-time bootstrap step — see
// ADR-0059.
const embeddedSeededKey = "embedded_plugins.seeded"

// listEmbeddedFiles is a package variable rather than a direct call to
// embedded.Files so tests can substitute a fake plugin without depending
// on the real embed — which is only ever populated for the platform
// `make build-embedded-plugins` (or the release pipeline) actually ran on
// (see CLAUDE.md §5: mock the transport, don't depend on an external
// process/build artifact).
var listEmbeddedFiles = embedded.Files

// SeedEmbedded installs Patchcord's bundled reference plugins (package
// embedded — text, json, encoding, http, time) into db's catalog the first
// time it is called for a given database. Every call after that, for that
// database, is a no-op: once seeded, an embedded plugin the user has since
// `plugin uninstall`ed stays uninstalled — this never reinstalls one
// behind their back, and it never upgrades one already present.
//
// Call it once, right before Supervisor.Start, from anywhere that starts a
// plugin supervisor (`patchcord serve`/`dev` via internal/runtime.NewAgent,
// `workflow run`, `connector test`) — see ADR-0059.
//
// A plugin that fails to extract or install is logged and skipped, exactly
// like a plugin Supervisor.Start itself fails to launch: a bundled plugin
// must never be able to fail agent or command startup. On a build where
// the embedding step never ran (e.g. a bare `go build` on a fresh
// checkout), package embedded reports no files and SeedEmbedded simply
// records itself as seeded with nothing to install.
func SeedEmbedded(ctx context.Context, db *sql.DB, dataDir string, logger *slog.Logger) error {
	if logger == nil {
		logger = slog.Default()
	}

	seeded, err := isEmbeddedSeeded(ctx, db)
	if err != nil {
		return err
	}
	if seeded {
		return nil
	}

	files, err := listEmbeddedFiles()
	if err != nil {
		return fmt.Errorf("list embedded plugins: %w", err)
	}

	if len(files) > 0 {
		binDir := filepath.Join(dataDir, "plugins", "embedded")
		if err := os.MkdirAll(binDir, 0o755); err != nil {
			return fmt.Errorf("create embedded plugins directory: %w", err)
		}

		for _, file := range files {
			path := filepath.Join(binDir, file.Name)
			if err := os.WriteFile(path, file.Data, 0o755); err != nil {
				logger.Error("extract embedded plugin", slog.String("file", file.Name), slog.String("error", err.Error()))
				continue
			}
			if _, err := Install(ctx, db, path); err != nil {
				logger.Error("install embedded plugin", slog.String("file", file.Name), slog.String("error", err.Error()))
				continue
			}
			logger.Info("embedded plugin installed", slog.String("file", file.Name))
		}
	}

	return markEmbeddedSeeded(ctx, db)
}

func isEmbeddedSeeded(ctx context.Context, db *sql.DB) (bool, error) {
	var value string
	err := db.QueryRowContext(ctx, `SELECT value FROM agent_meta WHERE key = ?`, embeddedSeededKey).Scan(&value)
	if errors.Is(err, sql.ErrNoRows) {
		return false, nil
	}
	if err != nil {
		return false, fmt.Errorf("check embedded plugins seed state: %w", err)
	}
	return true, nil
}

func markEmbeddedSeeded(ctx context.Context, db *sql.DB) error {
	_, err := db.ExecContext(ctx, `
		INSERT INTO agent_meta (key, value) VALUES (?, ?)
		ON CONFLICT (key) DO UPDATE SET value = excluded.value, updated_at = CURRENT_TIMESTAMP
	`, embeddedSeededKey, "true")
	if err != nil {
		return fmt.Errorf("record embedded plugins seed state: %w", err)
	}
	return nil
}
