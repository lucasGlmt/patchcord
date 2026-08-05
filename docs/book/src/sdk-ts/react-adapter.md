# React Adapter

`sdk/typescript-react` (published as `@glmtsolutions/patchcord-react`) is an optional companion to the SDK — a `useWorkflowRun` React hook built on top of [`Run.watch()`](events.md#watching-merged-state). It exists for one recurring pattern: run a workflow when the user clicks a button, and show its live progress (over SSE) without hand-writing the same state-merging plumbing in every application. See [ADR-0060](../../../adr/0060-run-watch-et-package-react-du-sdk.md) for why this lives in its own package rather than inside `sdk/typescript` itself — the short version: `sdk/typescript` stays free of any framework dependency, since not every Patchcord application is React.

## Install

```bash
npm install @glmtsolutions/patchcord-sdk @glmtsolutions/patchcord-react
```

`@glmtsolutions/patchcord-sdk` and `react` (>=18) are peer dependencies.

## Usage

```tsx
import { PatchcordClient } from "@glmtsolutions/patchcord-sdk";
import { useWorkflowRun } from "@glmtsolutions/patchcord-react";

const client = new PatchcordClient({ baseUrl: "http://127.0.0.1:7331" });

function RunButton() {
  const { phase, steps, error, outputs, start, cancel } = useWorkflowRun(client, "hello_patchcord");

  return (
    <div>
      <button onClick={() => start({ inputs: { text: "hi" } })} disabled={phase === "running"}>
        Run
      </button>
      {phase === "running" && <button onClick={cancel}>Cancel</button>}

      {steps.map((step) => (
        <div key={step.id}>{step.id}: {step.status}</div>
      ))}

      {phase === "succeeded" && <pre>{JSON.stringify(outputs)}</pre>}
      {phase === "failed" && <p>{error}</p>}
    </div>
  );
}
```

## API

```ts
function useWorkflowRun(client: PatchcordClient, workflowId: string): {
  phase: "idle" | "running" | "succeeded" | "failed" | "cancelled";
  run: Run | undefined;
  steps: RunStep[];
  error: string | undefined;
  outputs: Record<string, unknown> | undefined;
  start: (options?: RunWorkflowOptions) => void;
  cancel: () => void;
  reset: () => void;
};
```

- `phase` starts at `"idle"` and moves to `"running"` on `start()`, then to one of `"succeeded"`/`"failed"`/`"cancelled"` once the run reaches that terminal status.
- `steps` mirrors `RunSnapshot.steps` — every step the run's event stream has reported a status change for. It does not include steps that haven't started yet; seed a "pending" placeholder list yourself from `client.workflows.get(workflowId).steps` if the UI needs to show those.
- `start()` and `reset()` both discard the previous run's outcome — `start()` immediately begins a new one, `reset()` goes back to `"idle"` without starting one (e.g. for a "run again" button that returns to an input form).
- `cancel()` calls the tracked run's `Run.cancel()`. It is a no-op if no run is in flight.

## Lifecycle guarantees

- Calling `start()` again before a previous run finished, or unmounting the component mid-run, both stop that previous run's event stream from writing into state any further (an internal generation counter plus `AbortController` — see `src/useWorkflowRun.ts`).
- Unmounting does **not** cancel the underlying run — a workflow is meant to keep executing on the agent independently of whichever browser tab happened to be watching it. Call `cancel()` explicitly if that's the intent before unmounting.

## What this package does not do

It renders nothing — there is no bundled `<RunWorkflowButton>` component. Drawing the UI is the application's job ([vision document](../../../PATCHCORD_VISION_ARCHITECTURE.md): "applications provide the experience"); this hook only removes the state-management boilerplate around it. It also only wraps the one "run + watch" pattern — the rest of the SDK's surface (connectors, plugins, apps, workflow listing) is just as easy to call directly from `@glmtsolutions/patchcord-sdk` and doesn't have the same live-state ergonomics problem.
