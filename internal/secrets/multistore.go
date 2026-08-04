package secrets

import (
	"context"
	"fmt"
	"path/filepath"
)

// KeychainService is the fixed Service every KeychainStore this build
// constructs uses. Not configurable — one Patchcord agent, one keychain
// namespace, same convention as vaultFileName below for FileStore.
const KeychainService = "patchcord"

// vaultFileName is the FileStore vault's fixed name inside dataDir, next
// to the agent's SQLite database — same convention, not independently
// configurable (see ADR-0040).
const vaultFileName = "secrets.vault"

// NewKeychainStore returns the KeychainStore every entry point uses —
// internal/runtime.NewAgent via BuildStore, and internal/cli's `secret
// set`/`secret remove --type keychain`, which write to it directly.
func NewKeychainStore() KeychainStore {
	return KeychainStore{Service: KeychainService}
}

// NewFileStore loads the master key at masterKeyFile and returns the
// FileStore pointed at dataDir's vault — the same construction BuildStore
// uses for "file", exposed directly for internal/cli's `secret
// set`/`secret remove --type file`, which need a FileStore before any
// connector ever resolves against it.
func NewFileStore(dataDir, masterKeyFile string) (FileStore, error) {
	key, err := LoadMasterKeyFile(masterKeyFile)
	if err != nil {
		return FileStore{}, fmt.Errorf("load secrets master key: %w", err)
	}
	return FileStore{Path: filepath.Join(dataDir, vaultFileName), Key: key}, nil
}

// BuildStore assembles the MultiStore every entry point that resolves
// secrets uses — internal/runtime.NewAgent for the running agent, and the
// CLI commands that touch connectors/secrets directly (internal/cli) — so
// both resolve a given Reference exactly the same way (CLAUDE.md
// non-negotiable #8). "env" and "keychain" are always registered; "file"
// only once masterKeyFile is non-empty, since a "file" reference is valid
// to create before an operator has ever provisioned a master key
// (ValidateType doesn't require it — see its doc comment).
func BuildStore(dataDir, masterKeyFile string) (MultiStore, error) {
	store := MultiStore{
		"env":      EnvStore{},
		"keychain": NewKeychainStore(),
	}

	if masterKeyFile == "" {
		return store, nil
	}

	fileStore, err := NewFileStore(dataDir, masterKeyFile)
	if err != nil {
		return nil, err
	}
	store["file"] = fileStore

	return store, nil
}

// MultiStore dispatches Resolve to one of several Stores, keyed by
// Reference.Type — e.g. {"env": EnvStore{}, "file": FileStore{...}}. This
// is how an agent supports several secret adapters at once: which
// adapter resolves a given reference is a property of that reference
// (Type), not a single global choice for the whole agent (ADR-0040).
type MultiStore map[string]Store

func (m MultiStore) Resolve(ctx context.Context, ref Reference) (string, error) {
	store, ok := m[ref.Type]
	if !ok {
		return "", fmt.Errorf("no secret store configured for reference type %q", ref.Type)
	}
	return store.Resolve(ctx, ref)
}
