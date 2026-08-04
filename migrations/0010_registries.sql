-- Configured package registries (ADR-0044). A registry is nothing more
-- than a name pointing at a local directory or an http(s) URL serving a
-- static index.json plus package files (internal/registry) — no bespoke
-- server, no auth, no commerce; fully usable offline via a local
-- directory. Re-adding the same name updates its location instead of
-- failing, mirroring trusted_keys' upsert-on-conflict philosophy.
CREATE TABLE registries (
    name     TEXT PRIMARY KEY,
    location TEXT NOT NULL,
    added_at TIMESTAMP NOT NULL DEFAULT CURRENT_TIMESTAMP
);
