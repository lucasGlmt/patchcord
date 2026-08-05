# CLI Overview

`patchcord` is a single Go binary built with [Cobra](https://github.com/spf13/cobra) ([ADR-0012](../../../adr/0012-cobra-comme-framework-cli.md)). It has two distinct modes:

- **`patchcord serve`** starts the long-running agent: HTTP API, plugin supervisor, and everything else described in [Serving the Agent](serve.md).
- **Every other command** (`plugin`, `connector`, `workflow`, `run`, `app`) is a one-shot operation that opens the agent's SQLite database directly and calls the same internal service functions the HTTP API uses — never a duplicated implementation ([ADR-0005](../../../adr/0005-cli-et-api-meme-couche-applicative.md)).

## Command groups

| Command | Manages |
|---|---|
| [`plugin`](commands/plugin.md) | The plugin catalog: install, list, inspect, uninstall. |
| [`connector`](commands/connector.md) | Connectors: create, list, inspect, test, remove. |
| [`workflow`](commands/workflow.md) | Workflow versions: install, list, validate, export, run. |
| [`run`](commands/run.md) | Workflow runs: list, inspect, logs, cancel. |
| [`app`](commands/app.md) | Installed applications: install, list, remove. |
| [`serve`](serve.md) | Starts the agent. |

## A one-shot command does not talk to a running agent

Commands other than `serve` do not connect to a `patchcord serve` process over HTTP — they open the same SQLite file directly (see [ADR-0015](../../../adr/0015-catalogue-greffons-effet-au-redemarrage.md)). Two consequences worth knowing before you use them:

- Installing or uninstalling a plugin, or installing a workflow, only takes effect for a running agent the next time it restarts — there is no hot reload yet.
- `workflow run` and `connector test` are the two exceptions among one-shot commands: they actually need a live plugin process, so each of them launches and supervises the catalog's plugins for the duration of that single command, then tears them down. This is separate from, and does not affect, an already-running `patchcord serve`.

Every one-shot command accepts `--data-dir` (default: a per-user system directory, [ADR-0052](../../../adr/0052-defaut-data-dir-dossier-standard-du-systeme.md)) to point at a different database explicitly — see [Configuration](configuration.md).
