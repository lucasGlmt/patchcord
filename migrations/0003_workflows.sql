-- Published workflow versions. A row is inserted once per (workflow_id,
-- version) and never updated afterwards: published workflows are immutable
-- (ADR-0008). The raw YAML source is kept for auditability and export.
CREATE TABLE workflow_versions (
    workflow_id  TEXT NOT NULL,
    version      INTEGER NOT NULL,
    definition   TEXT NOT NULL,
    installed_at TIMESTAMP NOT NULL DEFAULT CURRENT_TIMESTAMP,
    PRIMARY KEY (workflow_id, version)
);

-- One row per workflow run. status follows the Run state machine
-- (internal/workflow.RunStatus): queued, running, succeeded, failed,
-- cancelled.
CREATE TABLE runs (
    id               TEXT PRIMARY KEY,
    workflow_id      TEXT NOT NULL,
    workflow_version INTEGER NOT NULL,
    status           TEXT NOT NULL,
    inputs           TEXT NOT NULL DEFAULT '{}',
    outputs          TEXT NOT NULL DEFAULT '{}',
    error            TEXT,
    started_at       TIMESTAMP,
    finished_at      TIMESTAMP,
    created_at       TIMESTAMP NOT NULL DEFAULT CURRENT_TIMESTAMP,
    FOREIGN KEY (workflow_id, workflow_version) REFERENCES workflow_versions (workflow_id, version)
);

-- One row per step of a run. status follows the Step state machine
-- (internal/workflow.StepStatus): pending, running, succeeded, failed,
-- skipped, cancelled.
CREATE TABLE run_steps (
    run_id      TEXT NOT NULL,
    step_id     TEXT NOT NULL,
    status      TEXT NOT NULL,
    input       TEXT NOT NULL DEFAULT '{}',
    output      TEXT NOT NULL DEFAULT '{}',
    error       TEXT,
    started_at  TIMESTAMP,
    finished_at TIMESTAMP,
    PRIMARY KEY (run_id, step_id),
    FOREIGN KEY (run_id) REFERENCES runs (id)
);
