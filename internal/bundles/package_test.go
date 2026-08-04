package bundles

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
	"os/exec"
	"path/filepath"
	"testing"

	"github.com/lucasglmt/patchcord/internal/apps"
	"github.com/lucasglmt/patchcord/internal/persistence"
	"github.com/lucasglmt/patchcord/internal/plugins"
	"github.com/lucasglmt/patchcord/internal/runs"
	"github.com/lucasglmt/patchcord/migrations"
)

// examplePluginPath is the real text example plugin, built once for this
// package's tests, so InstallPackage can be exercised against an actual
// installed plugin dependency instead of a fixture.
var examplePluginPath string

func TestMain(m *testing.M) {
	tmpDir, err := os.MkdirTemp("", "patchcord-bundles-fixtures")
	if err != nil {
		panic(err)
	}
	defer os.RemoveAll(tmpDir)

	examplePluginPath = filepath.Join(tmpDir, "text")
	if out, err := exec.Command("go", "build", "-o", examplePluginPath, "../../plugins/examples/text").CombinedOutput(); err != nil {
		panic("build example plugin: " + err.Error() + "\n" + string(out))
	}

	os.Exit(m.Run())
}

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

const bundleWorkflowYAML = `schema_version: 1
id: bundle_workflow
version: 1
trigger:
  type: manual
steps:
  - id: transform
    uses: text.uppercase@1
    with:
      value: "hi"
`

// newTestBundleSourceDir builds a bundle.yaml plus an embedded app and
// workflow, declaring a dependency on io.patchcord.example-text@1.0.0.
func newTestBundleSourceDir(t *testing.T) string {
	t.Helper()

	dir := t.TempDir()

	if err := os.MkdirAll(filepath.Join(dir, "app"), 0o755); err != nil {
		t.Fatalf("mkdir app: %v", err)
	}
	if err := os.WriteFile(filepath.Join(dir, "app", apps.ManifestFileName), []byte("id: dashboard\nversion: \"0.1.0\"\n"), 0o644); err != nil {
		t.Fatalf("write app manifest: %v", err)
	}

	if err := os.MkdirAll(filepath.Join(dir, "workflows"), 0o755); err != nil {
		t.Fatalf("mkdir workflows: %v", err)
	}
	if err := os.WriteFile(filepath.Join(dir, "workflows", "main.yaml"), []byte(bundleWorkflowYAML), 0o644); err != nil {
		t.Fatalf("write workflow: %v", err)
	}

	bundleYAML := "id: io.patchcord.example-bundle\n" +
		"version: \"1.0.0\"\n" +
		"app: app\n" +
		"workflows:\n  - workflows/main.yaml\n" +
		"requires_plugins:\n  - io.patchcord.example-text@1.0.0\n"
	if err := os.WriteFile(filepath.Join(dir, ManifestFileName), []byte(bundleYAML), 0o644); err != nil {
		t.Fatalf("write bundle.yaml: %v", err)
	}

	return dir
}

func TestPackAndInstallPackage(t *testing.T) {
	db := openTestDB(t)
	dataDir := t.TempDir()

	if _, err := plugins.Install(context.Background(), db, examplePluginPath); err != nil {
		t.Fatalf("install dependency plugin: %v", err)
	}
	knownActions, err := plugins.KnownActions(context.Background(), db)
	if err != nil {
		t.Fatalf("KnownActions() error = %v", err)
	}

	sourceDir := newTestBundleSourceDir(t)
	packagePath := filepath.Join(t.TempDir(), "bundle.patchcord-bundle")
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

	b, err := InstallPackage(context.Background(), db, dataDir, packagePath, knownActions)
	if err != nil {
		t.Fatalf("InstallPackage() error = %v", err)
	}
	if b.ID != "io.patchcord.example-bundle" || b.Version != "1.0.0" {
		t.Fatalf("bundle = %+v, want id=io.patchcord.example-bundle version=1.0.0", b)
	}

	app, err := apps.Get(context.Background(), db, "dashboard")
	if err != nil {
		t.Fatalf("embedded app was not installed: %v", err)
	}
	if app.Version != "0.1.0" {
		t.Fatalf("embedded app version = %q, want 0.1.0", app.Version)
	}

	def, err := runs.LatestWorkflow(context.Background(), db, "bundle_workflow")
	if err != nil {
		t.Fatalf("embedded workflow was not installed: %v", err)
	}
	if def.Version != 1 {
		t.Fatalf("embedded workflow version = %d, want 1", def.Version)
	}

	// The package file itself is untouched.
	if _, err := os.Stat(packagePath); err != nil {
		t.Fatalf("package file was removed: %v", err)
	}
}

func TestInstallPackage_FailsWhenARequiredPluginIsMissing(t *testing.T) {
	db := openTestDB(t)
	dataDir := t.TempDir()

	sourceDir := newTestBundleSourceDir(t)
	packagePath := filepath.Join(t.TempDir(), "bundle.patchcord-bundle")
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

	if _, err := InstallPackage(context.Background(), db, dataDir, packagePath, map[string]struct{}{}); err == nil {
		t.Fatal("expected an error for a missing required plugin, got nil")
	}

	if _, err := apps.Get(context.Background(), db, "dashboard"); !errors.Is(err, apps.ErrNotFound) {
		t.Fatalf("app was installed despite the missing dependency check failing first: err = %v", err)
	}
}

func TestPack_RejectsAnInvalidManifest(t *testing.T) {
	dir := t.TempDir() // no bundle.yaml

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

	packagePath := filepath.Join(t.TempDir(), "evil.patchcord-bundle")
	if err := os.WriteFile(packagePath, buf.Bytes(), 0o644); err != nil {
		t.Fatalf("write package file: %v", err)
	}

	if _, err := InstallPackage(context.Background(), db, dataDir, packagePath, map[string]struct{}{}); err == nil {
		t.Fatal("expected an error for a path-traversal entry, got nil")
	}
	if _, err := os.Stat(filepath.Join(filepath.Dir(dataDir), "escaped.txt")); !os.IsNotExist(err) {
		t.Fatal("path-traversal entry escaped the staging directory")
	}
}
