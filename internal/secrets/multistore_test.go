package secrets

import (
	"context"
	"os"
	"path/filepath"
	"testing"
)

func TestMultiStore_DispatchesByType(t *testing.T) {
	t.Setenv("PATCHCORD_TEST_MULTISTORE", "from-env")

	store := MultiStore{"env": EnvStore{}}

	got, err := store.Resolve(context.Background(), Reference{Type: "env", Key: "PATCHCORD_TEST_MULTISTORE"})
	if err != nil {
		t.Fatalf("Resolve() error = %v", err)
	}
	if got != "from-env" {
		t.Fatalf("Resolve() = %q, want %q", got, "from-env")
	}
}

func TestMultiStore_UnconfiguredTypeErrors(t *testing.T) {
	store := MultiStore{"env": EnvStore{}}

	if _, err := store.Resolve(context.Background(), Reference{Type: "file", Key: "ANY"}); err == nil {
		t.Fatal("expected an error for a type with no registered store, got nil")
	}
}

func TestBuildStore_WithoutMasterKeyFileOmitsFile(t *testing.T) {
	store, err := BuildStore(t.TempDir(), "")
	if err != nil {
		t.Fatalf("BuildStore() error = %v", err)
	}

	if _, ok := store["env"]; !ok {
		t.Fatal(`BuildStore() did not register "env"`)
	}
	if _, ok := store["keychain"]; !ok {
		t.Fatal(`BuildStore() did not register "keychain"`)
	}
	if _, ok := store["file"]; ok {
		t.Fatal(`BuildStore() registered "file" without a master key file`)
	}
}

func TestBuildStore_WithMasterKeyFileRegistersFile(t *testing.T) {
	dataDir := t.TempDir()
	keyPath := filepath.Join(t.TempDir(), "key")

	encoded, err := GenerateMasterKey()
	if err != nil {
		t.Fatalf("GenerateMasterKey() error = %v", err)
	}
	if err := os.WriteFile(keyPath, []byte(encoded), 0o600); err != nil {
		t.Fatalf("write master key file: %v", err)
	}

	store, err := BuildStore(dataDir, keyPath)
	if err != nil {
		t.Fatalf("BuildStore() error = %v", err)
	}

	fileStore, ok := store["file"].(FileStore)
	if !ok {
		t.Fatal(`BuildStore() did not register a FileStore under "file"`)
	}
	if fileStore.Path != filepath.Join(dataDir, "secrets.vault") {
		t.Fatalf("FileStore.Path = %q, want %q", fileStore.Path, filepath.Join(dataDir, "secrets.vault"))
	}
}

func TestBuildStore_InvalidMasterKeyFileErrors(t *testing.T) {
	if _, err := BuildStore(t.TempDir(), filepath.Join(t.TempDir(), "does-not-exist")); err == nil {
		t.Fatal("expected an error for a missing master key file, got nil")
	}
}
