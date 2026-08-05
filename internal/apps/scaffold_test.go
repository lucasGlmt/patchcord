package apps

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestScaffold_WritesAValidAppDir(t *testing.T) {
	dir := filepath.Join(t.TempDir(), "dashboard")

	if err := Scaffold(dir, "dashboard", "0.1.0"); err != nil {
		t.Fatalf("Scaffold() error = %v", err)
	}

	m, err := LoadManifest(dir)
	if err != nil {
		t.Fatalf("LoadManifest() error = %v", err)
	}
	if m.ID != "dashboard" {
		t.Fatalf("ID = %q, want %q", m.ID, "dashboard")
	}
	if m.Version != "0.1.0" {
		t.Fatalf("Version = %q, want %q", m.Version, "0.1.0")
	}

	if _, err := os.Stat(filepath.Join(dir, "index.html")); err != nil {
		t.Fatalf("index.html missing: %v", err)
	}
}

func TestScaffold_RefusesToOverwriteANonEmptyDir(t *testing.T) {
	dir := t.TempDir()
	if err := os.WriteFile(filepath.Join(dir, "existing.txt"), []byte("x"), 0o644); err != nil {
		t.Fatalf("write existing.txt: %v", err)
	}

	if err := Scaffold(dir, "dashboard", "0.1.0"); err == nil {
		t.Fatal("expected an error for a non-empty target directory, got nil")
	}
}

func TestScaffold_AllowsAnAlreadyExistingEmptyDir(t *testing.T) {
	dir := t.TempDir() // t.TempDir() already creates an empty directory

	if err := Scaffold(dir, "dashboard", "0.1.0"); err != nil {
		t.Fatalf("Scaffold() error = %v", err)
	}
}

func TestScaffoldVite_WritesAViteProjectThatBuildsIntoAValidAppDir(t *testing.T) {
	dir := filepath.Join(t.TempDir(), "dashboard")

	if err := ScaffoldVite(dir, "dashboard", "0.1.0"); err != nil {
		t.Fatalf("ScaffoldVite() error = %v", err)
	}

	for _, relPath := range []string{
		"package.json",
		"vite.config.ts",
		"tsconfig.json",
		"index.html",
		".gitignore",
		"src/main.ts",
		"src/vite-env.d.ts",
	} {
		if _, err := os.Stat(filepath.Join(dir, relPath)); err != nil {
			t.Fatalf("%s missing: %v", relPath, err)
		}
	}

	packageJSON, err := os.ReadFile(filepath.Join(dir, "package.json"))
	if err != nil {
		t.Fatalf("read package.json: %v", err)
	}
	if !strings.Contains(string(packageJSON), `"@glmtsolutions/patchcord-sdk"`) {
		t.Fatal("package.json does not declare a @glmtsolutions/patchcord-sdk dependency — the scaffold should be ready to call the agent's API out of the box")
	}

	mainTS, err := os.ReadFile(filepath.Join(dir, "src/main.ts"))
	if err != nil {
		t.Fatalf("read src/main.ts: %v", err)
	}
	if !strings.Contains(string(mainTS), `from "@glmtsolutions/patchcord-sdk"`) {
		t.Fatal("src/main.ts does not import @glmtsolutions/patchcord-sdk")
	}
	if !strings.Contains(string(mainTS), "baseUrl: window.location.origin") {
		t.Fatal("src/main.ts does not use window.location.origin as baseUrl — it should, so vite.config.ts's dev proxy (not a cross-origin fetch) is what reaches the agent")
	}

	viteConfig, err := os.ReadFile(filepath.Join(dir, "vite.config.ts"))
	if err != nil {
		t.Fatalf("read vite.config.ts: %v", err)
	}
	if !strings.Contains(string(viteConfig), "127.0.0.1:7331") {
		t.Fatal("vite.config.ts does not proxy to the agent — vite dev would hit CORS as soon as the agent has an admin token (ADR-0045)")
	}
	if !strings.Contains(string(viteConfig), `base: "./"`) {
		t.Fatal("vite.config.ts does not set a relative base — Vite's default absolute base emits asset URLs that 404 once installed under /apps/{id}/ (ADR-0058)")
	}

	// The manifest is not at dir's root — Vite's build is what moves it
	// (from public/) into dist/, which is what an app/bundle command must
	// be pointed at. Load it from public/ directly since there is no build
	// step in this test.
	m, err := LoadManifest(filepath.Join(dir, "public"))
	if err != nil {
		t.Fatalf("LoadManifest(public/) error = %v", err)
	}
	if m.ID != "dashboard" {
		t.Fatalf("ID = %q, want %q", m.ID, "dashboard")
	}
	if m.Version != "0.1.0" {
		t.Fatalf("Version = %q, want %q", m.Version, "0.1.0")
	}

	if _, err := os.Stat(filepath.Join(dir, ManifestFileName)); err == nil {
		t.Fatal("patchcord-app.yaml must not sit at dir's root for the Vite template — it belongs under public/ so the build copies it into dist/")
	}
}

func TestScaffoldVite_RefusesToOverwriteANonEmptyDir(t *testing.T) {
	dir := t.TempDir()
	if err := os.WriteFile(filepath.Join(dir, "existing.txt"), []byte("x"), 0o644); err != nil {
		t.Fatalf("write existing.txt: %v", err)
	}

	if err := ScaffoldVite(dir, "dashboard", "0.1.0"); err == nil {
		t.Fatal("expected an error for a non-empty target directory, got nil")
	}
}
