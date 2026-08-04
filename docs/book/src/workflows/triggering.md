# Triggering Workflows

Every way to start a run of a workflow's latest installed version — attended or not — ends up calling the same functions in `internal/runs` (`runs.Start`/`runs.Continue`, or the `runs.Execute` shorthand for both together) — the CLI holds no logic the HTTP API doesn't also go through ([ADR-0005](../../../adr/0005-cli-et-api-meme-couche-applicative.md), non-negotiable #8), and neither does an unattended trigger. Two are attended, called directly by whoever wants a run right now:

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

Two more are unattended — nobody calls them "right now"; they fire on their own, given the right trigger declared in the workflow's own `trigger:` field (see [Workflow Format](format.md)):

## `schedule` trigger — cron cadence

`internal/scheduler.Runner` polls for workflows whose latest installed version declares `trigger.type: schedule` and fires each due one — see [Schedule trigger](format.md#schedule-trigger) for the full cron/`on_missed` mechanics ([ADR-0035](../../../adr/0035-trigger-schedule-scheduler-persistant.md)).

## `webhook` trigger — inbound HTTP request

```bash
curl -X POST http://127.0.0.1:7331/v1/webhooks/webhook_demo \
  -H "X-Patchcord-Webhook-Token: s3cr3t" \
  -d '{"name": "world"}'
```

`handleWebhookTrigger` (`internal/api/webhooks.go`) fires a workflow declaring `trigger.type: webhook` when a request presents the right shared secret — see [Webhook trigger](format.md#webhook-trigger) for the secret verification and input-mapping details ([ADR-0037](../../../adr/0037-trigger-webhook-secret-partage.md)). Unlike the other three paths, `POST /v1/webhooks/{id}` is never gated by an admin token ([ADR-0036](../../../adr/0036-authentification-admin-jetons-opt-in.md)) — its own per-workflow secret is the credential.

## Same engine every way

All four paths call `runs.Start`/`runs.Continue` (`internal/runs/runner.go`) against an `ActionExecutor` — `internal/plugins.Supervisor` in every case, just launched differently (the CLI starts one for the run's duration; the agent's `Supervisor` is already running as part of `patchcord serve`, which is what both unattended triggers run inside). None of them bypasses workflow validation, the `Run`/`Step` state machines, or persistence — there is no separate "unattended execution mode," only a different caller.
