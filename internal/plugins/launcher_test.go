package plugins

import (
	"context"
	"os"
	"os/exec"
	"path/filepath"
	"testing"
	"time"
)

// fakePluginPath is built once in TestMain and reused by every test in this
// package, so Launch can be exercised against a real process without
// needing the (not yet built) plugin SDK.
var fakePluginPath string

func TestMain(m *testing.M) {
	tmpDir, err := os.MkdirTemp("", "patchcord-fakeplugin")
	if err != nil {
		panic(err)
	}
	defer os.RemoveAll(tmpDir)

	fakePluginPath = filepath.Join(tmpDir, "fakeplugin")
	build := exec.Command("go", "build", "-o", fakePluginPath, "./testdata/fakeplugin")
	if out, err := build.CombinedOutput(); err != nil {
		panic("build fake plugin: " + err.Error() + "\n" + string(out))
	}

	os.Exit(m.Run())
}

func TestLaunch_Success(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	proc, err := Launch(ctx, fakePluginPath, time.Second)
	if err != nil {
		t.Fatalf("Launch() error = %v", err)
	}

	manifest, err := Handshake(ctx, proc.Client)
	if err != nil {
		t.Fatalf("Handshake() error = %v", err)
	}
	if manifest.PluginID != "io.patchcord.fake" {
		t.Fatalf("PluginID = %q, want %q", manifest.PluginID, "io.patchcord.fake")
	}

	closeCtx, closeCancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer closeCancel()
	if err := proc.Close(closeCtx); err != nil {
		t.Fatalf("Close() error = %v", err)
	}
}

func TestLaunch_TimesOutWhenPluginIsSilent(t *testing.T) {
	t.Setenv("FAKE_PLUGIN_MODE", "silent")

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	_, err := Launch(ctx, fakePluginPath, 200*time.Millisecond)
	if err == nil {
		t.Fatal("expected a timeout error, got nil")
	}
}

func TestLaunch_FailsOnGarbageReadyLine(t *testing.T) {
	t.Setenv("FAKE_PLUGIN_MODE", "garbage")

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	_, err := Launch(ctx, fakePluginPath, time.Second)
	if err == nil {
		t.Fatal("expected an error for a non-JSON bootstrap line, got nil")
	}
}

func TestLaunch_FailsWhenBinaryDoesNotExist(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()

	_, err := Launch(ctx, filepath.Join(t.TempDir(), "does-not-exist"), time.Second)
	if err == nil {
		t.Fatal("expected an error for a missing binary, got nil")
	}
}
