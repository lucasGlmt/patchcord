package plugins

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"time"
)

// ErrNotInstalled is returned by Get and Uninstall when no plugin with the
// given id is in the catalog.
var ErrNotInstalled = errors.New("plugin not installed")

// CatalogEntry is one plugin recorded in the agent's catalog, as returned
// by its handshake at install time.
type CatalogEntry struct {
	PluginID        string
	Version         string
	ExecutablePath  string
	ProtocolVersion uint32
	Connectors      []string
	Actions         []string
	Permissions     []string
	InstalledAt     time.Time
}

// Install launches the plugin binary at path, completes the handshake to
// validate it and discover its manifest, then records it in the catalog.
// Installing a plugin whose id is already present replaces its entry.
func Install(ctx context.Context, db *sql.DB, path string) (*CatalogEntry, error) {
	proc, err := Launch(ctx, path, DefaultReadyTimeout)
	if err != nil {
		return nil, fmt.Errorf("launch plugin: %w", err)
	}
	defer func() {
		closeCtx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()
		_ = proc.Close(closeCtx)
	}()

	manifest, err := Handshake(ctx, proc.Client)
	if err != nil {
		return nil, fmt.Errorf("handshake plugin: %w", err)
	}

	entry := &CatalogEntry{
		PluginID:        manifest.PluginID,
		Version:         manifest.PluginVersion,
		ExecutablePath:  path,
		ProtocolVersion: manifest.ProtocolVersion,
		Connectors:      manifest.Connectors,
		Actions:         manifest.Actions,
		Permissions:     manifest.Permissions,
	}

	if err := upsertCatalogEntry(ctx, db, entry); err != nil {
		return nil, err
	}

	return entry, nil
}

func upsertCatalogEntry(ctx context.Context, db *sql.DB, entry *CatalogEntry) error {
	connectors, err := json.Marshal(entry.Connectors)
	if err != nil {
		return fmt.Errorf("encode connectors: %w", err)
	}
	actions, err := json.Marshal(entry.Actions)
	if err != nil {
		return fmt.Errorf("encode actions: %w", err)
	}
	permissions, err := json.Marshal(entry.Permissions)
	if err != nil {
		return fmt.Errorf("encode permissions: %w", err)
	}

	_, err = db.ExecContext(ctx, `
		INSERT INTO plugins (plugin_id, version, executable_path, protocol_version, connectors, actions, permissions)
		VALUES (?, ?, ?, ?, ?, ?, ?)
		ON CONFLICT (plugin_id) DO UPDATE SET
			version = excluded.version,
			executable_path = excluded.executable_path,
			protocol_version = excluded.protocol_version,
			connectors = excluded.connectors,
			actions = excluded.actions,
			permissions = excluded.permissions
	`, entry.PluginID, entry.Version, entry.ExecutablePath, entry.ProtocolVersion, connectors, actions, permissions)
	if err != nil {
		return fmt.Errorf("record plugin %q: %w", entry.PluginID, err)
	}

	return nil
}

// List returns every installed plugin, ordered by plugin id.
func List(ctx context.Context, db *sql.DB) ([]CatalogEntry, error) {
	rows, err := db.QueryContext(ctx, `
		SELECT plugin_id, version, executable_path, protocol_version, connectors, actions, permissions, installed_at
		FROM plugins
		ORDER BY plugin_id
	`)
	if err != nil {
		return nil, fmt.Errorf("list plugins: %w", err)
	}
	defer rows.Close()

	var entries []CatalogEntry
	for rows.Next() {
		entry, err := scanCatalogEntry(rows)
		if err != nil {
			return nil, err
		}
		entries = append(entries, entry)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("list plugins: %w", err)
	}

	return entries, nil
}

// KnownActions returns the set of action identifiers contributed by every
// installed plugin, for the workflow compiler (internal/workflow.Validate)
// to check a workflow's steps against.
func KnownActions(ctx context.Context, db *sql.DB) (map[string]struct{}, error) {
	entries, err := List(ctx, db)
	if err != nil {
		return nil, err
	}

	actions := make(map[string]struct{})
	for _, entry := range entries {
		for _, action := range entry.Actions {
			actions[action] = struct{}{}
		}
	}

	return actions, nil
}

// Get returns one installed plugin by id. It returns ErrNotInstalled if no
// plugin with that id is in the catalog.
func Get(ctx context.Context, db *sql.DB, id string) (*CatalogEntry, error) {
	row := db.QueryRowContext(ctx, `
		SELECT plugin_id, version, executable_path, protocol_version, connectors, actions, permissions, installed_at
		FROM plugins
		WHERE plugin_id = ?
	`, id)

	entry, err := scanCatalogEntry(row)
	if errors.Is(err, sql.ErrNoRows) {
		return nil, ErrNotInstalled
	}
	if err != nil {
		return nil, fmt.Errorf("get plugin %q: %w", id, err)
	}

	return &entry, nil
}

// Uninstall removes a plugin from the catalog. It returns ErrNotInstalled
// if no plugin with that id is in the catalog.
func Uninstall(ctx context.Context, db *sql.DB, id string) error {
	result, err := db.ExecContext(ctx, "DELETE FROM plugins WHERE plugin_id = ?", id)
	if err != nil {
		return fmt.Errorf("uninstall plugin %q: %w", id, err)
	}

	rows, err := result.RowsAffected()
	if err != nil {
		return fmt.Errorf("uninstall plugin %q: %w", id, err)
	}
	if rows == 0 {
		return ErrNotInstalled
	}

	return nil
}

// rowScanner is satisfied by both *sql.Row and *sql.Rows.
type rowScanner interface {
	Scan(dest ...any) error
}

func scanCatalogEntry(row rowScanner) (CatalogEntry, error) {
	var (
		entry                            CatalogEntry
		connectors, actions, permissions string
	)

	if err := row.Scan(
		&entry.PluginID,
		&entry.Version,
		&entry.ExecutablePath,
		&entry.ProtocolVersion,
		&connectors,
		&actions,
		&permissions,
		&entry.InstalledAt,
	); err != nil {
		return CatalogEntry{}, err
	}

	if err := json.Unmarshal([]byte(connectors), &entry.Connectors); err != nil {
		return CatalogEntry{}, fmt.Errorf("decode connectors: %w", err)
	}
	if err := json.Unmarshal([]byte(actions), &entry.Actions); err != nil {
		return CatalogEntry{}, fmt.Errorf("decode actions: %w", err)
	}
	if err := json.Unmarshal([]byte(permissions), &entry.Permissions); err != nil {
		return CatalogEntry{}, fmt.Errorf("decode permissions: %w", err)
	}

	return entry, nil
}
