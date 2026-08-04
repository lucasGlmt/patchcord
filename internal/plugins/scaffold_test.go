package plugins

import (
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"testing"
)

func TestScaffold_WritesAValidPluginDir(t *testing.T) {
	dir := filepath.Join(t.TempDir(), "my-plugin")

	if err := Scaffold(dir, "io.patchcord.my-plugin", "0.1.0"); err != nil {
		t.Fatalf("Scaffold() error = %v", err)
	}

	if _, err := os.Stat(filepath.Join(dir, "main.go")); err != nil {
		t.Fatalf("main.go missing: %v", err)
	}
	if _, err := os.Stat(filepath.Join(dir, "README.md")); err != nil {
		t.Fatalf("README.md missing: %v", err)
	}

	m, err := LoadPackageManifest(dir)
	if err != nil {
		t.Fatalf("LoadPackageManifest() error = %v", err)
	}
	if m.ID != "io.patchcord.my-plugin" {
		t.Fatalf("ID = %q, want %q", m.ID, "io.patchcord.my-plugin")
	}
	if m.Version != "0.1.0" {
		t.Fatalf("Version = %q, want %q", m.Version, "0.1.0")
	}
	platform := runtime.GOOS + "-" + runtime.GOARCH
	if _, ok := m.Executables[platform]; !ok {
		t.Fatalf("Executables = %v, want an entry for %q", m.Executables, platform)
	}
}

// TestScaffold_GeneratedSourceCompiles is the real proof: the scaffolded
// main.go must actually build against the real SDK, not just parse as
// syntactically plausible Go. Scaffold deliberately does not generate a
// go.mod of its own (the plan: a scaffolded plugin is meant to live inside
// this monorepo, like plugins/examples/*, not as a standalone module) — so
// the scaffold directory must be created *inside* the repo tree for `go
// build` to resolve the root module, not under the OS temp directory the
// way t.TempDir() does.
func TestScaffold_GeneratedSourceCompiles(t *testing.T) {
	dir, err := os.MkdirTemp(".", "scaffold-test-*")
	if err != nil {
		t.Fatalf("create scaffold dir inside the repo: %v", err)
	}
	t.Cleanup(func() { _ = os.RemoveAll(dir) })

	if err := Scaffold(dir, "io.patchcord.my-plugin", "0.1.0"); err != nil {
		t.Fatalf("Scaffold() error = %v", err)
	}

	binPath := filepath.Join(t.TempDir(), "my-plugin-bin")
	build := exec.Command("go", "build", "-o", binPath, ".")
	build.Dir = dir
	if out, err := build.CombinedOutput(); err != nil {
		t.Fatalf("go build error = %v\n%s", err, out)
	}
	if _, err := os.Stat(binPath); err != nil {
		t.Fatalf("built binary missing: %v", err)
	}
}

func TestScaffold_RefusesToOverwriteANonEmptyDir(t *testing.T) {
	dir := t.TempDir()
	if err := os.WriteFile(filepath.Join(dir, "existing.txt"), []byte("x"), 0o644); err != nil {
		t.Fatalf("write existing.txt: %v", err)
	}

	if err := Scaffold(dir, "io.patchcord.my-plugin", "0.1.0"); err == nil {
		t.Fatal("expected an error for a non-empty target directory, got nil")
	}
}
