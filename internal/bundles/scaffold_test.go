package bundles

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/lucasglmt/patchcord/internal/workflow"
)

func TestScaffold_WritesAValidBundleDir(t *testing.T) {
	dir := filepath.Join(t.TempDir(), "my-bundle")

	if err := Scaffold(dir, "io.patchcord.my-bundle", "0.1.0"); err != nil {
		t.Fatalf("Scaffold() error = %v", err)
	}

	m, err := LoadManifest(dir)
	if err != nil {
		t.Fatalf("LoadManifest() error = %v", err)
	}
	if m.ID != "io.patchcord.my-bundle" {
		t.Fatalf("ID = %q, want %q", m.ID, "io.patchcord.my-bundle")
	}
	if m.App != "app" {
		t.Fatalf("App = %q, want %q", m.App, "app")
	}
	if len(m.Workflows) != 1 || m.Workflows[0] != "workflows/main.yaml" {
		t.Fatalf("Workflows = %v, want [workflows/main.yaml]", m.Workflows)
	}
	if len(m.RequiresPlugins) != 0 {
		t.Fatalf("RequiresPlugins = %v, want empty", m.RequiresPlugins)
	}

	if _, err := os.Stat(filepath.Join(dir, "app", "patchcord-app.yaml")); err != nil {
		t.Fatalf("embedded app manifest missing: %v", err)
	}
	if _, err := os.Stat(filepath.Join(dir, "workflows", "main.yaml")); err != nil {
		t.Fatalf("embedded workflow missing: %v", err)
	}

	agentsMD, err := os.ReadFile(filepath.Join(dir, "AGENTS.md"))
	if err != nil {
		t.Fatalf("AGENTS.md missing: %v", err)
	}
	if !strings.Contains(string(agentsMD), "patchcord mcp serve") {
		t.Fatalf("AGENTS.md = %q, want it to point the agent at the MCP server (ADR-0064)", agentsMD)
	}
	// app/AGENTS.md is apps.Scaffold's own responsibility (internal/apps
	// already tests its content) — this only proves bundles.Scaffold
	// doesn't skip it by delegating there.
	if _, err := os.Stat(filepath.Join(dir, "app", "AGENTS.md")); err != nil {
		t.Fatalf("embedded app's AGENTS.md missing: %v", err)
	}
}

// TestScaffold_IsPackAndInstallReady proves the scaffold isn't just
// structurally valid but actually round-trips through Pack/InstallPackage
// — no plugin dependency needed, since requires_plugins starts empty.
func TestScaffold_IsPackAndInstallReady(t *testing.T) {
	sourceDir := filepath.Join(t.TempDir(), "my-bundle")
	if err := Scaffold(sourceDir, "io.patchcord.my-bundle", "0.1.0"); err != nil {
		t.Fatalf("Scaffold() error = %v", err)
	}

	packagePath := filepath.Join(t.TempDir(), "my-bundle.patchcord-bundle")
	f, err := os.Create(packagePath)
	if err != nil {
		t.Fatalf("create package file: %v", err)
	}
	if err := Pack(sourceDir, nil, f); err != nil {
		t.Fatalf("Pack() error = %v", err)
	}
	if err := f.Close(); err != nil {
		t.Fatalf("close package file: %v", err)
	}

	db := openTestDB(t)
	b, _, err := InstallPackage(context.Background(), db, t.TempDir(), packagePath, map[string]workflow.KnownAction{"text.uppercase@1": {}}, false)
	if err != nil {
		t.Fatalf("InstallPackage() error = %v", err)
	}
	if b.ID != "io.patchcord.my-bundle" {
		t.Fatalf("ID = %q, want %q", b.ID, "io.patchcord.my-bundle")
	}
}

func TestScaffold_RefusesToOverwriteANonEmptyDir(t *testing.T) {
	dir := t.TempDir()
	if err := os.WriteFile(filepath.Join(dir, "existing.txt"), []byte("x"), 0o644); err != nil {
		t.Fatalf("write existing.txt: %v", err)
	}

	if err := Scaffold(dir, "io.patchcord.my-bundle", "0.1.0"); err == nil {
		t.Fatal("expected an error for a non-empty target directory, got nil")
	}
}

// TestScaffoldVite_WritesABundleDirPointingAtAppDist proves the manifest
// already points at app/dist (where a build lands its output), not app —
// unlike the static template, there is no patchcord-app.yaml at app's root
// to point at until `npm run build` has run at least once.
func TestScaffoldVite_WritesABundleDirPointingAtAppDist(t *testing.T) {
	dir := filepath.Join(t.TempDir(), "my-bundle")

	if err := ScaffoldVite(dir, "io.patchcord.my-bundle", "0.1.0"); err != nil {
		t.Fatalf("ScaffoldVite() error = %v", err)
	}

	m, err := LoadManifest(dir)
	if err != nil {
		t.Fatalf("LoadManifest() error = %v", err)
	}
	if m.App != "app/dist" {
		t.Fatalf("App = %q, want %q", m.App, "app/dist")
	}
	if len(m.Workflows) != 1 || m.Workflows[0] != "workflows/main.yaml" {
		t.Fatalf("Workflows = %v, want [workflows/main.yaml]", m.Workflows)
	}

	for _, relPath := range []string{"AGENTS.md", "app/AGENTS.md", "app/package.json", "app/vite.config.ts", "app/src/main.ts", "app/public/patchcord-app.yaml"} {
		if _, err := os.Stat(filepath.Join(dir, relPath)); err != nil {
			t.Fatalf("%s missing: %v", relPath, err)
		}
	}
	if _, err := os.Stat(filepath.Join(dir, "workflows", "main.yaml")); err != nil {
		t.Fatalf("embedded workflow missing: %v", err)
	}
}

func TestScaffoldVite_RefusesToOverwriteANonEmptyDir(t *testing.T) {
	dir := t.TempDir()
	if err := os.WriteFile(filepath.Join(dir, "existing.txt"), []byte("x"), 0o644); err != nil {
		t.Fatalf("write existing.txt: %v", err)
	}

	if err := ScaffoldVite(dir, "io.patchcord.my-bundle", "0.1.0"); err == nil {
		t.Fatal("expected an error for a non-empty target directory, got nil")
	}
}
