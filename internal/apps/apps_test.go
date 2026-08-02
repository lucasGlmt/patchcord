package apps

import (
	"context"
	"database/sql"
	"errors"
	"io"
	"log/slog"
	"os"
	"path/filepath"
	"testing"

	"github.com/lucasglmt/patchcord/internal/persistence"
	"github.com/lucasglmt/patchcord/migrations"
)

// openTestDB returns a freshly migrated, empty database.
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

// newTestAppDir creates a temporary directory containing a valid
// patchcord-app.yaml (and nothing else — static assets are not needed for
// these tests).
func newTestAppDir(t *testing.T, id, version string, workflowsRun ...string) string {
	t.Helper()

	dir := t.TempDir()
	content := "id: " + id + "\nversion: \"" + version + "\"\npermissions:\n  workflows:\n    run:\n"
	for _, w := range workflowsRun {
		content += "      - " + w + "\n"
	}
	if err := os.WriteFile(filepath.Join(dir, ManifestFileName), []byte(content), 0o644); err != nil {
		t.Fatalf("write manifest: %v", err)
	}
	return dir
}

func TestInstall_RecordsAnApp(t *testing.T) {
	db := openTestDB(t)
	dir := newTestAppDir(t, "dashboard", "0.1.0", "hello_patchcord")

	a, err := Install(context.Background(), db, dir)
	if err != nil {
		t.Fatalf("Install() error = %v", err)
	}

	if a.ID != "dashboard" {
		t.Fatalf("ID = %q, want %q", a.ID, "dashboard")
	}
	if a.Version != "0.1.0" {
		t.Fatalf("Version = %q, want %q", a.Version, "0.1.0")
	}
	wantDir, _ := filepath.Abs(dir)
	if a.StaticDir != wantDir {
		t.Fatalf("StaticDir = %q, want %q", a.StaticDir, wantDir)
	}
	if len(a.Permissions.WorkflowsRun) != 1 || a.Permissions.WorkflowsRun[0] != "hello_patchcord" {
		t.Fatalf("Permissions.WorkflowsRun = %v, want [hello_patchcord]", a.Permissions.WorkflowsRun)
	}
	if a.CreatedAt.IsZero() {
		t.Fatal("CreatedAt is zero, want it populated from the database default")
	}
}

func TestInstall_RejectsAnInvalidManifest(t *testing.T) {
	db := openTestDB(t)
	dir := t.TempDir() // no patchcord-app.yaml

	if _, err := Install(context.Background(), db, dir); err == nil {
		t.Fatal("expected an error for a missing manifest, got nil")
	}
}

func TestInstall_RejectsADuplicateID(t *testing.T) {
	db := openTestDB(t)
	dir := newTestAppDir(t, "dashboard", "0.1.0", "hello_patchcord")

	if _, err := Install(context.Background(), db, dir); err != nil {
		t.Fatalf("first Install() error = %v", err)
	}

	_, err := Install(context.Background(), db, dir)
	if !errors.Is(err, ErrAlreadyExists) {
		t.Fatalf("second Install() error = %v, want ErrAlreadyExists", err)
	}
}

func TestGet_ReturnsErrNotFoundForAnUnknownID(t *testing.T) {
	db := openTestDB(t)

	if _, err := Get(context.Background(), db, "does-not-exist"); !errors.Is(err, ErrNotFound) {
		t.Fatalf("Get() error = %v, want ErrNotFound", err)
	}
}

func TestList_OrdersByID(t *testing.T) {
	db := openTestDB(t)

	for _, id := range []string{"charlie", "alpha", "bravo"} {
		dir := newTestAppDir(t, id, "0.1.0")
		if _, err := Install(context.Background(), db, dir); err != nil {
			t.Fatalf("Install(%q) error = %v", id, err)
		}
	}

	list, err := List(context.Background(), db)
	if err != nil {
		t.Fatalf("List() error = %v", err)
	}

	var ids []string
	for _, a := range list {
		ids = append(ids, a.ID)
	}
	want := []string{"alpha", "bravo", "charlie"}
	if len(ids) != len(want) {
		t.Fatalf("List() ids = %v, want %v", ids, want)
	}
	for i := range want {
		if ids[i] != want[i] {
			t.Fatalf("List() ids = %v, want %v", ids, want)
		}
	}
}

func TestUninstall(t *testing.T) {
	t.Run("removes an existing app", func(t *testing.T) {
		db := openTestDB(t)
		dir := newTestAppDir(t, "dashboard", "0.1.0")
		if _, err := Install(context.Background(), db, dir); err != nil {
			t.Fatalf("Install() error = %v", err)
		}

		if err := Uninstall(context.Background(), db, "dashboard"); err != nil {
			t.Fatalf("Uninstall() error = %v", err)
		}

		if _, err := Get(context.Background(), db, "dashboard"); !errors.Is(err, ErrNotFound) {
			t.Fatalf("Get() after Uninstall() error = %v, want ErrNotFound", err)
		}
	})

	t.Run("returns ErrNotFound for an unknown id", func(t *testing.T) {
		db := openTestDB(t)
		if err := Uninstall(context.Background(), db, "does-not-exist"); !errors.Is(err, ErrNotFound) {
			t.Fatalf("Uninstall() error = %v, want ErrNotFound", err)
		}
	})
}
