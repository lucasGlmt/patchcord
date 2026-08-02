# Concepts

## A published version is never edited

`runs.InstallWorkflow` (`internal/runs/runs.go`) records a workflow as a new row in `workflow_versions`, keyed by `(workflow_id, version)`. There is no `update`: installing the same `(id, version)` pair a second time fails — the row already exists — and there is no operation that mutates an installed version's `definition` in place ([ADR-0008](../../../adr/0008-workflows-publies-immuables.md)).

"Publishing a new version" means changing `version` in the YAML and installing again:

```bash
patchcord workflow install greet_twice.yaml   # version: 3, already installed
# edit greet_twice.yaml, bump version: 4
patchcord workflow install greet_twice.yaml   # version: 4, a new row
```

`patchcord workflow list` then shows both `greet_twice v3` and `greet_twice v4` — installing a version never removes an older one.

## Why immutability matters here

A `Run` records the exact `workflow_id` **and** `workflow_version` it executed (`runs.Run.WorkflowVersion`). Because a version's `definition` can never change after the fact, a run's recorded version is a permanent, faithful description of what actually ran — re-reading `workflow_versions` for that `(id, version)` a year later still returns exactly the YAML that produced the run's steps and outputs. This is also why a step's `connector:` field must be an indirection (`${{ bindings.<name> }}`), never a literal connector id — see [Workflow Format](format.md#connector-binding) and [ADR-0021](../../../adr/0021-binding-connecteur-workflow-protocole.md).

## Running vs. the latest version

Two operations pick a specific version explicitly or default to the latest:

- `runs.LatestWorkflow` — the highest installed `version` for an id. This is what `patchcord workflow run` and `POST /v1/workflows/{id}/run` use: **triggering a workflow always runs its latest installed version**, there is no way to run an older one.
- `runs.WorkflowSource` — returns one version's raw YAML; `--version 0` (the default) also means "latest" here, but any explicit version works, e.g. `patchcord workflow export greet_twice --version 3`.
