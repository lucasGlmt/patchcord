package packaging

import (
	"bytes"
	"crypto/ed25519"
	"crypto/rand"
	"errors"
	"os"
	"path/filepath"
	"testing"
)

func newTestSourceDir(t *testing.T) string {
	t.Helper()

	dir := t.TempDir()
	if err := os.WriteFile(filepath.Join(dir, "manifest.json"), []byte(`{"id":"x"}`), 0o644); err != nil {
		t.Fatalf("write manifest.json: %v", err)
	}
	if err := os.MkdirAll(filepath.Join(dir, "assets"), 0o755); err != nil {
		t.Fatalf("mkdir assets: %v", err)
	}
	if err := os.WriteFile(filepath.Join(dir, "assets", "app.js"), []byte("console.log(1)"), 0o644); err != nil {
		t.Fatalf("write assets/app.js: %v", err)
	}
	return dir
}

func TestSignedArchive_Unsigned_ChecksummedButNotSigned(t *testing.T) {
	sourceDir := newTestSourceDir(t)

	var buf bytes.Buffer
	if err := SignedArchive(sourceDir, nil, &buf); err != nil {
		t.Fatalf("SignedArchive() error = %v", err)
	}

	destDir := t.TempDir()
	if err := Extract(&buf, destDir); err != nil {
		t.Fatalf("Extract() error = %v", err)
	}
	if _, err := os.Stat(filepath.Join(destDir, ChecksumsFileName)); err != nil {
		t.Fatalf("checksums.json missing: %v", err)
	}
	if _, err := os.Stat(filepath.Join(destDir, SignatureFileName)); !os.IsNotExist(err) {
		t.Fatalf("signature.json should not exist for an unsigned package, stat err = %v", err)
	}

	outcome, err := Verify(destDir)
	if err != nil {
		t.Fatalf("Verify() error = %v", err)
	}
	if !outcome.Checksummed {
		t.Fatal("Checksummed = false, want true")
	}
	if outcome.Signed {
		t.Fatal("Signed = true, want false")
	}
}

func TestSignedArchive_Signed_VerifiesAndReturnsPublicKey(t *testing.T) {
	sourceDir := newTestSourceDir(t)
	pub, priv, err := ed25519.GenerateKey(rand.Reader)
	if err != nil {
		t.Fatalf("GenerateKey() error = %v", err)
	}

	var buf bytes.Buffer
	if err := SignedArchive(sourceDir, priv, &buf); err != nil {
		t.Fatalf("SignedArchive() error = %v", err)
	}

	destDir := t.TempDir()
	if err := Extract(&buf, destDir); err != nil {
		t.Fatalf("Extract() error = %v", err)
	}

	outcome, err := Verify(destDir)
	if err != nil {
		t.Fatalf("Verify() error = %v", err)
	}
	if !outcome.Checksummed {
		t.Fatal("Checksummed = false, want true")
	}
	if !outcome.Signed {
		t.Fatal("Signed = false, want true")
	}
	if !bytes.Equal(outcome.PublicKey, pub) {
		t.Fatal("PublicKey does not match the signing key's public key")
	}
}

func TestVerify_NoChecksumsFile_ReportsUnverifiedNotError(t *testing.T) {
	dir := t.TempDir()
	if err := os.WriteFile(filepath.Join(dir, "manifest.json"), []byte(`{}`), 0o644); err != nil {
		t.Fatalf("write manifest.json: %v", err)
	}

	outcome, err := Verify(dir)
	if err != nil {
		t.Fatalf("Verify() error = %v, want nil", err)
	}
	if outcome.Checksummed || outcome.Signed {
		t.Fatalf("outcome = %+v, want a zero-value outcome", outcome)
	}
}

func TestVerify_DetectsTamperedFile(t *testing.T) {
	sourceDir := newTestSourceDir(t)

	var buf bytes.Buffer
	if err := SignedArchive(sourceDir, nil, &buf); err != nil {
		t.Fatalf("SignedArchive() error = %v", err)
	}

	destDir := t.TempDir()
	if err := Extract(&buf, destDir); err != nil {
		t.Fatalf("Extract() error = %v", err)
	}

	// Tamper with a file after extraction, before verification.
	if err := os.WriteFile(filepath.Join(destDir, "assets", "app.js"), []byte("console.log('evil')"), 0o644); err != nil {
		t.Fatalf("tamper with assets/app.js: %v", err)
	}

	if _, err := Verify(destDir); !errors.Is(err, ErrChecksumMismatch) {
		t.Fatalf("Verify() error = %v, want ErrChecksumMismatch", err)
	}
}

func TestVerify_DetectsInvalidSignature(t *testing.T) {
	sourceDir := newTestSourceDir(t)
	_, priv, err := ed25519.GenerateKey(rand.Reader)
	if err != nil {
		t.Fatalf("GenerateKey() error = %v", err)
	}

	var buf bytes.Buffer
	if err := SignedArchive(sourceDir, priv, &buf); err != nil {
		t.Fatalf("SignedArchive() error = %v", err)
	}

	destDir := t.TempDir()
	if err := Extract(&buf, destDir); err != nil {
		t.Fatalf("Extract() error = %v", err)
	}

	// Swap in a signature.json signed by a different, unrelated key: the
	// embedded publicKey changes too, so the checksums.json bytes it claims
	// to cover no longer match what it actually signed once re-verified
	// against itself — simulate by replacing only the signature value.
	sigPath := filepath.Join(destDir, SignatureFileName)
	original, err := os.ReadFile(sigPath)
	if err != nil {
		t.Fatalf("read signature.json: %v", err)
	}
	tampered := bytes.Replace(original, []byte(`"signature":"`), []byte(`"signature":"AAAA`), 1)
	if bytes.Equal(tampered, original) {
		t.Fatal("test bug: tampering left signature.json unchanged")
	}
	if err := os.WriteFile(sigPath, tampered, 0o644); err != nil {
		t.Fatalf("write tampered signature.json: %v", err)
	}

	if _, err := Verify(destDir); err == nil {
		t.Fatal("expected an error for a tampered signature.json, got nil")
	}
}

func TestVerify_SignatureOverAnotherPackageIsRejected(t *testing.T) {
	// A signature produced for one package's checksums.json must not verify
	// against a different package's checksums.json, even if both are
	// otherwise well-formed and internally self-consistent.
	sourceA := newTestSourceDir(t)
	sourceB := t.TempDir()
	if err := os.WriteFile(filepath.Join(sourceB, "manifest.json"), []byte(`{"id":"other"}`), 0o644); err != nil {
		t.Fatalf("write manifest.json: %v", err)
	}

	_, priv, err := ed25519.GenerateKey(rand.Reader)
	if err != nil {
		t.Fatalf("GenerateKey() error = %v", err)
	}

	var bufA, bufB bytes.Buffer
	if err := SignedArchive(sourceA, priv, &bufA); err != nil {
		t.Fatalf("SignedArchive(A) error = %v", err)
	}
	if err := SignedArchive(sourceB, priv, &bufB); err != nil {
		t.Fatalf("SignedArchive(B) error = %v", err)
	}

	destA := t.TempDir()
	if err := Extract(&bufA, destA); err != nil {
		t.Fatalf("Extract(A) error = %v", err)
	}
	destB := t.TempDir()
	if err := Extract(&bufB, destB); err != nil {
		t.Fatalf("Extract(B) error = %v", err)
	}

	// Splice B's signature.json into A's extracted directory.
	sigB, err := os.ReadFile(filepath.Join(destB, SignatureFileName))
	if err != nil {
		t.Fatalf("read B's signature.json: %v", err)
	}
	if err := os.WriteFile(filepath.Join(destA, SignatureFileName), sigB, 0o644); err != nil {
		t.Fatalf("write spliced signature.json: %v", err)
	}

	if _, err := Verify(destA); !errors.Is(err, ErrInvalidSignature) {
		t.Fatalf("Verify() error = %v, want ErrInvalidSignature", err)
	}
}
