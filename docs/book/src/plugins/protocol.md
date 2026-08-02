# Protocol

The plugin protocol is gRPC + Protobuf, defined in [`api/plugin/v1/plugin.proto`](../../../../api/plugin/v1/plugin.proto) and versioned as a public contract ([ADR-0013](../../../adr/0013-protocole-greffons-grpc-protobuf.md)). It is one of the three boundaries `CLAUDE.md` (non-negotiable #5) requires to stay independent of Go — a plugin written in Rust, TypeScript, or Python only needs to generate stubs from this `.proto` and speak it correctly.

## Transport

A launched plugin listens on a local TCP port (`127.0.0.1:0`, ephemeral — not a Unix socket, so behavior is identical on macOS, Linux, and Windows) and prints a single JSON line to its own stdout once ready:

```json
{"address":"127.0.0.1:54321"}
```

The agent reads this line, dials the address, and only then proceeds to the handshake. This bootstrap pattern (subprocess exposing local gRPC, discovered via a stdout line) mirrors what HashiCorp's go-plugin, Terraform, and Vault do for the same process-to-process scenario.

## `PluginService`

Three RPCs, added incrementally as the protocol grew:

| RPC | Added by | Purpose |
|---|---|---|
| `Handshake` | [ADR-0013](../../../adr/0013-protocole-greffons-grpc-protobuf.md) | Negotiate protocol version, discover the plugin's manifest. |
| `ExecuteAction` | [ADR-0014](../../../adr/0014-executeaction-struct-generique.md) | Run one contributed action. |
| `TestConnector` | [ADR-0023](../../../adr/0023-protocole-test-connecteur.md) | Attempt a real connection using a connector's resolved config, without running an action. |

## Handshake

```protobuf
rpc Handshake(HandshakeRequest) returns (HandshakeResponse);
```

The agent sends the highest protocol version it supports (`CurrentProtocolVersion`, currently `1`). The plugin replies with the version it will speak for the rest of the session (must not exceed the agent's), its `plugin_id` and `plugin_version`, the actions and connector types it contributes (`Contributions`), and its declared permissions. The agent rejects the plugin if the negotiated version is `0` (no compatible version) or if `plugin_id`/`plugin_version` is missing. This is what `internal/plugins.Handshake` validates, tested against a mocked transport rather than a real process (`handshake_test.go`).

## `ExecuteAction`

```protobuf
rpc ExecuteAction(ExecuteActionRequest) returns (ExecuteActionResponse);
```

Input and output are both `google.protobuf.Struct` — a generic, JSON-like type — rather than a message typed per action ([ADR-0014](../../../adr/0014-executeaction-struct-generique.md)). This means there is no protocol-level type safety on an action's input: a mismatch (e.g. passing a number where `text.uppercase@1` expects a string) only fails at run time. `ExecuteActionRequest` also carries an optional `ConnectorConfig` — the resolved config and secrets of whatever connector the calling workflow step bound, or absent if none was bound. See [Manifest & Actions](manifest-and-actions.md).

## `TestConnector`

```protobuf
rpc TestConnector(TestConnectorRequest) returns (TestConnectorResponse);
```

`TestConnectorResponse{ok, message}` distinguishes a test that ran and failed (`ok: false`, a legitimate result) from a test that could not run at all — the latter is a gRPC error, in particular `codes.Unimplemented` if the plugin doesn't support connector testing. See [Connectors → Testing a Connector](connectors/testing.md).

## Numeric caveat

`Struct` cannot distinguish `int64` from `float64`: every number crossing the protocol arrives as `float64`. Every example plugin that reads a numeric config value (e.g. `postgresql`'s `port`) accounts for this explicitly.
