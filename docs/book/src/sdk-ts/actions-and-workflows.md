# Actions & Workflows

There is no `client.actions` namespace — the agent has no `/v1/actions` endpoint yet (actions are only invoked indirectly, as steps of a workflow run). This page covers `client.workflows` and `client.runs`, the client-side counterpart of [Workflows → Triggering](../workflows/triggering.md) and [Workflows → Runs](../workflows/runs.md).

## Listing installed workflows

```ts
const workflows = await client.workflows.list();
// [{ id: "hello_patchcord", version: 1, installedAt: "2026-08-01T12:00:00Z" }, ...]
```

Backed by `GET /v1/workflows`. The same workflow id can appear more than once: workflows are immutable once published ([ADR-0008](../../../adr/0008-workflows-publies-immuables.md)), so installing a new version never replaces an older one — both stay listed.

## Triggering a run

```ts
const run = await client.workflows.run("hello_patchcord", {
  inputs: { text: "hello" },              // ${{ workflow.inputs.text }}
  bindings: { db: "prod-postgres" },      // ${{ bindings.db }} → connector id
});
```

Both `inputs` and `bindings` are optional and default to `{}`. This always runs the **latest installed version** of the workflow — there is no way to pin an older version from this call. The returned `Run` reflects the run's status at creation time (`queued` or `running`); see [Core Concepts](concepts.md#run-a-handle-not-a-snapshot) for why you still need `.fetch()`/`.result()`/`.events()` to observe it.

## Listing and inspecting runs

```ts
const allRuns = await client.runs.list();
const thisWorkflowsRuns = await client.runs.list({ workflowId: "hello_patchcord" });

const one = await client.runs.get("01HZ...");
const summary = await one.fetch(); // full detail, including steps
```

`runs.list` is backed by `GET /v1/runs`, most recently created first; the optional `workflowId` maps to the `?workflow_id=` query parameter. `runs.get` is backed by `GET /v1/runs/{id}` and 404s (thrown as an `Error`) if the id doesn't exist.

## Cancelling a run

```ts
await client.runs.cancel("01HZ...");
// or, equivalently, from a Run you already hold:
await run.cancel();
```

Both are backed by `POST /v1/runs/{id}/cancel`. This marks a `queued` or `running` run as `cancelled` — it does **not** interrupt a step actively executing within the agent's own process; an in-flight step stops at its next persistence checkpoint (see [Timeouts & Cancellation](../workflows/timeouts-and-cancellation.md)). Cancelling a run already in a terminal status (`succeeded`, `failed`, `cancelled`) throws — the agent responds `409 Conflict`.
