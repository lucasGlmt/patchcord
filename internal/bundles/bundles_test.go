package bundles

import (
	"context"
	"errors"
	"testing"
)

// TestGet_ReturnsErrNotFoundForAnUnknownID guards Get's "not found"
// mapping: sql.ErrNoRows must surface as bundles.ErrNotFound, not leak the
// database's own sentinel to callers.
func TestGet_ReturnsErrNotFoundForAnUnknownID(t *testing.T) {
	db := openTestDB(t)

	_, err := Get(context.Background(), db, "does-not-exist")
	if !errors.Is(err, ErrNotFound) {
		t.Fatalf("Get() error = %v, want ErrNotFound", err)
	}
}

// TestRecordAndGet round-trips a bundle's provenance row, and confirms
// record() upserts rather than duplicating when called again for the same
// id — the behavior its doc comment promises for reinstalling a bundle
// after fixing a typo in bundle.yaml.
func TestRecordAndGet(t *testing.T) {
	db := openTestDB(t)
	ctx := context.Background()

	if err := record(ctx, db, "lead-crm", "1", "manifest v1"); err != nil {
		t.Fatalf("record() error = %v", err)
	}

	got, err := Get(ctx, db, "lead-crm")
	if err != nil {
		t.Fatalf("Get() error = %v", err)
	}
	if got.ID != "lead-crm" || got.Version != "1" || got.Manifest != "manifest v1" {
		t.Fatalf("Get() = %+v, want id/version/manifest = lead-crm/1/manifest v1", got)
	}

	if err := record(ctx, db, "lead-crm", "2", "manifest v2"); err != nil {
		t.Fatalf("record() (reinstall) error = %v", err)
	}

	got, err = Get(ctx, db, "lead-crm")
	if err != nil {
		t.Fatalf("Get() after reinstall error = %v", err)
	}
	if got.Version != "2" || got.Manifest != "manifest v2" {
		t.Fatalf("Get() after reinstall = %+v, want version/manifest = 2/manifest v2 (upsert, not a duplicate row)", got)
	}

	list, err := List(ctx, db)
	if err != nil {
		t.Fatalf("List() error = %v", err)
	}
	if len(list) != 1 {
		t.Fatalf("List() returned %d bundles, want 1 (reinstall must not duplicate the row)", len(list))
	}
}

// TestList_EmptyAndOrdered covers List's two untested edges: no bundles
// installed yet (nil slice, no error), and several installed bundles
// coming back ordered by id regardless of insertion order.
func TestList_EmptyAndOrdered(t *testing.T) {
	db := openTestDB(t)
	ctx := context.Background()

	empty, err := List(ctx, db)
	if err != nil {
		t.Fatalf("List() error = %v", err)
	}
	if len(empty) != 0 {
		t.Fatalf("List() = %v, want empty before anything is installed", empty)
	}

	for _, id := range []string{"zeta-bundle", "alpha-bundle", "mu-bundle"} {
		if err := record(ctx, db, id, "1", "manifest"); err != nil {
			t.Fatalf("record(%q) error = %v", id, err)
		}
	}

	list, err := List(ctx, db)
	if err != nil {
		t.Fatalf("List() error = %v", err)
	}
	if len(list) != 3 {
		t.Fatalf("List() returned %d bundles, want 3", len(list))
	}
	wantOrder := []string{"alpha-bundle", "mu-bundle", "zeta-bundle"}
	for i, want := range wantOrder {
		if list[i].ID != want {
			t.Fatalf("List()[%d].ID = %q, want %q (want id order)", i, list[i].ID, want)
		}
	}
}
