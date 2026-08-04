-- Trust store for package signing keys (ADR-0043). Trust is bound to the
-- pair (package id, public key), not the key alone: approving a key for
-- one id never implicitly trusts it for another. internal/trust.IsTrusted
-- is the only reader; internal/plugins, internal/apps, internal/bundles
-- InstallPackage consult it after internal/packaging.Verify confirms a
-- package's signature is cryptographically valid, to decide whether that
-- specific key is legitimate for that specific id.
CREATE TABLE trusted_keys (
    id         TEXT NOT NULL,
    public_key TEXT NOT NULL,
    label      TEXT NOT NULL DEFAULT '',
    trusted_at TIMESTAMP NOT NULL DEFAULT CURRENT_TIMESTAMP,
    PRIMARY KEY (id, public_key)
);
