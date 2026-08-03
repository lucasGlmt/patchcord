-- Admin tokens: full, unscoped bearer credentials for the public API
-- (ADR-0036) — distinct from an installed application's limited Session
-- (internal/auth.Store, in-memory, scoped to one app's declared
-- permissions). Created only via `patchcord auth token create`, never over
-- HTTP: the very first token can't pass through an API that would already
-- require one. Only the hash is stored; the plaintext is shown once, at
-- creation, and is never recoverable afterwards.
CREATE TABLE admin_tokens (
    id         TEXT PRIMARY KEY,
    name       TEXT NOT NULL,
    token_hash TEXT NOT NULL UNIQUE,
    created_at TIMESTAMP NOT NULL DEFAULT CURRENT_TIMESTAMP
);
