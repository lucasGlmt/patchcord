package cli

import (
	"bytes"
	"context"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
)

// examplePluginPath, exampleHTTPPluginPath and fakeConnectorPluginPath are
// built once for this package's tests: the real text and http example
// plugins, plus internal/plugins' hand-rolled protocol test fixture, so the
// plugin and connector command groups can be proven against actual
// binaries, not just their fail-fast paths. http is the fixture
// connector_test.go uses for a plugin that does NOT support connector
// testing (it declares a connector type but no Tester); fakeConnectorPlugin
// is the one used for a controllable Ok/failed connector test, since it
// alone exposes FAKE_PLUGIN_CONNECTOR_TYPE/FAKE_PLUGIN_CONNECTOR_TEST_MODE.
var (
	examplePluginPath       string
	exampleHTTPPluginPath   string
	fakeConnectorPluginPath string
)

func TestMain(m *testing.M) {
	tmpDir, err := os.MkdirTemp("", "patchcord-cli-fixtures")
	if err != nil {
		panic(err)
	}
	defer os.RemoveAll(tmpDir)

	examplePluginPath = filepath.Join(tmpDir, "text")
	build := exec.Command("go", "build", "-o", examplePluginPath, "../../plugins/examples/text")
	if out, err := build.CombinedOutput(); err != nil {
		panic("build example plugin: " + err.Error() + "\n" + string(out))
	}

	exampleHTTPPluginPath = filepath.Join(tmpDir, "http")
	buildHTTP := exec.Command("go", "build", "-o", exampleHTTPPluginPath, "../../plugins/examples/http")
	if out, err := buildHTTP.CombinedOutput(); err != nil {
		panic("build example http plugin: " + err.Error() + "\n" + string(out))
	}

	fakeConnectorPluginPath = filepath.Join(tmpDir, "fakeconnector")
	buildFake := exec.Command("go", "build", "-o", fakeConnectorPluginPath, "../../internal/plugins/testdata/fakeplugin")
	if out, err := buildFake.CombinedOutput(); err != nil {
		panic("build fake connector plugin: " + err.Error() + "\n" + string(out))
	}

	os.Exit(m.Run())
}

func TestNewRootCommand_HasPluginSubcommands(t *testing.T) {
	root := NewRootCommand()

	for _, name := range []string{"install", "pack", "list", "inspect", "uninstall"} {
		t.Run(name, func(t *testing.T) {
			cmd, _, err := root.Find([]string{"plugin", name})
			if err != nil {
				t.Fatalf("Find(plugin %s) error = %v", name, err)
			}
			if cmd.Name() != name {
				t.Fatalf("found command %q, want %q", cmd.Name(), name)
			}
		})
	}
}

func TestPluginInstallCommand_FailsForAMissingBinary(t *testing.T) {
	cmd := newPluginInstallCommand()
	cmd.SetArgs([]string{filepath.Join(t.TempDir(), "does-not-exist"), "--data-dir", t.TempDir()})
	cmd.SetContext(context.Background())

	if err := cmd.Execute(); err == nil {
		t.Fatal("expected an error for a missing plugin binary, got nil")
	}
}

func TestPluginListCommand_EmptyCatalog(t *testing.T) {
	cmd := newPluginListCommand()
	cmd.SetArgs([]string{"--data-dir", t.TempDir()})
	cmd.SetContext(context.Background())

	var out bytes.Buffer
	cmd.SetOut(&out)

	if err := cmd.Execute(); err != nil {
		t.Fatalf("Execute() error = %v", err)
	}
	if !strings.Contains(out.String(), "No plugin installed.") {
		t.Fatalf("output = %q, want it to mention an empty catalog", out.String())
	}
}

func TestPluginInspectCommand_UnknownPlugin(t *testing.T) {
	cmd := newPluginInspectCommand()
	cmd.SetArgs([]string{"io.patchcord.unknown", "--data-dir", t.TempDir()})
	cmd.SetContext(context.Background())

	err := cmd.Execute()
	if err == nil {
		t.Fatal("expected an error for an unknown plugin id, got nil")
	}
	if !strings.Contains(err.Error(), "not installed") {
		t.Fatalf("error = %q, want it to mention the plugin is not installed", err.Error())
	}
}

func TestPluginUninstallCommand_UnknownPlugin(t *testing.T) {
	cmd := newPluginUninstallCommand()
	cmd.SetArgs([]string{"io.patchcord.unknown", "--data-dir", t.TempDir()})
	cmd.SetContext(context.Background())

	err := cmd.Execute()
	if err == nil {
		t.Fatal("expected an error for an unknown plugin id, got nil")
	}
	if !strings.Contains(err.Error(), "not installed") {
		t.Fatalf("error = %q, want it to mention the plugin is not installed", err.Error())
	}
}

// TestPluginCommands_FullLifecycle exercises install, list, inspect and
// uninstall against the real example plugin binary, in sequence, exactly
// as a user would type them on the command line.
func TestPluginCommands_FullLifecycle(t *testing.T) {
	dataDir := t.TempDir()

	install := newPluginInstallCommand()
	install.SetArgs([]string{examplePluginPath, "--data-dir", dataDir})
	install.SetContext(context.Background())
	var installOut bytes.Buffer
	install.SetOut(&installOut)
	if err := install.Execute(); err != nil {
		t.Fatalf("plugin install error = %v", err)
	}
	if !strings.Contains(installOut.String(), "io.patchcord.example-text") {
		t.Fatalf("install output = %q, want it to mention the installed plugin id", installOut.String())
	}

	list := newPluginListCommand()
	list.SetArgs([]string{"--data-dir", dataDir})
	list.SetContext(context.Background())
	var listOut bytes.Buffer
	list.SetOut(&listOut)
	if err := list.Execute(); err != nil {
		t.Fatalf("plugin list error = %v", err)
	}
	if !strings.Contains(listOut.String(), "io.patchcord.example-text") {
		t.Fatalf("list output = %q, want it to mention the installed plugin id", listOut.String())
	}

	inspect := newPluginInspectCommand()
	inspect.SetArgs([]string{"io.patchcord.example-text", "--data-dir", dataDir})
	inspect.SetContext(context.Background())
	var inspectOut bytes.Buffer
	inspect.SetOut(&inspectOut)
	if err := inspect.Execute(); err != nil {
		t.Fatalf("plugin inspect error = %v", err)
	}
	if !strings.Contains(inspectOut.String(), "text.uppercase@1") {
		t.Fatalf("inspect output = %q, want it to mention the plugin's action", inspectOut.String())
	}

	uninstall := newPluginUninstallCommand()
	uninstall.SetArgs([]string{"io.patchcord.example-text", "--data-dir", dataDir})
	uninstall.SetContext(context.Background())
	if err := uninstall.Execute(); err != nil {
		t.Fatalf("plugin uninstall error = %v", err)
	}

	inspectAgain := newPluginInspectCommand()
	inspectAgain.SetArgs([]string{"io.patchcord.example-text", "--data-dir", dataDir})
	inspectAgain.SetContext(context.Background())
	if err := inspectAgain.Execute(); err == nil {
		t.Fatal("expected plugin inspect to fail after uninstall, got nil error")
	}
}

// TestPluginPackCommand_ThenInstall exercises `plugin pack` followed by
// `plugin install` against the resulting .patchcord-plugin archive, proving
// that install correctly tells a gzip archive apart from a raw executable
// (isPackageArchive) and routes it through plugins.InstallPackage.
func TestPluginPackCommand_ThenInstall(t *testing.T) {
	sourceDir := t.TempDir()
	platform := runtime.GOOS + "-" + runtime.GOARCH
	relExecutable := filepath.Join("binaries", platform, "plugin")

	execPath := filepath.Join(sourceDir, relExecutable)
	if err := os.MkdirAll(filepath.Dir(execPath), 0o755); err != nil {
		t.Fatalf("mkdir binaries dir: %v", err)
	}
	body, err := os.ReadFile(examplePluginPath)
	if err != nil {
		t.Fatalf("read example plugin binary: %v", err)
	}
	if err := os.WriteFile(execPath, body, 0o755); err != nil {
		t.Fatalf("write staged executable: %v", err)
	}

	manifest := fmt.Sprintf(`{
		"schemaVersion": 1,
		"kind": "plugin",
		"id": "io.patchcord.example-text",
		"version": "1.0.0",
		"protocolVersion": 1,
		"permissions": [],
		"executables": {%q: %q}
	}`, platform, filepath.ToSlash(relExecutable))
	if err := os.WriteFile(filepath.Join(sourceDir, "manifest.json"), []byte(manifest), 0o644); err != nil {
		t.Fatalf("write manifest.json: %v", err)
	}

	packagePath := filepath.Join(t.TempDir(), "text-1.0.0.patchcord-plugin")
	pack := newPluginPackCommand()
	pack.SetArgs([]string{sourceDir, "--output", packagePath})
	pack.SetContext(context.Background())
	var packOut bytes.Buffer
	pack.SetOut(&packOut)
	if err := pack.Execute(); err != nil {
		t.Fatalf("plugin pack error = %v", err)
	}
	if !strings.Contains(packOut.String(), packagePath) {
		t.Fatalf("pack output = %q, want it to mention %q", packOut.String(), packagePath)
	}

	dataDir := t.TempDir()
	install := newPluginInstallCommand()
	install.SetArgs([]string{packagePath, "--data-dir", dataDir})
	install.SetContext(context.Background())
	var installOut bytes.Buffer
	install.SetOut(&installOut)
	if err := install.Execute(); err != nil {
		t.Fatalf("plugin install error = %v", err)
	}
	if !strings.Contains(installOut.String(), "io.patchcord.example-text") {
		t.Fatalf("install output = %q, want it to mention the installed plugin id", installOut.String())
	}

	inspect := newPluginInspectCommand()
	inspect.SetArgs([]string{"io.patchcord.example-text", "--data-dir", dataDir})
	inspect.SetContext(context.Background())
	var inspectOut bytes.Buffer
	inspect.SetOut(&inspectOut)
	if err := inspect.Execute(); err != nil {
		t.Fatalf("plugin inspect error = %v", err)
	}
	if !strings.Contains(inspectOut.String(), "text.uppercase@1") {
		t.Fatalf("inspect output = %q, want it to mention the plugin's action", inspectOut.String())
	}
}

// TestPluginInstallCommand_SigningAndTrustLifecycle exercises the full
// signing/verification story through the CLI, exactly as a user would type
// it: pack a signed package, install it and see a warning about the
// untrusted key, trust that key, reinstall silently, then prove
// --require-signature actually gates on trust rather than just signedness.
func TestPluginInstallCommand_SigningAndTrustLifecycle(t *testing.T) {
	sourceDir := t.TempDir()
	platform := runtime.GOOS + "-" + runtime.GOARCH
	relExecutable := filepath.Join("binaries", platform, "plugin")
	execPath := filepath.Join(sourceDir, relExecutable)
	if err := os.MkdirAll(filepath.Dir(execPath), 0o755); err != nil {
		t.Fatalf("mkdir binaries dir: %v", err)
	}
	body, err := os.ReadFile(examplePluginPath)
	if err != nil {
		t.Fatalf("read example plugin binary: %v", err)
	}
	if err := os.WriteFile(execPath, body, 0o755); err != nil {
		t.Fatalf("write staged executable: %v", err)
	}
	manifest := fmt.Sprintf(`{
		"schemaVersion": 1,
		"kind": "plugin",
		"id": "io.patchcord.example-text",
		"version": "1.0.0",
		"protocolVersion": 1,
		"permissions": [],
		"executables": {%q: %q}
	}`, platform, filepath.ToSlash(relExecutable))
	if err := os.WriteFile(filepath.Join(sourceDir, "manifest.json"), []byte(manifest), 0o644); err != nil {
		t.Fatalf("write manifest.json: %v", err)
	}

	keyPath := filepath.Join(t.TempDir(), "signing-key")
	keygen := newKeyGenerateCommand()
	keygen.SetArgs([]string{"--output", keyPath})
	keygen.SetContext(context.Background())
	if err := keygen.Execute(); err != nil {
		t.Fatalf("key generate error = %v", err)
	}

	packagePath := filepath.Join(t.TempDir(), "text-1.0.0.patchcord-plugin")
	pack := newPluginPackCommand()
	pack.SetArgs([]string{sourceDir, "--output", packagePath, "--sign-key", keyPath})
	pack.SetContext(context.Background())
	if err := pack.Execute(); err != nil {
		t.Fatalf("plugin pack --sign-key error = %v", err)
	}

	t.Run("install without trust warns but succeeds", func(t *testing.T) {
		dataDir := t.TempDir()
		install := newPluginInstallCommand()
		install.SetArgs([]string{packagePath, "--data-dir", dataDir})
		install.SetContext(context.Background())
		var out bytes.Buffer
		install.SetOut(&out)
		if err := install.Execute(); err != nil {
			t.Fatalf("plugin install error = %v", err)
		}
		if !strings.Contains(out.String(), "untrusted key") {
			t.Fatalf("install output = %q, want a warning about an untrusted key", out.String())
		}
	})

	t.Run("install --require-signature fails before trust add", func(t *testing.T) {
		dataDir := t.TempDir()
		install := newPluginInstallCommand()
		install.SetArgs([]string{packagePath, "--data-dir", dataDir, "--require-signature"})
		install.SetContext(context.Background())
		if err := install.Execute(); err == nil {
			t.Fatal("expected an error for an untrusted signed package with --require-signature, got nil")
		}
	})

	t.Run("install --require-signature succeeds and is silent after trust add", func(t *testing.T) {
		dataDir := t.TempDir()

		add := newTrustAddCommand()
		add.SetArgs([]string{"io.patchcord.example-text", keyPath + ".pub", "--data-dir", dataDir})
		add.SetContext(context.Background())
		if err := add.Execute(); err != nil {
			t.Fatalf("trust add error = %v", err)
		}

		install := newPluginInstallCommand()
		install.SetArgs([]string{packagePath, "--data-dir", dataDir, "--require-signature"})
		install.SetContext(context.Background())
		var out bytes.Buffer
		install.SetOut(&out)
		if err := install.Execute(); err != nil {
			t.Fatalf("plugin install --require-signature error = %v", err)
		}
		if strings.Contains(out.String(), "untrusted") || strings.Contains(out.String(), "not signed") {
			t.Fatalf("install output = %q, want no warning once the key is trusted", out.String())
		}
	})

	t.Run("--require-signature on a raw executable errors immediately", func(t *testing.T) {
		install := newPluginInstallCommand()
		install.SetArgs([]string{examplePluginPath, "--data-dir", t.TempDir(), "--require-signature"})
		install.SetContext(context.Background())
		err := install.Execute()
		if err == nil {
			t.Fatal("expected an error for --require-signature on a raw executable, got nil")
		}
		if !strings.Contains(err.Error(), "nothing to verify") {
			t.Fatalf("error = %q, want it to explain there is nothing to verify", err.Error())
		}
	})
}
