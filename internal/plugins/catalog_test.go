package plugins

import (
	"context"
	"database/sql"
	"errors"
	"io"
	"log/slog"
	"testing"

	"github.com/lucasglmt/patchcord/internal/persistence"
	"github.com/lucasglmt/patchcord/migrations"
)

// openCatalogTestDB returns a freshly migrated, empty database, ready for
// the plugins table catalog.go operates on.
func openCatalogTestDB(t *testing.T) *sql.DB {
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

func TestInstall_LaunchesHandshakesAndRecordsTheManifest(t *testing.T) {
	db := openCatalogTestDB(t)

	entry, err := Install(context.Background(), db, examplePluginPath)
	if err != nil {
		t.Fatalf("Install() error = %v", err)
	}

	if entry.PluginID != "io.patchcord.example-text" {
		t.Fatalf("PluginID = %q, want %q", entry.PluginID, "io.patchcord.example-text")
	}
	if entry.Version != "1.0.0" {
		t.Fatalf("Version = %q, want %q", entry.Version, "1.0.0")
	}
	if entry.ExecutablePath != examplePluginPath {
		t.Fatalf("ExecutablePath = %q, want %q", entry.ExecutablePath, examplePluginPath)
	}
	if len(entry.Actions) != 1 || entry.Actions[0] != "text.uppercase@1" {
		t.Fatalf("Actions = %v, want [text.uppercase@1]", entry.Actions)
	}

	got, err := Get(context.Background(), db, entry.PluginID)
	if err != nil {
		t.Fatalf("Get() error = %v", err)
	}
	if got.PluginID != entry.PluginID {
		t.Fatalf("Get().PluginID = %q, want %q", got.PluginID, entry.PluginID)
	}
}

func TestInstall_FailsForABrokenBinary(t *testing.T) {
	t.Setenv("FAKE_PLUGIN_MODE", "garbage")
	db := openCatalogTestDB(t)

	if _, err := Install(context.Background(), db, fakePluginPath); err == nil {
		t.Fatal("expected an error for a plugin that never completes its bootstrap, got nil")
	}
}

func TestInstall_FailsForAMissingBinary(t *testing.T) {
	db := openCatalogTestDB(t)

	if _, err := Install(context.Background(), db, "/does/not/exist"); err == nil {
		t.Fatal("expected an error for a missing binary, got nil")
	}
}

func TestInstall_ReinstallingReplacesTheEntry(t *testing.T) {
	db := openCatalogTestDB(t)

	if _, err := Install(context.Background(), db, examplePluginPath); err != nil {
		t.Fatalf("first Install() error = %v", err)
	}
	if _, err := Install(context.Background(), db, examplePluginPath); err != nil {
		t.Fatalf("second Install() error = %v", err)
	}

	entries, err := List(context.Background(), db)
	if err != nil {
		t.Fatalf("List() error = %v", err)
	}
	if len(entries) != 1 {
		t.Fatalf("len(entries) = %d, want 1 (reinstall must not duplicate the catalog entry)", len(entries))
	}
}

func TestList_OrdersByPluginID(t *testing.T) {
	db := openCatalogTestDB(t)

	if err := upsertCatalogEntry(context.Background(), db, &CatalogEntry{
		PluginID: "io.patchcord.zzz", Version: "1.0.0", ExecutablePath: "/bin/zzz", ProtocolVersion: 1,
	}); err != nil {
		t.Fatalf("seed zzz: %v", err)
	}
	if err := upsertCatalogEntry(context.Background(), db, &CatalogEntry{
		PluginID: "io.patchcord.aaa", Version: "1.0.0", ExecutablePath: "/bin/aaa", ProtocolVersion: 1,
	}); err != nil {
		t.Fatalf("seed aaa: %v", err)
	}

	entries, err := List(context.Background(), db)
	if err != nil {
		t.Fatalf("List() error = %v", err)
	}
	if len(entries) != 2 || entries[0].PluginID != "io.patchcord.aaa" || entries[1].PluginID != "io.patchcord.zzz" {
		t.Fatalf("entries = %v, want [aaa, zzz] in order", entries)
	}
}

func TestGet_ReturnsErrNotInstalledForAnUnknownID(t *testing.T) {
	db := openCatalogTestDB(t)

	_, err := Get(context.Background(), db, "io.patchcord.unknown")
	if !errors.Is(err, ErrNotInstalled) {
		t.Fatalf("Get() error = %v, want ErrNotInstalled", err)
	}
}

func TestUninstall(t *testing.T) {
	db := openCatalogTestDB(t)

	if err := upsertCatalogEntry(context.Background(), db, &CatalogEntry{
		PluginID: "io.patchcord.test", Version: "1.0.0", ExecutablePath: "/bin/test", ProtocolVersion: 1,
	}); err != nil {
		t.Fatalf("seed entry: %v", err)
	}

	if err := Uninstall(context.Background(), db, "io.patchcord.test"); err != nil {
		t.Fatalf("Uninstall() error = %v", err)
	}

	if _, err := Get(context.Background(), db, "io.patchcord.test"); !errors.Is(err, ErrNotInstalled) {
		t.Fatalf("Get() after Uninstall() error = %v, want ErrNotInstalled", err)
	}
}

func TestUninstall_ReturnsErrNotInstalledForAnUnknownID(t *testing.T) {
	db := openCatalogTestDB(t)

	err := Uninstall(context.Background(), db, "io.patchcord.unknown")
	if !errors.Is(err, ErrNotInstalled) {
		t.Fatalf("Uninstall() error = %v, want ErrNotInstalled", err)
	}
}
