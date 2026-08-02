# Core Concepts

## One client, four namespaces

A `PatchcordClient` groups its methods by the resource they act on, matching the vocabulary in the [Introduction](../introduction.md#vocabulary):

- `client.system` — agent-level status, not tied to any resource (`health()`).
- `client.workflows` — read-only listing plus the one write operation applications actually need: triggering a run (`list()`, `run(workflowId, options)`).
- `client.runs` — everything about a [run](../workflows/runs.md), the instance of a workflow's execution (`list(options)`, `get(runId)`, `cancel(runId)`).
- `client.apps` — installed [applications](../apps/index.md) and the limited sessions they issue for themselves (`list()`, `createSession(appId)`).

Every method that produces or fetches a run — `workflows.run`, `runs.get`, `runs.list`, `runs.cancel` — returns a `Run` (or `Run[]`), not a plain object. This keeps a single type as the entry point for everything you can do with a run, instead of forcing you to pass ids back into `client.runs.*` by hand.

## `Run`: a handle, not a snapshot

A `Run` instance wraps one run's *current known* summary plus the operations that can change or refresh it:

```ts
class Run {
  readonly id: string;
  events(): AsyncGenerator<RunEvent>;  // stream status changes until terminal
  result(): Promise<RunSummary>;       // drain events(), then fetch()
  fetch(): Promise<RunSummary>;        // GET /v1/runs/{id} right now
  cancel(): Promise<RunSummary>;       // POST /v1/runs/{id}/cancel
}
```

The summary a `Run` was constructed with (e.g. right after `workflows.run`) is a snapshot at creation time — it does not update itself as the run progresses. Call `.fetch()` (or drain `.events()`, or call `.result()`) to get a current one. `client.runs.list()` returns `Run` handles whose initial snapshot has no `steps` populated (the list endpoint omits them) — call `.fetch()` on one to get its full detail.

## Wire types vs public types

Every type in `src/types.ts` is camelCase; everything the agent actually sends over HTTP is snake_case JSON, as `internal/api`'s handlers encode it. `src/wire.ts` holds the snake_case `Wire*` interfaces and one `*FromWire` mapping function per type. This split exists so a JSON field rename inside `internal/api` (an implementation detail) never forces application code to change — only `wire.ts` would. See [Types & Contracts](types-and-contracts.md) for how this stays true across a contract version bump.

Application code should only ever import from `src/types.ts` (re-exported by `src/index.ts`) — never from `src/wire.ts`, which is not part of the package's public exports.
