# Manifest & Actions

A plugin has two distinct manifests, serving different moments in its lifecycle:

- **The handshake manifest** is what a running plugin process returns from the [handshake](protocol.md#handshake): its ID, version, contributed actions, contributed connector types, and permissions. It is a live RPC response, computed from what a `patchcord.Plugin` (Go SDK) declares in code — the source of truth once the process is actually launched.
- **The package manifest** (`manifest.json`, only present for a plugin distributed as a `.patchcord-plugin` package) is a static, declarative file: ID, version, protocol version, declared permissions, and one executable path per supported platform. It exists so the agent can show permissions and pick the right platform executable *before* ever launching the process ([ADR-0042](../../../adr/0042-formats-de-package-plugin-workflow-bundle.md)). A plugin installed from a raw executable path has no package manifest at all — see [`patchcord plugin`](../cli/commands/plugin.md).

Both describe the same plugin, but neither replaces the other: the package manifest gets you to a safe launch, the handshake manifest tells you what actually got launched.

## What an action declares

As of [ADR-0062](../../../adr/0062-descripteurs-schema-actions-et-connecteurs.md), an action declares its full shape — closing the gap with what the vision document (section 7.4) specified from the start: a stable, versioned identifier, a human-readable description, JSON Schema for its input and output, and a `Run` function. Declared capabilities, known errors, and test-mode behavior from that same section still aren't built — don't document a richer contract than what actually exists.

```go
type Action interface {
    ID() string
    Description() string
    InputSchema() Schema
    OutputSchema() Schema
    Run(ctx context.Context, input ActionInput, connector *ConnectorConfig) (ActionOutput, error)
}
```

- **`ID()`** returns the action's identifier, by convention `<name>.<subtype>@<version>` (e.g. `text.uppercase@1`, `postgresql.query@1`). The version suffix exists so a breaking change to an action ships as a new identifier, not a silent behavior change under the same name.
- **`Description()`** is one human-readable sentence: what the action does. Shown by the CLI/dashboard and readable by a coding agent building a workflow — never left empty.
- **`InputSchema()`/`OutputSchema()`** return a `Schema` (`map[string]any`, a JSON Schema document) describing `Run`'s input and output. They cross the protocol as a `google.protobuf.Struct`, the same encoding the values themselves use at execution time — see [Protocol](protocol.md#contributions).
- **`Run`** receives `input` (a `map[string]any`, decoded from the workflow step's resolved inputs) and `connector` (non-nil only if the calling step bound one — see [Connectors](connectors/index.md)). It returns `output` (a `map[string]any`, encoded back across the protocol as `google.protobuf.Struct`) or an error.

Declaring a schema does not make the protocol type-safe: nothing on the wire validates a step's `input:` against `InputSchema()` before `Run` is called yet (that's a workflow-compiler feature, still to come) — an action must still check its own inputs and return a clear error on mismatch. The schema exists today for discovery and documentation, human or agentic, not yet for enforcement.

## Minimal reference example

`text.uppercase@1` ([`plugins/examples/text/main.go`](../../../../plugins/examples/text/main.go)) is the reference example from the vision document (section 20):

```go
type uppercaseAction struct{}

func (uppercaseAction) ID() string          { return "text.uppercase@1" }
func (uppercaseAction) Description() string { return "Converts a string to upper case." }

func (uppercaseAction) InputSchema() patchcord.Schema {
	return patchcord.Schema{
		"type":       "object",
		"properties": map[string]any{"value": map[string]any{"type": "string", "description": "The string to convert."}},
		"required":   []any{"value"},
	}
}

func (uppercaseAction) OutputSchema() patchcord.Schema {
	return patchcord.Schema{
		"type":       "object",
		"properties": map[string]any{"value": map[string]any{"type": "string", "description": "value, converted to upper case."}},
		"required":   []any{"value"},
	}
}

func (uppercaseAction) Run(_ context.Context, input patchcord.ActionInput, _ *patchcord.ConnectorConfig) (patchcord.ActionOutput, error) {
	value, ok := input["value"].(string)
	if !ok {
		return nil, fmt.Errorf("input %q must be a string", "value")
	}
	return patchcord.ActionOutput{"value": strings.ToUpper(value)}, nil
}
```

Note the manual type assertion on `input["value"]` inside `Run` — `InputSchema()` documents the contract, it does not enforce it before `Run` is called, so an action must still check its own inputs and return a clear error on mismatch.

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
