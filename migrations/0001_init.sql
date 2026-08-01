-- Bootstrap migration for the Patchcord persistence layer.
--
-- No domain schema exists yet: plugins, connectors, workflows and runs each
-- introduce their own tables in later phases. This migration only proves
-- that the migration engine (internal/persistence) discovers, applies and
-- records files from this directory correctly, in order, exactly once.
CREATE TABLE schema_bootstrap (
    initialized_at TIMESTAMP NOT NULL DEFAULT CURRENT_TIMESTAMP
);

INSERT INTO schema_bootstrap DEFAULT VALUES;
