# @glmtsolutions/patchcord-react

React bindings for [`@glmtsolutions/patchcord-sdk`](../typescript) — the TypeScript SDK for [Patchcord](https://github.com/lucasglmt/patchcord)'s public HTTP API.

This package exists for one recurring pattern: *run a workflow when the user clicks a button, and show its live progress properly (over SSE), without hand-writing the same state-merging plumbing in every app.* It wraps `Run.watch()` (the SDK's merged-snapshot event stream) into a `useWorkflowRun` hook.

## Install

```bash
npm install @glmtsolutions/patchcord-sdk @glmtsolutions/patchcord-react
```

`@glmtsolutions/patchcord-sdk` and `react` (>=18) are peer dependencies — this package doesn't pin either for you.

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
        <div key={step.id}>
          {step.id}: {step.status}
        </div>
      ))}

      {phase === "succeeded" && <pre>{JSON.stringify(outputs)}</pre>}
      {phase === "failed" && <p>{error}</p>}
    </div>
  );
}
```

That's the whole "run this workflow on a click" pattern — no manual `useState` for phase/steps/status, no hand-written reducer over `run.events()`, no forgotten cleanup on unmount.

## What the hook does and doesn't do

- **Does** call `client.workflows.run`, then drive React state from `Run.watch()`'s merged snapshots as they arrive.
- **Does** stop listening to a run's event stream when the component unmounts or `start()` is called again (via `AbortController`) — no dangling SSE connection.
- **Does not** cancel the underlying run when the component unmounts. A workflow is meant to keep executing on the agent independently of any one browser tab watching it; only an explicit `cancel()` call stops it (`Run.cancel()`).
- **Does not** render anything — no bundled `<RunWorkflowButton>` component. Rendering is the application's job (vision document: "applications provide the experience"); this package only removes the state-management boilerplate around it.

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

See `src/useWorkflowRun.ts` for the full doc comments on each field.

## Scope

One hook, one job. This package doesn't wrap the rest of the SDK's surface (connectors, plugins, apps, workflow listing) — those don't have the same "live state over SSE" ergonomics problem `useWorkflowRun` addresses, and are just as easy to call directly from `@glmtsolutions/patchcord-sdk`.
