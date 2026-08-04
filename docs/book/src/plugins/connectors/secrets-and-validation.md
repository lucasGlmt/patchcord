# Secrets & Validation

## Reference types

```go
type Reference struct {
	Type string // "env", "keychain", or "file"
	Key  string
}
```

Three reference types are supported, each resolved by a different `secrets.Store` adapter, dispatched by `Type` through `secrets.MultiStore` ([ADR-0040](../../../../adr/0040-secret-providers-keychain-et-fichier-aes.md)):

- **`env`** — `EnvStore.Resolve` reads an OS environment variable named `Key` ([ADR-0020](../../../../adr/0020-modele-connecteur-references-secrets-env.md)). Only as isolated as the process/user running the agent — the weakest of the three, but requires nothing to set up.
- **`keychain`** — `KeychainStore.Resolve` reads `Key` from the OS's native secret store (macOS Keychain, Windows Credential Manager, Linux Secret Service), under a fixed service name (`patchcord`). Best suited to local-first use (a desktop agent); a headless Linux server or container typically has no Secret Service daemon, so `keychain` references usually fail to resolve there — use `file` instead.
- **`file`** — `FileStore.Resolve` decrypts an AES-256-GCM vault file (`<data-dir>/secrets.vault`) and looks up `Key`. Requires a master key, provisioned via `--secrets-master-key-file` / `PATCHCORD_SECRETS_MASTER_KEY_FILE`. This is the adapter meant for server/Docker deployments.

`keychain` and `file` values are written with `patchcord secret set` (see [Command Reference](../../cli/commands/secret.md)) — never through `connector create` itself, since one secret can be referenced by several connectors.

```bash
# provision the value once
printf '%s' "$PG_PASSWORD" | patchcord secret set --type file PG_PASSWORD \
  --secrets-master-key-file /path/to/key

# reference it from a connector
patchcord connector create my-postgres \
  --type postgresql.connection@1 \
  --config host=localhost \
  --secret password=file:PG_PASSWORD
```

## Validated at two different times, on purpose

- **`Reference.Type` is validated at `connector create` time** (`secrets.ValidateType`) — a typo like `emv:MY_VAR` is rejected immediately rather than silently accepted.
- **Whether the reference actually resolves is checked lazily**, at `connector inspect` — you can legitimately create a connector before exporting the environment variable it points to.

## How a bound connector reaches a plugin

When a workflow step's `connector:` field resolves to a connector ID, the runner calls `connectors.Resolve`, which loads the connector and resolves every `SecretRef` through the `secrets.Store`, producing a `ResolvedConnector{Type, Config, Secrets}`. This is:

- **Assembled fresh for one action call and never persisted.** It is not written back to the database.
- **Resolved on the step's own context** (bounded by `--step-timeout`), not a shared bookkeeping context — because a `Store` adapter may do real I/O (`FileStore` reads and decrypts a file, `KeychainStore` calls into the OS), and because it must respect the same cancellation as the action call itself ([ADR-0021](../../../../adr/0021-binding-connecteur-workflow-protocole.md)).
- **Passed to the plugin as `ConnectorConfig`** over the [protocol](../protocol.md) — encoded as `google.protobuf.Struct`, translated in `internal/plugins/execute.go`, never in `internal/connectors` itself (which stays free of any dependency on the plugin transport).

## The rule every connector-consuming action must follow

**Never put a resolved secret in an action's output.** A step's output is persisted in run history in the clear — echoing `ResolvedConnector.Secrets` back would defeat the entire point of never persisting a secret ([ADR-0009](../../../../adr/0009-secrets-jamais-dans-workflows.md)). See [Manifest & Actions](../manifest-and-actions.md#a-connector-consuming-action) for the reference pattern (`text.echo_connector@1`, which reports `type`/`config` but never `Secrets`).

## `connector:` must always be an expression

A workflow step never hardcodes a literal connector ID — `connector: "some-id"` is rejected at validation time. It must be an expression, most commonly `${{ bindings.<name> }}`, resolved at run time via `patchcord workflow run --binding name=connector-id`. This exists because a published workflow is immutable ([ADR-0008](../../../../adr/0008-workflows-publies-immuables.md)): a literal ID baked into the YAML would break portability between deployments (a connector named `my-postgres` on your machine has no reason to exist under that name elsewhere) — `bindings` is exactly the indirection that preserves it.
