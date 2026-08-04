package apps

import (
	"os"
	"path/filepath"
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
