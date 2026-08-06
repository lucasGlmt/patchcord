package plugins

import (
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
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
	if _, err := os.Stat(filepath.Join(dir, "Makefile")); err != nil {
		t.Fatalf("Makefile missing: %v", err)
	}
	if _, err := os.Stat(filepath.Join(dir, ".gitignore")); err != nil {
		t.Fatalf(".gitignore missing: %v", err)
	}
	if _, err := os.Stat(filepath.Join(dir, "AGENTS.md")); err != nil {
		t.Fatalf("AGENTS.md missing: %v", err)
	}
	goMod, err := os.ReadFile(filepath.Join(dir, "go.mod"))
	if err != nil {
		t.Fatalf("go.mod missing: %v", err)
	}
	if !strings.Contains(string(goMod), "module io.patchcord.my-plugin") {
		t.Fatalf("go.mod = %q, want the plugin id as module path", goMod)
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
// main.go must actually build against the real SDK as a standalone Go
// module, not just parse as syntactically plausible Go. sdk/go-plugin and
// api/plugin are nested modules of this monorepo (see docs/adr/0066)
// without a published tag yet, so temporary replace directives point at
// this checkout instead — the test does not depend on the published
// modules being available.
func TestScaffold_GeneratedSourceCompiles(t *testing.T) {
	dir := filepath.Join(t.TempDir(), "my-plugin")

	if err := Scaffold(dir, "io.patchcord.my-plugin", "0.1.0"); err != nil {
		t.Fatalf("Scaffold() error = %v", err)
	}

	repoRoot, err := filepath.Abs(filepath.Join("..", ".."))
	if err != nil {
		t.Fatalf("resolve repo root: %v", err)
	}

	edit := exec.Command("go", "mod", "edit",
		"-replace", "github.com/lucasglmt/patchcord/sdk/go-plugin="+filepath.Join(repoRoot, "sdk", "go-plugin"),
		"-replace", "github.com/lucasglmt/patchcord/api/plugin="+filepath.Join(repoRoot, "api", "plugin"),
	)
	edit.Dir = dir
	if out, err := edit.CombinedOutput(); err != nil {
		t.Fatalf("go mod edit error = %v\n%s", err, out)
	}

	tidy := exec.Command("go", "mod", "tidy")
	tidy.Dir = dir
	if out, err := tidy.CombinedOutput(); err != nil {
		t.Fatalf("go mod tidy error = %v\n%s", err, out)
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

func TestScaffoldWithModule_WritesCustomGoModulePath(t *testing.T) {
	dir := filepath.Join(t.TempDir(), "my-plugin")

	if err := ScaffoldWithModule(dir, "io.patchcord.my-plugin", "0.1.0", "github.com/acme/my-plugin"); err != nil {
		t.Fatalf("ScaffoldWithModule() error = %v", err)
	}

	goMod, err := os.ReadFile(filepath.Join(dir, "go.mod"))
	if err != nil {
		t.Fatalf("read go.mod: %v", err)
	}
	if !strings.Contains(string(goMod), "module github.com/acme/my-plugin") {
		t.Fatalf("go.mod = %q, want custom module path", goMod)
	}

	m, err := LoadPackageManifest(dir)
	if err != nil {
		t.Fatalf("LoadPackageManifest() error = %v", err)
	}
	if m.ID != "io.patchcord.my-plugin" {
		t.Fatalf("manifest ID = %q, want plugin id unchanged", m.ID)
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
