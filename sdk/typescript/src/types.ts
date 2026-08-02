// Public types for the Patchcord agent's HTTP API (vision document, section
// 10.1/10.2). Field names are camelCase here — the wire format's snake_case
// JSON (as internal/api encodes it) is mapped in wire.ts, kept out of the
// public surface so a future protocol tweak doesn't leak into application
// code.

/** One state in a Run's explicit state machine (internal/workflow.RunStatus). */
export type RunStatus = "queued" | "running" | "succeeded" | "failed" | "cancelled";

/** One state in a Step's explicit state machine (internal/workflow.StepStatus). */
export type StepStatus = "pending" | "running" | "succeeded" | "failed" | "skipped" | "cancelled";

/** One step of a Run, as returned by GET /v1/runs/{id}. */
export interface RunStep {
  id: string;
  status: StepStatus;
  input?: Record<string, unknown>;
  output?: Record<string, unknown>;
  error?: string;
  startedAt?: string;
  finishedAt?: string;
}

/**
 * One execution of a workflow version, as returned by
 * POST /v1/workflows/{id}/run and GET /v1/runs/{id}. `steps` is only
 * present on the latter — the former responds as soon as the run is
 * created, before any step has run.
 */
export interface RunSummary {
  id: string;
  workflowId: string;
  workflowVersion: number;
  status: RunStatus;
  inputs?: Record<string, unknown>;
  outputs?: Record<string, unknown>;
  error?: string;
  createdAt: string;
  startedAt?: string;
  finishedAt?: string;
  steps?: RunStep[];
}

/**
 * One status change for a run or one of its steps, delivered by
 * GET /v1/runs/{id}/events (internal/runs.Event). `stepId` is absent for a
 * run-level event.
 */
export interface RunEvent {
  runId: string;
  stepId?: string;
  status: string;
  error?: string;
  time: string;
}

/** Options accepted by PatchcordClient.workflows.run. */
export interface RunWorkflowOptions {
  /** Values for the workflow's ${{ workflow.inputs.<key> }} expressions. */
  inputs?: Record<string, unknown>;
  /**
   * Maps a logical binding name (as referenced by a step's
   * ${{ bindings.<name> }} connector expression) to the id of the connector
   * to use.
   */
  bindings?: Record<string, string>;
}

/**
 * One installed workflow version, as returned by PatchcordClient.workflows.
 * list. The same workflow id can appear more than once — workflows are
 * immutable once published (ADR-0008), so installing a new version never
 * replaces an older one.
 */
export interface WorkflowSummary {
  id: string;
  version: number;
  installedAt: string;
}

/** Options accepted by PatchcordClient.runs.list. */
export interface ListRunsOptions {
  /** Restrict the result to runs of this workflow id. */
  workflowId?: string;
}

/** One installed application, as returned by PatchcordClient.apps.list. */
export interface AppSummary {
  id: string;
  version: string;
  /** Workflow ids a session for this app is permitted to run. */
  workflowsRun: string[];
}

/**
 * A newly issued application session, as returned by
 * PatchcordClient.apps.createSession — pass `token` as the `token` option to
 * a new PatchcordClient to act within this session's permissions.
 */
export interface AppSession {
  token: string;
  appId: string;
  workflowsRun: string[];
  issuedAt: string;
}

/** The agent's health, as returned by PatchcordClient.system.health. */
export interface HealthStatus {
  status: "ok" | "degraded";
  database: string;
}
