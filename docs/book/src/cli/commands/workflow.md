# patchcord workflow

Manage workflow definitions. See [Workflows](../../workflows/index.md) for the format and the immutability rule ([ADR-0008](../../../../adr/0008-workflows-publies-immuables.md)).

## `install <path.yaml>`

```bash
patchcord workflow install ./workflows/hello.yaml
```

Validates the workflow (its steps against every action currently contributed by an installed plugin) and publishes it as a new version. Prints `Installed <id> version <n> (<steps> step(s))`.

## `list`

```bash
patchcord workflow list
```

Prints one line per installed version: `<workflow-id>  v<version>  <installed-at>`.

## `validate <path.yaml>`

```bash
patchcord workflow validate ./workflows/hello.yaml
```

Runs the same checks as `install` but never publishes anything — useful before committing a workflow file.

## `export <workflow-id>`

```bash
patchcord workflow export hello_patchcord --version 2
```

Prints a workflow version's YAML source as-is to stdout. `--version` defaults to the latest. `-o/--output <path>` writes it to a file instead — conventionally named `<id>-v<version>.patchcord-workflow`, a pure naming convention with no archive format behind it: a workflow package is exactly this declarative YAML (vision document, section 9.3). See [ADR-0042](../../../../adr/0042-formats-de-package-plugin-workflow-bundle.md).

## `run <workflow-id>`

```bash
patchcord workflow run hello_patchcord \
  --input name=Ada \
  --binding ai_provider=my-openai-connector \
  --step-timeout 30s
```

Runs the latest installed version of a workflow synchronously in this process. This is one of the two one-shot commands that needs live plugins (see [CLI Overview](../index.md#a-one-shot-command-does-not-talk-to-a-running-agent)): it launches and supervises the catalog's plugins for the duration of this one run, independent of any `patchcord serve` that may also be running.

- `--input key=value` (repeatable) sets a workflow input.
- `--binding name=connector-id` (repeatable) resolves a `${{ bindings.<name> }}` expression used by a step's `connector:` field — see [ADR-0021](../../../../adr/0021-binding-connecteur-workflow-protocole.md).
- `--step-timeout` bounds each individual step's action call (default: `runs.DefaultStepTimeout`).
- `--secrets-master-key-file` is needed if any bound connector has a `file`-typed secret reference (see [Configuration](../configuration.md)).
- `Ctrl+C` cancels the run: remaining steps are marked `cancelled` rather than the process being killed mid-write.

Prints the run ID, final status, error (if any), and outputs. A run that finishes `failed` is not a command error — `workflow run` exits 0 either way, the same distinction `connector test` makes between a failed test and a failed command.
