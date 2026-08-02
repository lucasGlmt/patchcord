package cli

import (
	"bytes"
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/lucasglmt/patchcord/internal/apps"
)

// newTestAppDir creates a temporary directory containing a valid
// patchcord-app.yaml.
func newTestAppDir(t *testing.T, id string, workflowsRun ...string) string {
	t.Helper()

	dir := t.TempDir()
	content := "id: " + id + "\nversion: \"0.1.0\"\npermissions:\n  workflows:\n    run:\n"
	for _, w := range workflowsRun {
		content += "      - " + w + "\n"
	}
	if err := os.WriteFile(filepath.Join(dir, apps.ManifestFileName), []byte(content), 0o644); err != nil {
		t.Fatalf("write manifest: %v", err)
	}
	return dir
}

func TestNewRootCommand_HasAppSubcommands(t *testing.T) {
	root := NewRootCommand()

	for _, name := range []string{"install", "dev", "pack", "list", "remove"} {
		t.Run(name, func(t *testing.T) {
			cmd, _, err := root.Find([]string{"app", name})
			if err != nil {
				t.Fatalf("Find(app %s) error = %v", name, err)
			}
			if cmd.Name() != name {
				t.Fatalf("found command %q, want %q", cmd.Name(), name)
			}
		})
	}
}

func TestAppInstallCommand_RejectsAMissingManifest(t *testing.T) {
	cmd := newAppInstallCommand()
	cmd.SetArgs([]string{t.TempDir(), "--data-dir", t.TempDir()})
	cmd.SetContext(context.Background())

	if err := cmd.Execute(); err == nil {
		t.Fatal("expected an error for a directory with no manifest, got nil")
	}
}

func TestAppListCommand_EmptyCatalog(t *testing.T) {
	cmd := newAppListCommand()
	cmd.SetArgs([]string{"--data-dir", t.TempDir()})
	cmd.SetContext(context.Background())

	var out bytes.Buffer
	cmd.SetOut(&out)

	if err := cmd.Execute(); err != nil {
		t.Fatalf("Execute() error = %v", err)
	}
	if !strings.Contains(out.String(), "No app installed.") {
		t.Fatalf("output = %q, want it to mention an empty catalog", out.String())
	}
}

func TestAppRemoveCommand_UnknownApp(t *testing.T) {
	cmd := newAppRemoveCommand()
	cmd.SetArgs([]string{"does-not-exist", "--data-dir", t.TempDir()})
	cmd.SetContext(context.Background())

	err := cmd.Execute()
	if err == nil {
		t.Fatal("expected an error for an unknown app id, got nil")
	}
	if !strings.Contains(err.Error(), "not found") {
		t.Fatalf("error = %q, want it to mention the app was not found", err.Error())
	}
}

// TestAppCommands_FullLifecycle exercises install, list and remove in
// sequence, exactly as a user would type them on the command line.
func TestAppCommands_FullLifecycle(t *testing.T) {
	dataDir := t.TempDir()
	appDir := newTestAppDir(t, "dashboard", "hello_patchcord")

	install := newAppInstallCommand()
	install.SetArgs([]string{appDir, "--data-dir", dataDir})
	install.SetContext(context.Background())
	var installOut bytes.Buffer
	install.SetOut(&installOut)
	if err := install.Execute(); err != nil {
		t.Fatalf("app install error = %v", err)
	}
	if !strings.Contains(installOut.String(), "dashboard") {
		t.Fatalf("install output = %q, want it to mention the installed app id", installOut.String())
	}

	list := newAppListCommand()
	list.SetArgs([]string{"--data-dir", dataDir})
	list.SetContext(context.Background())
	var listOut bytes.Buffer
	list.SetOut(&listOut)
	if err := list.Execute(); err != nil {
		t.Fatalf("app list error = %v", err)
	}
	if !strings.Contains(listOut.String(), "dashboard") || !strings.Contains(listOut.String(), "hello_patchcord") {
		t.Fatalf("list output = %q, want it to mention the app and its permitted workflow", listOut.String())
	}

	installDuplicate := newAppInstallCommand()
	installDuplicate.SetArgs([]string{appDir, "--data-dir", dataDir})
	installDuplicate.SetContext(context.Background())
	if err := installDuplicate.Execute(); err == nil {
		t.Fatal("expected an error installing an app with a duplicate id, got nil")
	}

	remove := newAppRemoveCommand()
	remove.SetArgs([]string{"dashboard", "--data-dir", dataDir})
	remove.SetContext(context.Background())
	if err := remove.Execute(); err != nil {
		t.Fatalf("app remove error = %v", err)
	}

	removeAgain := newAppRemoveCommand()
	removeAgain.SetArgs([]string{"dashboard", "--data-dir", dataDir})
	removeAgain.SetContext(context.Background())
	if err := removeAgain.Execute(); err == nil {
		t.Fatal("expected app remove to fail for an already-removed app, got nil error")
	}
}

// TestAppDevCommand_UpdatesInPlace exercises the `app dev` loop: install,
// then reinstall from a changed directory, and confirm it succeeds where
// `app install` would fail with a duplicate id.
func TestAppDevCommand_UpdatesInPlace(t *testing.T) {
	dataDir := t.TempDir()
	appDir := newTestAppDir(t, "dashboard", "hello_patchcord")

	dev := newAppDevCommand()
	dev.SetArgs([]string{appDir, "--data-dir", dataDir})
	dev.SetContext(context.Background())
	var devOut bytes.Buffer
	dev.SetOut(&devOut)
	if err := dev.Execute(); err != nil {
		t.Fatalf("app dev error = %v", err)
	}
	if !strings.Contains(devOut.String(), "dashboard") {
		t.Fatalf("dev output = %q, want it to mention the app id", devOut.String())
	}

	devAgain := newAppDevCommand()
	devAgain.SetArgs([]string{appDir, "--data-dir", dataDir})
	devAgain.SetContext(context.Background())
	if err := devAgain.Execute(); err != nil {
		t.Fatalf("second app dev error = %v, want it to update in place instead of failing", err)
	}
}

// TestAppPackAndInstallCommands packs an application directory and
// installs the resulting archive, exactly as a user would type it.
func TestAppPackAndInstallCommands(t *testing.T) {
	appDir := newTestAppDir(t, "dashboard", "hello_patchcord")
	packagePath := filepath.Join(t.TempDir(), "dashboard.patchcord-app")

	pack := newAppPackCommand()
	pack.SetArgs([]string{appDir, "-o", packagePath})
	pack.SetContext(context.Background())
	var packOut bytes.Buffer
	pack.SetOut(&packOut)
	if err := pack.Execute(); err != nil {
		t.Fatalf("app pack error = %v", err)
	}
	if !strings.Contains(packOut.String(), packagePath) {
		t.Fatalf("pack output = %q, want it to mention %q", packOut.String(), packagePath)
	}

	dataDir := t.TempDir()
	install := newAppInstallCommand()
	install.SetArgs([]string{packagePath, "--data-dir", dataDir})
	install.SetContext(context.Background())
	var installOut bytes.Buffer
	install.SetOut(&installOut)
	if err := install.Execute(); err != nil {
		t.Fatalf("app install (package) error = %v", err)
	}
	if !strings.Contains(installOut.String(), "dashboard") {
		t.Fatalf("install output = %q, want it to mention the installed app id", installOut.String())
	}

	list := newAppListCommand()
	list.SetArgs([]string{"--data-dir", dataDir})
	list.SetContext(context.Background())
	var listOut bytes.Buffer
	list.SetOut(&listOut)
	if err := list.Execute(); err != nil {
		t.Fatalf("app list error = %v", err)
	}
	if !strings.Contains(listOut.String(), "dashboard") {
		t.Fatalf("list output = %q, want it to mention the app installed from a package", listOut.String())
	}
}
