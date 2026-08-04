// Package signing manages Ed25519 key pairs used to sign and verify
// packages (.patchcord-plugin, .patchcord-app, .patchcord-bundle). It only
// deals with key material on disk — computing checksums, signing them, and
// verifying a signature against a package's content is
// internal/packaging's job; deciding whether a given public key is
// legitimate for a given package id is internal/trust's.
package signing

import (
	"crypto/ed25519"
	"crypto/rand"
	"crypto/sha256"
	"encoding/base64"
	"encoding/hex"
	"fmt"
	"os"
	"path/filepath"
)

// PublicKeyExtension is appended to a private key's path to name its
// public counterpart (e.g. "my-key" -> "my-key.pub"), the same convention
// ssh-keygen uses.
const PublicKeyExtension = ".pub"

// GenerateKeyPair returns a new random Ed25519 key pair.
func GenerateKeyPair() (ed25519.PublicKey, ed25519.PrivateKey, error) {
	pub, priv, err := ed25519.GenerateKey(rand.Reader)
	if err != nil {
		return nil, nil, fmt.Errorf("generate signing key pair: %w", err)
	}
	return pub, priv, nil
}

// WriteKeyPair writes priv to path (base64, 0o600) and pub to
// path+PublicKeyExtension (base64, 0o644). The private key is written
// through a temp file + rename, so a reader never observes a partially
// written key file — same pattern as secrets.FileStore's vault writes.
func WriteKeyPair(path string, pub ed25519.PublicKey, priv ed25519.PrivateKey) error {
	if err := writeKeyFile(path, priv, 0o600); err != nil {
		return fmt.Errorf("write private key %q: %w", path, err)
	}
	pubPath := path + PublicKeyExtension
	if err := writeKeyFile(pubPath, pub, 0o644); err != nil {
		return fmt.Errorf("write public key %q: %w", pubPath, err)
	}
	return nil
}

func writeKeyFile(path string, key []byte, mode os.FileMode) error {
	dir := filepath.Dir(path)
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return fmt.Errorf("create directory %q: %w", dir, err)
	}

	tmp, err := os.CreateTemp(dir, filepath.Base(path)+".tmp-*")
	if err != nil {
		return fmt.Errorf("create temp file: %w", err)
	}
	tmpPath := tmp.Name()
	defer os.Remove(tmpPath) // no-op once Rename below has succeeded

	encoded := base64.StdEncoding.EncodeToString(key)
	if _, err := tmp.WriteString(encoded + "\n"); err != nil {
		_ = tmp.Close()
		return fmt.Errorf("write: %w", err)
	}
	if err := tmp.Close(); err != nil {
		return fmt.Errorf("close temp file: %w", err)
	}
	if err := os.Chmod(tmpPath, mode); err != nil {
		return fmt.Errorf("set permissions: %w", err)
	}
	if err := os.Rename(tmpPath, path); err != nil {
		return fmt.Errorf("replace %q: %w", path, err)
	}

	return nil
}

// LoadPrivateKey reads and decodes a private key file produced by
// WriteKeyPair.
func LoadPrivateKey(path string) (ed25519.PrivateKey, error) {
	decoded, err := readKeyFile(path)
	if err != nil {
		return nil, err
	}
	if len(decoded) != ed25519.PrivateKeySize {
		return nil, fmt.Errorf("private key %q: expected %d decoded bytes, got %d", path, ed25519.PrivateKeySize, len(decoded))
	}
	return ed25519.PrivateKey(decoded), nil
}

// LoadPublicKey reads and decodes a public key file produced by
// WriteKeyPair (path+PublicKeyExtension).
func LoadPublicKey(path string) (ed25519.PublicKey, error) {
	decoded, err := readKeyFile(path)
	if err != nil {
		return nil, err
	}
	if len(decoded) != ed25519.PublicKeySize {
		return nil, fmt.Errorf("public key %q: expected %d decoded bytes, got %d", path, ed25519.PublicKeySize, len(decoded))
	}
	return ed25519.PublicKey(decoded), nil
}

func readKeyFile(path string) ([]byte, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, fmt.Errorf("read %q: %w", path, err)
	}

	decoded, err := base64.StdEncoding.DecodeString(trimNewline(string(data)))
	if err != nil {
		return nil, fmt.Errorf("decode %q: %w", path, err)
	}
	return decoded, nil
}

func trimNewline(s string) string {
	for len(s) > 0 && (s[len(s)-1] == '\n' || s[len(s)-1] == '\r') {
		s = s[:len(s)-1]
	}
	return s
}

// Fingerprint returns a short, human-comparable digest of pub for CLI
// output (e.g. "warn: signed by untrusted key <fingerprint>"). It is
// purely cosmetic — every actual trust decision (internal/trust.IsTrusted)
// compares the full public key, never this truncated form.
func Fingerprint(pub ed25519.PublicKey) string {
	sum := sha256.Sum256(pub)
	return hex.EncodeToString(sum[:8])
}
