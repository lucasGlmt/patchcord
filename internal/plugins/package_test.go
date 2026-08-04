package plugins

import (
	"archive/tar"
	"bytes"
	"compress/gzip"
	"context"
	"database/sql"
	"encoding/json"
	"io"
	"log/slog"
	"os"
	"path/filepath"
	"runtime"
	"testing"

	"github.com/lucasglmt/patchcord/internal/persistence"
	"github.com/lucasglmt/patchcord/migrations"
)

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

// currentPlatform mirrors the "GOOS-GOARCH" key InstallPackage looks up.
func currentPlatform() string {
	return runtime.GOOS + "-" + runtime.GOARCH
}

// newTestPluginPackageDir builds a source directory for Pack: a
// manifest.json declaring a single executable for the current platform,
// copied from the real example plugin binary built by TestMain, so
// InstallPackage can be exercised end to end (launch, handshake, catalog).
func newTestPluginPackageDir(t *testing.T, id, version string) string {
	t.Helper()

	dir := t.TempDir()
	relExecutable := filepath.Join("binaries", currentPlatform(), "plugin")
	execPath := filepath.Join(dir, relExecutable)
	if err := os.MkdirAll(filepath.Dir(execPath), 0o755); err != nil {
		t.Fatalf("mkdir binaries dir: %v", err)
	}
	copyFile(t, examplePluginPath, execPath)

	manifestBody := validPackageManifestJSONWith(t, id, version, map[string]string{
		currentPlatform(): filepath.ToSlash(relExecutable),
	})
	if err := os.WriteFile(filepath.Join(dir, PackageManifestFileName), manifestBody, 0o644); err != nil {
		t.Fatalf("write manifest.json: %v", err)
	}

	return dir
}

func validPackageManifestJSONWith(t *testing.T, id, version string, executables map[string]string) []byte {
	t.Helper()

	m := packageManifestJSON{
		SchemaVersion:   1,
		Kind:            "plugin",
		ID:              id,
		Version:         version,
		ProtocolVersion: 1,
		Permissions:     []string{},
		Executables:     executables,
	}
	body, err := json.Marshal(m)
	if err != nil {
		t.Fatalf("marshal manifest: %v", err)
	}
	return body
}

func copyFile(t *testing.T, src, dst string) {
	t.Helper()

	body, err := os.ReadFile(src)
	if err != nil {
		t.Fatalf("read %q: %v", src, err)
	}
	if err := os.WriteFile(dst, body, 0o755); err != nil {
		t.Fatalf("write %q: %v", dst, err)
	}
}

func TestPackAndInstallPackage(t *testing.T) {
	sourceDir := newTestPluginPackageDir(t, "io.patchcord.example-text", "1.0.0")

	packagePath := filepath.Join(t.TempDir(), "text.patchcord-plugin")
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

	entry, err := InstallPackage(context.Background(), db, dataDir, packagePath)
	if err != nil {
		t.Fatalf("InstallPackage() error = %v", err)
	}
	if entry.PluginID != "io.patchcord.example-text" {
		t.Fatalf("PluginID = %q, want %q", entry.PluginID, "io.patchcord.example-text")
	}
	if entry.Version != "1.0.0" {
		t.Fatalf("Version = %q, want %q", entry.Version, "1.0.0")
	}

	wantExecPath := filepath.Join(dataDir, "plugins", "io.patchcord.example-text", "1.0.0", "binaries", currentPlatform(), "plugin")
	if entry.ExecutablePath != wantExecPath {
		t.Fatalf("ExecutablePath = %q, want %q", entry.ExecutablePath, wantExecPath)
	}

	found := false
	for _, action := range entry.Actions {
		if action == "text.uppercase@1" {
			found = true
		}
	}
	if !found {
		t.Fatalf("Actions = %v, want it to include text.uppercase@1", entry.Actions)
	}

	// The package file itself is untouched and can be installed elsewhere.
	if _, err := os.Stat(packagePath); err != nil {
		t.Fatalf("package file was removed: %v", err)
	}
}

func TestPack_RejectsAnInvalidManifest(t *testing.T) {
	dir := t.TempDir() // no manifest.json

	if err := Pack(dir, io.Discard); err == nil {
		t.Fatal("expected an error for a directory with no manifest, got nil")
	}
}

func TestInstallPackage_RejectsAMissingPlatformExecutable(t *testing.T) {
	dir := t.TempDir()
	execPath := filepath.Join(dir, "binaries", "some-other-platform", "plugin")
	if err := os.MkdirAll(filepath.Dir(execPath), 0o755); err != nil {
		t.Fatalf("mkdir binaries dir: %v", err)
	}
	copyFile(t, examplePluginPath, execPath)
	body := validPackageManifestJSONWith(t, "io.patchcord.example-text", "1.0.0", map[string]string{
		"some-other-platform": "binaries/some-other-platform/plugin",
	})
	if err := os.WriteFile(filepath.Join(dir, PackageManifestFileName), body, 0o644); err != nil {
		t.Fatalf("write manifest.json: %v", err)
	}

	packagePath := filepath.Join(t.TempDir(), "text.patchcord-plugin")
	f, err := os.Create(packagePath)
	if err != nil {
		t.Fatalf("create package file: %v", err)
	}
	if err := Pack(dir, f); err != nil {
		t.Fatalf("Pack() error = %v", err)
	}
	if err := f.Close(); err != nil {
		t.Fatalf("close package file: %v", err)
	}

	db := openTestDB(t)
	if _, err := InstallPackage(context.Background(), db, t.TempDir(), packagePath); err == nil {
		t.Fatal("expected an error for a package with no executable for the current platform, got nil")
	}
}

func TestInstallPackage_RejectsPathTraversalInExecutablesEntry(t *testing.T) {
	dir := t.TempDir()
	body := validPackageManifestJSONWith(t, "io.patchcord.example-text", "1.0.0", map[string]string{
		currentPlatform(): "../../escaped",
	})
	if err := os.WriteFile(filepath.Join(dir, PackageManifestFileName), body, 0o644); err != nil {
		t.Fatalf("write manifest.json: %v", err)
	}

	packagePath := filepath.Join(t.TempDir(), "text.patchcord-plugin")
	f, err := os.Create(packagePath)
	if err != nil {
		t.Fatalf("create package file: %v", err)
	}
	if err := Pack(dir, f); err != nil {
		t.Fatalf("Pack() error = %v", err)
	}
	if err := f.Close(); err != nil {
		t.Fatalf("close package file: %v", err)
	}

	db := openTestDB(t)
	if _, err := InstallPackage(context.Background(), db, t.TempDir(), packagePath); err == nil {
		t.Fatal("expected an error for a path-traversal executables entry, got nil")
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

	packagePath := filepath.Join(t.TempDir(), "evil.patchcord-plugin")
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
