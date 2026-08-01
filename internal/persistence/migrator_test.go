package persistence

import (
	"context"
	"database/sql"
	"testing"
	"testing/fstest"

	_ "modernc.org/sqlite"
)

func openMemoryDB(t *testing.T) *sql.DB {
	t.Helper()
	db, err := sql.Open("sqlite", "file::memory:")
	if err != nil {
		t.Fatalf("open in-memory database: %v", err)
	}
	t.Cleanup(func() { _ = db.Close() })
	return db
}

func TestLoadMigrations(t *testing.T) {
	tests := []struct {
		name      string
		fsys      fstest.MapFS
		wantErr   bool
		wantOrder []int
	}{
		{
			name: "sorts by version regardless of file order",
			fsys: fstest.MapFS{
				"0002_second.sql": {Data: []byte("SELECT 1;")},
				"0001_first.sql":  {Data: []byte("SELECT 1;")},
			},
			wantOrder: []int{1, 2},
		},
		{
			name: "rejects duplicate versions",
			fsys: fstest.MapFS{
				"0001_first.sql": {Data: []byte("SELECT 1;")},
				"0001_again.sql": {Data: []byte("SELECT 1;")},
			},
			wantErr: true,
		},
		{
			name: "rejects files that don't follow the naming convention",
			fsys: fstest.MapFS{
				"init.sql": {Data: []byte("SELECT 1;")},
			},
			wantErr: true,
		},
		{
			name: "empty directory yields no migrations",
			fsys: fstest.MapFS{},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			migrations, err := LoadMigrations(tt.fsys)

			if tt.wantErr {
				if err == nil {
					t.Fatal("expected an error, got nil")
				}
				return
			}
			if err != nil {
				t.Fatalf("LoadMigrations() error = %v", err)
			}

			if len(migrations) != len(tt.wantOrder) {
				t.Fatalf("got %d migrations, want %d", len(migrations), len(tt.wantOrder))
			}
			for i, wantVersion := range tt.wantOrder {
				if migrations[i].Version != wantVersion {
					t.Fatalf("migrations[%d].Version = %d, want %d", i, migrations[i].Version, wantVersion)
				}
			}
		})
	}
}

func TestMigrate_AppliesInOrderAndIsIdempotent(t *testing.T) {
	db := openMemoryDB(t)
	fsys := fstest.MapFS{
		"0001_create_widgets.sql": {Data: []byte(`CREATE TABLE widgets (id INTEGER PRIMARY KEY);`)},
		"0002_seed_widgets.sql":   {Data: []byte(`INSERT INTO widgets (id) VALUES (1);`)},
	}

	if err := Migrate(context.Background(), db, fsys, nil); err != nil {
		t.Fatalf("first Migrate() error = %v", err)
	}

	var count int
	if err := db.QueryRow("SELECT COUNT(*) FROM widgets").Scan(&count); err != nil {
		t.Fatalf("query widgets: %v", err)
	}
	if count != 1 {
		t.Fatalf("widgets count = %d, want 1", count)
	}

	// Calling Migrate again must not re-apply already-applied migrations
	// (it would fail on "table widgets already exists" if it tried).
	if err := Migrate(context.Background(), db, fsys, nil); err != nil {
		t.Fatalf("second Migrate() error = %v", err)
	}

	var appliedCount int
	if err := db.QueryRow("SELECT COUNT(*) FROM schema_migrations").Scan(&appliedCount); err != nil {
		t.Fatalf("query schema_migrations: %v", err)
	}
	if appliedCount != 2 {
		t.Fatalf("schema_migrations count = %d, want 2", appliedCount)
	}
}

func TestMigrate_RollsBackFailedMigration(t *testing.T) {
	db := openMemoryDB(t)
	fsys := fstest.MapFS{
		"0001_broken.sql": {Data: []byte(`THIS IS NOT VALID SQL;`)},
	}

	if err := Migrate(context.Background(), db, fsys, nil); err == nil {
		t.Fatal("expected an error from an invalid migration, got nil")
	}

	var count int
	if err := db.QueryRow("SELECT COUNT(*) FROM schema_migrations").Scan(&count); err != nil {
		t.Fatalf("query schema_migrations: %v", err)
	}
	if count != 0 {
		t.Fatalf("schema_migrations count = %d, want 0 after a rolled-back migration", count)
	}
}
