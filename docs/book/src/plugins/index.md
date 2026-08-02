# Plugins Overview

A plugin is an independent process, launched and supervised by the agent, that communicates over gRPC. It is never loaded into the core's memory as a native Go `.so` plugin ([ADR-0002](../../../adr/0002-greffons-processus-externes.md)) — this is why a plugin crash never brings the agent down, why a plugin can be written in any language that can speak the protocol, and why the core never imports a concrete business integration (non-negotiable #3, `CLAUDE.md` section 1).

A plugin contributes:

- **Actions** — atomic operations, e.g. `text.uppercase@1` (see [Manifest & Actions](manifest-and-actions.md)).
- **Connectors** — types it can be bound to, e.g. `postgresql.connection@1` (see [Connectors](connectors/index.md)).

## Example plugins

`plugins/examples/` contains the reference plugins used to develop and test the protocol:

| Plugin | Contributes | Notes |
|---|---|---|
| `text` | `text.uppercase@1`, `text.lowercase@1`, `text.join@1`, `text.split@1`, `text.echo_connector@1` | The minimal reference plugin (vision document section 20). No real connector — `demo.connection@1` exists only to validate connector binding end to end. |
| `encoding`, `json`, `time` | Various stateless actions | No connector. |
| `http` | HTTP request actions | Connector: `http.connection@1`. No `ConnectorTester`. |
| `openai` | AI actions | Connector: `openai.connection@1`. No `ConnectorTester`. |
| `postgresql` | `postgresql.query@1`, `postgresql.execute@1` | Connector: `postgresql.connection@1`. Implements `ConnectorTester` (opens a connection and pings it). |
| `mysql` | Same shape as `postgresql` | Connector: `mysql.connection@1`. Implements `ConnectorTester`. |

See [Example Plugins](example-plugins.md) for every action's inputs/outputs and a runnable workflow snippet for each.

## Where to go next

- [Concepts](concepts.md) — process isolation and the plugin/action/connector distinction.
- [Protocol](protocol.md) — the gRPC/Protobuf contract and the handshake.
- [Manifest & Actions](manifest-and-actions.md) — how a plugin declares what it contributes.
- [Writing a Plugin in Go](writing-a-plugin-go.md) — build one with `sdk/go-plugin`.
- [Example Plugins](example-plugins.md) — a full action reference and usage example for each example plugin.
- [Supervision & Lifecycle](supervision.md) — health checks, restarts, quarantine.
- [Connectors](connectors/index.md) — the connector model plugins expose.
