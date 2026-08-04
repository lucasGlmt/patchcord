package secrets

import (
	"context"
	"errors"
	"fmt"

	"github.com/zalando/go-keyring"
)

// KeychainStore resolves "keychain" references against the current OS's
// native secret store — macOS Keychain, Windows Credential Manager, or
// (on Linux) whatever implements the freedesktop Secret Service, via
// github.com/zalando/go-keyring. Every entry lives under the same
// Service, keyed by Reference.Key.
//
// This is a local-first adapter: a headless Linux server (in particular
// the distroless Docker image, ADR-0039) typically has no Secret Service
// daemon running, so Resolve fails there at call time — expected, see
// ADR-0040. FileStore is the adaptor meant for that deployment shape.
type KeychainStore struct {
	Service string
}

func (s KeychainStore) Resolve(_ context.Context, ref Reference) (string, error) {
	if ref.Type != "keychain" {
		return "", fmt.Errorf("unsupported secret reference type %q (KeychainStore only resolves \"keychain\")", ref.Type)
	}

	value, err := keyring.Get(s.Service, ref.Key)
	if err != nil {
		if errors.Is(err, keyring.ErrNotFound) {
			return "", fmt.Errorf("no secret named %q in OS keychain (service %q)", ref.Key, s.Service)
		}
		return "", fmt.Errorf("resolve %q from OS keychain (service %q): %w", ref.Key, s.Service, err)
	}
	return value, nil
}

// Set stores value under key in the OS keychain, overwriting any existing
// entry of the same key.
func (s KeychainStore) Set(_ context.Context, key, value string) error {
	if err := keyring.Set(s.Service, key, value); err != nil {
		return fmt.Errorf("set %q in OS keychain (service %q): %w", key, s.Service, err)
	}
	return nil
}

// Remove deletes key from the OS keychain. Removing a key that isn't set
// is an error, same convention as FileStore.Remove.
func (s KeychainStore) Remove(_ context.Context, key string) error {
	if err := keyring.Delete(s.Service, key); err != nil {
		if errors.Is(err, keyring.ErrNotFound) {
			return fmt.Errorf("no secret named %q in OS keychain (service %q)", key, s.Service)
		}
		return fmt.Errorf("remove %q from OS keychain (service %q): %w", key, s.Service, err)
	}
	return nil
}
