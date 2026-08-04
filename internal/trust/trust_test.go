package trust

import (
	"bytes"
	"context"
	"crypto/ed25519"
	"crypto/rand"
	"database/sql"
	"errors"
	"io"
	"log/slog"
	"testing"

	"github.com/lucasglmt/patchcord/internal/persistence"
	"github.com/lucasglmt/patchcord/migrations"
)

func openTestDB(t *testing.T) *sql.DB {
	t.Helper()

	db, err := persistence.Open(t.TempDir())
	if err != nil {
		t.Fatalf("persistence.Open() error = %v", err)
	}
	t.Cleanup(func() { _ = db.Close() })

	logger := slog.New(slog.NewTextHandler(io.Discard, nil))
	if err := persistence.Migrate(context.Background(), db, migrations.FS, logger); err != nil {
		t.Fatalf("persistence.Migrate() error = %v", err)
	}

	return db
}

func newTestKey(t *testing.T) ed25519.PublicKey {
	t.Helper()
	pub, _, err := ed25519.GenerateKey(rand.Reader)
	if err != nil {
		t.Fatalf("GenerateKey() error = %v", err)
	}
	return pub
}

func TestIsTrusted_FalseForAnUnrecordedKey(t *testing.T) {
	db := openTestDB(t)
	pub := newTestKey(t)

	trusted, err := IsTrusted(context.Background(), db, "io.patchcord.example-text", pub)
	if err != nil {
		t.Fatalf("IsTrusted() error = %v", err)
	}
	if trusted {
		t.Fatal("IsTrusted() = true, want false for a key never added")
	}
}

func TestAddAndIsTrusted(t *testing.T) {
	db := openTestDB(t)
	pub := newTestKey(t)
	id := "io.patchcord.example-text"

	if err := Add(context.Background(), db, id, pub, "my key"); err != nil {
		t.Fatalf("Add() error = %v", err)
	}

	trusted, err := IsTrusted(context.Background(), db, id, pub)
	if err != nil {
		t.Fatalf("IsTrusted() error = %v", err)
	}
	if !trusted {
		t.Fatal("IsTrusted() = false, want true after Add")
	}
}

func TestIsTrusted_ScopedToID(t *testing.T) {
	db := openTestDB(t)
	pub := newTestKey(t)

	if err := Add(context.Background(), db, "io.patchcord.example-text", pub, ""); err != nil {
		t.Fatalf("Add() error = %v", err)
	}

	trusted, err := IsTrusted(context.Background(), db, "io.patchcord.some-other-plugin", pub)
	if err != nil {
		t.Fatalf("IsTrusted() error = %v", err)
	}
	if trusted {
		t.Fatal("IsTrusted() = true for a different id, want false: trust must not be global to the key")
	}
}

func TestAdd_ReAddingUpdatesLabelInsteadOfFailing(t *testing.T) {
	db := openTestDB(t)
	pub := newTestKey(t)
	id := "io.patchcord.example-text"

	if err := Add(context.Background(), db, id, pub, "first label"); err != nil {
		t.Fatalf("first Add() error = %v", err)
	}
	if err := Add(context.Background(), db, id, pub, "second label"); err != nil {
		t.Fatalf("second Add() error = %v", err)
	}

	keys, err := List(context.Background(), db, id)
	if err != nil {
		t.Fatalf("List() error = %v", err)
	}
	if len(keys) != 1 {
		t.Fatalf("List() = %d keys, want 1 (re-adding must not duplicate)", len(keys))
	}
	if keys[0].Label != "second label" {
		t.Fatalf("Label = %q, want %q", keys[0].Label, "second label")
	}
}

func TestList_FiltersByIDOrListsAllWhenEmpty(t *testing.T) {
	db := openTestDB(t)
	keyA := newTestKey(t)
	keyB := newTestKey(t)

	if err := Add(context.Background(), db, "io.patchcord.a", keyA, ""); err != nil {
		t.Fatalf("Add(a) error = %v", err)
	}
	if err := Add(context.Background(), db, "io.patchcord.b", keyB, ""); err != nil {
		t.Fatalf("Add(b) error = %v", err)
	}

	onlyA, err := List(context.Background(), db, "io.patchcord.a")
	if err != nil {
		t.Fatalf("List(a) error = %v", err)
	}
	if len(onlyA) != 1 || onlyA[0].ID != "io.patchcord.a" {
		t.Fatalf("List(a) = %+v, want exactly one entry for io.patchcord.a", onlyA)
	}

	all, err := List(context.Background(), db, "")
	if err != nil {
		t.Fatalf("List(\"\") error = %v", err)
	}
	if len(all) != 2 {
		t.Fatalf("List(\"\") = %d entries, want 2", len(all))
	}
}

func TestRemove(t *testing.T) {
	t.Run("revokes trust", func(t *testing.T) {
		db := openTestDB(t)
		pub := newTestKey(t)
		id := "io.patchcord.example-text"
		if err := Add(context.Background(), db, id, pub, ""); err != nil {
			t.Fatalf("Add() error = %v", err)
		}

		if err := Remove(context.Background(), db, id, pub); err != nil {
			t.Fatalf("Remove() error = %v", err)
		}

		trusted, err := IsTrusted(context.Background(), db, id, pub)
		if err != nil {
			t.Fatalf("IsTrusted() error = %v", err)
		}
		if trusted {
			t.Fatal("IsTrusted() = true after Remove, want false")
		}
	})

	t.Run("returns ErrNotFound for an unrecorded pair", func(t *testing.T) {
		db := openTestDB(t)
		pub := newTestKey(t)
		if err := Remove(context.Background(), db, "io.patchcord.example-text", pub); !errors.Is(err, ErrNotFound) {
			t.Fatalf("Remove() error = %v, want ErrNotFound", err)
		}
	})
}

func TestAdd_RejectsWrongSizeKey(t *testing.T) {
	db := openTestDB(t)
	if err := Add(context.Background(), db, "io.patchcord.example-text", ed25519.PublicKey([]byte("too short")), ""); err == nil {
		t.Fatal("expected an error for a wrong-size public key, got nil")
	}
}

func TestListedPublicKeyRoundTrips(t *testing.T) {
	db := openTestDB(t)
	pub := newTestKey(t)
	id := "io.patchcord.example-text"

	if err := Add(context.Background(), db, id, pub, ""); err != nil {
		t.Fatalf("Add() error = %v", err)
	}

	keys, err := List(context.Background(), db, id)
	if err != nil {
		t.Fatalf("List() error = %v", err)
	}
	if len(keys) != 1 {
		t.Fatalf("List() = %d keys, want 1", len(keys))
	}
	if !bytes.Equal(keys[0].PublicKey, pub) {
		t.Fatal("listed public key does not match the added one")
	}
}
