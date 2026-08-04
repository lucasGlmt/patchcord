# Manifest & Actions

A plugin has two distinct manifests, serving different moments in its lifecycle:

- **The handshake manifest** is what a running plugin process returns from the [handshake](protocol.md#handshake): its ID, version, contributed actions, contributed connector types, and permissions. It is a live RPC response, computed from what a `patchcord.Plugin` (Go SDK) declares in code — the source of truth once the process is actually launched.
- **The package manifest** (`manifest.json`, only present for a plugin distributed as a `.patchcord-plugin` package) is a static, declarative file: ID, version, protocol version, declared permissions, and one executable path per supported platform. It exists so the agent can show permissions and pick the right platform executable *before* ever launching the process ([ADR-0042](../../../adr/0042-formats-de-package-plugin-workflow-bundle.md)). A plugin installed from a raw executable path has no package manifest at all — see [`patchcord plugin`](../cli/commands/plugin.md).

Both describe the same plugin, but neither replaces the other: the package manifest gets you to a safe launch, the handshake manifest tells you what actually got launched.

## What an action declares

In this version, an action is deliberately minimal ([ADR-0014](../../../adr/0014-executeaction-struct-generique.md)): a stable, versioned identifier, and a `Run` function taking generic input/output maps. There is no typed schema, no declared capability list, no declared timeout, and no declared list of known errors yet — the full model sketched in the vision document (section 7.4) belongs to the workflow compiler (phase 3+) and hasn't been built. Don't document a richer contract than what actually exists.

```go
type Action interface {
    ID() string
    Run(ctx context.Context, input ActionInput, connector *ConnectorConfig) (ActionOutput, error)
}
```

- **`ID()`** returns the action's identifier, by convention `<name>.<subtype>@<version>` (e.g. `text.uppercase@1`, `postgresql.query@1`). The version suffix exists so a breaking change to an action ships as a new identifier, not a silent behavior change under the same name.
- **`Run`** receives `input` (a `map[string]any`, decoded from the workflow step's resolved inputs) and `connector` (non-nil only if the calling step bound one — see [Connectors](connectors/index.md)). It returns `output` (a `map[string]any`, encoded back across the protocol as `google.protobuf.Struct`) or an error.

## Minimal reference example

`text.uppercase@1` ([`plugins/examples/text/main.go`](../../../../plugins/examples/text/main.go)) is the reference example from the vision document (section 20):

```go
type uppercaseAction struct{}

func (uppercaseAction) ID() string { return "text.uppercase@1" }

func (uppercaseAction) Run(_ context.Context, input patchcord.ActionInput, _ *patchcord.ConnectorConfig) (patchcord.ActionOutput, error) {
	value, ok := input["value"].(string)
	if !ok {
		return nil, fmt.Errorf("input %q must be a string", "value")
	}
	return patchcord.ActionOutput{"value": strings.ToUpper(value)}, nil
}
```

Note the manual type assertion on `input["value"]` — there is no schema validating this before `Run` is called, so an action must check its own inputs and return a clear error on mismatch.

## Encoding caveats

Since input/output cross the wire as `google.protobuf.Struct`:

- A list must be built as `[]any`, not a concrete slice type like `[]string` — `text.split@1` boxes each string individually for exactly this reason.
- Every number arrives and leaves as `float64` — there is no `int64` on the wire.

## A connector-consuming action

An action that requires a connector receives it as its third parameter, never nil-checked away — it must reject a missing connector explicitly:

```go
func (queryAction) Run(ctx context.Context, input patchcord.ActionInput, connector *patchcord.ConnectorConfig) (patchcord.ActionOutput, error) {
	if connector == nil {
		return nil, fmt.Errorf("action %q requires a bound connector", "postgresql.query@1")
	}
	// ...
}
```

**Rule, not just a convention**: an action must never put a resolved secret (`connector.Secrets`) into its output. A step's output is persisted in run history in the clear ([ADR-0009](../../../adr/0009-secrets-jamais-dans-workflows.md), [ADR-0021](../../../adr/0021-binding-connecteur-workflow-protocole.md)) — echoing a secret back would defeat the point of never persisting one. `text.echo_connector@1` demonstrates the correct pattern: it reports `connector.Type` and `connector.Config`, never `connector.Secrets`.
