package apps

import (
	"archive/tar"
	"bytes"
	"compress/gzip"
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

func TestInstallOrUpdate(t *testing.T) {
	t.Run("installs a new app", func(t *testing.T) {
		db := openTestDB(t)
		dir := newTestAppDir(t, "dashboard", "0.1.0", "hello_patchcord")

		a, err := InstallOrUpdate(context.Background(), db, dir)
		if err != nil {
			t.Fatalf("InstallOrUpdate() error = %v", err)
		}
		if a.Version != "0.1.0" {
			t.Fatalf("Version = %q, want %q", a.Version, "0.1.0")
		}
	})

	t.Run("updates an already-installed app in place instead of failing", func(t *testing.T) {
		db := openTestDB(t)
		dir := newTestAppDir(t, "dashboard", "0.1.0", "hello_patchcord")
		if _, err := InstallOrUpdate(context.Background(), db, dir); err != nil {
			t.Fatalf("first InstallOrUpdate() error = %v", err)
		}

		dir2 := newTestAppDir(t, "dashboard", "0.2.0", "hello_patchcord", "greet_twice")
		a, err := InstallOrUpdate(context.Background(), db, dir2)
		if err != nil {
			t.Fatalf("second InstallOrUpdate() error = %v", err)
		}
		if a.Version != "0.2.0" {
			t.Fatalf("Version = %q, want %q", a.Version, "0.2.0")
		}
		wantDir, _ := filepath.Abs(dir2)
		if a.StaticDir != wantDir {
			t.Fatalf("StaticDir = %q, want %q", a.StaticDir, wantDir)
		}
		if len(a.Permissions.WorkflowsRun) != 2 {
			t.Fatalf("Permissions.WorkflowsRun = %v, want 2 entries", a.Permissions.WorkflowsRun)
		}

		list, err := List(context.Background(), db)
		if err != nil {
			t.Fatalf("List() error = %v", err)
		}
		if len(list) != 1 {
			t.Fatalf("List() = %d apps, want 1 (update, not a new row)", len(list))
		}
	})
}

func TestPackAndInstallPackage(t *testing.T) {
	sourceDir := newTestAppDir(t, "dashboard", "0.1.0", "hello_patchcord")
	if err := os.WriteFile(filepath.Join(sourceDir, "index.html"), []byte("<html></html>"), 0o644); err != nil {
		t.Fatalf("write index.html: %v", err)
	}
	if err := os.MkdirAll(filepath.Join(sourceDir, "assets"), 0o755); err != nil {
		t.Fatalf("mkdir assets: %v", err)
	}
	if err := os.WriteFile(filepath.Join(sourceDir, "assets", "app.js"), []byte("console.log(1)"), 0o644); err != nil {
		t.Fatalf("write assets/app.js: %v", err)
	}

	packagePath := filepath.Join(t.TempDir(), "dashboard.patchcord-app")
	f, err := os.Create(packagePath)
	if err != nil {
		t.Fatalf("create package file: %v", err)
	}
	if err := Pack(sourceDir, f); err != nil {
		t.Fatalf("Pack() error = %v", err)
	}
	if err := f.Close(); err != nil {
		t.Fatalf("close package file: %v", err)
	}

	db := openTestDB(t)
	dataDir := t.TempDir()

	a, err := InstallPackage(context.Background(), db, dataDir, packagePath)
	if err != nil {
		t.Fatalf("InstallPackage() error = %v", err)
	}
	if a.ID != "dashboard" || a.Version != "0.1.0" {
		t.Fatalf("app = %+v, want id=dashboard version=0.1.0", a)
	}

	wantStaticDir := filepath.Join(dataDir, "apps", "dashboard", "0.1.0")
	if a.StaticDir != wantStaticDir {
		t.Fatalf("StaticDir = %q, want %q", a.StaticDir, wantStaticDir)
	}
	if _, err := os.Stat(filepath.Join(a.StaticDir, ManifestFileName)); err != nil {
		t.Fatalf("extracted manifest missing: %v", err)
	}
	body, err := os.ReadFile(filepath.Join(a.StaticDir, "assets", "app.js"))
	if err != nil {
		t.Fatalf("extracted assets/app.js missing: %v", err)
	}
	if string(body) != "console.log(1)" {
		t.Fatalf("assets/app.js content = %q, want %q", body, "console.log(1)")
	}

	// The package file itself is untouched and can be installed elsewhere.
	if _, err := os.Stat(packagePath); err != nil {
		t.Fatalf("package file was removed: %v", err)
	}

	t.Run("duplicate id is rejected like a directory install", func(t *testing.T) {
		if _, err := InstallPackage(context.Background(), db, dataDir, packagePath); !errors.Is(err, ErrAlreadyExists) {
			t.Fatalf("InstallPackage() error = %v, want ErrAlreadyExists", err)
		}
	})
}

func TestPack_RejectsAnInvalidManifest(t *testing.T) {
	dir := t.TempDir() // no patchcord-app.yaml

	if err := Pack(dir, io.Discard); err == nil {
		t.Fatal("expected an error for a directory with no manifest, got nil")
	}
}

func TestExtractPackage_RejectsPathTraversal(t *testing.T) {
	var buf bytes.Buffer
	gz := gzip.NewWriter(&buf)
	tw := tar.NewWriter(gz)
	if err := tw.WriteHeader(&tar.Header{
		Name:     "../escaped.txt",
		Typeflag: tar.TypeReg,
		Mode:     0o644,
		Size:     4,
	}); err != nil {
		t.Fatalf("write tar header: %v", err)
	}
	if _, err := tw.Write([]byte("evil")); err != nil {
		t.Fatalf("write tar body: %v", err)
	}
	if err := tw.Close(); err != nil {
		t.Fatalf("close tar writer: %v", err)
	}
	if err := gz.Close(); err != nil {
		t.Fatalf("close gzip writer: %v", err)
	}

	dataDir := t.TempDir()
	db := openTestDB(t)

	packagePath := filepath.Join(t.TempDir(), "evil.patchcord-app")
	if err := os.WriteFile(packagePath, buf.Bytes(), 0o644); err != nil {
		t.Fatalf("write package file: %v", err)
	}

	if _, err := InstallPackage(context.Background(), db, dataDir, packagePath); err == nil {
		t.Fatal("expected an error for a path-traversal entry, got nil")
	}
	if _, err := os.Stat(filepath.Join(filepath.Dir(dataDir), "escaped.txt")); !os.IsNotExist(err) {
		t.Fatal("path-traversal entry escaped the staging directory")
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
