-- The plugin catalog: every plugin installed via `patchcord plugin install`.
-- Connectors, actions and permissions come straight from the manifest the
-- plugin returned during the handshake performed at install time; they are
-- stored as JSON arrays since SQLite has no native array type.
CREATE TABLE plugins (
    plugin_id        TEXT PRIMARY KEY,
    version          TEXT NOT NULL,
    executable_path  TEXT NOT NULL,
    protocol_version INTEGER NOT NULL,
    connectors       TEXT NOT NULL DEFAULT '[]',
    actions          TEXT NOT NULL DEFAULT '[]',
    permissions      TEXT NOT NULL DEFAULT '[]',
    installed_at     TIMESTAMP NOT NULL DEFAULT CURRENT_TIMESTAMP
);
