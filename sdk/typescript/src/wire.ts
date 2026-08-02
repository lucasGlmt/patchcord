// Wire-format (snake_case JSON, as internal/api/runs.go and events.go
// encode it) counterparts of the public types in types.ts, plus the
// mapping functions between them. Kept separate so a transport-shape
// change (a renamed JSON field) touches only this file.

import type { RunEvent, RunStatus, RunStep, RunSummary, StepStatus } from "./types.js";

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
