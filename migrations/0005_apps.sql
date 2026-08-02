-- Installed applications (vision doc, section 7.6). An application never
-- receives the agent's full privileges: permissions holds the manifest's
-- declared permission set (internal/apps.AppPermissions, JSON-encoded),
-- which internal/auth uses to limit the sessions it issues for this app.
-- static_dir points at the application's static files on disk — no
-- packaging/copy step yet, see ADR-0026.
CREATE TABLE apps (
    id          TEXT PRIMARY KEY,
    version     TEXT NOT NULL,
    static_dir  TEXT NOT NULL,
    permissions TEXT NOT NULL DEFAULT '{}',
    created_at  TIMESTAMP NOT NULL DEFAULT CURRENT_TIMESTAMP
);
