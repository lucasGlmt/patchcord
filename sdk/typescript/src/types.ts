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

/** One step within a WorkflowDetail. */
export interface WorkflowStepDetail {
  id: string;
  uses: string;
  with?: Record<string, unknown>;
  connector?: string;
  /**
   * The logical binding name `connector` refers to (the "db" in
   * `"${{ bindings.db }}"`), present only when `connector` is exactly that
   * shape. Absent for a connector expression over `workflow.inputs` or
   * `steps.*.outputs` — those compute a connector id dynamically, so there's
   * no static binding to offer a picker for ahead of a run.
   */
  bindingName?: string;
  /**
   * The connector type inferred for bindingName — see WorkflowBindingDetail.
   * Absent when it couldn't be inferred (no installed plugin contributes
   * this step's action, or that plugin declares more than one connector
   * type).
   */
  connectorType?: string;
}

/**
 * One connector binding a WorkflowDetail declares across its steps,
 * deduplicated by name — the same design as WorkflowInputDetail: a small
 * typed array a client iterates to render one form control per binding (a
 * `<select>` of connectors of `connectorType`) instead of a free-form JSON
 * blob.
 */
export interface WorkflowBindingDetail {
  name: string;
  /** Absent when it could not be inferred, or when different steps referring to this name disagree. */
  connectorType?: string;
}

/** The type a WorkflowInputDetail declares — see internal/workflow/inputs.go. */
export type WorkflowInputType = "string" | "number" | "boolean" | "enum";

/**
 * One declared input within a WorkflowDetail — enough to render a typed
 * form field (a text input, a number input, a switch, a select) instead of
 * a free-form JSON blob. See docs/book/src/workflows/format.md#declared-inputs.
 */
export interface WorkflowInputDetail {
  name: string;
  type: WorkflowInputType;
  required: boolean;
  description?: string;
  default?: unknown;
  /** The values a "enum"-typed input may take. Only set when type is "enum". */
  enum?: string[];
}

/**
 * One workflow version's full definition, as returned by
 * PatchcordClient.workflows.get — unlike WorkflowSummary, this carries the
 * parsed steps needed to render the workflow's structure, plus its raw
 * YAML source (the same text `patchcord workflow export` prints).
 */
export interface WorkflowDetail {
  id: string;
  version: number;
  schemaVersion: number;
  triggerType: string;
  /**
   * A standard 5-field cron expression, present only when triggerType is
   * "schedule" — see docs/book/src/workflows/format.md#schedule-trigger.
   */
  triggerCron?: string;
  /**
   * The effective missed-occurrence policy ("skip" or "fire_once"), present
   * only when triggerType is "schedule" — always resolved, never empty even
   * when the workflow's YAML left on_missed implicit.
   */
  triggerOnMissed?: string;
  /**
   * This workflow's next scheduled firing (ISO 8601), present only when
   * triggerType is "schedule". Reflects the live schedules table, not the
   * definition alone, so it stays accurate even when viewing an older
   * installed version.
   */
  nextRunAt?: string;
  /** The workflow's declared input schema. Empty when it declares none. */
  inputs: WorkflowInputDetail[];
  steps: WorkflowStepDetail[];
  /** Every distinct connector binding name referenced by steps. Empty when the workflow uses none. */
  bindings: WorkflowBindingDetail[];
  source: string;
}

/** Options accepted by PatchcordClient.workflows.get. */
export interface GetWorkflowOptions {
  /** Workflow version to fetch. Defaults to the latest installed one. */
  version?: number;
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

/**
 * A logical reference to a secret's value — never the value itself
 * (ADR-0009, ADR-0020, ADR-0021). Only type "env" is supported today.
 */
export interface ConnectorSecretRef {
  type: string;
  key: string;
}

/**
 * One connector: a persistent, named configuration for accessing an
 * external system (vision document, section 7.3), as returned by
 * PatchcordClient.connectors.list/get/create. Never carries a resolved
 * secret value — only each secret reference's logical type/key.
 */
export interface Connector {
  id: string;
  type: string;
  config: Record<string, unknown>;
  secretRefs: Record<string, ConnectorSecretRef>;
  createdAt: string;
}

/** Options accepted by PatchcordClient.connectors.create. */
export interface CreateConnectorOptions {
  id: string;
  type: string;
  config?: Record<string, unknown>;
  secretRefs?: Record<string, ConnectorSecretRef>;
}

/** The outcome of PatchcordClient.connectors.test — a connection attempt that ran but failed reports ok=false, not a thrown error. */
export interface ConnectorTestResult {
  ok: boolean;
  message?: string;
}

/**
 * One installed plugin, as returned by PatchcordClient.plugins.list — the
 * connector types and action ids its manifest declares, enough for a client
 * to build a connector-type picker.
 */
export interface PluginSummary {
  id: string;
  version: string;
  connectors: string[];
  actions: string[];
}
