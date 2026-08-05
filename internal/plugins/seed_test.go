package plugins

import (
	"context"
	"io"
	"log/slog"
	"os"
	"testing"

	"github.com/lucasglmt/patchcord/internal/plugins/embedded"
)

// withEmbeddedFiles temporarily substitutes listEmbeddedFiles, restoring it
// once the test finishes — see listEmbeddedFiles's doc comment for why
// SeedEmbedded's tests never depend on the real, platform-specific embed.
func withEmbeddedFiles(t *testing.T, files []embedded.File, err error) {
	t.Helper()
	original := listEmbeddedFiles
	listEmbeddedFiles = func() ([]embedded.File, error) { return files, err }
	t.Cleanup(func() { listEmbeddedFiles = original })
}

func readFileFixture(t *testing.T, name, path string) embedded.File {
	t.Helper()
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("read fixture %q: %v", path, err)
	}
	return embedded.File{Name: name, Data: data}
}

func TestSeedEmbedded_InstallsEveryBundledPluginOnce(t *testing.T) {
	db := openCatalogTestDB(t)
	withEmbeddedFiles(t, []embedded.File{readFileFixture(t, "text", examplePluginPath)}, nil)

	logger := slog.New(slog.NewTextHandler(io.Discard, nil))
	if err := SeedEmbedded(context.Background(), db, t.TempDir(), logger); err != nil {
		t.Fatalf("SeedEmbedded() error = %v", err)
	}

	entries, err := List(context.Background(), db)
	if err != nil {
		t.Fatalf("List() error = %v", err)
	}
	if len(entries) != 1 || entries[0].PluginID != "io.patchcord.example-text" {
		t.Fatalf("catalog after seeding = %+v, want exactly io.patchcord.example-text", entries)
	}
}

func TestSeedEmbedded_IsANoOpOnceAlreadySeeded(t *testing.T) {
	db := openCatalogTestDB(t)
	withEmbeddedFiles(t, []embedded.File{readFileFixture(t, "text", examplePluginPath)}, nil)

	logger := slog.New(slog.NewTextHandler(io.Discard, nil))
	if err := SeedEmbedded(context.Background(), db, t.TempDir(), logger); err != nil {
		t.Fatalf("first SeedEmbedded() error = %v", err)
	}

	// A plugin the user uninstalled after the first seeding must stay
	// uninstalled — SeedEmbedded must never re-add it behind their back.
	if err := Uninstall(context.Background(), db, "io.patchcord.example-text"); err != nil {
		t.Fatalf("Uninstall() error = %v", err)
	}

	if err := SeedEmbedded(context.Background(), db, t.TempDir(), logger); err != nil {
		t.Fatalf("second SeedEmbedded() error = %v", err)
	}

	entries, err := List(context.Background(), db)
	if err != nil {
		t.Fatalf("List() error = %v", err)
	}
	if len(entries) != 0 {
		t.Fatalf("catalog after re-seeding = %+v, want the user's uninstall to stick", entries)
	}
}

func TestSeedEmbedded_NoEmbeddedFilesIsANoOp(t *testing.T) {
	db := openCatalogTestDB(t)
	withEmbeddedFiles(t, nil, nil)

	logger := slog.New(slog.NewTextHandler(io.Discard, nil))
	if err := SeedEmbedded(context.Background(), db, t.TempDir(), logger); err != nil {
		t.Fatalf("SeedEmbedded() error = %v", err)
	}

	entries, err := List(context.Background(), db)
	if err != nil {
		t.Fatalf("List() error = %v", err)
	}
	if len(entries) != 0 {
		t.Fatalf("catalog after seeding with nothing embedded = %+v, want empty", entries)
	}

	seeded, err := isEmbeddedSeeded(context.Background(), db)
	if err != nil {
		t.Fatalf("isEmbeddedSeeded() error = %v", err)
	}
	if !seeded {
		t.Fatal("isEmbeddedSeeded() = false, want true even when there was nothing to install")
	}
}

func TestSeedEmbedded_SkipsABrokenBundledPluginWithoutFailing(t *testing.T) {
	t.Setenv("FAKE_PLUGIN_MODE", "garbage")
	db := openCatalogTestDB(t)
	withEmbeddedFiles(t, []embedded.File{readFileFixture(t, "fake", fakePluginPath)}, nil)

	logger := slog.New(slog.NewTextHandler(io.Discard, nil))
	if err := SeedEmbedded(context.Background(), db, t.TempDir(), logger); err != nil {
		t.Fatalf("SeedEmbedded() error = %v, want a broken bundled plugin to be skipped, not returned", err)
	}

	entries, err := List(context.Background(), db)
	if err != nil {
		t.Fatalf("List() error = %v", err)
	}
	if len(entries) != 0 {
		t.Fatalf("catalog after seeding a broken plugin = %+v, want empty", entries)
	}
}
