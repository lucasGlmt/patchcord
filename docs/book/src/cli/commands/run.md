# patchcord run

Inspect and manage workflow runs recorded by the agent — whether started via `patchcord workflow run` or triggered through the HTTP API. See [Workflows → Runs](../../workflows/runs.md) for the run state machine.

## `list`

```bash
patchcord run list
patchcord run list --workflow hello_patchcord
```

Prints one line per run: `<run-id>  <workflow-id> v<version>  <status>  <created-at>`. `--workflow` filters to one workflow's runs.

## `inspect <run-id>`

```bash
patchcord run inspect 01HZ...
```

Prints the run's workflow, status, error (if any), inputs, outputs, and the status of every step.

## `logs <run-id>`

```bash
patchcord run logs 01HZ...
```

Prints a timestamped transcript: run creation, each step's start/finish (with its error if it failed), and the run's final status.

## `cancel <run-id>`

```bash
patchcord run cancel 01HZ...
```

Marks a run stuck in `queued` or `running` as `cancelled`. `patchcord workflow run` executes synchronously within its own process, so this **cannot interrupt a run actively in progress elsewhere** — it exists to clean up a run left behind by a crashed process, not to stop a live one. Fails if the run doesn't exist or has already finished.
