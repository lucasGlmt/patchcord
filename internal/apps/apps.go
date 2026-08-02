package apps

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"path/filepath"
	"time"

	"modernc.org/sqlite"
	sqlite3 "modernc.org/sqlite/lib"
)

// ErrNotFound is returned by Get and Uninstall when no application with
// the given id has been recorded.
var ErrNotFound = errors.New("app not found")

// ErrAlreadyExists is returned by Install when an application with the
// manifest's id is already installed.
var ErrAlreadyExists = errors.New("app already exists")

// App is one installed application, as recorded in the database.
type App struct {
	ID          string
	Version     string
	StaticDir   string
	Permissions AppPermissions
	CreatedAt   time.Time
}

// Install reads sourceDir's manifest (patchcord-app.yaml) and records the
// application, serving its static files straight from sourceDir. There is
// no packaging or copy step yet — the real .patchcord-app bundle format
// (vision document, section 9.3) is deferred, see ADR-0026.
//
// It returns ErrAlreadyExists if an application with the manifest's id is
// already installed, or an error wrapping ErrInvalidManifest if the
// manifest is malformed.
func Install(ctx context.Context, db *sql.DB, sourceDir string) (*App, error) {
	manifest, err := LoadManifest(sourceDir)
	if err != nil {
		return nil, err
	}

	staticDir, err := filepath.Abs(sourceDir)
	if err != nil {
		return nil, fmt.Errorf("resolve app directory %q: %w", sourceDir, err)
	}

	permissionsJSON, err := json.Marshal(manifest.Permissions)
	if err != nil {
		return nil, fmt.Errorf("encode app permissions: %w", err)
	}

	_, err = db.ExecContext(ctx, `
		INSERT INTO apps (id, version, static_dir, permissions)
		VALUES (?, ?, ?, ?)
	`, manifest.ID, manifest.Version, staticDir, string(permissionsJSON))
	if err != nil {
		var sqliteErr *sqlite.Error
		// modernc.org/sqlite returns extended result codes (e.g.
		// SQLITE_CONSTRAINT_PRIMARYKEY, 1555), which encode the primary
		// code (SQLITE_CONSTRAINT, 19) in their low byte — mask it off to
		// match regardless of which specific constraint fired.
		if errors.As(err, &sqliteErr) && sqliteErr.Code()&0xff == sqlite3.SQLITE_CONSTRAINT {
			return nil, fmt.Errorf("%w: %q", ErrAlreadyExists, manifest.ID)
		}
		return nil, fmt.Errorf("install app %q: %w", manifest.ID, err)
	}

	return Get(ctx, db, manifest.ID)
}

// InstallOrUpdate is Install, except that installing over an application
// whose id is already recorded updates it in place (new version,
// static_dir, permissions) instead of returning ErrAlreadyExists. It backs
// `patchcord app dev`: since handleServeApp reads static_dir straight off
// disk on every request (no copy, no cache), an application registered
// this way is already "hot reloaded" for free — rebuilding it in place
// (e.g. `vite build --watch`) is visible on the next browser refresh with
// no further agent involvement. What InstallOrUpdate removes is the
// friction Install has for this loop: without it, iterating would require
// `app remove` before every `app install`.
func InstallOrUpdate(ctx context.Context, db *sql.DB, sourceDir string) (*App, error) {
	manifest, err := LoadManifest(sourceDir)
	if err != nil {
		return nil, err
	}

	staticDir, err := filepath.Abs(sourceDir)
	if err != nil {
		return nil, fmt.Errorf("resolve app directory %q: %w", sourceDir, err)
	}

	permissionsJSON, err := json.Marshal(manifest.Permissions)
	if err != nil {
		return nil, fmt.Errorf("encode app permissions: %w", err)
	}

	_, err = db.ExecContext(ctx, `
		INSERT INTO apps (id, version, static_dir, permissions)
		VALUES (?, ?, ?, ?)
		ON CONFLICT(id) DO UPDATE SET
			version = excluded.version,
			static_dir = excluded.static_dir,
			permissions = excluded.permissions
	`, manifest.ID, manifest.Version, staticDir, string(permissionsJSON))
	if err != nil {
		return nil, fmt.Errorf("install or update app %q: %w", manifest.ID, err)
	}

	return Get(ctx, db, manifest.ID)
}

// Get returns one installed application by id. It returns ErrNotFound if
// no application with that id has been recorded.
func Get(ctx context.Context, db *sql.DB, id string) (*App, error) {
	row := db.QueryRowContext(ctx, `
		SELECT id, version, static_dir, permissions, created_at
		FROM apps WHERE id = ?
	`, id)

	a, err := scanApp(row)
	if errors.Is(err, sql.ErrNoRows) {
		return nil, ErrNotFound
	}
	if err != nil {
		return nil, fmt.Errorf("get app %q: %w", id, err)
	}

	return &a, nil
}

// List returns every installed application, ordered by id.
func List(ctx context.Context, db *sql.DB) ([]App, error) {
	rows, err := db.QueryContext(ctx, `
		SELECT id, version, static_dir, permissions, created_at
		FROM apps ORDER BY id
	`)
	if err != nil {
		return nil, fmt.Errorf("list apps: %w", err)
	}
	defer rows.Close()

	var result []App
	for rows.Next() {
		a, err := scanApp(rows)
		if err != nil {
			return nil, fmt.Errorf("scan app: %w", err)
		}
		result = append(result, a)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("list apps: %w", err)
	}

	return result, nil
}

// Uninstall removes an application. It returns ErrNotFound if no
// application with that id has been recorded.
func Uninstall(ctx context.Context, db *sql.DB, id string) error {
	result, err := db.ExecContext(ctx, "DELETE FROM apps WHERE id = ?", id)
	if err != nil {
		return fmt.Errorf("uninstall app %q: %w", id, err)
	}

	rows, err := result.RowsAffected()
	if err != nil {
		return fmt.Errorf("uninstall app %q: %w", id, err)
	}
	if rows == 0 {
		return ErrNotFound
	}

	return nil
}

// rowScanner is satisfied by both *sql.Row and *sql.Rows.
type rowScanner interface {
	Scan(dest ...any) error
}

func scanApp(row rowScanner) (App, error) {
	var (
		a               App
		permissionsJSON string
	)

	if err := row.Scan(&a.ID, &a.Version, &a.StaticDir, &permissionsJSON, &a.CreatedAt); err != nil {
		return App{}, err
	}

	if err := json.Unmarshal([]byte(permissionsJSON), &a.Permissions); err != nil {
		return App{}, fmt.Errorf("decode app permissions: %w", err)
	}

	return a, nil
}
