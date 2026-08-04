// Package trust is the trust store for package signing keys (ADR-0043): it
// answers "is this public key approved to sign this package id?" — nothing
// about whether a signature is cryptographically valid in the first place,
// that's internal/packaging.Verify's job. Trust is bound to the pair
// (package id, public key), not the key alone.
package trust

import (
	"context"
	"crypto/ed25519"
	"database/sql"
	"encoding/base64"
	"errors"
	"fmt"
	"time"
)

// ErrNotFound is returned by Remove when no trusted key matches the given
// id and public key.
var ErrNotFound = errors.New("trusted key not found")

// TrustedKey is one approved (package id, public key) pair, as recorded in
// the database.
type TrustedKey struct {
	ID        string
	PublicKey ed25519.PublicKey
	Label     string
	TrustedAt time.Time
}

// Add records pub as trusted for id. Re-adding the same (id, pub) pair
// updates its label and trusted_at instead of failing — approving a key
// twice is not an error.
func Add(ctx context.Context, db *sql.DB, id string, pub ed25519.PublicKey, label string) error {
	if len(pub) != ed25519.PublicKeySize {
		return fmt.Errorf("public key has wrong size: got %d bytes, want %d", len(pub), ed25519.PublicKeySize)
	}

	_, err := db.ExecContext(ctx, `
		INSERT INTO trusted_keys (id, public_key, label)
		VALUES (?, ?, ?)
		ON CONFLICT(id, public_key) DO UPDATE SET
			label = excluded.label,
			trusted_at = CURRENT_TIMESTAMP
	`, id, encodeKey(pub), label)
	if err != nil {
		return fmt.Errorf("trust key for %q: %w", id, err)
	}

	return nil
}

// Remove revokes trust for the (id, pub) pair. It returns ErrNotFound if
// no such trusted key was recorded.
func Remove(ctx context.Context, db *sql.DB, id string, pub ed25519.PublicKey) error {
	result, err := db.ExecContext(ctx, `
		DELETE FROM trusted_keys WHERE id = ? AND public_key = ?
	`, id, encodeKey(pub))
	if err != nil {
		return fmt.Errorf("remove trusted key for %q: %w", id, err)
	}

	affected, err := result.RowsAffected()
	if err != nil {
		return fmt.Errorf("remove trusted key for %q: %w", id, err)
	}
	if affected == 0 {
		return ErrNotFound
	}

	return nil
}

// List returns every trusted key recorded for id, most recently trusted
// first. An empty id lists every trusted key regardless of package id.
func List(ctx context.Context, db *sql.DB, id string) ([]TrustedKey, error) {
	query := `SELECT id, public_key, label, trusted_at FROM trusted_keys`
	args := []any{}
	if id != "" {
		query += ` WHERE id = ?`
		args = append(args, id)
	}
	query += ` ORDER BY trusted_at DESC`

	rows, err := db.QueryContext(ctx, query, args...)
	if err != nil {
		return nil, fmt.Errorf("list trusted keys: %w", err)
	}
	defer rows.Close()

	var keys []TrustedKey
	for rows.Next() {
		key, err := scanTrustedKey(rows)
		if err != nil {
			return nil, fmt.Errorf("scan trusted key: %w", err)
		}
		keys = append(keys, key)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("list trusted keys: %w", err)
	}

	return keys, nil
}

// IsTrusted reports whether pub is recorded as trusted for id.
func IsTrusted(ctx context.Context, db *sql.DB, id string, pub ed25519.PublicKey) (bool, error) {
	var exists bool
	err := db.QueryRowContext(ctx, `
		SELECT EXISTS(SELECT 1 FROM trusted_keys WHERE id = ? AND public_key = ?)
	`, id, encodeKey(pub)).Scan(&exists)
	if err != nil {
		return false, fmt.Errorf("check trust for %q: %w", id, err)
	}
	return exists, nil
}

// rowScanner is satisfied by both *sql.Row and *sql.Rows.
type rowScanner interface {
	Scan(dest ...any) error
}

func scanTrustedKey(row rowScanner) (TrustedKey, error) {
	var (
		key        TrustedKey
		encodedKey string
	)
	if err := row.Scan(&key.ID, &encodedKey, &key.Label, &key.TrustedAt); err != nil {
		return TrustedKey{}, err
	}

	pub, err := base64.StdEncoding.DecodeString(encodedKey)
	if err != nil {
		return TrustedKey{}, fmt.Errorf("decode public key: %w", err)
	}
	key.PublicKey = ed25519.PublicKey(pub)

	return key, nil
}

func encodeKey(pub ed25519.PublicKey) string {
	return base64.StdEncoding.EncodeToString(pub)
}
