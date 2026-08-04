package cli

import (
	"bytes"
	"context"
	"crypto/ed25519"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/lucasglmt/patchcord/internal/signing"
)

func TestNewRootCommand_HasKeySubcommands(t *testing.T) {
	root := NewRootCommand()

	cmd, _, err := root.Find([]string{"key", "generate"})
	if err != nil {
		t.Fatalf("Find(key generate) error = %v", err)
	}
	if cmd.Name() != "generate" {
		t.Fatalf("found command %q, want %q", cmd.Name(), "generate")
	}
}

func TestKeyGenerateCommand_WritesAUsableKeyPair(t *testing.T) {
	path := filepath.Join(t.TempDir(), "my-key")

	cmd := newKeyGenerateCommand()
	var out bytes.Buffer
	cmd.SetOut(&out)
	cmd.SetArgs([]string{"--output", path})
	cmd.SetContext(context.Background())

	if err := cmd.Execute(); err != nil {
		t.Fatalf("Execute() error = %v", err)
	}

	if _, err := os.Stat(path); err != nil {
		t.Fatalf("private key file missing: %v", err)
	}
	if _, err := os.Stat(path + signing.PublicKeyExtension); err != nil {
		t.Fatalf("public key file missing: %v", err)
	}

	priv, err := signing.LoadPrivateKey(path)
	if err != nil {
		t.Fatalf("LoadPrivateKey() error = %v", err)
	}
	pub, err := signing.LoadPublicKey(path + signing.PublicKeyExtension)
	if err != nil {
		t.Fatalf("LoadPublicKey() error = %v", err)
	}
	if !bytes.Equal(priv.Public().(ed25519.PublicKey), pub) {
		t.Fatal("private key's public half does not match the written public key file")
	}

	if !strings.Contains(out.String(), signing.Fingerprint(pub)) {
		t.Fatalf("output = %q, want it to mention the public key's fingerprint", out.String())
	}
}
