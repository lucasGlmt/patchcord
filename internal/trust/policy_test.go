package trust

import (
	"context"
	"errors"
	"testing"

	"github.com/lucasglmt/patchcord/internal/packaging"
)

func TestCheckPolicy_UnsignedNotRequired_Proceeds(t *testing.T) {
	db := openTestDB(t)

	result, err := CheckPolicy(context.Background(), db, "io.patchcord.example-text", packaging.VerificationOutcome{Checksummed: true}, false)
	if err != nil {
		t.Fatalf("CheckPolicy() error = %v", err)
	}
	if result.Trusted {
		t.Fatal("Trusted = true for an unsigned package, want false")
	}
}

func TestCheckPolicy_UnsignedRequired_Fails(t *testing.T) {
	db := openTestDB(t)

	_, err := CheckPolicy(context.Background(), db, "io.patchcord.example-text", packaging.VerificationOutcome{Checksummed: true}, true)
	if !errors.Is(err, ErrSignatureRequired) {
		t.Fatalf("CheckPolicy() error = %v, want ErrSignatureRequired", err)
	}
}

func TestCheckPolicy_SignedButUntrusted_RequiredFails(t *testing.T) {
	db := openTestDB(t)
	pub := newTestKey(t)
	id := "io.patchcord.example-text"

	outcome := packaging.VerificationOutcome{Checksummed: true, Signed: true, PublicKey: pub}

	result, err := CheckPolicy(context.Background(), db, id, outcome, false)
	if err != nil {
		t.Fatalf("CheckPolicy(requireSignature=false) error = %v", err)
	}
	if result.Trusted {
		t.Fatal("Trusted = true for a key never added to the trust store, want false")
	}

	if _, err := CheckPolicy(context.Background(), db, id, outcome, true); !errors.Is(err, ErrSignatureRequired) {
		t.Fatalf("CheckPolicy(requireSignature=true) error = %v, want ErrSignatureRequired", err)
	}
}

func TestCheckPolicy_SignedAndTrusted_AlwaysProceeds(t *testing.T) {
	db := openTestDB(t)
	pub := newTestKey(t)
	id := "io.patchcord.example-text"

	if err := Add(context.Background(), db, id, pub, "test"); err != nil {
		t.Fatalf("Add() error = %v", err)
	}

	outcome := packaging.VerificationOutcome{Checksummed: true, Signed: true, PublicKey: pub}

	for _, requireSignature := range []bool{false, true} {
		result, err := CheckPolicy(context.Background(), db, id, outcome, requireSignature)
		if err != nil {
			t.Fatalf("CheckPolicy(requireSignature=%v) error = %v", requireSignature, err)
		}
		if !result.Trusted {
			t.Fatalf("CheckPolicy(requireSignature=%v).Trusted = false, want true", requireSignature)
		}
	}
}

func TestCheckPolicy_TrustIsScopedToID(t *testing.T) {
	db := openTestDB(t)
	pub := newTestKey(t)

	if err := Add(context.Background(), db, "io.patchcord.a", pub, ""); err != nil {
		t.Fatalf("Add() error = %v", err)
	}

	outcome := packaging.VerificationOutcome{Checksummed: true, Signed: true, PublicKey: pub}
	if _, err := CheckPolicy(context.Background(), db, "io.patchcord.b", outcome, true); !errors.Is(err, ErrSignatureRequired) {
		t.Fatalf("CheckPolicy() error = %v, want ErrSignatureRequired (trust for a different id must not apply)", err)
	}
}
