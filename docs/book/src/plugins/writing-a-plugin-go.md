# Writing a Plugin in Go

This walks through building a plugin with `sdk/go-plugin`, the official Go SDK. A plugin built this way depends only on this SDK and the public protocol (`api/plugin/v1`) — never on any `internal/` package of the agent (non-negotiable #4, `CLAUDE.md` section 1).

`patchcord plugin new <id>` (see [`patchcord plugin`](../cli/commands/plugin.md)) scaffolds the boilerplate below for you — a `main.go` with one example action and a `manifest.json` ready for `plugin pack`. This page walks through what it generates and why, useful whether you use the scaffold or write it by hand.

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

func (uppercaseAction) ID() string { return "text.uppercase@1" }

func (uppercaseAction) Run(_ context.Context, input patchcord.ActionInput, _ *patchcord.ConnectorConfig) (patchcord.ActionOutput, error) {
	value, ok := input["value"].(string)
	if !ok {
		return nil, fmt.Errorf("input %q must be a string", "value")
	}
	return patchcord.ActionOutput{"value": strings.ToUpper(value)}, nil
}
```

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

If your plugin's actions need a bound connector (e.g. a database connection), declare the connector type(s) they accept:

```go
plugin := patchcord.Plugin{
	// ...
	Connectors: []string{"postgresql.connection@1"},
}
```

This is declarative only — the SDK does not enforce that an action actually requires a connector; each action must check `connector == nil` itself. See [Connectors](connectors/index.md) for how a connector reaches your action.

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
go build -o bin/plugins/text ./plugins/examples/text   # or your own module path
patchcord plugin install bin/plugins/text
patchcord plugin inspect io.patchcord.example-text
```

`plugin install` launches the binary, performs the handshake, and records it in the catalog. It takes effect the next time `patchcord serve` (or `workflow run` / `connector test`) starts — see [CLI → plugin](../cli/commands/plugin.md).

## Reference

`plugins/examples/text/main.go` shows a plugin contributing five actions from one process, plus a demo connector type used only to validate binding end to end. `plugins/examples/postgresql/main.go` shows a real connector-consuming plugin implementing `ConnectorTester`.
