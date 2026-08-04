# Example Walkthrough

This walks through `apps/examples/greeter`, the minimal reference application — a single static `index.html`, no build step, permitted to run exactly one workflow.

## 1. Install the workflow it runs

`greeter` calls `hello_patchcord`, so that workflow must be installed first:

```bash
patchcord workflow install workflows/examples/hello_patchcord.yaml
```

## 2. Install the application

```bash
patchcord app install apps/examples/greeter
```

`apps.Install` reads `apps/examples/greeter/patchcord-app.yaml`:

```yaml
id: greeter
version: "0.1.0"
permissions:
  workflows:
    run:
      - hello_patchcord
```

and records `static_dir` as that directory's absolute path — `index.html` sits right next to the manifest, no separate build output to point at.

## 3. Serve and open it

```bash
patchcord serve
```

then open `http://127.0.0.1:7331/apps/greeter/` — `handleServeApp` resolves `greeter` and serves `index.html` from its `static_dir` (see [Hosting & Sessions](hosting-and-sessions.md#static-hosting)).

## 4. Trace one click through the permission

Clicking "Say hello" runs `sayHello()` (`apps/examples/greeter/index.html`'s inline script):

1. `POST /v1/apps/greeter/sessions` — no credential needed to ask on a fresh agent (no admin token created yet, [ADR-0036](../../../adr/0036-authentification-admin-jetons-opt-in.md)); the response's `token` carries exactly `{"workflows_run": ["hello_patchcord"]}`, read straight from the manifest installed in step 2.
2. `POST /v1/workflows/hello_patchcord/run`, with `Authorization: Bearer <token>`. `withRunAuth` validates the token as a session, then `Session.CanRunWorkflow("hello_patchcord")` checks it against the session's permissions — `true`, so the request reaches `handleRunWorkflow` and a run starts.
3. The page polls `GET /v1/runs/{id}` every 150ms until the run leaves `queued`/`running`, then reads `run.outputs.value` — `text.uppercase@1`'s output for the input `"Welcome Patchcord"` — into the page.

## 5. See the permission actually enforced

Try running a workflow `greeter`'s manifest does *not* list, using the same session token:

```bash
TOKEN=$(curl -s -X POST http://127.0.0.1:7331/v1/apps/greeter/sessions | jq -r .token)
curl -i -X POST http://127.0.0.1:7331/v1/workflows/greet_twice/run \
  -H "Authorization: Bearer $TOKEN" -d '{}'
# HTTP/1.1 403 Forbidden
# app "greeter" is not permitted to run workflow "greet_twice"
```

That `403` is `Session.CanRunWorkflow` rejecting the request — the same check step 4 passed for `hello_patchcord`, now failing for a workflow outside the manifest's `permissions.workflows.run`. This is the entire session-limited hosting model (vision document, §15.4) demonstrated end to end, with nothing else in the application beyond a manifest and a static page.
