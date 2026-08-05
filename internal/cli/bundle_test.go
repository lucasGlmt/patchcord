package cli

import (
	"bytes"
	"context"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

func TestNewRootCommand_HasBundleSubcommands(t *testing.T) {
	root := NewRootCommand()

	for _, name := range []string{"install", "update", "dev", "pack", "list", "inspect"} {
		t.Run(name, func(t *testing.T) {
			cmd, _, err := root.Find([]string{"bundle", name})
			if err != nil {
				t.Fatalf("Find(bundle %s) error = %v", name, err)
			}
			if cmd.Name() != name {
				t.Fatalf("found command %q, want %q", cmd.Name(), name)
			}
		})
	}
}

const bundleTestWorkflowYAMLTemplate = `schema_version: 1
id: bundle_cli_workflow
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
// on the bundle's, embedded app's, and embedded workflow's version — used
// to build a "next version" source directory for the same bundle/app/
// workflow id, so a test can exercise `bundle update`. workflowVersion
// must increase between calls for the same workflow id: published
// workflow versions are immutable (ADR-0008).
func newTestBundleSourceDirVersions(t *testing.T, bundleVersion, appVersion string, workflowVersion int) string {
	t.Helper()

	dir := t.TempDir()

	if err := os.MkdirAll(filepath.Join(dir, "app"), 0o755); err != nil {
		t.Fatalf("mkdir app: %v", err)
	}
	appManifest := fmt.Sprintf("id: bundle-dashboard\nversion: %q\n", appVersion)
	if err := os.WriteFile(filepath.Join(dir, "app", "patchcord-app.yaml"), []byte(appManifest), 0o644); err != nil {
		t.Fatalf("write app manifest: %v", err)
	}

	if err := os.MkdirAll(filepath.Join(dir, "workflows"), 0o755); err != nil {
		t.Fatalf("mkdir workflows: %v", err)
	}
	workflowYAML := fmt.Sprintf(bundleTestWorkflowYAMLTemplate, workflowVersion)
	if err := os.WriteFile(filepath.Join(dir, "workflows", "main.yaml"), []byte(workflowYAML), 0o644); err != nil {
		t.Fatalf("write workflow: %v", err)
	}

	bundleYAML := "id: io.patchcord.example-bundle\n" +
		fmt.Sprintf("version: %q\n", bundleVersion) +
		"app: app\n" +
		"workflows:\n  - workflows/main.yaml\n" +
		"requires_plugins:\n  - io.patchcord.example-text@1.0.0\n"
	if err := os.WriteFile(filepath.Join(dir, "bundle.yaml"), []byte(bundleYAML), 0o644); err != nil {
		t.Fatalf("write bundle.yaml: %v", err)
	}

	return dir
}

// writeLocalRegistryFixture builds a local-directory registry (index.json
// plus the given packages, keyed by their path relative to the
// registry's root) — the same fixture shape internal/registry's own tests
// use, reused here to exercise `bundle install <ref>`/`bundle update`
// through the CLI.
func writeLocalRegistryFixture(t *testing.T, indexJSON string) string {
	t.Helper()

	dir := t.TempDir()
	if err := os.WriteFile(filepath.Join(dir, "index.json"), []byte(indexJSON), 0o644); err != nil {
		t.Fatalf("write index.json: %v", err)
	}
	return dir
}

// TestBundleCommands_FullLifecycle exercises pack, install, list and
// inspect against a bundle built on the real example plugin, exactly as a
// user would type them on the command line.
func TestBundleCommands_FullLifecycle(t *testing.T) {
	dataDir := t.TempDir()

	pluginInstall := newPluginInstallCommand()
	pluginInstall.SetArgs([]string{examplePluginPath, "--data-dir", dataDir})
	pluginInstall.SetContext(context.Background())
	if err := pluginInstall.Execute(); err != nil {
		t.Fatalf("plugin install error = %v", err)
	}

	sourceDir := newTestBundleSourceDir(t)
	packagePath := filepath.Join(t.TempDir(), "bundle.patchcord-bundle")

	pack := newBundlePackCommand()
	pack.SetArgs([]string{sourceDir, "--output", packagePath})
	pack.SetContext(context.Background())
	var packOut bytes.Buffer
	pack.SetOut(&packOut)
	if err := pack.Execute(); err != nil {
		t.Fatalf("bundle pack error = %v", err)
	}
	if !strings.Contains(packOut.String(), packagePath) {
		t.Fatalf("pack output = %q, want it to mention %q", packOut.String(), packagePath)
	}

	install := newBundleInstallCommand()
	install.SetArgs([]string{packagePath, "--data-dir", dataDir})
	install.SetContext(context.Background())
	var installOut bytes.Buffer
	install.SetOut(&installOut)
	if err := install.Execute(); err != nil {
		t.Fatalf("bundle install error = %v", err)
	}
	if !strings.Contains(installOut.String(), "io.patchcord.example-bundle") {
		t.Fatalf("install output = %q, want it to mention the installed bundle id", installOut.String())
	}

	list := newBundleListCommand()
	list.SetArgs([]string{"--data-dir", dataDir})
	list.SetContext(context.Background())
	var listOut bytes.Buffer
	list.SetOut(&listOut)
	if err := list.Execute(); err != nil {
		t.Fatalf("bundle list error = %v", err)
	}
	if !strings.Contains(listOut.String(), "io.patchcord.example-bundle") {
		t.Fatalf("list output = %q, want it to mention the installed bundle id", listOut.String())
	}

	inspect := newBundleInspectCommand()
	inspect.SetArgs([]string{"io.patchcord.example-bundle", "--data-dir", dataDir})
	inspect.SetContext(context.Background())
	var inspectOut bytes.Buffer
	inspect.SetOut(&inspectOut)
	if err := inspect.Execute(); err != nil {
		t.Fatalf("bundle inspect error = %v", err)
	}
	if !strings.Contains(inspectOut.String(), "requires_plugins") {
		t.Fatalf("inspect output = %q, want it to include the raw manifest", inspectOut.String())
	}

	appInspect := newAppListCommand()
	appInspect.SetArgs([]string{"--data-dir", dataDir})
	appInspect.SetContext(context.Background())
	var appOut bytes.Buffer
	appInspect.SetOut(&appOut)
	if err := appInspect.Execute(); err != nil {
		t.Fatalf("app list error = %v", err)
	}
	if !strings.Contains(appOut.String(), "bundle-dashboard") {
		t.Fatalf("app list output = %q, want it to mention the embedded app", appOut.String())
	}
}

func TestBundleInstallCommand_FailsWhenARequiredPluginIsMissing(t *testing.T) {
	sourceDir := newTestBundleSourceDir(t)
	packagePath := filepath.Join(t.TempDir(), "bundle.patchcord-bundle")

	pack := newBundlePackCommand()
	pack.SetArgs([]string{sourceDir, "--output", packagePath})
	pack.SetContext(context.Background())
	if err := pack.Execute(); err != nil {
		t.Fatalf("bundle pack error = %v", err)
	}

	install := newBundleInstallCommand()
	install.SetArgs([]string{packagePath, "--data-dir", t.TempDir()})
	install.SetContext(context.Background())
	if err := install.Execute(); err == nil {
		t.Fatal("expected an error for a missing required plugin, got nil")
	}
}

// TestBundleDevCommand_InstallsFromDirectoryAndUpdatesInPlace exercises the
// `bundle dev` loop without --watch: install straight from a directory
// (no pack/install round trip), then reinstall from a changed directory,
// and confirm it succeeds where `bundle install` (from a fresh package)
// would need a full re-pack — the same shape as TestAppDevCommand_UpdatesInPlace.
func TestBundleDevCommand_InstallsFromDirectoryAndUpdatesInPlace(t *testing.T) {
	dataDir := t.TempDir()
	installExampleTextPlugin(t, dataDir)

	firstDir := newTestBundleSourceDirVersions(t, "1.0.0", "0.1.0", 1)

	dev := newBundleDevCommand()
	dev.SetArgs([]string{firstDir, "--data-dir", dataDir})
	dev.SetContext(context.Background())
	var devOut bytes.Buffer
	dev.SetOut(&devOut)
	if err := dev.Execute(); err != nil {
		t.Fatalf("bundle dev error = %v", err)
	}
	if !strings.Contains(devOut.String(), "io.patchcord.example-bundle") {
		t.Fatalf("dev output = %q, want it to mention the bundle id", devOut.String())
	}

	secondDir := newTestBundleSourceDirVersions(t, "1.1.0", "0.2.0", 2)

	devAgain := newBundleDevCommand()
	devAgain.SetArgs([]string{secondDir, "--data-dir", dataDir})
	devAgain.SetContext(context.Background())
	if err := devAgain.Execute(); err != nil {
		t.Fatalf("second bundle dev error = %v, want it to update in place instead of failing", err)
	}

	appList := newAppListCommand()
	appList.SetArgs([]string{"--data-dir", dataDir})
	appList.SetContext(context.Background())
	var appOut bytes.Buffer
	appList.SetOut(&appOut)
	if err := appList.Execute(); err != nil {
		t.Fatalf("app list error = %v", err)
	}
	if !strings.Contains(appOut.String(), "0.2.0") {
		t.Fatalf("app list output = %q, want it to show the updated embedded app version 0.2.0", appOut.String())
	}
}

func TestBundleDevCommand_FailsWhenARequiredPluginIsMissing(t *testing.T) {
	dev := newBundleDevCommand()
	dev.SetArgs([]string{newTestBundleSourceDir(t), "--data-dir", t.TempDir()})
	dev.SetContext(context.Background())

	if err := dev.Execute(); err == nil {
		t.Fatal("expected an error for a missing required plugin, got nil")
	}
}

// TestBundleDevCommand_Watch exercises the --watch path end to end through
// the CLI: initial install, then a change to the embedded app's manifest
// on disk (a version bump — no version to bump on a workflow this time,
// since re-triggering the same one would hit ADR-0008) picked up
// automatically, then Ctrl+C (context cancellation) stops it cleanly.
func TestBundleDevCommand_Watch(t *testing.T) {
	dataDir := t.TempDir()
	installExampleTextPlugin(t, dataDir)

	sourceDir := newTestBundleSourceDirVersions(t, "1.0.0", "0.1.0", 1)

	dev := newBundleDevCommand()
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	dev.SetContext(ctx)
	dev.SetArgs([]string{sourceDir, "--data-dir", dataDir, "--watch"})
	var out bytes.Buffer
	dev.SetOut(&out)
	dev.SetErr(&out)

	done := make(chan error, 1)
	go func() { done <- dev.Execute() }()

	// Wait for the initial install to complete and the watch to start.
	deadline := time.Now().Add(5 * time.Second)
	for !strings.Contains(out.String(), "Watching") {
		if time.Now().After(deadline) {
			t.Fatalf("bundle dev --watch did not start watching in time, output so far: %q", out.String())
		}
		time.Sleep(20 * time.Millisecond)
	}

	appManifestPath := filepath.Join(sourceDir, "app", "patchcord-app.yaml")
	if err := os.WriteFile(appManifestPath, []byte("id: bundle-dashboard\nversion: \"0.2.0\"\n"), 0o644); err != nil {
		t.Fatalf("rewrite app manifest: %v", err)
	}

	deadline = time.Now().Add(5 * time.Second)
	for {
		app, err := getAppOutput(t, dataDir)
		if err == nil && strings.Contains(app, "0.2.0") {
			break
		}
		if time.Now().After(deadline) {
			t.Fatalf("app was not reinstalled at 0.2.0 within the deadline; last app list output: %q", app)
		}
		time.Sleep(50 * time.Millisecond)
	}

	cancel()
	select {
	case err := <-done:
		if err != nil {
			t.Fatalf("bundle dev --watch error = %v", err)
		}
	case <-time.After(5 * time.Second):
		t.Fatal("bundle dev --watch did not stop after context cancellation")
	}
}

// getAppOutput runs `app list` against dataDir and returns its output, for
// polling in TestBundleDevCommand_Watch.
func getAppOutput(t *testing.T, dataDir string) (string, error) {
	t.Helper()

	appList := newAppListCommand()
	appList.SetArgs([]string{"--data-dir", dataDir})
	appList.SetContext(context.Background())
	var appOut bytes.Buffer
	appList.SetOut(&appOut)
	err := appList.Execute()
	return appOut.String(), err
}

func TestBundleInspectCommand_UnknownBundle(t *testing.T) {
	cmd := newBundleInspectCommand()
	cmd.SetArgs([]string{"io.patchcord.unknown", "--data-dir", t.TempDir()})
	cmd.SetContext(context.Background())

	err := cmd.Execute()
	if err == nil {
		t.Fatal("expected an error for an unknown bundle id, got nil")
	}
	if !strings.Contains(err.Error(), "not found") {
		t.Fatalf("error = %q, want it to mention the bundle was not found", err.Error())
	}
}

func installExampleTextPlugin(t *testing.T, dataDir string) {
	t.Helper()

	install := newPluginInstallCommand()
	install.SetArgs([]string{examplePluginPath, "--data-dir", dataDir})
	install.SetContext(context.Background())
	if err := install.Execute(); err != nil {
		t.Fatalf("plugin install error = %v", err)
	}
}

func addRegistry(t *testing.T, name, location, dataDir string) {
	t.Helper()

	add := newRegistryAddCommand()
	add.SetArgs([]string{name, location, "--data-dir", dataDir})
	add.SetContext(context.Background())
	if err := add.Execute(); err != nil {
		t.Fatalf("registry add error = %v", err)
	}
}

// TestBundleInstallCommand_FromRegistryReference exercises `bundle
// install <ref>` against a local-directory registry: both a bare id
// (resolves to the index's declared latest) and an explicit id@version.
func TestBundleInstallCommand_FromRegistryReference(t *testing.T) {
	registryDir := writeLocalRegistryFixture(t, `{
		"schemaVersion": 1,
		"packages": {
			"io.patchcord.example-bundle": {
				"kind": "bundle",
				"latest": "1.0.0",
				"versions": {"1.0.0": "example-bundle-1.0.0.patchcord-bundle"}
			}
		}
	}`)

	pack := newBundlePackCommand()
	pack.SetArgs([]string{newTestBundleSourceDir(t), "--output", filepath.Join(registryDir, "example-bundle-1.0.0.patchcord-bundle")})
	pack.SetContext(context.Background())
	if err := pack.Execute(); err != nil {
		t.Fatalf("bundle pack error = %v", err)
	}

	t.Run("bare id resolves to latest", func(t *testing.T) {
		dataDir := t.TempDir()
		installExampleTextPlugin(t, dataDir)
		addRegistry(t, "local", registryDir, dataDir)

		install := newBundleInstallCommand()
		install.SetArgs([]string{"io.patchcord.example-bundle", "--data-dir", dataDir})
		install.SetContext(context.Background())
		var out bytes.Buffer
		install.SetOut(&out)
		if err := install.Execute(); err != nil {
			t.Fatalf("bundle install error = %v", err)
		}
		if !strings.Contains(out.String(), "io.patchcord.example-bundle") {
			t.Fatalf("install output = %q, want it to mention the installed bundle id", out.String())
		}
	})

	t.Run("explicit id@version resolves to that version", func(t *testing.T) {
		dataDir := t.TempDir()
		installExampleTextPlugin(t, dataDir)
		addRegistry(t, "local", registryDir, dataDir)

		install := newBundleInstallCommand()
		install.SetArgs([]string{"io.patchcord.example-bundle@1.0.0", "--data-dir", dataDir})
		install.SetContext(context.Background())
		var out bytes.Buffer
		install.SetOut(&out)
		if err := install.Execute(); err != nil {
			t.Fatalf("bundle install error = %v", err)
		}
		if !strings.Contains(out.String(), "1.0.0") {
			t.Fatalf("install output = %q, want it to mention version 1.0.0", out.String())
		}
	})
}

// TestBundleUpdateCommand_InstallsNewerVersionAndUpdatesEmbeddedApp exercises
// `bundle update` end to end: an installed bundle is updated to a newer
// version resolved from a configured registry, and its embedded app's
// version actually changes — the CLI-level exercise of the
// installEmbeddedApp fix (ADR-0044).
func TestBundleUpdateCommand_InstallsNewerVersionAndUpdatesEmbeddedApp(t *testing.T) {
	dataDir := t.TempDir()
	installExampleTextPlugin(t, dataDir)

	firstPackage := filepath.Join(t.TempDir(), "bundle-1.0.0.patchcord-bundle")
	pack1 := newBundlePackCommand()
	pack1.SetArgs([]string{newTestBundleSourceDirVersions(t, "1.0.0", "0.1.0", 1), "--output", firstPackage})
	pack1.SetContext(context.Background())
	if err := pack1.Execute(); err != nil {
		t.Fatalf("bundle pack (v1) error = %v", err)
	}

	install := newBundleInstallCommand()
	install.SetArgs([]string{firstPackage, "--data-dir", dataDir})
	install.SetContext(context.Background())
	if err := install.Execute(); err != nil {
		t.Fatalf("bundle install (v1) error = %v", err)
	}

	registryDir := writeLocalRegistryFixture(t, `{
		"schemaVersion": 1,
		"packages": {
			"io.patchcord.example-bundle": {
				"kind": "bundle",
				"latest": "1.1.0",
				"versions": {"1.1.0": "example-bundle-1.1.0.patchcord-bundle"}
			}
		}
	}`)
	pack2 := newBundlePackCommand()
	pack2.SetArgs([]string{newTestBundleSourceDirVersions(t, "1.1.0", "0.2.0", 2), "--output", filepath.Join(registryDir, "example-bundle-1.1.0.patchcord-bundle")})
	pack2.SetContext(context.Background())
	if err := pack2.Execute(); err != nil {
		t.Fatalf("bundle pack (v2) error = %v", err)
	}
	addRegistry(t, "local", registryDir, dataDir)

	update := newBundleUpdateCommand()
	update.SetArgs([]string{"io.patchcord.example-bundle", "--data-dir", dataDir})
	update.SetContext(context.Background())
	var updateOut bytes.Buffer
	update.SetOut(&updateOut)
	if err := update.Execute(); err != nil {
		t.Fatalf("bundle update error = %v", err)
	}
	if !strings.Contains(updateOut.String(), "Updated io.patchcord.example-bundle: 1.0.0 -> 1.1.0") {
		t.Fatalf("update output = %q, want it to report 1.0.0 -> 1.1.0", updateOut.String())
	}

	appList := newAppListCommand()
	appList.SetArgs([]string{"--data-dir", dataDir})
	appList.SetContext(context.Background())
	var appOut bytes.Buffer
	appList.SetOut(&appOut)
	if err := appList.Execute(); err != nil {
		t.Fatalf("app list error = %v", err)
	}
	if !strings.Contains(appOut.String(), "0.2.0") {
		t.Fatalf("app list output = %q, want it to show the updated embedded app version 0.2.0", appOut.String())
	}
}

func TestBundleUpdateCommand_AlreadyUpToDate(t *testing.T) {
	dataDir := t.TempDir()
	installExampleTextPlugin(t, dataDir)

	packagePath := filepath.Join(t.TempDir(), "bundle-1.0.0.patchcord-bundle")
	pack := newBundlePackCommand()
	pack.SetArgs([]string{newTestBundleSourceDir(t), "--output", packagePath})
	pack.SetContext(context.Background())
	if err := pack.Execute(); err != nil {
		t.Fatalf("bundle pack error = %v", err)
	}

	install := newBundleInstallCommand()
	install.SetArgs([]string{packagePath, "--data-dir", dataDir})
	install.SetContext(context.Background())
	if err := install.Execute(); err != nil {
		t.Fatalf("bundle install error = %v", err)
	}

	registryDir := writeLocalRegistryFixture(t, `{
		"schemaVersion": 1,
		"packages": {
			"io.patchcord.example-bundle": {
				"kind": "bundle",
				"latest": "1.0.0",
				"versions": {"1.0.0": "example-bundle-1.0.0.patchcord-bundle"}
			}
		}
	}`)
	if err := os.Rename(packagePath, filepath.Join(registryDir, "example-bundle-1.0.0.patchcord-bundle")); err != nil {
		// packagePath was already installed from, so its bytes don't need
		// to survive — reuse them for the registry's copy instead of
		// packing a second time.
		t.Fatalf("stage registry package: %v", err)
	}
	addRegistry(t, "local", registryDir, dataDir)

	update := newBundleUpdateCommand()
	update.SetArgs([]string{"io.patchcord.example-bundle", "--data-dir", dataDir})
	update.SetContext(context.Background())
	var out bytes.Buffer
	update.SetOut(&out)
	if err := update.Execute(); err != nil {
		t.Fatalf("bundle update error = %v", err)
	}
	if !strings.Contains(out.String(), "is already up to date") {
		t.Fatalf("update output = %q, want the already-up-to-date message", out.String())
	}
}

func TestBundleUpdateCommand_NotInstalled(t *testing.T) {
	update := newBundleUpdateCommand()
	update.SetArgs([]string{"io.patchcord.never-installed", "--data-dir", t.TempDir()})
	update.SetContext(context.Background())

	err := update.Execute()
	if err == nil {
		t.Fatal("expected an error for a never-installed bundle id, got nil")
	}
	if !strings.Contains(err.Error(), "bundle install") {
		t.Fatalf("error = %q, want it to mention running `bundle install` first", err.Error())
	}
}

// TestBundleNewCommand_ThenPackThenInstall exercises `bundle new` through
// to a real install. requires_plugins starts empty, so no plugin needs to
// be installed first.
func TestBundleNewCommand_ThenPackThenInstall(t *testing.T) {
	dir := filepath.Join(t.TempDir(), "scaffold-test")
	id := "io.patchcord.scaffold-test-bundle"

	newCmd := newBundleNewCommand()
	newCmd.SetArgs([]string{id, "--output", dir})
	newCmd.SetContext(context.Background())
	var newOut bytes.Buffer
	newCmd.SetOut(&newOut)
	if err := newCmd.Execute(); err != nil {
		t.Fatalf("bundle new error = %v", err)
	}
	if !strings.Contains(newOut.String(), id) {
		t.Fatalf("new output = %q, want it to mention %q", newOut.String(), id)
	}

	packagePath := filepath.Join(t.TempDir(), "scaffold-test.patchcord-bundle")
	pack := newBundlePackCommand()
	pack.SetArgs([]string{dir, "--output", packagePath})
	pack.SetContext(context.Background())
	if err := pack.Execute(); err != nil {
		t.Fatalf("bundle pack error = %v", err)
	}

	dataDir := t.TempDir()

	// The scaffolded workflow's step uses text.uppercase@1 (see
	// internal/workflow/scaffold.go) — install the reference plugin first
	// so `bundle install`'s workflow validation has that action to check
	// against.
	pluginInstall := newPluginInstallCommand()
	pluginInstall.SetArgs([]string{examplePluginPath, "--data-dir", dataDir})
	pluginInstall.SetContext(context.Background())
	if err := pluginInstall.Execute(); err != nil {
		t.Fatalf("plugin install error = %v", err)
	}

	install := newBundleInstallCommand()
	install.SetArgs([]string{packagePath, "--data-dir", dataDir})
	install.SetContext(context.Background())
	var installOut bytes.Buffer
	install.SetOut(&installOut)
	if err := install.Execute(); err != nil {
		t.Fatalf("bundle install error = %v", err)
	}
	if !strings.Contains(installOut.String(), id) {
		t.Fatalf("install output = %q, want it to mention %q", installOut.String(), id)
	}
}

// TestBundleNewCommand_TemplateVite_ScaffoldsAViteEmbeddedApp exercises
// `bundle new --template vite`: it must delegate to bundles.ScaffoldVite,
// so the embedded app is a Vite project and bundle.yaml's app field
// already points at app/dist (only populated once that project is built).
func TestBundleNewCommand_TemplateVite_ScaffoldsAViteEmbeddedApp(t *testing.T) {
	dir := filepath.Join(t.TempDir(), "scaffold-test")
	id := "io.patchcord.scaffold-test-bundle"

	newCmd := newBundleNewCommand()
	newCmd.SetArgs([]string{id, "--output", dir, "--template", "vite"})
	newCmd.SetContext(context.Background())
	var newOut bytes.Buffer
	newCmd.SetOut(&newOut)
	if err := newCmd.Execute(); err != nil {
		t.Fatalf("bundle new --template vite error = %v", err)
	}
	if !strings.Contains(newOut.String(), "npm install") {
		t.Fatalf("new output = %q, want it to mention the npm install/build step", newOut.String())
	}

	if _, err := os.Stat(filepath.Join(dir, "app", "package.json")); err != nil {
		t.Fatalf("app/package.json missing: %v", err)
	}
	manifestBytes, err := os.ReadFile(filepath.Join(dir, "bundle.yaml"))
	if err != nil {
		t.Fatalf("read bundle.yaml: %v", err)
	}
	if !strings.Contains(string(manifestBytes), "app: app/dist") {
		t.Fatalf("bundle.yaml = %q, want it to point app at app/dist", manifestBytes)
	}
}

// TestBundleNewCommand_UnknownTemplate_Errors guards --template's
// validation: an unrecognized value must fail clearly rather than
// silently falling back to the static template.
func TestBundleNewCommand_UnknownTemplate_Errors(t *testing.T) {
	dir := filepath.Join(t.TempDir(), "scaffold-test")

	newCmd := newBundleNewCommand()
	newCmd.SetArgs([]string{"io.patchcord.scaffold-test-bundle", "--output", dir, "--template", "svelte"})
	newCmd.SetContext(context.Background())
	if err := newCmd.Execute(); err == nil {
		t.Fatal("expected an error for an unknown --template value, got nil")
	}
}
