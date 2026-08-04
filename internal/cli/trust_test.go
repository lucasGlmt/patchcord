package cli

import (
	"bytes"
	"context"
	"path/filepath"
	"strings"
	"testing"
)

func TestNewRootCommand_HasTrustSubcommands(t *testing.T) {
	root := NewRootCommand()

	for _, name := range []string{"add", "list", "remove"} {
		t.Run(name, func(t *testing.T) {
			cmd, _, err := root.Find([]string{"trust", name})
			if err != nil {
				t.Fatalf("Find(trust %s) error = %v", name, err)
			}
			if cmd.Name() != name {
				t.Fatalf("found command %q, want %q", cmd.Name(), name)
			}
		})
	}
}

// generateTestKeyFile writes a fresh key pair via `key generate` and
// returns the public key's path, for trust command tests that only need a
// pubkey file to point at.
func generateTestKeyFile(t *testing.T) string {
	t.Helper()

	path := filepath.Join(t.TempDir(), "test-key")
	gen := newKeyGenerateCommand()
	gen.SetArgs([]string{"--output", path})
	gen.SetContext(context.Background())
	if err := gen.Execute(); err != nil {
		t.Fatalf("key generate error = %v", err)
	}

	return path + ".pub"
}

func TestTrustCommands_FullLifecycle(t *testing.T) {
	dataDir := t.TempDir()
	pubKeyPath := generateTestKeyFile(t)
	id := "io.patchcord.example-text"

	add := newTrustAddCommand()
	add.SetArgs([]string{id, pubKeyPath, "--data-dir", dataDir})
	add.SetContext(context.Background())
	var addOut bytes.Buffer
	add.SetOut(&addOut)
	if err := add.Execute(); err != nil {
		t.Fatalf("trust add error = %v", err)
	}
	if !strings.Contains(addOut.String(), id) {
		t.Fatalf("add output = %q, want it to mention %q", addOut.String(), id)
	}

	list := newTrustListCommand()
	list.SetArgs([]string{"--data-dir", dataDir})
	list.SetContext(context.Background())
	var listOut bytes.Buffer
	list.SetOut(&listOut)
	if err := list.Execute(); err != nil {
		t.Fatalf("trust list error = %v", err)
	}
	if !strings.Contains(listOut.String(), id) {
		t.Fatalf("list output = %q, want it to mention %q", listOut.String(), id)
	}

	remove := newTrustRemoveCommand()
	remove.SetArgs([]string{id, pubKeyPath, "--data-dir", dataDir})
	remove.SetContext(context.Background())
	if err := remove.Execute(); err != nil {
		t.Fatalf("trust remove error = %v", err)
	}

	listAfterRemove := newTrustListCommand()
	listAfterRemove.SetArgs([]string{"--data-dir", dataDir})
	listAfterRemove.SetContext(context.Background())
	var listAfterRemoveOut bytes.Buffer
	listAfterRemove.SetOut(&listAfterRemoveOut)
	if err := listAfterRemove.Execute(); err != nil {
		t.Fatalf("trust list (after remove) error = %v", err)
	}
	if !strings.Contains(listAfterRemoveOut.String(), "No trusted key") {
		t.Fatalf("list output after remove = %q, want it to report no trusted key", listAfterRemoveOut.String())
	}
}

func TestTrustRemoveCommand_UnknownPairFails(t *testing.T) {
	dataDir := t.TempDir()
	pubKeyPath := generateTestKeyFile(t)

	cmd := newTrustRemoveCommand()
	cmd.SetArgs([]string{"io.patchcord.example-text", pubKeyPath, "--data-dir", dataDir})
	cmd.SetContext(context.Background())

	if err := cmd.Execute(); err == nil {
		t.Fatal("expected an error for a never-trusted (id, key) pair, got nil")
	}
}
