# Writing a Plugin in Go

This walks through building a plugin with `sdk/go-plugin`, the official Go SDK. A plugin built this way depends only on this SDK and the public protocol (`api/plugin/v1`) — never on any `internal/` package of the agent (non-negotiable #4, `CLAUDE.md` section 1).

`patchcord plugin new <id>` (see [`patchcord plugin`](../cli/commands/plugin.md)) scaffolds the boilerplate below for you as a standalone Go module — `go.mod`, `main.go` with one example action, `manifest.json`, `README.md`, `Makefile`, and `.gitignore`. By default the Go module path is the plugin id; use `--module` when the plugin will live under a different git import path. This page walks through what it generates and why, useful whether you use the scaffold or write it by hand.

## 1. Implement one or more actions

An action implements the `patchcord.Action` interface — see [Manifest & Actions](manifest-and-actions.md) for the full contract:

```go
package main

import (
	"context"
	"fmt"
	"strings"

	patchcord "github.com/lucasglmt/patchcord/sdk/go-plugin"
)

type uppercaseAction struct{}

func (uppercaseAction) ID() string          { return "text.uppercase@1" }
func (uppercaseAction) Description() string { return "Converts a string to upper case." }

func (uppercaseAction) InputSchema() patchcord.Schema {
	return patchcord.Schema{
		"type":       "object",
		"properties": map[string]any{"value": map[string]any{"type": "string"}},
		"required":   []any{"value"},
	}
}

func (uppercaseAction) OutputSchema() patchcord.Schema {
	return patchcord.Schema{
		"type":       "object",
		"properties": map[string]any{"value": map[string]any{"type": "string"}},
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

`Description()`, `InputSchema()` and `OutputSchema()` are mandatory, not optional — see [Manifest & Actions](manifest-and-actions.md) for why ([ADR-0062](../../../adr/0062-descripteurs-schema-actions-et-connecteurs.md)).

## 2. Declare the plugin and serve it

```go
func main() {
	plugin := patchcord.Plugin{
		Manifest: patchcord.Manifest{
			ID:      "io.patchcord.example-text",
			Version: "1.0.0",
		},
		Actions: []patchcord.Action{
			uppercaseAction{},
		},
	}

	if err := patchcord.Serve(plugin); err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
}
```

`patchcord.Serve` starts the gRPC server on a local ephemeral port, registers the standard gRPC health service (reporting `SERVING` immediately — a minimal plugin has nothing else to report), prints the bootstrap ready line to stdout, and blocks. It implements `Handshake` and `ExecuteAction` for you from what `Plugin` declares — you never touch `api/plugin/v1` types directly.

## 3. Optional: contribute connector types

If your plugin's actions need a bound connector (e.g. a database connection), declare the connector type(s) they accept, each with a description and a JSON Schema for its non-secret configuration ([ADR-0062](../../../adr/0062-descripteurs-schema-actions-et-connecteurs.md)):

```go
plugin := patchcord.Plugin{
	// ...
	Connectors: []patchcord.Connector{
		{
			Type:        "postgresql.connection@1",
			Description: "A PostgreSQL server, with an optional password secret.",
			ConfigSchema: patchcord.Schema{
				"type":       "object",
				"properties": map[string]any{"host": map[string]any{"type": "string"}},
				"required":   []any{"host"},
			},
		},
	},
}
```

Never describe a secret field in `ConfigSchema` — a connector's secrets never appear in a schema meant to be shown or stored ([ADR-0009](../../../adr/0009-secrets-jamais-dans-workflows.md)). This is declarative only — the SDK does not enforce that an action actually requires a connector; each action must check `connector == nil` itself. See [Connectors](connectors/index.md) for how a connector reaches your action.

## 4. Optional: support `patchcord connector test`

Implement `ConnectorTester` and set `Plugin.Tester` to let `patchcord connector test` attempt a real connection through your plugin without running a full action:

```go
type pingTester struct{}

func (pingTester) TestConnector(ctx context.Context, connector patchcord.ConnectorConfig) error {
	// open a connection using connector.Config / connector.Secrets, then ping it
	return nil // nil = success; a returned error becomes Ok: false, Message: err.Error()
}
```

Never include a secret value in the returned error's message — it becomes the message the CLI user sees. Leaving `Tester` unset is fine: the plugin then reports `Unimplemented` for `TestConnector`, which the agent surfaces distinctly from a test that ran and failed.

## 5. Build and install it

```bash
patchcord plugin new io.patchcord.example-text --module github.com/acme/example-text
cd example-text
go mod tidy
go build -o binaries/$(go env GOOS)-$(go env GOARCH)/plugin .
patchcord plugin pack .
patchcord plugin install io.patchcord.example-text-0.1.0.patchcord-plugin
patchcord plugin inspect io.patchcord.example-text
```

`plugin pack` archives the directory using `manifest.json`; `plugin install` extracts the matching executable, launches it, performs the handshake, and records it in the catalog. It takes effect the next time `patchcord serve` (or `workflow run` / `connector test`) starts — see [CLI → plugin](../cli/commands/plugin.md).

`sdk/go-plugin` and `api/plugin` are independently versioned nested Go modules of the `patchcord_core` repository (ADR-0066), tagged separately from the core (`sdk/go-plugin/vX.Y.Z`, `api/plugin/vX.Y.Z`) — `go mod tidy` in your plugin only pulls in these two modules and their own dependencies, never the whole agent. Until their first tagged release, resolve them against a local checkout instead:

```bash
go mod edit -replace github.com/lucasglmt/patchcord/sdk/go-plugin=/path/to/patchcord_core/sdk/go-plugin
go mod edit -replace github.com/lucasglmt/patchcord/api/plugin=/path/to/patchcord_core/api/plugin
go mod tidy
```

## Reference

`plugins/examples/text/main.go` shows a plugin contributing five actions from one process, plus a demo connector type used only to validate binding end to end. `plugins/examples/postgresql/main.go` shows a real connector-consuming plugin implementing `ConnectorTester`.
