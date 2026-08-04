package trust

import (
	"context"
	"database/sql"
	"errors"
	"fmt"

	"github.com/lucasglmt/patchcord/internal/packaging"
)

// ErrSignatureRequired is returned by CheckPolicy when requireSignature is
// true and the package is not signed by a key trusted for id.
var ErrSignatureRequired = errors.New("package requires a trusted signature")

// PolicyResult is what CheckPolicy found for one package install: its raw
// verification outcome (internal/packaging.Verify), plus whether its
// signer (if any) is trusted for its id. Callers (internal/apps,
// internal/plugins, internal/bundles InstallPackage) return this to their
// own caller so the CLI can print an accurate warning even when
// requireSignature is false and the install proceeds anyway.
type PolicyResult struct {
	Outcome packaging.VerificationOutcome
	Trusted bool
}

// CheckPolicy decides whether an InstallPackage call should proceed, given
// a package's already-computed verification outcome: it looks up whether
// outcome.PublicKey is trusted for id (only meaningful when the package is
// signed at all), then — if requireSignature is true — fails unless the
// package is both signed and trusted.
//
// It never second-guesses outcome itself: a checksum mismatch or an
// invalid signature already aborted the install before CheckPolicy is ever
// called (see internal/packaging.Verify) — this function only decides what
// to do about a package that is cryptographically sound but possibly
// unsigned or signed by a key nobody has approved yet.
func CheckPolicy(ctx context.Context, db *sql.DB, id string, outcome packaging.VerificationOutcome, requireSignature bool) (PolicyResult, error) {
	result := PolicyResult{Outcome: outcome}

	if outcome.Signed {
		trusted, err := IsTrusted(ctx, db, id, outcome.PublicKey)
		if err != nil {
			return PolicyResult{}, err
		}
		result.Trusted = trusted
	}

	if requireSignature && !(outcome.Signed && result.Trusted) {
		return result, fmt.Errorf("%w: %q", ErrSignatureRequired, id)
	}

	return result, nil
}
