# SDK TypeScript Overview

`sdk/typescript` (published as `@glmtsolutions/patchcord-sdk`) is the official TypeScript client for the agent's public HTTP API. It is the only supported way for an [application](../apps/index.md) to talk to the agent from JavaScript/TypeScript — never hand-roll `fetch` calls against `/v1/*` in application code (see [Building an App with the TS SDK](../apps/building-with-sdk-ts.md)).

## Package layout

| File | Responsibility |
|---|---|
| `src/client.ts` | `PatchcordClient` — one instance per agent connection, exposing `system`, `workflows`, `runs`, `apps`, `connectors`, and `plugins` namespaces. |
| `src/run.ts` | `Run` — a handle on one workflow execution, returned by every method that produces or fetches a run. |
| `src/sse.ts` | A minimal `text/event-stream` parser used by `Run.events()`. Not `EventSource`-based — see the file's own header comment for why. |
| `src/types.ts` | The public, camelCase types (`RunSummary`, `WorkflowSummary`, `AppSession`, ...). This is the surface application code imports. |
| `src/wire.ts` | The snake_case JSON shapes the agent actually sends, plus the mapping functions into `types.ts`. Kept separate so a wire-format tweak in `internal/api` never leaks into application code — see [Types & Contracts](types-and-contracts.md). |
| `src/index.ts` | The package's public exports. |

## What it covers

The SDK wraps every `/v1/*` route the agent implements today — no more, no less:

| Namespace | Backed by |
|---|---|
| `client.system.health()` | `GET /v1/system/health` |
| `client.workflows.list()`, `client.workflows.get(...)`, `client.workflows.run(...)` | `GET /v1/workflows`, `GET /v1/workflows/{id}`, `POST /v1/workflows/{id}/run` |
| `client.runs.list(...)`, `client.runs.get(...)`, `client.runs.cancel(...)` | `GET /v1/runs`, `GET /v1/runs/{id}`, `POST /v1/runs/{id}/cancel` |
| `client.apps.list()`, `client.apps.createSession(...)` | `GET /v1/apps`, `POST /v1/apps/{id}/sessions` |
| `client.connectors.list()`, `.get(...)`, `.create(...)`, `.delete(...)`, `.test(...)` | `GET /v1/connectors`, `GET/DELETE /v1/connectors/{id}`, `POST /v1/connectors`, `POST /v1/connectors/{id}/test` ([ADR-0034](../../../adr/0034-connecteurs-catalogue-greffons-http-bindings-dashboard.md)) |
| `client.plugins.list()` | `GET /v1/plugins` |

The vision document's fuller `client.*` surface (section 10.2) also describes `actions`, `files`, `notifications`, and `storage` namespaces. None of those have a server-side HTTP implementation yet (`internal/api/doc.go` is explicit about this), so the SDK does not expose them either — adding a client method ahead of its endpoint would let application code compile against something that doesn't exist at runtime.

## Where to go next

- [Getting Started](getting-started.md) — install the package and make your first call.
- [Core Concepts](concepts.md) — how the SDK's types map to Patchcord's vocabulary.
- [Actions & Workflows](actions-and-workflows.md) — listing and triggering workflows, listing/inspecting/cancelling runs.
- [Events (SSE)](events.md) — observing a run live.
- [Types & Contracts](types-and-contracts.md) — how the SDK stays in sync with the agent's OpenAPI contract.
- [React Adapter](react-adapter.md) — `@glmtsolutions/patchcord-react`'s `useWorkflowRun` hook, for "run this workflow on a click" without hand-writing the state management around it.
