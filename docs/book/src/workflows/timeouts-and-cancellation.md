# Timeouts & Cancellation

## Step timeout

Each step's action call is bounded by `ExecuteOptions.StepTimeout`, defaulting to `runs.DefaultStepTimeout` (30s) when left zero ([ADR-0018](../../../adr/0018-timeouts-annulation-commandes-restantes.md)). `runs.Continue` (`internal/runs/runner.go`) derives a fresh `context.WithTimeout(ctx, opts.StepTimeout)` for every step — resolving its connector and calling its action both run under that same per-step context, cancelled again as soon as the step finishes so the timeout never leaks into the next one.

```bash
patchcord workflow run slow_workflow --step-timeout 5s
```

`--step-timeout` on `patchcord workflow run` is the only way to change it today; `POST /v1/workflows/{id}/run` always uses the default.

## What a timeout does to the run

A step that times out fails **that step**, not the run as a "user cancellation" — `stepFailureStatus` only reports `StepCancelled` when the error is `context.Canceled` (the run's own context, not a step's derived timeout context); anything else, including a step's own `context.DeadlineExceeded`, is `StepFailed`. Either way, the first step to fail stops the run: every step that never got a chance to run is recorded as `skipped` (or `cancelled`, see below) so no step is left dangling in `pending` — see [Runs](runs.md#step-states) for the state machine this respects.

## Cancelling a run

Two distinct things both called "cancellation" behave differently:

- **A run's context is cancelled while it is executing** — e.g. `Ctrl+C` on `patchcord workflow run`, which `signal.NotifyContext` turns into `ctx.Err() != nil`, checked between every step in `Continue`'s loop. This *is* treated as a user-requested cancellation: the run's final status is `RunCancelled`, and every step that hadn't started yet is marked `StepCancelled` (not `StepSkipped` — the distinction matters because a step failing on its own is a different situation than the whole run being asked to stop).
- **`patchcord run cancel <run-id>` / `POST /v1/runs/{id}/cancel`** (`runs.CancelRun`) — marks a run still `queued` or `running` as `cancelled` directly in the database, along with any of its steps not yet in a terminal state. Because a workflow executes synchronously within one process from start to finish, this **cannot interrupt** a run actively in progress in another process — it exists to clean up a run left behind `running` by a crashed process, or (for a run started over HTTP) to signal a run whose background goroutine hasn't reached its next persistence checkpoint yet. `ErrRunNotCancellable` is returned if the run has already reached a terminal state.

## Persistence writes are never bound to the cancelled context

Every bookkeeping write `Continue` makes — recording a step's transition to `running`, its final status, the run's own final status — uses a separate `persistTimeout`-bounded context (10s), deliberately **not** derived from the run's own `ctx`. Once a run needs to record that it failed or was cancelled, that write must still go through even though `ctx` is exactly what triggered the cancellation — otherwise a cancelled run would be left stuck as `running` in the database forever. Only the actual action call is bound to the caller's `ctx`; every write around it uses `persistTimeout`.
