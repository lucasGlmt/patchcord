# Events (SSE)

`Run.events()` streams a run's status changes, and its steps', as they happen — the client-side counterpart of [Workflows → Events](../workflows/events.md).

```ts
const run = await client.workflows.run("hello_patchcord", { inputs: { text: "hi" } });

for await (const event of run.events()) {
  const label = event.stepId ? `step ${event.stepId}` : "run";
  console.log(`[${event.time}] ${label} → ${event.status}${event.error ? ` (${event.error})` : ""}`);
}

console.log("stream closed — run reached a terminal status");
```

`events()` is an `AsyncGenerator<RunEvent>`, backed by `GET /v1/runs/{id}/events`. Each `RunEvent` is:

```ts
interface RunEvent {
  runId: string;
  stepId?: string;   // absent for a run-level event
  status: string;
  error?: string;
  time: string;
}
```

The stream closes on its own once the run reaches a terminal status (`succeeded`, `failed`, or `cancelled`) — draining it with `for await` is a correct way to wait for completion, which is exactly what `Run.result()` does internally.

## Reconnecting

`src/sse.ts` implements a minimal `text/event-stream` parser — only `event:`/`data:` fields framed by a blank line, matching exactly what `internal/api/events.go` emits. It does **not** implement the full SSE reconnection protocol (`id:`/`retry:`), because the server side doesn't need it: a client connecting after a run already finished still receives each entity's *current* (final) status once, not a replay of every status it passed through. If a stream is interrupted, simply call `.events()` again (or `.fetch()` if you only need the current snapshot, not the transitions).

## Result vs live events

Use `run.result()` when you only care about the final outcome — it drains `events()` internally, then does one extra `GET /v1/runs/{id}` for the authoritative summary (events only ever carry a status string, never a run's inputs/outputs). Use `run.events()` directly when you want to react to intermediate step transitions (e.g. updating a progress UI) rather than just waiting.

## Watching merged state

`events()` yields raw deltas — one status change at a time — which means any UI that wants to render "where is this run right now" has to reduce that stream into a picture itself: keep a map of step id → status, update the run's own status separately, and handle the final `fetch()` for outputs once the stream closes. `Run.watch()` does exactly that reduction internally and yields the result directly:

```ts
const run = await client.workflows.run("hello_patchcord", { inputs: { text: "hi" } });

for await (const snapshot of run.watch()) {
  console.log(snapshot.status, snapshot.steps);
}
// the loop above ends once the run reaches a terminal status
```

Each `RunSnapshot` is the full current picture, not a delta:

```ts
interface RunSnapshot {
  status: RunStatus;
  error?: string;
  steps: RunStep[]; // every step watch() has seen an event for so far
  outputs?: Record<string, unknown>; // the run's own outputs
}
```

Two things worth knowing:

- `steps` only contains steps `events()` has reported a status change for — `watch()` has no way to know a workflow's full step list ahead of time (that's `WorkflowDetail.steps`, fetched separately via `client.workflows.get`). A UI that wants to render not-yet-started steps as "pending" seeds that list itself before iterating.
- The **last** snapshot `watch()` yields is always the re-fetched, authoritative one (same extra `GET /v1/runs/{id}` `result()` makes) — so it's the only one with `outputs` and each step's `input`/`output` populated, since `events()` itself never carries those.

Pass `{ signal }` (an `AbortSignal`) to stop listening early — e.g. a UI component unmounting mid-run — without waiting for the run to finish:

```ts
const controller = new AbortController();
for await (const snapshot of run.watch({ signal: controller.signal })) {
  /* ... */
}
// later: controller.abort()
```

This only tears down the client's own SSE connection; the run keeps executing on the agent regardless — call `run.cancel()` to actually stop it. `events()` accepts the same option.

For a ready-made React binding built on top of `watch()` — a `useWorkflowRun` hook that turns "run this workflow on a button click, with live progress" into a few lines — see [`@glmtsolutions/patchcord-react`](https://github.com/lucasglmt/patchcord/tree/main/sdk/typescript-react).
