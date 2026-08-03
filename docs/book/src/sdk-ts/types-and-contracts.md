# Types & Contracts

## Source of truth

The agent generates a Swagger 2.0 spec from its handlers' annotations (`swag init`, [ADR-0025](../../../adr/0025-documentation-openapi-swaggo.md)), committed at `api/agent/swagger.json`/`.yaml` and served live at `GET /v1/openapi.json`. That spec — not this SDK — is the contractual definition of the agent's public HTTP API (non-negotiable #5, `CLAUDE.md` section 1). `sdk/typescript` is a hand-written client kept manually in sync with it; there is no code generation step yet ([ADR-0025](../../../adr/0025-documentation-openapi-swaggo.md) explicitly scopes an `openapi-generator`-style client out — the SDK stays hand-written for now).

## Keeping the SDK in sync

When `internal/api` gains a new route or changes a response shape:

1. The Go handler's `@Router`/`@Success`/`@Param` annotations change, and `make swagger` regenerates `api/agent/swagger.json`/`.yaml`.
2. `sdk/typescript/src/wire.ts` gets (or updates) a `Wire*` interface matching the new snake_case JSON shape, plus a `*FromWire` mapping function.
3. `sdk/typescript/src/types.ts` gets (or updates) the corresponding camelCase public type.
4. `sdk/typescript/src/client.ts` gets (or updates) the method that calls the route, under whichever namespace (`system`/`workflows`/`runs`/`apps`) matches the resource.
5. `sdk/typescript/test/client.test.ts` gets a table-driven-style test against a real local `http.createServer` fake — not a mocked `fetch` — mirroring the Go side's "mock the transport, not the network" testing philosophy (`CLAUDE.md` section 5).

A route that exists in `internal/api` but has no SDK method yet is a gap, not a design choice — see [SDK TypeScript Overview](index.md#what-it-covers) for the current coverage table. The reverse must never happen: no SDK method should call a route that doesn't exist in `internal/api`, since `api/agent/swagger.json` and the Go router are what's authoritative, not the vision document's aspirational surface.

## Why wire types are a separate layer

`RunSummary`, `WorkflowSummary`, `AppSummary`, `AppSession`, `Connector`, and `PluginSummary` all have `Wire*` counterparts in `wire.ts` with snake_case fields (`workflow_id`, `installed_at`, `workflows_run`, `secret_refs`, ...) matching `internal/api`'s JSON encoding exactly. Application code never sees these — only `src/types.ts`'s camelCase types, re-exported from `src/index.ts`. This means a JSON field rename inside a Go handler is a one-file fix (`wire.ts`), not a breaking change application code has to react to, as long as the public type in `types.ts` keeps the same shape.

## Versioning

The agent's public HTTP API has no version segment in its paths (`/v1/...` is the only version marker, and it hasn't moved). `@patchcord/sdk` itself is versioned independently in `sdk/typescript/package.json` (currently `0.1.0`, pre-1.0 — no stability guarantee yet). Until the SDK reaches 1.0, treat a minor version bump as potentially containing a breaking type or method-signature change; there is no separate changelog process yet beyond the package's own commit history.
