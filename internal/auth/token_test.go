package auth

import (
	"context"
	"database/sql"
	"errors"
	"io"
	"log/slog"
	"strings"
	"testing"

	"github.com/lucasglmt/patchcord/internal/persistence"
	"github.com/lucasglmt/patchcord/migrations"
)

// openTestDB returns a freshly migrated, empty database — same pattern as
// internal/runs's and internal/scheduler's helpers_test.go.
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

func TestAnyTokensExist_FalseUntilOneIsCreated(t *testing.T) {
	db := openTestDB(t)
	ctx := context.Background()

	if exists, err := AnyTokensExist(ctx, db); err != nil || exists {
		t.Fatalf("AnyTokensExist() = (%v, %v), want (false, nil)", exists, err)
	}

	if _, _, err := CreateToken(ctx, db, "ci"); err != nil {
		t.Fatalf("CreateToken() error = %v", err)
	}

	if exists, err := AnyTokensExist(ctx, db); err != nil || !exists {
		t.Fatalf("AnyTokensExist() = (%v, %v), want (true, nil)", exists, err)
	}
}

func TestCreateAndValidateToken(t *testing.T) {
	db := openTestDB(t)
	ctx := context.Background()

	plaintext, token, err := CreateToken(ctx, db, "ci")
	if err != nil {
		t.Fatalf("CreateToken() error = %v", err)
	}
	if !strings.HasPrefix(plaintext, tokenPrefix) {
		t.Fatalf("plaintext = %q, want it to start with %q", plaintext, tokenPrefix)
	}
	if token.ID == "" {
		t.Fatal("token.ID is empty")
	}
	if token.Name != "ci" {
		t.Fatalf("token.Name = %q, want %q", token.Name, "ci")
	}
	if token.CreatedAt.IsZero() {
		t.Fatal("token.CreatedAt is zero, want it populated")
	}

	got, err := ValidateToken(ctx, db, plaintext)
	if err != nil {
		t.Fatalf("ValidateToken() error = %v", err)
	}
	if got.ID != token.ID {
		t.Fatalf("ValidateToken() ID = %q, want %q", got.ID, token.ID)
	}
}

func TestValidateToken_RejectsAnUnknownToken(t *testing.T) {
	db := openTestDB(t)

	if _, err := ValidateToken(context.Background(), db, tokenPrefix+"does-not-exist"); !errors.Is(err, ErrInvalidToken) {
		t.Fatalf("ValidateToken() error = %v, want ErrInvalidToken", err)
	}
}

func TestCreateToken_GeneratesDistinctTokens(t *testing.T) {
	db := openTestDB(t)
	ctx := context.Background()

	first, _, err := CreateToken(ctx, db, "a")
	if err != nil {
		t.Fatalf("CreateToken() error = %v", err)
	}
	second, _, err := CreateToken(ctx, db, "b")
	if err != nil {
		t.Fatalf("CreateToken() error = %v", err)
	}

	if first == second {
		t.Fatalf("CreateToken() returned the same token twice: %q", first)
	}
}

func TestListTokens_MostRecentFirst(t *testing.T) {
	db := openTestDB(t)
	ctx := context.Background()

	_, first, err := CreateToken(ctx, db, "first")
	if err != nil {
		t.Fatalf("CreateToken() error = %v", err)
	}
	_, second, err := CreateToken(ctx, db, "second")
	if err != nil {
		t.Fatalf("CreateToken() error = %v", err)
	}

	tokens, err := ListTokens(ctx, db)
	if err != nil {
		t.Fatalf("ListTokens() error = %v", err)
	}
	if len(tokens) != 2 {
		t.Fatalf("ListTokens() returned %d tokens, want 2", len(tokens))
	}
	if tokens[0].ID != second.ID || tokens[1].ID != first.ID {
		t.Fatalf("ListTokens() = %+v, want most recently created first", tokens)
	}
}

func TestRevokeToken(t *testing.T) {
	db := openTestDB(t)
	ctx := context.Background()

	plaintext, token, err := CreateToken(ctx, db, "ci")
	if err != nil {
		t.Fatalf("CreateToken() error = %v", err)
	}

	if err := RevokeToken(ctx, db, token.ID); err != nil {
		t.Fatalf("RevokeToken() error = %v", err)
	}

	if _, err := ValidateToken(ctx, db, plaintext); !errors.Is(err, ErrInvalidToken) {
		t.Fatalf("ValidateToken() after revoke error = %v, want ErrInvalidToken", err)
	}
}

func TestRevokeToken_RejectsAnUnknownID(t *testing.T) {
	db := openTestDB(t)

	if err := RevokeToken(context.Background(), db, "does-not-exist"); !errors.Is(err, ErrInvalidToken) {
		t.Fatalf("RevokeToken() error = %v, want ErrInvalidToken", err)
	}
}
