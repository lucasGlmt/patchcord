package bundles

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"time"
)

// ErrNotFound is returned by Get when no bundle with the given id has been
// recorded.
var ErrNotFound = errors.New("bundle not found")

// Bundle is one installed bundle's provenance record: what was declared,
// not a second source of truth for the app/workflows it installed (see the
// package doc comment).
type Bundle struct {
	ID          string
	Version     string
	Manifest    string // raw bundle.yaml source
	InstalledAt time.Time
}

// record upserts a bundle's provenance row. Re-installing the same bundle
// id (e.g. after fixing a typo in bundle.yaml) replaces it — there is no
// separate `bundle remove` in this first pass to require before retrying.
func record(ctx context.Context, db *sql.DB, id, version, manifestSource string) error {
	_, err := db.ExecContext(ctx, `
		INSERT INTO bundles (id, version, manifest)
		VALUES (?, ?, ?)
		ON CONFLICT(id) DO UPDATE SET
			version = excluded.version,
			manifest = excluded.manifest
	`, id, version, manifestSource)
	if err != nil {
		return fmt.Errorf("record bundle %q: %w", id, err)
	}
	return nil
}

// Get returns one installed bundle's provenance record by id. It returns
// ErrNotFound if no bundle with that id has been recorded.
func Get(ctx context.Context, db *sql.DB, id string) (*Bundle, error) {
	row := db.QueryRowContext(ctx, `
		SELECT id, version, manifest, installed_at FROM bundles WHERE id = ?
	`, id)

	b, err := scanBundle(row)
	if errors.Is(err, sql.ErrNoRows) {
		return nil, ErrNotFound
	}
	if err != nil {
		return nil, fmt.Errorf("get bundle %q: %w", id, err)
	}

	return &b, nil
}

// List returns every installed bundle's provenance record, ordered by id.
func List(ctx context.Context, db *sql.DB) ([]Bundle, error) {
	rows, err := db.QueryContext(ctx, `
		SELECT id, version, manifest, installed_at FROM bundles ORDER BY id
	`)
	if err != nil {
		return nil, fmt.Errorf("list bundles: %w", err)
	}
	defer rows.Close()

	var result []Bundle
	for rows.Next() {
		b, err := scanBundle(rows)
		if err != nil {
			return nil, fmt.Errorf("scan bundle: %w", err)
		}
		result = append(result, b)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("list bundles: %w", err)
	}

	return result, nil
}

// rowScanner is satisfied by both *sql.Row and *sql.Rows.
type rowScanner interface {
	Scan(dest ...any) error
}

func scanBundle(row rowScanner) (Bundle, error) {
	var b Bundle
	if err := row.Scan(&b.ID, &b.Version, &b.Manifest, &b.InstalledAt); err != nil {
		return Bundle{}, err
	}
	return b, nil
}
