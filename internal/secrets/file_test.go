package secrets

import (
	"context"
	"encoding/base64"
	"os"
	"path/filepath"
	"testing"
)

func newTestFileStore(t *testing.T) FileStore {
	t.Helper()
	return FileStore{Path: filepath.Join(t.TempDir(), "secrets.vault"), Key: newTestMasterKey(t)}
}

func newTestMasterKey(t *testing.T) [32]byte {
	t.Helper()
	encoded, err := GenerateMasterKey()
	if err != nil {
		t.Fatalf("GenerateMasterKey() error = %v", err)
	}
	decoded, err := base64.StdEncoding.DecodeString(encoded)
	if err != nil {
		t.Fatalf("decode generated key: %v", err)
	}
	var key [32]byte
	copy(key[:], decoded)
	return key
}

func TestFileStore_SetThenResolve(t *testing.T) {
	store := newTestFileStore(t)
	ctx := context.Background()

	if err := store.Set(ctx, "PG_PASSWORD", "s3cr3t"); err != nil {
		t.Fatalf("Set() error = %v", err)
	}

	got, err := store.Resolve(ctx, Reference{Type: "file", Key: "PG_PASSWORD"})
	if err != nil {
		t.Fatalf("Resolve() error = %v", err)
	}
	if got != "s3cr3t" {
		t.Fatalf("Resolve() = %q, want %q", got, "s3cr3t")
	}
}

func TestFileStore_ResolveMissingKey(t *testing.T) {
	store := newTestFileStore(t)
	ctx := context.Background()

	if err := store.Set(ctx, "OTHER", "value"); err != nil {
		t.Fatalf("Set() error = %v", err)
	}

	if _, err := store.Resolve(ctx, Reference{Type: "file", Key: "MISSING"}); err == nil {
		t.Fatal("expected an error for a missing key, got nil")
	}
}

func TestFileStore_ResolveVaultDoesNotExist(t *testing.T) {
	store := newTestFileStore(t)

	if _, err := store.Resolve(context.Background(), Reference{Type: "file", Key: "ANY"}); err == nil {
		t.Fatal("expected an error when the vault file does not exist yet, got nil")
	}
}

func TestFileStore_ResolveRejectsWrongType(t *testing.T) {
	store := newTestFileStore(t)

	if _, err := store.Resolve(context.Background(), Reference{Type: "env", Key: "ANY"}); err == nil {
		t.Fatal("expected an error for a non-\"file\" reference type, got nil")
	}
}

func TestFileStore_MultipleKeysCoexist(t *testing.T) {
	store := newTestFileStore(t)
	ctx := context.Background()

	if err := store.Set(ctx, "A", "1"); err != nil {
		t.Fatalf("Set(A) error = %v", err)
	}
	if err := store.Set(ctx, "B", "2"); err != nil {
		t.Fatalf("Set(B) error = %v", err)
	}

	gotA, err := store.Resolve(ctx, Reference{Type: "file", Key: "A"})
	if err != nil || gotA != "1" {
		t.Fatalf("Resolve(A) = %q, %v, want %q, nil", gotA, err, "1")
	}
	gotB, err := store.Resolve(ctx, Reference{Type: "file", Key: "B"})
	if err != nil || gotB != "2" {
		t.Fatalf("Resolve(B) = %q, %v, want %q, nil", gotB, err, "2")
	}
}

func TestFileStore_Remove(t *testing.T) {
	store := newTestFileStore(t)
	ctx := context.Background()

	if err := store.Set(ctx, "KEY", "value"); err != nil {
		t.Fatalf("Set() error = %v", err)
	}
	if err := store.Remove(ctx, "KEY"); err != nil {
		t.Fatalf("Remove() error = %v", err)
	}
	if _, err := store.Resolve(ctx, Reference{Type: "file", Key: "KEY"}); err == nil {
		t.Fatal("expected Resolve() to fail after Remove(), got nil error")
	}
}

func TestFileStore_RemoveMissingKey(t *testing.T) {
	store := newTestFileStore(t)
	ctx := context.Background()

	if err := store.Set(ctx, "KEY", "value"); err != nil {
		t.Fatalf("Set() error = %v", err)
	}
	if err := store.Remove(ctx, "MISSING"); err == nil {
		t.Fatal("expected an error removing a key that was never set, got nil")
	}
}

func TestFileStore_WrongMasterKeyFailsToDecrypt(t *testing.T) {
	store := newTestFileStore(t)
	ctx := context.Background()

	if err := store.Set(ctx, "KEY", "value"); err != nil {
		t.Fatalf("Set() error = %v", err)
	}

	wrongStore := FileStore{Path: store.Path, Key: newTestMasterKey(t)}
	if _, err := wrongStore.Resolve(ctx, Reference{Type: "file", Key: "KEY"}); err == nil {
		t.Fatal("expected Resolve() with the wrong master key to fail, got nil")
	}
}

func TestFileStore_CorruptedVaultRejected(t *testing.T) {
	store := newTestFileStore(t)
	ctx := context.Background()

	if err := store.Set(ctx, "KEY", "value"); err != nil {
		t.Fatalf("Set() error = %v", err)
	}

	if err := os.WriteFile(store.Path, []byte("not valid json"), 0o600); err != nil {
		t.Fatalf("corrupt vault file: %v", err)
	}

	if _, err := store.Resolve(ctx, Reference{Type: "file", Key: "KEY"}); err == nil {
		t.Fatal("expected Resolve() against a corrupted vault to fail, got nil")
	}
}

func TestFileStore_AtomicWriteLeavesNoTempFiles(t *testing.T) {
	store := newTestFileStore(t)
	ctx := context.Background()

	if err := store.Set(ctx, "KEY", "value"); err != nil {
		t.Fatalf("Set() error = %v", err)
	}

	entries, err := os.ReadDir(filepath.Dir(store.Path))
	if err != nil {
		t.Fatalf("read vault directory: %v", err)
	}
	if len(entries) != 1 {
		t.Fatalf("expected exactly one file in the vault directory, got %d", len(entries))
	}
	if entries[0].Name() != filepath.Base(store.Path) {
		t.Fatalf("unexpected leftover file %q", entries[0].Name())
	}
}

func TestLoadMasterKeyFile_RejectsWrongLength(t *testing.T) {
	path := filepath.Join(t.TempDir(), "key")
	if err := os.WriteFile(path, []byte("dG9vc2hvcnQ="), 0o600); err != nil { // base64("tooshort")
		t.Fatalf("write key file: %v", err)
	}

	if _, err := LoadMasterKeyFile(path); err == nil {
		t.Fatal("expected an error for a key file that doesn't decode to 32 bytes, got nil")
	}
}

func TestLoadMasterKeyFile_RejectsInvalidBase64(t *testing.T) {
	path := filepath.Join(t.TempDir(), "key")
	if err := os.WriteFile(path, []byte("not-base64!!!"), 0o600); err != nil {
		t.Fatalf("write key file: %v", err)
	}

	if _, err := LoadMasterKeyFile(path); err == nil {
		t.Fatal("expected an error for invalid base64, got nil")
	}
}

func TestGenerateMasterKey_RoundTripsThroughLoadMasterKeyFile(t *testing.T) {
	key, err := GenerateMasterKey()
	if err != nil {
		t.Fatalf("GenerateMasterKey() error = %v", err)
	}

	path := filepath.Join(t.TempDir(), "key")
	if err := os.WriteFile(path, []byte(key+"\n"), 0o600); err != nil {
		t.Fatalf("write key file: %v", err)
	}

	if _, err := LoadMasterKeyFile(path); err != nil {
		t.Fatalf("LoadMasterKeyFile() error = %v", err)
	}
}
