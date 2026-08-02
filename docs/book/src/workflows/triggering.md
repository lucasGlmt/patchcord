# Triggering Workflows

There are two ways to start a run of a workflow's latest installed version, and both end up calling the same functions in `internal/runs` — the CLI holds no logic the HTTP API doesn't also go through ([ADR-0005](../../../adr/0005-cli-et-api-meme-couche-applicative.md), non-negotiable #8).

## `patchcord workflow run` — blocking

```bash
patchcord workflow run greet_twice --input name=world --binding demo=demo_conn
```

`newWorkflowRunCommand` (`internal/cli/workflow.go`) launches and supervises the installed plugins for the duration of this one run (mirroring what `patchcord serve` does at startup — see [ADR-0017](../../../adr/0017-moteur-workflows-tranche-minimale.md)), then calls `runs.Execute`, which is `runs.Start` followed by `runs.Continue` — it blocks until the run reaches a terminal status and prints the result. A `SIGINT`/`SIGTERM` while it's running cancels the run's context, which the runner checks between steps and passes down to the in-flight action call: `Ctrl+C` marks the run (and its remaining steps) `cancelled` rather than killing the process mid-write.

## `POST /v1/workflows/{id}/run` — asynchronous

```bash
curl -X POST http://127.0.0.1:7331/v1/workflows/hello_patchcord/run \
  -H 'Content-Type: application/json' \
  -d '{"inputs": {}, "bindings": {}}'
```

`handleRunWorkflow` (`internal/api/workflows.go`) calls `runs.Start` synchronously — fast, it only persists the run's creation — then runs `runs.Continue` in a background goroutine and responds `202 Accepted` immediately with the run's id and initial status, without waiting for any step to execute ([ADR-0024](../../../adr/0024-declenchement-asynchrone-workflows-api-http.md)). The background goroutine is bound to the agent's own long-lived context, not the HTTP request's — the run must keep going after the response has already been sent and the request is gone. A client watches [`GET /v1/runs/{id}/events`](events.md) or polls `GET /v1/runs/{id}` for progress and the final result.

## Same engine either way

Both paths call `runs.Start`/`runs.Continue` (`internal/runs/runner.go`) against an `ActionExecutor` — `internal/plugins.Supervisor` in both cases, just launched differently (the CLI starts one for the run's duration; the agent's `Supervisor` is already running as part of `patchcord serve`). Neither path bypasses workflow validation, the `Run`/`Step` state machines, or persistence — there is no separate "HTTP execution mode."
