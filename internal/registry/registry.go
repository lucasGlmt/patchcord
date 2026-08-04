// Package registry resolves a package id (optionally pinned to a version)
// against a small set of user-configured registries, closing the vision
// document's aspirational install-by-identifier command (section 9.2:
// "patchcord plugin install io.patchcord.postgresql@1.0.0", "depuis un
// registre futur") and unblocking bundle updates (ADR-0044).
//
// A registry is nothing more than a name pointing at a local directory or
// a plain http(s) URL serving a static index.json plus package files — no
// bespoke server, no auth, no commerce (CLAUDE.md §1.9: the cloud is never
// required). This package only resolves and downloads; installing what it
// downloads is unchanged and stays the job of internal/plugins,
// internal/apps, internal/bundles InstallPackage.
package registry

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"time"
)

// ErrNotFound is returned by Remove when no registry with the given name
// is configured, and by Resolve when no configured registry lists the
// requested package id.
var ErrNotFound = errors.New("registry entry not found")

// Registry is one configured package registry, as recorded in the
// database.
type Registry struct {
	Name     string
	Location string
	AddedAt  time.Time
}

// Add records location under name. Re-adding the same name updates its
// location instead of failing — reconfiguring a registry's address is not
// an error. location is not validated here (no existence check, no
// reachability probe): like trust.Add does not check a key file exists, a
// registry is only ever validated lazily, the first time something
// resolves against it.
func Add(ctx context.Context, db *sql.DB, name, location string) error {
	_, err := db.ExecContext(ctx, `
		INSERT INTO registries (name, location)
		VALUES (?, ?)
		ON CONFLICT(name) DO UPDATE SET location = excluded.location
	`, name, location)
	if err != nil {
		return fmt.Errorf("configure registry %q: %w", name, err)
	}
	return nil
}

// Remove deletes a configured registry by name. It returns ErrNotFound if
// no registry with that name is configured.
func Remove(ctx context.Context, db *sql.DB, name string) error {
	result, err := db.ExecContext(ctx, "DELETE FROM registries WHERE name = ?", name)
	if err != nil {
		return fmt.Errorf("remove registry %q: %w", name, err)
	}

	affected, err := result.RowsAffected()
	if err != nil {
		return fmt.Errorf("remove registry %q: %w", name, err)
	}
	if affected == 0 {
		return ErrNotFound
	}

	return nil
}

// List returns every configured registry, oldest-added first — the order
// Resolve consults them in. rowid breaks ties between registries added
// within the same second.
func List(ctx context.Context, db *sql.DB) ([]Registry, error) {
	rows, err := db.QueryContext(ctx, `
		SELECT name, location, added_at FROM registries ORDER BY added_at, rowid
	`)
	if err != nil {
		return nil, fmt.Errorf("list registries: %w", err)
	}
	defer rows.Close()

	var result []Registry
	for rows.Next() {
		var r Registry
		if err := rows.Scan(&r.Name, &r.Location, &r.AddedAt); err != nil {
			return nil, fmt.Errorf("scan registry: %w", err)
		}
		result = append(result, r)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("list registries: %w", err)
	}

	return result, nil
}
