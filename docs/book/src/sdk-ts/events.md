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
