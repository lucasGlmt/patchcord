package packaging

import (
	"archive/tar"
	"bytes"
	"compress/gzip"
	"os"
	"path/filepath"
	"testing"
)

func TestArchiveAndExtract_RoundTrips(t *testing.T) {
	sourceDir := t.TempDir()
	if err := os.WriteFile(filepath.Join(sourceDir, "manifest.json"), []byte(`{"id":"x"}`), 0o644); err != nil {
		t.Fatalf("write manifest.json: %v", err)
	}
	if err := os.MkdirAll(filepath.Join(sourceDir, "assets"), 0o755); err != nil {
		t.Fatalf("mkdir assets: %v", err)
	}
	if err := os.WriteFile(filepath.Join(sourceDir, "assets", "app.js"), []byte("console.log(1)"), 0o644); err != nil {
		t.Fatalf("write assets/app.js: %v", err)
	}

	var buf bytes.Buffer
	if err := Archive(sourceDir, &buf); err != nil {
		t.Fatalf("Archive() error = %v", err)
	}

	destDir := t.TempDir()
	if err := Extract(&buf, destDir); err != nil {
		t.Fatalf("Extract() error = %v", err)
	}

	body, err := os.ReadFile(filepath.Join(destDir, "manifest.json"))
	if err != nil {
		t.Fatalf("extracted manifest.json missing: %v", err)
	}
	if string(body) != `{"id":"x"}` {
		t.Fatalf("manifest.json content = %q, want %q", body, `{"id":"x"}`)
	}

	body, err = os.ReadFile(filepath.Join(destDir, "assets", "app.js"))
	if err != nil {
		t.Fatalf("extracted assets/app.js missing: %v", err)
	}
	if string(body) != "console.log(1)" {
		t.Fatalf("assets/app.js content = %q, want %q", body, "console.log(1)")
	}
}

func TestArchive_RejectsUnsupportedFileType(t *testing.T) {
	sourceDir := t.TempDir()
	if err := os.WriteFile(filepath.Join(sourceDir, "real.txt"), []byte("x"), 0o644); err != nil {
		t.Fatalf("write real.txt: %v", err)
	}
	if err := os.Symlink(filepath.Join(sourceDir, "real.txt"), filepath.Join(sourceDir, "link.txt")); err != nil {
		t.Skipf("symlinks unsupported on this platform: %v", err)
	}

	if err := Archive(sourceDir, new(bytes.Buffer)); err == nil {
		t.Fatal("expected an error for a symlink entry, got nil")
	}
}

func TestExtract_RejectsPathTraversal(t *testing.T) {
	var buf bytes.Buffer
	gz := gzip.NewWriter(&buf)
	tw := tar.NewWriter(gz)
	if err := tw.WriteHeader(&tar.Header{
		Name:     "../escaped.txt",
		Typeflag: tar.TypeReg,
		Mode:     0o644,
		Size:     4,
	}); err != nil {
		t.Fatalf("write tar header: %v", err)
	}
	if _, err := tw.Write([]byte("evil")); err != nil {
		t.Fatalf("write tar body: %v", err)
	}
	if err := tw.Close(); err != nil {
		t.Fatalf("close tar writer: %v", err)
	}
	if err := gz.Close(); err != nil {
		t.Fatalf("close gzip writer: %v", err)
	}

	destDir := t.TempDir()
	if err := Extract(&buf, destDir); err == nil {
		t.Fatal("expected an error for a path-traversal entry, got nil")
	}
	if _, err := os.Stat(filepath.Join(filepath.Dir(destDir), "escaped.txt")); !os.IsNotExist(err) {
		t.Fatal("path-traversal entry escaped the destination directory")
	}
}

func TestExtract_WritesRegularFilesWithFixedMode(t *testing.T) {
	sourceDir := t.TempDir()
	if err := os.WriteFile(filepath.Join(sourceDir, "plugin"), []byte("#!/bin/sh\n"), 0o755); err != nil {
		t.Fatalf("write plugin: %v", err)
	}

	var buf bytes.Buffer
	if err := Archive(sourceDir, &buf); err != nil {
		t.Fatalf("Archive() error = %v", err)
	}

	destDir := t.TempDir()
	if err := Extract(&buf, destDir); err != nil {
		t.Fatalf("Extract() error = %v", err)
	}

	info, err := os.Stat(filepath.Join(destDir, "plugin"))
	if err != nil {
		t.Fatalf("extracted plugin missing: %v", err)
	}
	if info.Mode().Perm() != 0o644 {
		t.Fatalf("extracted mode = %v, want 0o644 (archive header mode must never be trusted)", info.Mode().Perm())
	}
}

func TestSafeJoin(t *testing.T) {
	destDir := "/data/apps"

	if _, err := SafeJoin(destDir, "../../etc/passwd"); err == nil {
		t.Fatal("expected an error for an escaping name, got nil")
	}

	got, err := SafeJoin(destDir, "assets/app.js")
	if err != nil {
		t.Fatalf("SafeJoin() error = %v", err)
	}
	want := filepath.Join(destDir, "assets", "app.js")
	if got != want {
		t.Fatalf("SafeJoin() = %q, want %q", got, want)
	}
}
