package connectors

import (
	"context"
	"database/sql"
	"errors"
	"io"
	"log/slog"
	"testing"

	"github.com/lucasglmt/patchcord/internal/persistence"
	"github.com/lucasglmt/patchcord/internal/secrets"
	"github.com/lucasglmt/patchcord/migrations"
)

// openTestDB returns a freshly migrated, empty database.
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

func TestCreate_RecordsAConnector(t *testing.T) {
	db := openTestDB(t)

	config := map[string]any{"base_url": "https://api.example.com"}
	secretRefs := map[string]secrets.Reference{"api_key": {Type: "env", Key: "DEMO_API_KEY"}}

	conn, err := Create(context.Background(), db, "my_api", "http.request@1", config, secretRefs)
	if err != nil {
		t.Fatalf("Create() error = %v", err)
	}

	if conn.ID != "my_api" {
		t.Fatalf("ID = %q, want %q", conn.ID, "my_api")
	}
	if conn.Type != "http.request@1" {
		t.Fatalf("Type = %q, want %q", conn.Type, "http.request@1")
	}
	if conn.Config["base_url"] != "https://api.example.com" {
		t.Fatalf("Config[base_url] = %v, want %q", conn.Config["base_url"], "https://api.example.com")
	}
	if conn.SecretRefs["api_key"] != (secrets.Reference{Type: "env", Key: "DEMO_API_KEY"}) {
		t.Fatalf("SecretRefs[api_key] = %v, want {env DEMO_API_KEY}", conn.SecretRefs["api_key"])
	}
	if conn.CreatedAt.IsZero() {
		t.Fatal("CreatedAt is zero, want it populated from the database default")
	}
}

func TestCreate_RejectsAnEmptyIDOrType(t *testing.T) {
	db := openTestDB(t)

	if _, err := Create(context.Background(), db, "", "http.request@1", nil, nil); err == nil {
		t.Fatal("expected an error for an empty id, got nil")
	}
	if _, err := Create(context.Background(), db, "my_api", "", nil, nil); err == nil {
		t.Fatal("expected an error for an empty type, got nil")
	}
}

func TestCreate_RejectsAnUnsupportedSecretReferenceType(t *testing.T) {
	db := openTestDB(t)

	secretRefs := map[string]secrets.Reference{"api_key": {Type: "vault", Key: "DEMO_API_KEY"}}
	if _, err := Create(context.Background(), db, "my_api", "http.request@1", nil, secretRefs); err == nil {
		t.Fatal("expected an error for an unsupported secret reference type, got nil")
	}

	if _, err := Get(context.Background(), db, "my_api"); !errors.Is(err, ErrNotFound) {
		t.Fatalf("Get() error = %v, want ErrNotFound (rejected creation must not leave a row behind)", err)
	}
}

func TestCreate_RejectsADuplicateID(t *testing.T) {
	db := openTestDB(t)

	if _, err := Create(context.Background(), db, "my_api", "http.request@1", nil, nil); err != nil {
		t.Fatalf("first Create() error = %v", err)
	}

	_, err := Create(context.Background(), db, "my_api", "http.request@2", nil, nil)
	if !errors.Is(err, ErrAlreadyExists) {
		t.Fatalf("second Create() error = %v, want ErrAlreadyExists", err)
	}

	// The first connector's data must survive the rejected second create.
	conn, err := Get(context.Background(), db, "my_api")
	if err != nil {
		t.Fatalf("Get() error = %v", err)
	}
	if conn.Type != "http.request@1" {
		t.Fatalf("Type = %q, want %q (must not have been overwritten)", conn.Type, "http.request@1")
	}
}

func TestGet_ReturnsErrNotFoundForAnUnknownID(t *testing.T) {
	db := openTestDB(t)

	if _, err := Get(context.Background(), db, "does-not-exist"); !errors.Is(err, ErrNotFound) {
		t.Fatalf("Get() error = %v, want ErrNotFound", err)
	}
}

func TestList_OrdersByID(t *testing.T) {
	db := openTestDB(t)

	for _, id := range []string{"charlie", "alpha", "bravo"} {
		if _, err := Create(context.Background(), db, id, "http.request@1", nil, nil); err != nil {
			t.Fatalf("Create(%q) error = %v", id, err)
		}
	}

	conns, err := List(context.Background(), db)
	if err != nil {
		t.Fatalf("List() error = %v", err)
	}

	var ids []string
	for _, c := range conns {
		ids = append(ids, c.ID)
	}
	want := []string{"alpha", "bravo", "charlie"}
	if len(ids) != len(want) {
		t.Fatalf("List() ids = %v, want %v", ids, want)
	}
	for i := range want {
		if ids[i] != want[i] {
			t.Fatalf("List() ids = %v, want %v", ids, want)
		}
	}
}

// failingStore always fails to resolve, used to exercise Resolve's error
// path without depending on a real secrets.Store adapter.
type failingStore struct{ err error }

func (f failingStore) Resolve(context.Context, secrets.Reference) (string, error) {
	return "", f.err
}

func TestResolve(t *testing.T) {
	t.Run("resolves config and every secret reference", func(t *testing.T) {
		db := openTestDB(t)
		t.Setenv("PATCHCORD_CONNECTORS_TEST_SECRET", "s3cr3t")

		config := map[string]any{"base_url": "https://api.example.com"}
		secretRefs := map[string]secrets.Reference{"api_key": {Type: "env", Key: "PATCHCORD_CONNECTORS_TEST_SECRET"}}
		if _, err := Create(context.Background(), db, "my_api", "http.request@1", config, secretRefs); err != nil {
			t.Fatalf("Create() error = %v", err)
		}

		resolved, err := Resolve(context.Background(), db, "my_api", secrets.EnvStore{})
		if err != nil {
			t.Fatalf("Resolve() error = %v", err)
		}

		if resolved.Type != "http.request@1" {
			t.Fatalf("Type = %q, want %q", resolved.Type, "http.request@1")
		}
		if resolved.Config["base_url"] != "https://api.example.com" {
			t.Fatalf("Config[base_url] = %v, want %q", resolved.Config["base_url"], "https://api.example.com")
		}
		if resolved.Secrets["api_key"] != "s3cr3t" {
			t.Fatalf("Secrets[api_key] = %v, want %q", resolved.Secrets["api_key"], "s3cr3t")
		}
	})

	t.Run("returns ErrNotFound for an unknown connector", func(t *testing.T) {
		db := openTestDB(t)

		if _, err := Resolve(context.Background(), db, "does-not-exist", secrets.EnvStore{}); !errors.Is(err, ErrNotFound) {
			t.Fatalf("Resolve() error = %v, want ErrNotFound", err)
		}
	})

	t.Run("fails when a secret cannot be resolved", func(t *testing.T) {
		db := openTestDB(t)
		secretRefs := map[string]secrets.Reference{"api_key": {Type: "env", Key: "PATCHCORD_CONNECTORS_TEST_SECRET_UNSET"}}
		if _, err := Create(context.Background(), db, "my_api", "http.request@1", nil, secretRefs); err != nil {
			t.Fatalf("Create() error = %v", err)
		}

		if _, err := Resolve(context.Background(), db, "my_api", secrets.EnvStore{}); err == nil {
			t.Fatal("expected an error for an unresolvable secret, got nil")
		}
	})

	t.Run("wraps the store's error", func(t *testing.T) {
		db := openTestDB(t)
		secretRefs := map[string]secrets.Reference{"api_key": {Type: "env", Key: "X"}}
		if _, err := Create(context.Background(), db, "my_api", "http.request@1", nil, secretRefs); err != nil {
			t.Fatalf("Create() error = %v", err)
		}

		boom := errors.New("boom")
		_, err := Resolve(context.Background(), db, "my_api", failingStore{err: boom})
		if !errors.Is(err, boom) {
			t.Fatalf("Resolve() error = %v, want it to wrap %v", err, boom)
		}
	})
}

func TestDelete(t *testing.T) {
	t.Run("removes an existing connector", func(t *testing.T) {
		db := openTestDB(t)
		if _, err := Create(context.Background(), db, "my_api", "http.request@1", nil, nil); err != nil {
			t.Fatalf("Create() error = %v", err)
		}

		if err := Delete(context.Background(), db, "my_api"); err != nil {
			t.Fatalf("Delete() error = %v", err)
		}

		if _, err := Get(context.Background(), db, "my_api"); !errors.Is(err, ErrNotFound) {
			t.Fatalf("Get() after Delete() error = %v, want ErrNotFound", err)
		}
	})

	t.Run("returns ErrNotFound for an unknown id", func(t *testing.T) {
		db := openTestDB(t)
		if err := Delete(context.Background(), db, "does-not-exist"); !errors.Is(err, ErrNotFound) {
			t.Fatalf("Delete() error = %v, want ErrNotFound", err)
		}
	})
}
