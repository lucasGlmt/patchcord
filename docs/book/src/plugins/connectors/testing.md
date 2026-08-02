# Testing a Connector

`patchcord connector test <id>` proves a connector's credentials actually work, as opposed to `connector inspect`, which only proves its secret references *resolve*. It calls into a live plugin process, so — like `workflow run` — it launches and supervises the catalog's plugins for the duration of this one command ([ADR-0023](../../../../adr/0023-protocole-test-connecteur.md)).

## The `TestConnector` RPC

Third RPC on `PluginService`, after `Handshake` and `ExecuteAction` — see [Protocol](../protocol.md#testconnector). The agent resolves the connector (same mechanism as an action call), routes the request to whichever running plugin currently declares the connector's type (same routing logic `ExecuteAction` uses for action IDs), and reports the result.

## Three distinct outcomes

| Outcome | What it means | Command exit code |
|---|---|---|
| `OK` | The plugin's `ConnectorTester` ran and succeeded. | 0 |
| `FAILED: <message>` | The plugin's `ConnectorTester` ran and returned an error (wrong password, host unreachable, ...) — a legitimate result. | 0 |
| Command error | No installed plugin declares the connector's type, or the plugin that does hasn't implemented `ConnectorTester` (`codes.Unimplemented`), or the connector ID doesn't exist. | non-zero |

A test that ran and failed is never a command error — the same distinction `workflow run` makes between a run that finishes `failed` and the CLI command itself failing.

## Implementing `ConnectorTester`

Optional on the plugin side:

```go
type ConnectorTester interface {
	TestConnector(ctx context.Context, connector ConnectorConfig) error
}
```

`nil` error means success; any returned error becomes `TestConnectorResponse{Ok: false, Message: err.Error()}`. A plugin that doesn't set `Plugin.Tester` responds `Unimplemented` automatically — adding `TestConnector` support later is additive, not a breaking SDK change (unlike the `Action.Run` signature change in [ADR-0021](../../../../adr/0021-binding-connecteur-workflow-protocole.md), which did require every existing plugin to be updated).

`postgresql` and `mysql` implement it by opening a connection and calling `PingContext` — no query is run, so it works whether or not any table exists. `http` and `openai` do not implement it in this version: a generic "test" for an HTTP connection has no obvious semantics (`HEAD`? `GET`? which path?) without a concrete use case, so it was deliberately left unbuilt rather than guessed at.

## What it doesn't do

- No dedicated timeout for `connector test` — it inherits the Supervisor's default behavior, same as `workflow run` before per-step timeouts existed. A plugin whose test hangs (e.g. a TCP connect that never resolves) blocks the command for just as long.
- Never include a secret value in the message returned from `TestConnector` — it becomes exactly what the CLI prints after `FAILED:`.
