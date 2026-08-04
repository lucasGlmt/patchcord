// Package secrets resolves logical references to secret values. A
// Reference is what connectors and (eventually) workflows are allowed to
// hold — never a secret's actual value (ADR-0009, ADR-0020).
package secrets

import (
	"context"
	"fmt"
	"os"
)

// Reference is a logical pointer to a secret's value, resolved on demand by
// a Store. Type selects which adapter resolves it — "env" (EnvStore),
// "keychain" (KeychainStore) or "file" (FileStore); see ADR-0020 for why
// environment variables were the first adapter and ADR-0040 for the other
// two.
type Reference struct {
	Type string `json:"type"`
	Key  string `json:"key"`
}

// validTypes are the Reference.Type values a Store in this build can ever
// resolve — checked at connector/trigger creation time so a typo (e.g.
// "emv" instead of "env") is caught immediately, independent of whether
// this particular running agent has actually configured an adapter for
// that type (a "file" reference is valid to create before
// --secrets-master-key-file is ever set — resolution stays a separate,
// lazy check, same as it always has been for "env").
var validTypes = map[string]struct{}{
	"env":      {},
	"keychain": {},
	"file":     {},
}

// ValidateType returns an error unless t is a Reference type a Store in
// this build can resolve.
func ValidateType(t string) error {
	if _, ok := validTypes[t]; !ok {
		return fmt.Errorf("unsupported secret reference type %q (must be one of \"env\", \"keychain\", \"file\")", t)
	}
	return nil
}

// Store resolves a Reference to its actual secret value.
type Store interface {
	Resolve(ctx context.Context, ref Reference) (string, error)
}

// WritableStore is a Store an operator can also write to directly —
// KeychainStore and FileStore, not EnvStore (an "env" reference is
// provisioned by however the process's environment gets set, never
// through this package). internal/cli's `secret set`/`secret remove`
// commands are the only callers.
type WritableStore interface {
	Set(ctx context.Context, key, value string) error
	Remove(ctx context.Context, key string) error
}

// EnvStore resolves "env" references by reading an environment variable.
type EnvStore struct{}

func (EnvStore) Resolve(_ context.Context, ref Reference) (string, error) {
	if ref.Type != "env" {
		return "", fmt.Errorf("unsupported secret reference type %q (EnvStore only resolves \"env\")", ref.Type)
	}

	value, ok := os.LookupEnv(ref.Key)
	if !ok {
		return "", fmt.Errorf("environment variable %q is not set", ref.Key)
	}
	return value, nil
}
