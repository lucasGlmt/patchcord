# Runs

A run (`runs.Run`) is one execution instance of a specific workflow version. Its `Status` and each of its steps' `Status` (`runs.Step`) follow explicit state machines (`internal/workflow/state.go`) — every transition is validated before being persisted, and the same table-driven tests that guard the engine (`state_test.go`) cover both the valid and the invalid transitions.

## Run states

```text
queued ──▶ running ──▶ succeeded
              │    └──▶ failed
              └───────▶ cancelled
   └────────────────────▶ cancelled
```

| From | May move to |
|---|---|
| `queued` | `running`, `cancelled` |
| `running` | `succeeded`, `failed`, `cancelled` |
| `succeeded`, `failed`, `cancelled` | *(terminal — nothing)* |

`workflow.ValidateRunTransition(from, to)` returns an error for anything not in this table. `runs.updateRunStatus` calls it before every write, so a run's recorded status can never skip a state or move out of a terminal one.

## Step states

```text
pending ──▶ running ──▶ succeeded
               │    └──▶ failed
               └───────▶ cancelled
   └─────────────────────▶ skipped
   └─────────────────────▶ cancelled
```

| From | May move to |
|---|---|
| `pending` | `running`, `skipped`, `cancelled` |
| `running` | `succeeded`, `failed`, `cancelled` |
| `succeeded`, `failed`, `skipped`, `cancelled` | *(terminal — nothing)* |

`workflow.ValidateStepTransition` guards `runs.updateStepStatus` the same way. `skipped` is reserved for a step that never got a chance to run because an earlier step in the same run failed or the run was cancelled first (see [Timeouts & Cancellation](timeouts-and-cancellation.md)) — it is never reached from `running`.

## What gets persisted

`createRun` (`internal/runs/store.go`) inserts the `runs` row (`queued`) and one `run_steps` row per step of the workflow (`pending`) in a single transaction, before any step executes — so a run's full step list exists in the database from the start, not built up incrementally. As execution proceeds, each step's row records its resolved `input`, its `output` once it succeeds, and `error` if it didn't; the run's own row records `outputs` (its last step's output) and `error` once it reaches a terminal status. `started_at`/`finished_at` timestamps are set on the corresponding transitions, both on the run and on each step.

`patchcord run inspect <run-id>` and `patchcord run logs <run-id>` (`internal/cli/run.go`) both read this same persisted state — `inspect` as a snapshot, `logs` as a timestamped transcript ordered by each step's `started_at`/`finished_at`. `GET /v1/runs/{id}` (`internal/api/runs.go`) returns the identical shape as JSON.
