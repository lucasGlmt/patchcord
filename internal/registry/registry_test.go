package registry

import (
	"context"
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

func TestAddAndList(t *testing.T) {
	db := openTestDB(t)

	if err := Add(context.Background(), db, "local", "/srv/registry"); err != nil {
		t.Fatalf("Add() error = %v", err)
	}

	registries, err := List(context.Background(), db)
	if err != nil {
		t.Fatalf("List() error = %v", err)
	}
	if len(registries) != 1 {
		t.Fatalf("List() = %d entries, want 1", len(registries))
	}
	if registries[0].Name != "local" || registries[0].Location != "/srv/registry" {
		t.Fatalf("registry = %+v, want name=local location=/srv/registry", registries[0])
	}
}

func TestAdd_ReAddingUpdatesLocationInsteadOfFailing(t *testing.T) {
	db := openTestDB(t)

	if err := Add(context.Background(), db, "local", "/srv/registry"); err != nil {
		t.Fatalf("first Add() error = %v", err)
	}
	if err := Add(context.Background(), db, "local", "/srv/registry-v2"); err != nil {
		t.Fatalf("second Add() error = %v", err)
	}

	registries, err := List(context.Background(), db)
	if err != nil {
		t.Fatalf("List() error = %v", err)
	}
	if len(registries) != 1 {
		t.Fatalf("List() = %d entries, want 1 (re-adding must not duplicate)", len(registries))
	}
	if registries[0].Location != "/srv/registry-v2" {
		t.Fatalf("Location = %q, want %q", registries[0].Location, "/srv/registry-v2")
	}
}

func TestRemove(t *testing.T) {
	t.Run("deletes a configured registry", func(t *testing.T) {
		db := openTestDB(t)
		if err := Add(context.Background(), db, "local", "/srv/registry"); err != nil {
			t.Fatalf("Add() error = %v", err)
		}

		if err := Remove(context.Background(), db, "local"); err != nil {
			t.Fatalf("Remove() error = %v", err)
		}

		registries, err := List(context.Background(), db)
		if err != nil {
			t.Fatalf("List() error = %v", err)
		}
		if len(registries) != 0 {
			t.Fatalf("List() = %d entries, want 0 after Remove", len(registries))
		}
	})

	t.Run("returns ErrNotFound for an unconfigured name", func(t *testing.T) {
		db := openTestDB(t)
		if err := Remove(context.Background(), db, "unknown"); !errors.Is(err, ErrNotFound) {
			t.Fatalf("Remove() error = %v, want ErrNotFound", err)
		}
	})
}

func TestList_OrderedByAddedAt(t *testing.T) {
	db := openTestDB(t)

	for _, name := range []string{"first", "second", "third"} {
		if err := Add(context.Background(), db, name, "/srv/"+name); err != nil {
			t.Fatalf("Add(%q) error = %v", name, err)
		}
	}

	registries, err := List(context.Background(), db)
	if err != nil {
		t.Fatalf("List() error = %v", err)
	}
	if len(registries) != 3 {
		t.Fatalf("List() = %d entries, want 3", len(registries))
	}
	for i, want := range []string{"first", "second", "third"} {
		if registries[i].Name != want {
			t.Fatalf("registries[%d].Name = %q, want %q (insertion order)", i, registries[i].Name, want)
		}
	}
}

func TestList_EmptyWhenNoneConfigured(t *testing.T) {
	db := openTestDB(t)

	registries, err := List(context.Background(), db)
	if err != nil {
		t.Fatalf("List() error = %v", err)
	}
	if len(registries) != 0 {
		t.Fatalf("List() = %d entries, want 0", len(registries))
	}
}
