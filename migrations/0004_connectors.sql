-- Connector instances: persistent, named configuration for accessing an
-- external system (vision doc, section 7.3). Secrets are never stored here
-- — secret_refs holds logical references (internal/secrets.Reference)
-- resolved live at use time, never persisted (ADR-0009, ADR-0020).
CREATE TABLE connectors (
    id           TEXT PRIMARY KEY,
    type         TEXT NOT NULL,
    config       TEXT NOT NULL DEFAULT '{}',
    secret_refs  TEXT NOT NULL DEFAULT '{}',
    created_at   TIMESTAMP NOT NULL DEFAULT CURRENT_TIMESTAMP
);
