package signing

import (
	"bytes"
	"crypto/ed25519"
	"path/filepath"
	"testing"
)

func TestGenerateKeyPair(t *testing.T) {
	pub, priv, err := GenerateKeyPair()
	if err != nil {
		t.Fatalf("GenerateKeyPair() error = %v", err)
	}
	if len(pub) != ed25519.PublicKeySize {
		t.Fatalf("len(pub) = %d, want %d", len(pub), ed25519.PublicKeySize)
	}
	if len(priv) != ed25519.PrivateKeySize {
		t.Fatalf("len(priv) = %d, want %d", len(priv), ed25519.PrivateKeySize)
	}
	if !bytes.Equal(priv.Public().(ed25519.PublicKey), pub) {
		t.Fatal("priv.Public() does not match the returned public key")
	}
}

func TestWriteKeyPairAndLoad(t *testing.T) {
	pub, priv, err := GenerateKeyPair()
	if err != nil {
		t.Fatalf("GenerateKeyPair() error = %v", err)
	}

	path := filepath.Join(t.TempDir(), "my-key")
	if err := WriteKeyPair(path, pub, priv); err != nil {
		t.Fatalf("WriteKeyPair() error = %v", err)
	}

	loadedPriv, err := LoadPrivateKey(path)
	if err != nil {
		t.Fatalf("LoadPrivateKey() error = %v", err)
	}
	if !bytes.Equal(loadedPriv, priv) {
		t.Fatal("loaded private key does not match the generated one")
	}

	loadedPub, err := LoadPublicKey(path + PublicKeyExtension)
	if err != nil {
		t.Fatalf("LoadPublicKey() error = %v", err)
	}
	if !bytes.Equal(loadedPub, pub) {
		t.Fatal("loaded public key does not match the generated one")
	}
}

func TestLoadPrivateKey_RejectsWrongSize(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "bad-key")
	if err := writeKeyFile(path, []byte("too short"), 0o600); err != nil {
		t.Fatalf("writeKeyFile() error = %v", err)
	}

	if _, err := LoadPrivateKey(path); err == nil {
		t.Fatal("expected an error for a wrong-size private key, got nil")
	}
}

func TestLoadPrivateKey_FailsForMissingFile(t *testing.T) {
	if _, err := LoadPrivateKey(filepath.Join(t.TempDir(), "does-not-exist")); err == nil {
		t.Fatal("expected an error for a missing key file, got nil")
	}
}

func TestFingerprint_IsStableAndShort(t *testing.T) {
	pub, _, err := GenerateKeyPair()
	if err != nil {
		t.Fatalf("GenerateKeyPair() error = %v", err)
	}

	a := Fingerprint(pub)
	b := Fingerprint(pub)
	if a != b {
		t.Fatalf("Fingerprint() is not stable: %q != %q", a, b)
	}
	if len(a) != 16 {
		t.Fatalf("len(Fingerprint()) = %d, want 16", len(a))
	}

	other, _, err := GenerateKeyPair()
	if err != nil {
		t.Fatalf("GenerateKeyPair() error = %v", err)
	}
	if Fingerprint(other) == a {
		t.Fatal("two different keys produced the same fingerprint")
	}
}
