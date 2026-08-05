-- Generic key/value store for one-shot agent bootstrap state that isn't
-- tied to any single domain table — e.g. "have the bundled reference
-- plugins already been seeded into the catalog once" (ADR-0059). Deliberately
-- schemaless (TEXT value): a bootstrap flag doesn't need its own migration
-- every time a new one is introduced.
CREATE TABLE agent_meta (
    key        TEXT PRIMARY KEY,
    value      TEXT NOT NULL,
    updated_at TIMESTAMP NOT NULL DEFAULT CURRENT_TIMESTAMP
);
