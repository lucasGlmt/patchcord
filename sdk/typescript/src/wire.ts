// Wire-format (snake_case JSON, as internal/api/runs.go and events.go
// encode it) counterparts of the public types in types.ts, plus the
// mapping functions between them. Kept separate so a transport-shape
// change (a renamed JSON field) touches only this file.

import type {
  AppSession,
  AppSummary,
  HealthStatus,
  RunEvent,
  RunStatus,
  RunStep,
  RunSummary,
  StepStatus,
  WorkflowDetail,
  WorkflowInputDetail,
  WorkflowInputType,
  WorkflowStepDetail,
  WorkflowSummary,
} from "./types.js";

export interface WireRunStep {
  id: string;
  status: StepStatus;
  input?: Record<string, unknown>;
  output?: Record<string, unknown>;
  error?: string;
  started_at?: string;
  finished_at?: string;
}

export interface WireRunSummary {
  id: string;
  workflow_id: string;
  workflow_version: number;
  status: RunStatus;
  inputs?: Record<string, unknown>;
  outputs?: Record<string, unknown>;
  error?: string;
  created_at: string;
  started_at?: string;
  finished_at?: string;
  steps?: WireRunStep[];
}

export interface WireRunEvent {
  run_id: string;
  step_id?: string;
  status: string;
  error?: string;
  time: string;
}

export function runStepFromWire(wire: WireRunStep): RunStep {
  return {
    id: wire.id,
    status: wire.status,
    input: wire.input,
    output: wire.output,
    error: wire.error,
    startedAt: wire.started_at,
    finishedAt: wire.finished_at,
  };
}

export function runSummaryFromWire(wire: WireRunSummary): RunSummary {
  return {
    id: wire.id,
    workflowId: wire.workflow_id,
    workflowVersion: wire.workflow_version,
    status: wire.status,
    inputs: wire.inputs,
    outputs: wire.outputs,
    error: wire.error,
    createdAt: wire.created_at,
    startedAt: wire.started_at,
    finishedAt: wire.finished_at,
    steps: wire.steps?.map(runStepFromWire),
  };
}

export function runEventFromWire(wire: WireRunEvent): RunEvent {
  return {
    runId: wire.run_id,
    stepId: wire.step_id,
    status: wire.status,
    error: wire.error,
    time: wire.time,
  };
}

export interface WireWorkflowSummary {
  id: string;
  version: number;
  installed_at: string;
}

export function workflowSummaryFromWire(wire: WireWorkflowSummary): WorkflowSummary {
  return {
    id: wire.id,
    version: wire.version,
    installedAt: wire.installed_at,
  };
}

export interface WireWorkflowStepDetail {
  id: string;
  uses: string;
  with?: Record<string, unknown>;
  connector?: string;
}

export interface WireWorkflowInputDetail {
  name: string;
  type: WorkflowInputType;
  required: boolean;
  description?: string;
  default?: unknown;
  enum?: string[];
}

export interface WireWorkflowDetail {
  id: string;
  version: number;
  schema_version: number;
  trigger_type: string;
  inputs?: WireWorkflowInputDetail[];
  steps: WireWorkflowStepDetail[];
  source: string;
}

export function workflowStepDetailFromWire(wire: WireWorkflowStepDetail): WorkflowStepDetail {
  return {
    id: wire.id,
    uses: wire.uses,
    with: wire.with,
    connector: wire.connector,
  };
}

export function workflowInputDetailFromWire(wire: WireWorkflowInputDetail): WorkflowInputDetail {
  return {
    name: wire.name,
    type: wire.type,
    required: wire.required,
    description: wire.description,
    default: wire.default,
    enum: wire.enum,
  };
}

export function workflowDetailFromWire(wire: WireWorkflowDetail): WorkflowDetail {
  return {
    id: wire.id,
    version: wire.version,
    schemaVersion: wire.schema_version,
    triggerType: wire.trigger_type,
    inputs: (wire.inputs ?? []).map(workflowInputDetailFromWire),
    steps: wire.steps.map(workflowStepDetailFromWire),
    source: wire.source,
  };
}

export interface WireAppSummary {
  id: string;
  version: string;
  workflows_run: string[];
}

export function appSummaryFromWire(wire: WireAppSummary): AppSummary {
  return {
    id: wire.id,
    version: wire.version,
    workflowsRun: wire.workflows_run,
  };
}

export interface WireAppSession {
  token: string;
  app_id: string;
  workflows_run: string[];
  issued_at: string;
}

export function appSessionFromWire(wire: WireAppSession): AppSession {
  return {
    token: wire.token,
    appId: wire.app_id,
    workflowsRun: wire.workflows_run,
    issuedAt: wire.issued_at,
  };
}

export interface WireHealthStatus {
  status: "ok" | "degraded";
  database: string;
}

export function healthStatusFromWire(wire: WireHealthStatus): HealthStatus {
  return { status: wire.status, database: wire.database };
}
