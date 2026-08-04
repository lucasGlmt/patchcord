package cli

import (
	"bytes"
	"context"
	"encoding/base64"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/zalando/go-keyring"

	"github.com/lucasglmt/patchcord/internal/secrets"
)

func TestNewRootCommand_HasSecretSubcommands(t *testing.T) {
	root := NewRootCommand()

	for _, name := range []string{"keygen", "set", "remove"} {
		t.Run(name, func(t *testing.T) {
			cmd, _, err := root.Find([]string{"secret", name})
			if err != nil {
				t.Fatalf("Find(secret %s) error = %v", name, err)
			}
			if cmd.Name() != name {
				t.Fatalf("found command %q, want %q", cmd.Name(), name)
			}
		})
	}
}

func TestSecretKeygenCommand_PrintsAValidMasterKey(t *testing.T) {
	cmd := newSecretKeygenCommand()
	var out bytes.Buffer
	cmd.SetOut(&out)
	cmd.SetContext(context.Background())

	if err := cmd.Execute(); err != nil {
		t.Fatalf("Execute() error = %v", err)
	}

	decoded, err := base64.StdEncoding.DecodeString(strings.TrimSpace(out.String()))
	if err != nil {
		t.Fatalf("keygen output %q does not decode as base64: %v", out.String(), err)
	}
	if len(decoded) != 32 {
		t.Fatalf("decoded key length = %d, want 32", len(decoded))
	}
}

func writeMasterKeyFile(t *testing.T) string {
	t.Helper()
	key, err := generateMasterKeyForTest(t)
	if err != nil {
		t.Fatalf("generate master key: %v", err)
	}
	path := filepath.Join(t.TempDir(), "key")
	if err := os.WriteFile(path, []byte(key), 0o600); err != nil {
		t.Fatalf("write master key file: %v", err)
	}
	return path
}

func generateMasterKeyForTest(t *testing.T) (string, error) {
	t.Helper()
	cmd := newSecretKeygenCommand()
	var out bytes.Buffer
	cmd.SetOut(&out)
	cmd.SetContext(context.Background())
	if err := cmd.Execute(); err != nil {
		return "", err
	}
	return strings.TrimSpace(out.String()), nil
}

func TestSecretSetAndRemoveCommand_File(t *testing.T) {
	dataDir := t.TempDir()
	keyPath := writeMasterKeyFile(t)

	setCmd := newSecretSetCommand()
	setCmd.SetIn(strings.NewReader("s3cr3t"))
	var setOut bytes.Buffer
	setCmd.SetOut(&setOut)
	setCmd.SetArgs([]string{"PG_PASSWORD", "--type", "file", "--data-dir", dataDir, "--secrets-master-key-file", keyPath})
	setCmd.SetContext(context.Background())
	if err := setCmd.Execute(); err != nil {
		t.Fatalf("secret set Execute() error = %v", err)
	}
	if !strings.Contains(setOut.String(), "PG_PASSWORD") {
		t.Fatalf("set output = %q, want it to mention the key", setOut.String())
	}

	removeCmd := newSecretRemoveCommand()
	var removeOut bytes.Buffer
	removeCmd.SetOut(&removeOut)
	removeCmd.SetArgs([]string{"PG_PASSWORD", "--type", "file", "--data-dir", dataDir, "--secrets-master-key-file", keyPath})
	removeCmd.SetContext(context.Background())
	if err := removeCmd.Execute(); err != nil {
		t.Fatalf("secret remove Execute() error = %v", err)
	}

	removeAgainCmd := newSecretRemoveCommand()
	removeAgainCmd.SetOut(new(bytes.Buffer))
	removeAgainCmd.SetArgs([]string{"PG_PASSWORD", "--type", "file", "--data-dir", dataDir, "--secrets-master-key-file", keyPath})
	removeAgainCmd.SetContext(context.Background())
	if err := removeAgainCmd.Execute(); err == nil {
		t.Fatal("expected an error removing an already-removed key, got nil")
	}
}

func TestSecretSetCommand_FileWithoutMasterKeyFileErrors(t *testing.T) {
	cmd := newSecretSetCommand()
	cmd.SetIn(strings.NewReader("value"))
	cmd.SetOut(new(bytes.Buffer))
	cmd.SetArgs([]string{"KEY", "--type", "file", "--data-dir", t.TempDir()})
	cmd.SetContext(context.Background())

	if err := cmd.Execute(); err == nil {
		t.Fatal("expected an error for --type file without --secrets-master-key-file, got nil")
	}
}

func TestSecretSetCommand_UnknownTypeErrors(t *testing.T) {
	cmd := newSecretSetCommand()
	cmd.SetIn(strings.NewReader("value"))
	cmd.SetOut(new(bytes.Buffer))
	cmd.SetArgs([]string{"KEY", "--type", "vault", "--data-dir", t.TempDir()})
	cmd.SetContext(context.Background())

	if err := cmd.Execute(); err == nil {
		t.Fatal("expected an error for an unsupported --type, got nil")
	}
}

func TestSecretSetAndRemoveCommand_Keychain(t *testing.T) {
	keyring.MockInit()

	setCmd := newSecretSetCommand()
	setCmd.SetIn(strings.NewReader("s3cr3t\n"))
	setCmd.SetOut(new(bytes.Buffer))
	setCmd.SetArgs([]string{"API_KEY", "--type", "keychain", "--data-dir", t.TempDir()})
	setCmd.SetContext(context.Background())
	if err := setCmd.Execute(); err != nil {
		t.Fatalf("secret set Execute() error = %v", err)
	}

	got, err := keyring.Get(secrets.KeychainService, "API_KEY")
	if err != nil {
		t.Fatalf("keyring.Get() error = %v", err)
	}
	if got != "s3cr3t" {
		t.Fatalf("stored value = %q, want %q (trailing newline should be trimmed)", got, "s3cr3t")
	}

	removeCmd := newSecretRemoveCommand()
	removeCmd.SetOut(new(bytes.Buffer))
	removeCmd.SetArgs([]string{"API_KEY", "--type", "keychain", "--data-dir", t.TempDir()})
	removeCmd.SetContext(context.Background())
	if err := removeCmd.Execute(); err != nil {
		t.Fatalf("secret remove Execute() error = %v", err)
	}
}
