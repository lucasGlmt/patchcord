package auth

import (
	"context"
	"crypto/rand"
	"crypto/sha256"
	"database/sql"
	"encoding/base64"
	"encoding/hex"
	"errors"
	"fmt"
	"time"

	"github.com/google/uuid"
)

// tokenPrefix marks a plaintext admin token as such — visually distinct
// from an app Session's token (a bare UUID) and from anything else that
// might end up in a log line, making the two credential kinds impossible to
// confuse by inspection alone.
const tokenPrefix = "pcat_"

// ErrInvalidToken is returned by ValidateToken when the token is unknown
// (never created, mistyped, or already revoked).
var ErrInvalidToken = errors.New("invalid admin token")

// AdminToken is one issued admin credential: full, unscoped access to the
// public API (ADR-0036) — unlike a Session, which is limited to one
// installed application's declared permissions. Its plaintext value is
// never stored or returned again after CreateToken; only its hash is kept.
type AdminToken struct {
	ID        string
	Name      string
	CreatedAt time.Time
}

// CreateToken generates a new random admin token, records its hash under
// name, and returns the plaintext once — the only time it is ever
// available. There is no recovery for a lost token, only creating another
// one and revoking it (RevokeToken).
func CreateToken(ctx context.Context, db *sql.DB, name string) (plaintext string, token AdminToken, err error) {
	secret := make([]byte, 32)
	if _, err := rand.Read(secret); err != nil {
		return "", AdminToken{}, fmt.Errorf("generate admin token: %w", err)
	}
	plaintext = tokenPrefix + base64.RawURLEncoding.EncodeToString(secret)

	token = AdminToken{
		ID:        uuid.NewString(),
		Name:      name,
		CreatedAt: time.Now().UTC(),
	}

	if _, err := db.ExecContext(ctx, `
		INSERT INTO admin_tokens (id, name, token_hash, created_at)
		VALUES (?, ?, ?, ?)
	`, token.ID, token.Name, hashToken(plaintext), token.CreatedAt); err != nil {
		return "", AdminToken{}, fmt.Errorf("record admin token: %w", err)
	}

	return plaintext, token, nil
}

// ValidateToken reports whether plaintext matches a currently recorded
// admin token, returning ErrInvalidToken otherwise (unknown or already
// revoked — indistinguishable on purpose, same as an invalid Session).
func ValidateToken(ctx context.Context, db *sql.DB, plaintext string) (AdminToken, error) {
	var token AdminToken
	err := db.QueryRowContext(ctx, `
		SELECT id, name, created_at FROM admin_tokens WHERE token_hash = ?
	`, hashToken(plaintext)).Scan(&token.ID, &token.Name, &token.CreatedAt)
	if errors.Is(err, sql.ErrNoRows) {
		return AdminToken{}, ErrInvalidToken
	}
	if err != nil {
		return AdminToken{}, fmt.Errorf("validate admin token: %w", err)
	}
	return token, nil
}

// AnyTokensExist reports whether at least one admin token has ever been
// created. internal/api's admin-auth middleware only starts requiring one
// once this is true — an opt-in flipped by data, not by which address
// `patchcord serve` binds to (CLAUDE.md's non-negotiable #2 forbids
// branching core behavior on local-vs-server deployment) — so a fresh agent
// stays exactly as open as it was before this feature existed, until an
// operator deliberately creates a first token (`patchcord auth token
// create`). See ADR-0036.
func AnyTokensExist(ctx context.Context, db *sql.DB) (bool, error) {
	var exists bool
	if err := db.QueryRowContext(ctx, `SELECT EXISTS(SELECT 1 FROM admin_tokens)`).Scan(&exists); err != nil {
		return false, fmt.Errorf("check for admin tokens: %w", err)
	}
	return exists, nil
}

// ListTokens returns every recorded admin token, most recently created
// first — never their plaintext or hash, which ValidateToken never exposes
// either.
func ListTokens(ctx context.Context, db *sql.DB) ([]AdminToken, error) {
	rows, err := db.QueryContext(ctx, `
		SELECT id, name, created_at FROM admin_tokens ORDER BY created_at DESC
	`)
	if err != nil {
		return nil, fmt.Errorf("list admin tokens: %w", err)
	}
	defer rows.Close()

	var tokens []AdminToken
	for rows.Next() {
		var token AdminToken
		if err := rows.Scan(&token.ID, &token.Name, &token.CreatedAt); err != nil {
			return nil, fmt.Errorf("scan admin token: %w", err)
		}
		tokens = append(tokens, token)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("list admin tokens: %w", err)
	}

	return tokens, nil
}

// RevokeToken deletes the admin token identified by id, returning
// ErrInvalidToken if no such token exists.
func RevokeToken(ctx context.Context, db *sql.DB, id string) error {
	result, err := db.ExecContext(ctx, `DELETE FROM admin_tokens WHERE id = ?`, id)
	if err != nil {
		return fmt.Errorf("revoke admin token: %w", err)
	}
	affected, err := result.RowsAffected()
	if err != nil {
		return fmt.Errorf("revoke admin token: %w", err)
	}
	if affected == 0 {
		return ErrInvalidToken
	}
	return nil
}

// hashToken is a one-way digest of an admin token's plaintext — what's
// actually stored, so a leaked database backup never yields a usable
// credential. A token is already a high-entropy random secret, not a
// user-chosen password, so a plain fast hash (unlike bcrypt/argon2, built
// for low-entropy input) is the right tool here.
func hashToken(plaintext string) string {
	sum := sha256.Sum256([]byte(plaintext))
	return hex.EncodeToString(sum[:])
}
