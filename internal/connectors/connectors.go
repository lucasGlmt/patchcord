// Package connectors manages connector instances: persistent, named
// configurations for accessing an external system (vision document,
// section 7.3). A connector is not an action — it only holds configuration
// and secret references; using it to actually do something is a later
// phase's job (see ADR-0020 for what this package deliberately does not
// do yet).
package connectors

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"time"

	"modernc.org/sqlite"
	sqlite3 "modernc.org/sqlite/lib"

	"github.com/lucasglmt/patchcord/internal/secrets"
)

// ErrNotFound is returned by Get and Delete when no connector with the
// given id has been recorded.
var ErrNotFound = errors.New("connector not found")

// ErrAlreadyExists is returned by Create when a connector with the given
// id already exists.
var ErrAlreadyExists = errors.New("connector already exists")

// Connector is one persistent, named configuration for accessing an
// external system, as recorded in the database.
type Connector struct {
	ID   string
	Type string
	// Config holds non-secret settings (host, port, base URL, ...).
	Config map[string]any
	// SecretRefs holds logical references to secret values, by logical
	// name — never a secret's actual value (ADR-0009, ADR-0020).
	SecretRefs map[string]secrets.Reference
	CreatedAt  time.Time
}

// Create records a new connector. It returns ErrAlreadyExists if id is
// already in use, and an error if any of secretRefs uses an unsupported
// reference type (see secrets.ValidateType) — caught here rather than only
// the first time something tries to resolve it.
//
// knownTypes is the set of connector type identifiers currently installed
// plugins contribute; the caller (internal/cli, internal/api) is
// responsible for fetching it from the plugin catalog
// (plugins.KnownConnectorTypes), keeping this package free of any process
// dependency — the same split workflow.Validate uses for knownActions.
func Create(ctx context.Context, db *sql.DB, id, connectorType string, config map[string]any, secretRefs map[string]secrets.Reference, knownTypes map[string]struct{}) (*Connector, error) {
	if id == "" {
		return nil, fmt.Errorf("connector id must not be empty")
	}
	if connectorType == "" {
		return nil, fmt.Errorf("connector type must not be empty")
	}
	if _, ok := knownTypes[connectorType]; !ok {
		return nil, fmt.Errorf("connector type %q is not declared by any installed plugin", connectorType)
	}
	for name, ref := range secretRefs {
		if err := secrets.ValidateType(ref.Type); err != nil {
			return nil, fmt.Errorf("secret %q: %w", name, err)
		}
	}

	configJSON, err := json.Marshal(config)
	if err != nil {
		return nil, fmt.Errorf("encode connector config: %w", err)
	}
	secretRefsJSON, err := json.Marshal(secretRefs)
	if err != nil {
		return nil, fmt.Errorf("encode connector secret references: %w", err)
	}

	_, err = db.ExecContext(ctx, `
		INSERT INTO connectors (id, type, config, secret_refs)
		VALUES (?, ?, ?, ?)
	`, id, connectorType, string(configJSON), string(secretRefsJSON))
	if err != nil {
		var sqliteErr *sqlite.Error
		// modernc.org/sqlite returns extended result codes (e.g.
		// SQLITE_CONSTRAINT_PRIMARYKEY, 1555), which encode the primary
		// code (SQLITE_CONSTRAINT, 19) in their low byte — mask it off to
		// match regardless of which specific constraint fired.
		if errors.As(err, &sqliteErr) && sqliteErr.Code()&0xff == sqlite3.SQLITE_CONSTRAINT {
			return nil, fmt.Errorf("%w: %q", ErrAlreadyExists, id)
		}
		return nil, fmt.Errorf("create connector %q: %w", id, err)
	}

	return Get(ctx, db, id)
}

// Get returns one connector by id. It returns ErrNotFound if no connector
// with that id has been recorded.
func Get(ctx context.Context, db *sql.DB, id string) (*Connector, error) {
	row := db.QueryRowContext(ctx, `
		SELECT id, type, config, secret_refs, created_at
		FROM connectors WHERE id = ?
	`, id)

	conn, err := scanConnector(row)
	if errors.Is(err, sql.ErrNoRows) {
		return nil, ErrNotFound
	}
	if err != nil {
		return nil, fmt.Errorf("get connector %q: %w", id, err)
	}

	return &conn, nil
}

// List returns every recorded connector, ordered by id.
func List(ctx context.Context, db *sql.DB) ([]Connector, error) {
	rows, err := db.QueryContext(ctx, `
		SELECT id, type, config, secret_refs, created_at
		FROM connectors ORDER BY id
	`)
	if err != nil {
		return nil, fmt.Errorf("list connectors: %w", err)
	}
	defer rows.Close()

	var result []Connector
	for rows.Next() {
		conn, err := scanConnector(rows)
		if err != nil {
			return nil, fmt.Errorf("scan connector: %w", err)
		}
		result = append(result, conn)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("list connectors: %w", err)
	}

	return result, nil
}

// Delete removes a connector. It returns ErrNotFound if no connector with
// that id has been recorded.
func Delete(ctx context.Context, db *sql.DB, id string) error {
	result, err := db.ExecContext(ctx, "DELETE FROM connectors WHERE id = ?", id)
	if err != nil {
		return fmt.Errorf("delete connector %q: %w", id, err)
	}

	rows, err := result.RowsAffected()
	if err != nil {
		return fmt.Errorf("delete connector %q: %w", id, err)
	}
	if rows == 0 {
		return ErrNotFound
	}

	return nil
}

// ResolvedConnector carries one connector's non-secret config and its
// secret references already resolved to values, ready to hand to a plugin
// for one action call. It is assembled by Resolve and never persisted —
// callers must never write it back to the database or echo Secrets into an
// action's output, which would land resolved secret values in run history
// in the clear (see ADR-0021).
type ResolvedConnector struct {
	Type    string
	Config  map[string]any
	Secrets map[string]any
}

// Resolve loads connector id and resolves every one of its secret
// references through store, returning a ResolvedConnector ready to pass to
// an action call. It returns ErrNotFound if no such connector exists, or an
// error naming the secret whose resolution failed.
func Resolve(ctx context.Context, db *sql.DB, id string, store secrets.Store) (*ResolvedConnector, error) {
	conn, err := Get(ctx, db, id)
	if err != nil {
		return nil, err
	}

	resolved := &ResolvedConnector{
		Type:    conn.Type,
		Config:  conn.Config,
		Secrets: make(map[string]any, len(conn.SecretRefs)),
	}
	for name, ref := range conn.SecretRefs {
		value, err := store.Resolve(ctx, ref)
		if err != nil {
			return nil, fmt.Errorf("resolve secret %q for connector %q: %w", name, id, err)
		}
		resolved.Secrets[name] = value
	}

	return resolved, nil
}

// rowScanner is satisfied by both *sql.Row and *sql.Rows.
type rowScanner interface {
	Scan(dest ...any) error
}

func scanConnector(row rowScanner) (Connector, error) {
	var (
		conn           Connector
		configJSON     string
		secretRefsJSON string
	)

	if err := row.Scan(&conn.ID, &conn.Type, &configJSON, &secretRefsJSON, &conn.CreatedAt); err != nil {
		return Connector{}, err
	}

	if err := json.Unmarshal([]byte(configJSON), &conn.Config); err != nil {
		return Connector{}, fmt.Errorf("decode connector config: %w", err)
	}
	if err := json.Unmarshal([]byte(secretRefsJSON), &conn.SecretRefs); err != nil {
		return Connector{}, fmt.Errorf("decode connector secret references: %w", err)
	}

	return conn, nil
}
