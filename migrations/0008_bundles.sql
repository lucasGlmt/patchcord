-- Installed bundles (vision doc, section 9.3: "regroupe application,
-- workflows, configuration et dépendances"). A bundle install delegates to
-- apps.Install and runs.InstallWorkflow — this table is a provenance record
-- of what was declared, not a second source of truth for what got
-- installed (see internal/apps.apps, internal/runs.workflow_versions for
-- that). manifest stores the raw bundle.yaml source, the same
-- don't-over-normalize choice workflow_versions.definition already makes.
CREATE TABLE bundles (
    id           TEXT PRIMARY KEY,
    version      TEXT NOT NULL,
    manifest     TEXT NOT NULL,
    installed_at TIMESTAMP NOT NULL DEFAULT CURRENT_TIMESTAMP
);
