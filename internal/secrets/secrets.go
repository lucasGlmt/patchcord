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
// a Store. Only Type "env" is supported today; see ADR-0020 for why
// environment variables were chosen as the first adapter and for the
// others (OS keychain, Vault, ...) deliberately deferred.
type Reference struct {
	Type string `json:"type"`
	Key  string `json:"key"`
}

// ValidateType returns an error unless t is a Reference type a Store in
// this build can resolve. Connectors call this at creation time, so a
// typo (e.g. "emv" instead of "env") is caught immediately rather than
// only surfacing the first time something tries to resolve it.
func ValidateType(t string) error {
	if t != "env" {
		return fmt.Errorf("unsupported secret reference type %q (only \"env\" is supported)", t)
	}
	return nil
}

// Store resolves a Reference to its actual secret value.
type Store interface {
	Resolve(ctx context.Context, ref Reference) (string, error)
}

// EnvStore resolves "env" references by reading an environment variable.
// It is the only Store this phase implements.
type EnvStore struct{}

func (EnvStore) Resolve(_ context.Context, ref Reference) (string, error) {
	if err := ValidateType(ref.Type); err != nil {
		return "", err
	}

	value, ok := os.LookupEnv(ref.Key)
	if !ok {
		return "", fmt.Errorf("environment variable %q is not set", ref.Key)
	}
	return value, nil
}
