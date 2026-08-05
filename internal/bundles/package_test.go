package bundles

import (
	"archive/tar"
	"bytes"
	"compress/gzip"
	"context"
	"crypto/ed25519"
	"crypto/rand"
	"database/sql"
	"errors"
	"fmt"
	"io"
	"log/slog"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"

	"github.com/lucasglmt/patchcord/internal/apps"
	"github.com/lucasglmt/patchcord/internal/persistence"
	"github.com/lucasglmt/patchcord/internal/plugins"
	"github.com/lucasglmt/patchcord/internal/runs"
	"github.com/lucasglmt/patchcord/internal/trust"
	"github.com/lucasglmt/patchcord/migrations"
)

// examplePluginPath, httpPluginPath and postgresqlPluginPath are the real
// text/http/postgresql example plugins, built once for this package's
// tests, so InstallPackage can be exercised against actual installed
// plugin dependencies instead of a fixture. http and postgresql back the
// lead-crm example bundle's requires_plugins (lead_crm_example_test.go).
var (
	examplePluginPath    string
	httpPluginPath       string
	postgresqlPluginPath string
)

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

	httpPluginPath = filepath.Join(tmpDir, "http")
	if out, err := exec.Command("go", "build", "-o", httpPluginPath, "../../plugins/examples/http").CombinedOutput(); err != nil {
		panic("build http example plugin: " + err.Error() + "\n" + string(out))
	}

	postgresqlPluginPath = filepath.Join(tmpDir, "postgresql")
	if out, err := exec.Command("go", "build", "-o", postgresqlPluginPath, "../../plugins/examples/postgresql").CombinedOutput(); err != nil {
		panic("build postgresql example plugin: " + err.Error() + "\n" + string(out))
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

const bundleWorkflowYAMLTemplate = `schema_version: 1
id: bundle_workflow
version: %d
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
	return newTestBundleSourceDirVersions(t, "1.0.0", "0.1.0", 1)
}

// newTestBundleSourceDirVersions is newTestBundleSourceDir, parameterized
// on the bundle's version, the embedded app's version, and the embedded
// workflow's version — used to build a "next version" source directory
// for the same bundle/app/workflow id, so a test can exercise
// re-installing/updating a bundle in place. workflowVersion must increase
// between calls for the same workflow id: published workflow versions are
// immutable (ADR-0008), so re-declaring the same version is rejected.
func newTestBundleSourceDirVersions(t *testing.T, bundleVersion, appVersion string, workflowVersion int) string {
	t.Helper()

	dir := t.TempDir()

	if err := os.MkdirAll(filepath.Join(dir, "app"), 0o755); err != nil {
		t.Fatalf("mkdir app: %v", err)
	}
	appManifest := fmt.Sprintf("id: dashboard\nversion: %q\n", appVersion)
	if err := os.WriteFile(filepath.Join(dir, "app", apps.ManifestFileName), []byte(appManifest), 0o644); err != nil {
		t.Fatalf("write app manifest: %v", err)
	}

	if err := os.MkdirAll(filepath.Join(dir, "workflows"), 0o755); err != nil {
		t.Fatalf("mkdir workflows: %v", err)
	}
	workflowYAML := fmt.Sprintf(bundleWorkflowYAMLTemplate, workflowVersion)
	if err := os.WriteFile(filepath.Join(dir, "workflows", "main.yaml"), []byte(workflowYAML), 0o644); err != nil {
		t.Fatalf("write workflow: %v", err)
	}

	bundleYAML := "id: io.patchcord.example-bundle\n" +
		fmt.Sprintf("version: %q\n", bundleVersion) +
		"app: app\n" +
		"workflows:\n  - workflows/main.yaml\n" +
		"requires_plugins:\n  - io.patchcord.example-text@1.0.0\n"
	if err := os.WriteFile(filepath.Join(dir, ManifestFileName), []byte(bundleYAML), 0o644); err != nil {
		t.Fatalf("write bundle.yaml: %v", err)
	}

	return dir
}

// packBundle packs sourceDir into a fresh .patchcord-bundle file under
// t.TempDir() and returns its path — the "write sourceDir, Pack, create
// the package file" sequence every InstallPackage test below needs.
func packBundle(t *testing.T, sourceDir string, key ed25519.PrivateKey) string {
	t.Helper()

	packagePath := filepath.Join(t.TempDir(), "bundle.patchcord-bundle")
	f, err := os.Create(packagePath)
	if err != nil {
		t.Fatalf("create package file: %v", err)
	}
	defer f.Close()

	if err := Pack(sourceDir, key, f); err != nil {
		t.Fatalf("Pack() error = %v", err)
	}

	return packagePath
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
	packagePath := packBundle(t, sourceDir, nil)

	b, _, err := InstallPackage(context.Background(), db, dataDir, packagePath, knownActions, false)
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

// TestInstallPackage_ReinstallWithNewVersionUpdatesEmbeddedApp is a
// regression test for installEmbeddedApp calling apps.Install (strict)
// instead of apps.InstallOrUpdate: before the fix, re-installing an
// already-installed bundle whose manifest embeds an app always failed
// with apps.ErrAlreadyExists, even though bundles.record() already
// upserts the bundle's own provenance row on every install — making
// `bundle install`/`bundle update` unusable a second time (ADR-0044).
func TestInstallPackage_ReinstallWithNewVersionUpdatesEmbeddedApp(t *testing.T) {
	db := openTestDB(t)
	dataDir := t.TempDir()

	if _, err := plugins.Install(context.Background(), db, examplePluginPath); err != nil {
		t.Fatalf("install dependency plugin: %v", err)
	}
	knownActions, err := plugins.KnownActions(context.Background(), db)
	if err != nil {
		t.Fatalf("KnownActions() error = %v", err)
	}

	firstPackage := packBundle(t, newTestBundleSourceDirVersions(t, "1.0.0", "0.1.0", 1), nil)
	if _, _, err := InstallPackage(context.Background(), db, dataDir, firstPackage, knownActions, false); err != nil {
		t.Fatalf("first InstallPackage() error = %v", err)
	}

	secondPackage := packBundle(t, newTestBundleSourceDirVersions(t, "1.1.0", "0.2.0", 2), nil)
	b, _, err := InstallPackage(context.Background(), db, dataDir, secondPackage, knownActions, false)
	if err != nil {
		t.Fatalf("re-installing a bundle at a new version should succeed, got: %v", err)
	}
	if b.Version != "1.1.0" {
		t.Fatalf("bundle version = %q, want 1.1.0", b.Version)
	}

	app, err := apps.Get(context.Background(), db, "dashboard")
	if err != nil {
		t.Fatalf("embedded app was not installed: %v", err)
	}
	if app.Version != "0.2.0" {
		t.Fatalf("embedded app version = %q, want 0.2.0 (update did not take effect)", app.Version)
	}
}

// TestInstallPackage_ReinstallingUnchangedPackageIsANoOp is the
// InstallPackage counterpart of
// TestInstallDir_ReinstallingUnchangedWorkflowIsANoOp: re-running `bundle
// install` on the exact same package a second time must succeed, not hit
// ADR-0008's rejection on the embedded workflow it already installed
// byte-for-byte.
func TestInstallPackage_ReinstallingUnchangedPackageIsANoOp(t *testing.T) {
	db := openTestDB(t)
	dataDir := t.TempDir()

	if _, err := plugins.Install(context.Background(), db, examplePluginPath); err != nil {
		t.Fatalf("install dependency plugin: %v", err)
	}
	knownActions, err := plugins.KnownActions(context.Background(), db)
	if err != nil {
		t.Fatalf("KnownActions() error = %v", err)
	}

	packagePath := packBundle(t, newTestBundleSourceDir(t), nil)
	if _, _, err := InstallPackage(context.Background(), db, dataDir, packagePath, knownActions, false); err != nil {
		t.Fatalf("first InstallPackage() error = %v", err)
	}

	if _, _, err := InstallPackage(context.Background(), db, dataDir, packagePath, knownActions, false); err != nil {
		t.Fatalf("reinstalling the same package error = %v, want nil (no-op)", err)
	}

	def, err := runs.LatestWorkflow(context.Background(), db, "bundle_workflow")
	if err != nil {
		t.Fatalf("embedded workflow no longer installed: %v", err)
	}
	if def.Version != 1 {
		t.Fatalf("embedded workflow version = %d, want 1 (reinstall must not have bumped or duplicated it)", def.Version)
	}
}

func TestInstallPackage_FailsWhenARequiredPluginIsMissing(t *testing.T) {
	db := openTestDB(t)
	dataDir := t.TempDir()

	sourceDir := newTestBundleSourceDir(t)
	packagePath := packBundle(t, sourceDir, nil)

	if _, _, err := InstallPackage(context.Background(), db, dataDir, packagePath, map[string]struct{}{}, false); err == nil {
		t.Fatal("expected an error for a missing required plugin, got nil")
	}

	if _, err := apps.Get(context.Background(), db, "dashboard"); !errors.Is(err, apps.ErrNotFound) {
		t.Fatalf("app was installed despite the missing dependency check failing first: err = %v", err)
	}
}

func TestInstallPackage_SignatureAndTrustPolicy(t *testing.T) {
	db := openTestDB(t)

	if _, err := plugins.Install(context.Background(), db, examplePluginPath); err != nil {
		t.Fatalf("install dependency plugin: %v", err)
	}
	knownActions, err := plugins.KnownActions(context.Background(), db)
	if err != nil {
		t.Fatalf("KnownActions() error = %v", err)
	}

	sourceDir := newTestBundleSourceDir(t)
	pub, priv, err := ed25519.GenerateKey(rand.Reader)
	if err != nil {
		t.Fatalf("GenerateKey() error = %v", err)
	}

	packagePath := packBundle(t, sourceDir, priv)

	t.Run("signed but untrusted key: rejected with requireSignature", func(t *testing.T) {
		if _, _, err := InstallPackage(context.Background(), db, t.TempDir(), packagePath, knownActions, true); !errors.Is(err, trust.ErrSignatureRequired) {
			t.Fatalf("InstallPackage() error = %v, want ErrSignatureRequired", err)
		}
	})

	t.Run("signed and trusted: accepted with requireSignature", func(t *testing.T) {
		if err := trust.Add(context.Background(), db, "io.patchcord.example-bundle", pub, "test"); err != nil {
			t.Fatalf("trust.Add() error = %v", err)
		}

		b, policy, err := InstallPackage(context.Background(), db, t.TempDir(), packagePath, knownActions, true)
		if err != nil {
			t.Fatalf("InstallPackage() error = %v", err)
		}
		if b.ID != "io.patchcord.example-bundle" {
			t.Fatalf("ID = %q, want %q", b.ID, "io.patchcord.example-bundle")
		}
		if !policy.Trusted {
			t.Fatal("Trusted = false, want true")
		}
	})
}

// TestInstallDir_InstallsAppLiveAndWorkflow exercises InstallDir directly
// against a source directory — no Pack/InstallPackage round trip — and
// confirms the embedded app is served straight from dir's app
// subdirectory (apps.Get's StaticDir), not copied or moved anywhere,
// mirroring apps.InstallOrUpdate's own "live" contract behind `app dev`.
func TestInstallDir_InstallsAppLiveAndWorkflow(t *testing.T) {
	db := openTestDB(t)

	if _, err := plugins.Install(context.Background(), db, examplePluginPath); err != nil {
		t.Fatalf("install dependency plugin: %v", err)
	}
	knownActions, err := plugins.KnownActions(context.Background(), db)
	if err != nil {
		t.Fatalf("KnownActions() error = %v", err)
	}

	sourceDir := newTestBundleSourceDir(t)

	b, err := InstallDir(context.Background(), db, sourceDir, knownActions)
	if err != nil {
		t.Fatalf("InstallDir() error = %v", err)
	}
	if b.ID != "io.patchcord.example-bundle" || b.Version != "1.0.0" {
		t.Fatalf("bundle = %+v, want id=io.patchcord.example-bundle version=1.0.0", b)
	}

	app, err := apps.Get(context.Background(), db, "dashboard")
	if err != nil {
		t.Fatalf("embedded app was not installed: %v", err)
	}
	wantStaticDir, err := filepath.Abs(filepath.Join(sourceDir, "app"))
	if err != nil {
		t.Fatalf("filepath.Abs() error = %v", err)
	}
	if app.StaticDir != wantStaticDir {
		t.Fatalf("app.StaticDir = %q, want %q (InstallDir must serve the app live off sourceDir, never copy or move it)", app.StaticDir, wantStaticDir)
	}

	def, err := runs.LatestWorkflow(context.Background(), db, "bundle_workflow")
	if err != nil {
		t.Fatalf("embedded workflow was not installed: %v", err)
	}
	if def.Version != 1 {
		t.Fatalf("embedded workflow version = %d, want 1", def.Version)
	}
}

// TestInstallDir_ReinstallUpdatesAppInPlace exercises the `bundle dev`
// loop: install, then reinstall from a changed source directory (new app
// version, new workflow version — bumping the workflow version is
// required, ADR-0008), and confirm it succeeds and updates in place,
// exactly like `app dev`'s own reinstall test.
func TestInstallDir_ReinstallUpdatesAppInPlace(t *testing.T) {
	db := openTestDB(t)

	if _, err := plugins.Install(context.Background(), db, examplePluginPath); err != nil {
		t.Fatalf("install dependency plugin: %v", err)
	}
	knownActions, err := plugins.KnownActions(context.Background(), db)
	if err != nil {
		t.Fatalf("KnownActions() error = %v", err)
	}

	firstDir := newTestBundleSourceDirVersions(t, "1.0.0", "0.1.0", 1)
	if _, err := InstallDir(context.Background(), db, firstDir, knownActions); err != nil {
		t.Fatalf("first InstallDir() error = %v", err)
	}

	secondDir := newTestBundleSourceDirVersions(t, "1.1.0", "0.2.0", 2)
	b, err := InstallDir(context.Background(), db, secondDir, knownActions)
	if err != nil {
		t.Fatalf("second InstallDir() error = %v, want it to update in place instead of failing", err)
	}
	if b.Version != "1.1.0" {
		t.Fatalf("bundle version = %q, want 1.1.0", b.Version)
	}

	app, err := apps.Get(context.Background(), db, "dashboard")
	if err != nil {
		t.Fatalf("embedded app was not installed: %v", err)
	}
	if app.Version != "0.2.0" {
		t.Fatalf("embedded app version = %q, want 0.2.0 (reinstall did not take effect)", app.Version)
	}
}

// TestInstallDir_AutoBumpsVersionWhenContentChangesWithoutABump is the
// regression test for ADR-0055: editing an embedded workflow's body
// without bumping its `version:` field must no longer fail `bundle
// dev`/`patchcord dev` — it must auto-install under the next unused
// version instead, leaving the source file and the original version's row
// untouched.
func TestInstallDir_AutoBumpsVersionWhenContentChangesWithoutABump(t *testing.T) {
	db := openTestDB(t)

	if _, err := plugins.Install(context.Background(), db, examplePluginPath); err != nil {
		t.Fatalf("install dependency plugin: %v", err)
	}
	knownActions, err := plugins.KnownActions(context.Background(), db)
	if err != nil {
		t.Fatalf("KnownActions() error = %v", err)
	}

	sourceDir := newTestBundleSourceDir(t)
	if _, err := InstallDir(context.Background(), db, sourceDir, knownActions); err != nil {
		t.Fatalf("first InstallDir() error = %v", err)
	}

	// Edit the workflow's step body in place, without bumping its
	// `version: 1` field.
	workflowPath := filepath.Join(sourceDir, "workflows", "main.yaml")
	original := fmt.Sprintf(bundleWorkflowYAMLTemplate, 1)
	changed := strings.Replace(original, `value: "hi"`, `value: "bye"`, 1)
	if err := os.WriteFile(workflowPath, []byte(changed), 0o644); err != nil {
		t.Fatalf("rewrite workflow: %v", err)
	}

	if _, err := InstallDir(context.Background(), db, sourceDir, knownActions); err != nil {
		t.Fatalf("second InstallDir() with an unbumped but changed workflow error = %v, want nil (auto-bumped)", err)
	}

	// The source file on disk still declares version 1 — InstallDir must
	// never rewrite it.
	onDisk, err := os.ReadFile(workflowPath)
	if err != nil {
		t.Fatalf("read workflow file: %v", err)
	}
	if string(onDisk) != changed {
		t.Fatalf("workflow file on disk was modified, want it left exactly as written")
	}

	latest, err := runs.LatestWorkflow(context.Background(), db, "bundle_workflow")
	if err != nil {
		t.Fatalf("LatestWorkflow() error = %v", err)
	}
	if latest.Version != 2 {
		t.Fatalf("LatestWorkflow().Version = %d, want 2 (auto-bumped)", latest.Version)
	}

	v1Source, err := runs.WorkflowSource(context.Background(), db, "bundle_workflow", 1)
	if err != nil {
		t.Fatalf("WorkflowSource(version 1) error = %v", err)
	}
	if v1Source != original {
		t.Fatalf("version 1's recorded content changed, want the original left immutable")
	}
}

// TestInstallDir_ReinstallingUnchangedWorkflowIsANoOp is the regression
// test for the `bundle dev --watch` bug Lucas reported: reinstalling from
// the exact same, untouched source directory (e.g. because an unrelated
// file under dir changed and the watcher reinstalled the whole bundle,
// internal/cli/bundle.go's newBundleDevCommand) must succeed, not hit
// ADR-0008's "UNIQUE constraint failed: workflow_versions.workflow_id,
// workflow_versions.version" rejection on the untouched embedded workflow.
func TestInstallDir_ReinstallingUnchangedWorkflowIsANoOp(t *testing.T) {
	db := openTestDB(t)

	if _, err := plugins.Install(context.Background(), db, examplePluginPath); err != nil {
		t.Fatalf("install dependency plugin: %v", err)
	}
	knownActions, err := plugins.KnownActions(context.Background(), db)
	if err != nil {
		t.Fatalf("KnownActions() error = %v", err)
	}

	sourceDir := newTestBundleSourceDir(t)
	if _, err := InstallDir(context.Background(), db, sourceDir, knownActions); err != nil {
		t.Fatalf("first InstallDir() error = %v", err)
	}

	if _, err := InstallDir(context.Background(), db, sourceDir, knownActions); err != nil {
		t.Fatalf("second InstallDir() on an unchanged directory error = %v, want nil (no-op)", err)
	}

	def, err := runs.LatestWorkflow(context.Background(), db, "bundle_workflow")
	if err != nil {
		t.Fatalf("embedded workflow no longer installed: %v", err)
	}
	if def.Version != 1 {
		t.Fatalf("embedded workflow version = %d, want 1 (reinstall must not have bumped or duplicated it)", def.Version)
	}
}

// TestInstallDir_FailsWhenARequiredPluginIsMissing mirrors
// TestInstallPackage_FailsWhenARequiredPluginIsMissing for the directory
// path: the plugin dependency check runs before anything is installed.
func TestInstallDir_FailsWhenARequiredPluginIsMissing(t *testing.T) {
	db := openTestDB(t)
	sourceDir := newTestBundleSourceDir(t)

	if _, err := InstallDir(context.Background(), db, sourceDir, map[string]struct{}{}); err == nil {
		t.Fatal("expected an error for a missing required plugin, got nil")
	}

	if _, err := apps.Get(context.Background(), db, "dashboard"); !errors.Is(err, apps.ErrNotFound) {
		t.Fatalf("app was installed despite the missing dependency check failing first: err = %v", err)
	}
}

func TestPack_RejectsAnInvalidManifest(t *testing.T) {
	dir := t.TempDir() // no bundle.yaml

	if err := Pack(dir, nil, io.Discard); err == nil {
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

	if _, _, err := InstallPackage(context.Background(), db, dataDir, packagePath, map[string]struct{}{}, false); err == nil {
		t.Fatal("expected an error for a path-traversal entry, got nil")
	}
	if _, err := os.Stat(filepath.Join(filepath.Dir(dataDir), "escaped.txt")); !os.IsNotExist(err) {
		t.Fatal("path-traversal entry escaped the staging directory")
	}
}
