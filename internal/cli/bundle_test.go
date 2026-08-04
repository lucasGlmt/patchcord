package cli

import (
	"bytes"
	"context"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestNewRootCommand_HasBundleSubcommands(t *testing.T) {
	root := NewRootCommand()

	for _, name := range []string{"install", "update", "pack", "list", "inspect"} {
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
