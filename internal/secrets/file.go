package secrets

import (
	"context"
	"crypto/aes"
	"crypto/cipher"
	"crypto/rand"
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
)

// masterKeySize is the AES-256 key size in bytes. FileStore takes the key
// as-is (no KDF/passphrase stretching): it is meant to be a
// machine-generated random value (GenerateMasterKey), never a
// user-chosen password, so there is nothing to stretch — see ADR-0040.
const masterKeySize = 32

// vaultFile is the on-disk JSON representation of a FileStore's vault: an
// AES-256-GCM seal of a map[string]string (secret name -> value), plus the
// random nonce used to produce it. The map is encrypted as a whole rather
// than entry by entry, so a single Set/Remove re-encrypts the entire vault
// with a fresh nonce — simpler than tracking one nonce per entry, and the
// vault is small enough (secret material, not application data) that
// re-encrypting it whole is cheap.
type vaultFile struct {
	Nonce      string `json:"nonce"`
	Ciphertext string `json:"ciphertext"`
}

// FileStore resolves "file" references against a single AES-256-GCM
// encrypted vault file on disk. Key must be exactly masterKeySize bytes
// (LoadMasterKeyFile enforces this); it is never persisted in Path or
// anywhere else the vault itself lives — see ADR-0040.
type FileStore struct {
	Path string
	Key  [32]byte
}

// GenerateMasterKey returns a new random 32-byte AES-256 key, base64
// encoded — the value an operator writes to the file pointed at by
// --secrets-master-key-file. Same construction as
// internal/auth.CreateToken's random secret: crypto/rand, never derived
// from anything guessable.
func GenerateMasterKey() (string, error) {
	key := make([]byte, masterKeySize)
	if _, err := rand.Read(key); err != nil {
		return "", fmt.Errorf("generate master key: %w", err)
	}
	return base64.StdEncoding.EncodeToString(key), nil
}

// LoadMasterKeyFile reads and decodes the base64 master key stored at
// path, rejecting anything that doesn't decode to exactly masterKeySize
// bytes — a truncated or corrupted key file fails loudly at startup rather
// than silently producing a FileStore that can never decrypt its vault.
func LoadMasterKeyFile(path string) ([32]byte, error) {
	var key [32]byte

	data, err := os.ReadFile(path)
	if err != nil {
		return key, fmt.Errorf("read master key file %q: %w", path, err)
	}

	decoded, err := base64.StdEncoding.DecodeString(trimNewline(string(data)))
	if err != nil {
		return key, fmt.Errorf("decode master key file %q: %w", path, err)
	}
	if len(decoded) != masterKeySize {
		return key, fmt.Errorf("master key file %q: expected %d decoded bytes, got %d", path, masterKeySize, len(decoded))
	}

	copy(key[:], decoded)
	return key, nil
}

func trimNewline(s string) string {
	for len(s) > 0 && (s[len(s)-1] == '\n' || s[len(s)-1] == '\r') {
		s = s[:len(s)-1]
	}
	return s
}

// Resolve decrypts the vault at s.Path and returns the value stored under
// ref.Key. A vault that doesn't exist yet (no secret was ever Set) and a
// vault that exists but doesn't contain ref.Key are both reported as "not
// found" — indistinguishable on purpose, same convention as
// EnvStore.Resolve's unset-variable error.
func (s FileStore) Resolve(_ context.Context, ref Reference) (string, error) {
	if ref.Type != "file" {
		return "", fmt.Errorf("unsupported secret reference type %q (FileStore only resolves \"file\")", ref.Type)
	}

	entries, err := s.readVault()
	if err != nil {
		return "", err
	}

	value, ok := entries[ref.Key]
	if !ok {
		return "", fmt.Errorf("no secret named %q in file vault %q", ref.Key, s.Path)
	}
	return value, nil
}

// Set encrypts value under key, re-encrypting the whole vault with a fresh
// nonce and writing it atomically (temp file + rename, so a reader never
// observes a partially written vault). Creates the vault if it doesn't
// exist yet.
func (s FileStore) Set(_ context.Context, key, value string) error {
	entries, err := s.readVault()
	if err != nil {
		if !errors.Is(err, os.ErrNotExist) {
			return err
		}
		entries = make(map[string]string)
	}

	entries[key] = value
	return s.writeVault(entries)
}

// Remove deletes key from the vault, re-encrypting and writing the rest.
// Removing a key that isn't set is an error, same as Resolve on a missing
// key — a typo'd key silently no-op'ing would be a worse failure mode.
func (s FileStore) Remove(_ context.Context, key string) error {
	entries, err := s.readVault()
	if err != nil {
		return err
	}

	if _, ok := entries[key]; !ok {
		return fmt.Errorf("no secret named %q in file vault %q", key, s.Path)
	}

	delete(entries, key)
	return s.writeVault(entries)
}

func (s FileStore) readVault() (map[string]string, error) {
	data, err := os.ReadFile(s.Path)
	if err != nil {
		if os.IsNotExist(err) {
			return nil, fmt.Errorf("file vault %q does not exist: %w", s.Path, err)
		}
		return nil, fmt.Errorf("read file vault %q: %w", s.Path, err)
	}

	var vf vaultFile
	if err := json.Unmarshal(data, &vf); err != nil {
		return nil, fmt.Errorf("parse file vault %q: %w", s.Path, err)
	}

	nonce, err := base64.StdEncoding.DecodeString(vf.Nonce)
	if err != nil {
		return nil, fmt.Errorf("decode file vault %q nonce: %w", s.Path, err)
	}
	ciphertext, err := base64.StdEncoding.DecodeString(vf.Ciphertext)
	if err != nil {
		return nil, fmt.Errorf("decode file vault %q ciphertext: %w", s.Path, err)
	}

	gcm, err := s.gcm()
	if err != nil {
		return nil, err
	}

	plaintext, err := gcm.Open(nil, nonce, ciphertext, nil)
	if err != nil {
		return nil, fmt.Errorf("decrypt file vault %q: wrong master key or corrupted vault: %w", s.Path, err)
	}

	var entries map[string]string
	if err := json.Unmarshal(plaintext, &entries); err != nil {
		return nil, fmt.Errorf("parse decrypted file vault %q: %w", s.Path, err)
	}
	return entries, nil
}

func (s FileStore) writeVault(entries map[string]string) error {
	gcm, err := s.gcm()
	if err != nil {
		return err
	}

	nonce := make([]byte, gcm.NonceSize())
	if _, err := rand.Read(nonce); err != nil {
		return fmt.Errorf("generate vault nonce: %w", err)
	}

	plaintext, err := json.Marshal(entries)
	if err != nil {
		return fmt.Errorf("encode file vault entries: %w", err)
	}
	ciphertext := gcm.Seal(nil, nonce, plaintext, nil)

	data, err := json.Marshal(vaultFile{
		Nonce:      base64.StdEncoding.EncodeToString(nonce),
		Ciphertext: base64.StdEncoding.EncodeToString(ciphertext),
	})
	if err != nil {
		return fmt.Errorf("encode file vault %q: %w", s.Path, err)
	}

	dir := filepath.Dir(s.Path)
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return fmt.Errorf("create file vault directory %q: %w", dir, err)
	}

	tmp, err := os.CreateTemp(dir, filepath.Base(s.Path)+".tmp-*")
	if err != nil {
		return fmt.Errorf("create temp file vault: %w", err)
	}
	tmpPath := tmp.Name()
	defer os.Remove(tmpPath) // no-op once Rename below has succeeded

	if _, err := tmp.Write(data); err != nil {
		_ = tmp.Close()
		return fmt.Errorf("write temp file vault: %w", err)
	}
	if err := tmp.Close(); err != nil {
		return fmt.Errorf("close temp file vault: %w", err)
	}
	if err := os.Chmod(tmpPath, 0o600); err != nil {
		return fmt.Errorf("set file vault permissions: %w", err)
	}
	if err := os.Rename(tmpPath, s.Path); err != nil {
		return fmt.Errorf("replace file vault %q: %w", s.Path, err)
	}

	return nil
}

func (s FileStore) gcm() (cipher.AEAD, error) {
	block, err := aes.NewCipher(s.Key[:])
	if err != nil {
		return nil, fmt.Errorf("build AES cipher: %w", err)
	}
	gcm, err := cipher.NewGCM(block)
	if err != nil {
		return nil, fmt.Errorf("build AES-GCM: %w", err)
	}
	return gcm, nil
}
