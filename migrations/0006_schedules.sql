-- One row per workflow currently installed with a "schedule" trigger
-- (internal/workflow.Trigger, ADR-0035). Keyed by workflow_id, not by
-- (workflow_id, version): a schedule always tracks whichever version is
-- currently latest, mirroring runs.Execute's own "latest installed version"
-- resolution — installing a new version overwrites this row (internal/
-- scheduler.Sync), and reverting to a "manual" trigger deletes it.
CREATE TABLE schedules (
    workflow_id  TEXT PRIMARY KEY,
    cron         TEXT NOT NULL,
    on_missed    TEXT NOT NULL,
    next_run_at  TIMESTAMP NOT NULL,
    updated_at   TIMESTAMP NOT NULL DEFAULT CURRENT_TIMESTAMP
);
