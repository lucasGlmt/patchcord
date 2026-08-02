# Secrets & Validation

## Reference types

```go
type Reference struct {
	Type string // only "env" is supported
	Key  string
}
```

`env` is the only supported reference type in this version ([ADR-0020](../../../../adr/0020-modele-connecteur-references-secrets-env.md)): `EnvStore.Resolve` reads an OS environment variable named `Key`. This was chosen because it needs no new crypto/keychain dependency and behaves identically local and on a server (non-negotiable #2) — but a caveat is explicit in the ADR: an environment variable is only as isolated as the process/user running the agent, well short of a real Keychain/Credential Manager/Vault. Treat it as a starting adapter, not the final answer; the [`secrets.Store`](../../../../../internal/secrets/secrets.go) interface exists precisely so a stronger adapter can be added later without changing callers.

```bash
patchcord connector create my-postgres \
  --type postgresql.connection@1 \
  --config host=localhost \
  --secret password=env:PG_PASSWORD
```

## Validated at two different times, on purpose

- **`Reference.Type` is validated at `connector create` time** (`secrets.ValidateType`) — a typo like `emv:MY_VAR` is rejected immediately rather than silently accepted.
- **Whether the reference actually resolves is checked lazily**, at `connector inspect` — you can legitimately create a connector before exporting the environment variable it points to.

## How a bound connector reaches a plugin

When a workflow step's `connector:` field resolves to a connector ID, the runner calls `connectors.Resolve`, which loads the connector and resolves every `SecretRef` through the `secrets.Store`, producing a `ResolvedConnector{Type, Config, Secrets}`. This is:

- **Assembled fresh for one action call and never persisted.** It is not written back to the database.
- **Resolved on the step's own context** (bounded by `--step-timeout`), not a shared bookkeeping context — because a future `Store` adapter (Vault, Keychain) may make a real network call, and because it must respect the same cancellation as the action call itself ([ADR-0021](../../../../adr/0021-binding-connecteur-workflow-protocole.md)).
- **Passed to the plugin as `ConnectorConfig`** over the [protocol](../protocol.md) — encoded as `google.protobuf.Struct`, translated in `internal/plugins/execute.go`, never in `internal/connectors` itself (which stays free of any dependency on the plugin transport).

## The rule every connector-consuming action must follow

**Never put a resolved secret in an action's output.** A step's output is persisted in run history in the clear — echoing `ResolvedConnector.Secrets` back would defeat the entire point of never persisting a secret ([ADR-0009](../../../../adr/0009-secrets-jamais-dans-workflows.md)). See [Manifest & Actions](../manifest-and-actions.md#a-connector-consuming-action) for the reference pattern (`text.echo_connector@1`, which reports `type`/`config` but never `Secrets`).

## `connector:` must always be an expression

A workflow step never hardcodes a literal connector ID — `connector: "some-id"` is rejected at validation time. It must be an expression, most commonly `${{ bindings.<name> }}`, resolved at run time via `patchcord workflow run --binding name=connector-id`. This exists because a published workflow is immutable ([ADR-0008](../../../../adr/0008-workflows-publies-immuables.md)): a literal ID baked into the YAML would break portability between deployments (a connector named `my-postgres` on your machine has no reason to exist under that name elsewhere) — `bindings` is exactly the indirection that preserves it.
