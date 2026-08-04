package registry

import (
	"context"
	"errors"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"testing"
)

// writeLocalRegistry builds a local-directory registry at a fresh
// t.TempDir(): index.json plus one package file per entry in indexJSON,
// with contents equal to their own relative path (so a test can assert
// Fetch copied the right bytes without a real archive).
func writeLocalRegistry(t *testing.T, indexJSON string, packages map[string]string) string {
	t.Helper()

	dir := t.TempDir()
	if err := os.WriteFile(filepath.Join(dir, IndexFileName), []byte(indexJSON), 0o644); err != nil {
		t.Fatalf("write index.json: %v", err)
	}
	for relPath, content := range packages {
		full := filepath.Join(dir, relPath)
		if err := os.MkdirAll(filepath.Dir(full), 0o755); err != nil {
			t.Fatalf("mkdir for %q: %v", relPath, err)
		}
		if err := os.WriteFile(full, []byte(content), 0o644); err != nil {
			t.Fatalf("write package %q: %v", relPath, err)
		}
	}
	return dir
}

const exampleBundleIndexJSON = `{
	"schemaVersion": 1,
	"packages": {
		"io.patchcord.example-bundle": {
			"kind": "bundle",
			"latest": "1.1.0",
			"versions": {
				"1.0.0": "packages/example-bundle-1.0.0.patchcord-bundle",
				"1.1.0": "packages/example-bundle-1.1.0.patchcord-bundle"
			}
		}
	}
}`

func TestResolve_LocalDirectoryRegistry_LatestAndPinnedVersion(t *testing.T) {
	dir := writeLocalRegistry(t, exampleBundleIndexJSON, map[string]string{
		"packages/example-bundle-1.0.0.patchcord-bundle": "v1.0.0 bytes",
		"packages/example-bundle-1.1.0.patchcord-bundle": "v1.1.0 bytes",
	})
	db := openTestDB(t)
	if err := Add(context.Background(), db, "local", dir); err != nil {
		t.Fatalf("Add() error = %v", err)
	}

	t.Run("bare id resolves to latest", func(t *testing.T) {
		resolved, err := Resolve(context.Background(), db, "io.patchcord.example-bundle", "")
		if err != nil {
			t.Fatalf("Resolve() error = %v", err)
		}
		if resolved.Version != "1.1.0" || resolved.Kind != "bundle" {
			t.Fatalf("resolved = %+v, want version=1.1.0 kind=bundle", resolved)
		}
	})

	t.Run("pinned version resolves to that version", func(t *testing.T) {
		resolved, err := Resolve(context.Background(), db, "io.patchcord.example-bundle", "1.0.0")
		if err != nil {
			t.Fatalf("Resolve() error = %v", err)
		}
		if resolved.Version != "1.0.0" {
			t.Fatalf("Version = %q, want 1.0.0", resolved.Version)
		}
	})
}

func TestResolve_UnknownID_ReturnsErrNotFound(t *testing.T) {
	dir := writeLocalRegistry(t, exampleBundleIndexJSON, nil)
	db := openTestDB(t)
	if err := Add(context.Background(), db, "local", dir); err != nil {
		t.Fatalf("Add() error = %v", err)
	}

	if _, err := Resolve(context.Background(), db, "io.patchcord.unknown", ""); !errors.Is(err, ErrNotFound) {
		t.Fatalf("Resolve() error = %v, want ErrNotFound", err)
	}
}

func TestResolve_UnknownVersion_ReturnsErrUnknownVersion(t *testing.T) {
	dir := writeLocalRegistry(t, exampleBundleIndexJSON, nil)
	db := openTestDB(t)
	if err := Add(context.Background(), db, "local", dir); err != nil {
		t.Fatalf("Add() error = %v", err)
	}

	if _, err := Resolve(context.Background(), db, "io.patchcord.example-bundle", "9.9.9"); !errors.Is(err, ErrUnknownVersion) {
		t.Fatalf("Resolve() error = %v, want ErrUnknownVersion", err)
	}
}

func TestResolve_NoRegistryConfigured_ReturnsErrNotFound(t *testing.T) {
	db := openTestDB(t)

	if _, err := Resolve(context.Background(), db, "io.patchcord.example-bundle", ""); !errors.Is(err, ErrNotFound) {
		t.Fatalf("Resolve() error = %v, want ErrNotFound", err)
	}
}

func TestResolve_FirstMatchWins(t *testing.T) {
	firstDir := writeLocalRegistry(t, `{"schemaVersion":1,"packages":{"io.patchcord.example-bundle":{"kind":"bundle","latest":"1.0.0","versions":{"1.0.0":"a.patchcord-bundle"}}}}`, nil)
	secondDir := writeLocalRegistry(t, `{"schemaVersion":1,"packages":{"io.patchcord.example-bundle":{"kind":"bundle","latest":"2.0.0","versions":{"2.0.0":"b.patchcord-bundle"}}}}`, nil)

	db := openTestDB(t)
	if err := Add(context.Background(), db, "first", firstDir); err != nil {
		t.Fatalf("Add(first) error = %v", err)
	}
	if err := Add(context.Background(), db, "second", secondDir); err != nil {
		t.Fatalf("Add(second) error = %v", err)
	}

	resolved, err := Resolve(context.Background(), db, "io.patchcord.example-bundle", "")
	if err != nil {
		t.Fatalf("Resolve() error = %v", err)
	}
	if resolved.RegistryName != "first" || resolved.Version != "1.0.0" {
		t.Fatalf("resolved = %+v, want the first-added registry (version 1.0.0)", resolved)
	}
}

func TestResolve_BrokenRegistryFailsFast(t *testing.T) {
	workingDir := writeLocalRegistry(t, exampleBundleIndexJSON, nil)

	db := openTestDB(t)
	if err := Add(context.Background(), db, "broken", filepath.Join(t.TempDir(), "does-not-exist")); err != nil {
		t.Fatalf("Add(broken) error = %v", err)
	}
	if err := Add(context.Background(), db, "working", workingDir); err != nil {
		t.Fatalf("Add(working) error = %v", err)
	}

	_, err := Resolve(context.Background(), db, "io.patchcord.example-bundle", "")
	if err == nil {
		t.Fatal("expected an error naming the broken registry, got nil")
	}
	if errors.Is(err, ErrNotFound) {
		t.Fatalf("Resolve() error = %v, want a read failure for %q, not ErrNotFound (a broken registry must never be silently skipped)", err, "broken")
	}
}

func TestFetch_LocalDirectory_CopiesExactBytesAndLeavesSourceUntouched(t *testing.T) {
	dir := writeLocalRegistry(t, exampleBundleIndexJSON, map[string]string{
		"packages/example-bundle-1.0.0.patchcord-bundle": "v1.0.0 bytes",
		"packages/example-bundle-1.1.0.patchcord-bundle": "v1.1.0 bytes",
	})
	db := openTestDB(t)
	if err := Add(context.Background(), db, "local", dir); err != nil {
		t.Fatalf("Add() error = %v", err)
	}

	resolved, err := Resolve(context.Background(), db, "io.patchcord.example-bundle", "1.0.0")
	if err != nil {
		t.Fatalf("Resolve() error = %v", err)
	}

	destDir := t.TempDir()
	path, err := Fetch(context.Background(), resolved, destDir)
	if err != nil {
		t.Fatalf("Fetch() error = %v", err)
	}

	got, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("read fetched file: %v", err)
	}
	if string(got) != "v1.0.0 bytes" {
		t.Fatalf("fetched content = %q, want %q", got, "v1.0.0 bytes")
	}

	sourcePath := filepath.Join(dir, "packages/example-bundle-1.0.0.patchcord-bundle")
	sourceBytes, err := os.ReadFile(sourcePath)
	if err != nil {
		t.Fatalf("source file was removed: %v", err)
	}
	if string(sourceBytes) != "v1.0.0 bytes" {
		t.Fatal("source file content changed")
	}
}

func TestFetch_RejectsPathTraversalInIndexRelPath(t *testing.T) {
	dir := writeLocalRegistry(t, `{"schemaVersion":1,"packages":{"evil":{"kind":"bundle","latest":"1.0.0","versions":{"1.0.0":"../../escaped.txt"}}}}`, nil)
	db := openTestDB(t)
	if err := Add(context.Background(), db, "local", dir); err != nil {
		t.Fatalf("Add() error = %v", err)
	}

	resolved, err := Resolve(context.Background(), db, "evil", "")
	if err != nil {
		t.Fatalf("Resolve() error = %v", err)
	}

	if _, err := Fetch(context.Background(), resolved, t.TempDir()); err == nil {
		t.Fatal("expected an error for a path-traversal entry, got nil")
	}
}

func TestResolve_HTTPRegistry(t *testing.T) {
	mux := http.NewServeMux()
	mux.HandleFunc("/index.json", func(w http.ResponseWriter, r *http.Request) {
		w.Write([]byte(exampleBundleIndexJSON))
	})
	mux.HandleFunc("/packages/example-bundle-1.1.0.patchcord-bundle", func(w http.ResponseWriter, r *http.Request) {
		w.Write([]byte("http-served bytes"))
	})
	server := httptest.NewServer(mux)
	t.Cleanup(server.Close)

	db := openTestDB(t)
	if err := Add(context.Background(), db, "remote", server.URL); err != nil {
		t.Fatalf("Add() error = %v", err)
	}

	resolved, err := Resolve(context.Background(), db, "io.patchcord.example-bundle", "")
	if err != nil {
		t.Fatalf("Resolve() error = %v", err)
	}
	if resolved.Version != "1.1.0" {
		t.Fatalf("Version = %q, want 1.1.0", resolved.Version)
	}

	path, err := Fetch(context.Background(), resolved, t.TempDir())
	if err != nil {
		t.Fatalf("Fetch() error = %v", err)
	}

	got, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("read fetched file: %v", err)
	}
	if string(got) != "http-served bytes" {
		t.Fatalf("fetched content = %q, want %q", got, "http-served bytes")
	}
}
