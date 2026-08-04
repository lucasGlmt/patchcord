package secrets

import (
	"context"
	"testing"

	"github.com/zalando/go-keyring"
)

func TestKeychainStore_SetThenResolve(t *testing.T) {
	keyring.MockInit()
	store := KeychainStore{Service: "patchcord-test"}
	ctx := context.Background()

	if err := store.Set(ctx, "API_KEY", "s3cr3t"); err != nil {
		t.Fatalf("Set() error = %v", err)
	}

	got, err := store.Resolve(ctx, Reference{Type: "keychain", Key: "API_KEY"})
	if err != nil {
		t.Fatalf("Resolve() error = %v", err)
	}
	if got != "s3cr3t" {
		t.Fatalf("Resolve() = %q, want %q", got, "s3cr3t")
	}
}

func TestKeychainStore_ResolveMissingKey(t *testing.T) {
	keyring.MockInit()
	store := KeychainStore{Service: "patchcord-test"}

	if _, err := store.Resolve(context.Background(), Reference{Type: "keychain", Key: "MISSING"}); err == nil {
		t.Fatal("expected an error for a key that was never set, got nil")
	}
}

func TestKeychainStore_ResolveRejectsWrongType(t *testing.T) {
	keyring.MockInit()
	store := KeychainStore{Service: "patchcord-test"}

	if _, err := store.Resolve(context.Background(), Reference{Type: "env", Key: "ANY"}); err == nil {
		t.Fatal("expected an error for a non-\"keychain\" reference type, got nil")
	}
}

func TestKeychainStore_Remove(t *testing.T) {
	keyring.MockInit()
	store := KeychainStore{Service: "patchcord-test"}
	ctx := context.Background()

	if err := store.Set(ctx, "KEY", "value"); err != nil {
		t.Fatalf("Set() error = %v", err)
	}
	if err := store.Remove(ctx, "KEY"); err != nil {
		t.Fatalf("Remove() error = %v", err)
	}
	if _, err := store.Resolve(ctx, Reference{Type: "keychain", Key: "KEY"}); err == nil {
		t.Fatal("expected Resolve() to fail after Remove(), got nil error")
	}
}

func TestKeychainStore_RemoveMissingKey(t *testing.T) {
	keyring.MockInit()
	store := KeychainStore{Service: "patchcord-test"}

	if err := store.Remove(context.Background(), "MISSING"); err == nil {
		t.Fatal("expected an error removing a key that was never set, got nil")
	}
}
