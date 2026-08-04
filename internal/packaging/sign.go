package packaging

import (
	"archive/tar"
	"compress/gzip"
	"crypto/ed25519"
	"crypto/sha256"
	"encoding/base64"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"io/fs"
	"os"
	"path/filepath"
)

// ChecksumsFileName and SignatureFileName are the reserved root-level names
// SignedArchive writes and Verify reads (vision document, section 9.1).
// computeChecksums always skips them: they describe a package's content,
// they are not part of it.
const (
	ChecksumsFileName = "checksums.json"
	SignatureFileName = "signature.json"
)

// ErrChecksumMismatch is returned by Verify when an extracted package's
// files do not match its checksums.json — corruption or tampering, never a
// soft warning regardless of the caller's signing policy.
var ErrChecksumMismatch = errors.New("package checksum mismatch")

// ErrInvalidSignature is returned by Verify when a package's signature.json
// does not verify against its checksums.json — same treatment as
// ErrChecksumMismatch, always a hard failure.
var ErrInvalidSignature = errors.New("package signature is invalid")

// signatureFile mirrors signature.json's on-disk shape: an Ed25519
// signature over checksums.json's exact raw bytes, plus the public key it
// was produced with, so Verify can check the signature itself without
// consulting any trust store — deciding whether that public key is
// legitimate for a given package id is internal/trust's job, not this
// package's.
type signatureFile struct {
	Algorithm string `json:"algorithm"`
	PublicKey string `json:"publicKey"`
	Signature string `json:"signature"`
}

// VerificationOutcome reports what Verify found. It carries no verdict of
// its own — whether a caller treats an unsigned or untrusted package as
// acceptable is a policy decision made above internal/packaging (see
// internal/apps, internal/plugins, internal/bundles InstallPackage, and
// internal/trust).
type VerificationOutcome struct {
	// Checksummed is true if the package had a checksums.json and every
	// file matched it. A package with no checksums.json at all (produced
	// before this feature existed, or packed without signing support)
	// reports false here, not an error.
	Checksummed bool
	// Signed is true if the package had a signature.json and its Ed25519
	// signature over checksums.json verified.
	Signed bool
	// PublicKey is the key signature.json was verified with. Only
	// meaningful when Signed is true.
	PublicKey ed25519.PublicKey
}

// SignedArchive archives sourceDir exactly like Archive, plus a
// checksums.json covering every file in sourceDir. If key is non-nil, it
// also signs checksums.json's exact bytes with Ed25519 and adds
// signature.json. key == nil produces a package with integrity data but no
// provenance — the default when `pack` is run without --sign-key.
func SignedArchive(sourceDir string, key ed25519.PrivateKey, w io.Writer) error {
	checksums, err := computeChecksums(sourceDir)
	if err != nil {
		return err
	}
	checksumsJSON, err := json.Marshal(checksums)
	if err != nil {
		return fmt.Errorf("encode %s: %w", ChecksumsFileName, err)
	}

	gz := gzip.NewWriter(w)
	tw := tar.NewWriter(gz)

	if err := writeDirToTar(tw, sourceDir); err != nil {
		return fmt.Errorf("archive %q: %w", sourceDir, err)
	}
	if err := writeFileToTar(tw, ChecksumsFileName, checksumsJSON); err != nil {
		return fmt.Errorf("write %s: %w", ChecksumsFileName, err)
	}

	if key != nil {
		pub, ok := key.Public().(ed25519.PublicKey)
		if !ok {
			return errors.New("signing key has no Ed25519 public key")
		}
		sigJSON, err := json.Marshal(signatureFile{
			Algorithm: "ed25519",
			PublicKey: base64.StdEncoding.EncodeToString(pub),
			Signature: base64.StdEncoding.EncodeToString(ed25519.Sign(key, checksumsJSON)),
		})
		if err != nil {
			return fmt.Errorf("encode %s: %w", SignatureFileName, err)
		}
		if err := writeFileToTar(tw, SignatureFileName, sigJSON); err != nil {
			return fmt.Errorf("write %s: %w", SignatureFileName, err)
		}
	}

	if err := tw.Close(); err != nil {
		return fmt.Errorf("archive %q: %w", sourceDir, err)
	}
	if err := gz.Close(); err != nil {
		return fmt.Errorf("archive %q: %w", sourceDir, err)
	}

	return nil
}

// Verify checks an already-extracted package directory's integrity and
// authenticity:
//
//   - no checksums.json at all: VerificationOutcome{}, nil — an old-style
//     or deliberately unsigned package, not an error.
//   - checksums.json present but any file's digest doesn't match:
//     ErrChecksumMismatch.
//   - checksums.json matches, no signature.json: {Checksummed: true}, nil.
//   - signature.json present: its Ed25519 signature is checked against
//     checksums.json's raw stored bytes (never a recomputed serialization —
//     avoids any JSON-encoding determinism concern). Invalid:
//     ErrInvalidSignature. Valid: {Checksummed: true, Signed: true,
//     PublicKey: ...}.
//
// Verify never consults a trust store and knows nothing about package ids
// — it only proves "this is what was signed", not "this signer is
// legitimate for this package".
func Verify(dir string) (VerificationOutcome, error) {
	rawChecksums, err := os.ReadFile(filepath.Join(dir, ChecksumsFileName))
	if err != nil {
		if os.IsNotExist(err) {
			return VerificationOutcome{}, nil
		}
		return VerificationOutcome{}, fmt.Errorf("read %s: %w", ChecksumsFileName, err)
	}

	var claimed map[string]string
	if err := json.Unmarshal(rawChecksums, &claimed); err != nil {
		return VerificationOutcome{}, fmt.Errorf("parse %s: %w", ChecksumsFileName, err)
	}

	actual, err := computeChecksums(dir)
	if err != nil {
		return VerificationOutcome{}, err
	}
	if !checksumsEqual(claimed, actual) {
		return VerificationOutcome{}, ErrChecksumMismatch
	}

	outcome := VerificationOutcome{Checksummed: true}

	rawSig, err := os.ReadFile(filepath.Join(dir, SignatureFileName))
	if err != nil {
		if os.IsNotExist(err) {
			return outcome, nil
		}
		return VerificationOutcome{}, fmt.Errorf("read %s: %w", SignatureFileName, err)
	}

	var sig signatureFile
	if err := json.Unmarshal(rawSig, &sig); err != nil {
		return VerificationOutcome{}, fmt.Errorf("parse %s: %w", SignatureFileName, err)
	}
	if sig.Algorithm != "ed25519" {
		return VerificationOutcome{}, fmt.Errorf("%s: unsupported algorithm %q", SignatureFileName, sig.Algorithm)
	}

	pubKeyBytes, err := base64.StdEncoding.DecodeString(sig.PublicKey)
	if err != nil {
		return VerificationOutcome{}, fmt.Errorf("%s: decode publicKey: %w", SignatureFileName, err)
	}
	if len(pubKeyBytes) != ed25519.PublicKeySize {
		return VerificationOutcome{}, fmt.Errorf("%s: publicKey has wrong size", SignatureFileName)
	}
	sigBytes, err := base64.StdEncoding.DecodeString(sig.Signature)
	if err != nil {
		return VerificationOutcome{}, fmt.Errorf("%s: decode signature: %w", SignatureFileName, err)
	}

	pubKey := ed25519.PublicKey(pubKeyBytes)
	if !ed25519.Verify(pubKey, rawChecksums, sigBytes) {
		return VerificationOutcome{}, ErrInvalidSignature
	}

	outcome.Signed = true
	outcome.PublicKey = pubKey
	return outcome, nil
}

// computeChecksums walks dir and returns the sha256 hex digest of every
// regular file, keyed by its slash-separated path relative to dir.
// ChecksumsFileName and SignatureFileName themselves are always skipped —
// they describe a package's content, they are never part of it.
func computeChecksums(dir string) (map[string]string, error) {
	checksums := make(map[string]string)

	walkErr := filepath.WalkDir(dir, func(path string, d fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if d.IsDir() {
			return nil
		}
		rel, err := filepath.Rel(dir, path)
		if err != nil {
			return err
		}
		rel = filepath.ToSlash(rel)
		if rel == ChecksumsFileName || rel == SignatureFileName {
			return nil
		}

		info, err := d.Info()
		if err != nil {
			return err
		}
		if !info.Mode().IsRegular() {
			return fmt.Errorf("%s: unsupported file type", rel)
		}

		f, err := os.Open(path)
		if err != nil {
			return err
		}
		defer f.Close()

		h := sha256.New()
		if _, err := io.Copy(h, f); err != nil {
			return err
		}
		checksums[rel] = hex.EncodeToString(h.Sum(nil))
		return nil
	})
	if walkErr != nil {
		return nil, fmt.Errorf("compute checksums for %q: %w", dir, walkErr)
	}

	return checksums, nil
}

func checksumsEqual(a, b map[string]string) bool {
	if len(a) != len(b) {
		return false
	}
	for k, v := range a {
		if b[k] != v {
			return false
		}
	}
	return true
}
