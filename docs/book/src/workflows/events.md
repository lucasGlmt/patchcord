# Events

```bash
curl -N http://127.0.0.1:7331/v1/runs/<run-id>/events
```

`GET /v1/runs/{id}/events` streams a run's status changes, and its steps', as Server-Sent Events until the run reaches a terminal status or the client disconnects. Each event's `event:` field is `run.<status>` or `step.<status>` (`runs.Event.Name()`); `data:` is a JSON object: `{"run_id", "step_id" (omitted for a run-level event), "status", "error" (if any), "time"}`.

## Polling, not a push from the runner

`runs.WatchRun` (`internal/runs/watch.go`) implements this by polling the database every `watchPollInterval` (250ms, a package variable so tests can shrink it) and diffing against the last status it saw for the run and for each step. This is deliberate, not a stopgap: `patchcord workflow run` executes a workflow synchronously within its own process from start to finish ([ADR-0018](../../../adr/0018-timeouts-annulation-commandes-restantes.md)), so there is no long-lived in-process event bus another process — such as the agent's HTTP server — could subscribe to. The database is the only channel shared between a run in progress and anyone watching it ([ADR-0019](../../../adr/0019-evenements-temps-reel-sse-par-scrutation.md)).

## What this means for consumers

- **Latency**: an event can be observed up to ~250ms after it actually happened — the poll interval, not a hard real-time guarantee.
- **No event log, no replay**: the database only ever holds each entity's *current* status, there is no append-only log of every transition it passed through. A client that connects after a fast run has already finished only ever observes each entity's single final status, not the intermediate ones — it never sees `step.running` if the step both started and finished between two polls. A client watching a run already in flight observes every transition from the moment it connects onward.
- **Starts from an empty baseline**: `WatchRun` always emits the status each entity *currently* holds, even for a run that was already `running` before the client connected — so a client joining mid-run is caught up immediately, not left waiting for the next change.
- **Closes automatically**: the channel (and the SSE stream) closes once the run reaches `succeeded`, `failed` or `cancelled`, or when the request context is cancelled (client disconnects).

The TypeScript SDK wraps this endpoint — see [SDK TypeScript: Events (SSE)](../sdk-ts/events.md) for the client-side API.
