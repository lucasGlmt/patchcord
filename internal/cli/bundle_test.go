package cli

import (
	"bytes"
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestNewRootCommand_HasBundleSubcommands(t *testing.T) {
	root := NewRootCommand()

	for _, name := range []string{"install", "pack", "list", "inspect"} {
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

const bundleTestWorkflowYAML = `schema_version: 1
id: bundle_cli_workflow
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
	if err := os.WriteFile(filepath.Join(dir, "app", "patchcord-app.yaml"), []byte("id: bundle-dashboard\nversion: \"0.1.0\"\n"), 0o644); err != nil {
		t.Fatalf("write app manifest: %v", err)
	}

	if err := os.MkdirAll(filepath.Join(dir, "workflows"), 0o755); err != nil {
		t.Fatalf("mkdir workflows: %v", err)
	}
	if err := os.WriteFile(filepath.Join(dir, "workflows", "main.yaml"), []byte(bundleTestWorkflowYAML), 0o644); err != nil {
		t.Fatalf("write workflow: %v", err)
	}

	bundleYAML := "id: io.patchcord.example-bundle\n" +
		"version: \"1.0.0\"\n" +
		"app: app\n" +
		"workflows:\n  - workflows/main.yaml\n" +
		"requires_plugins:\n  - io.patchcord.example-text@1.0.0\n"
	if err := os.WriteFile(filepath.Join(dir, "bundle.yaml"), []byte(bundleYAML), 0o644); err != nil {
		t.Fatalf("write bundle.yaml: %v", err)
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
