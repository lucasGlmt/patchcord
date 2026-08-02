# Serving the Agent

```bash
patchcord serve --listen 127.0.0.1:7331 --data-dir ./data
```

`serve` is the only long-running command. On startup it, in order:

1. Opens and migrates the SQLite database at `--data-dir`.
2. Binds `--listen` (default `127.0.0.1:7331`).
3. Launches and starts supervising every plugin recorded in the catalog (see [Plugins → Supervision & Lifecycle](../plugins/supervision.md)). A plugin that fails to launch is logged and skipped — it never prevents the agent from starting.
4. Starts serving the local HTTP API (`internal/api.NewRouter`) on the bound address.

It then blocks, logging structured output to stdout, until it receives `SIGINT` or `SIGTERM`.

## Shutdown

On signal, `serve` shuts down in this order, bounded by a 10-second timeout:

1. Cancels the base context any HTTP-triggered workflow run derives from — an in-flight background run is recorded `cancelled` rather than left running against plugins that are about to disappear.
2. Gracefully shuts down the HTTP server (lets in-flight requests finish, refuses new ones).
3. Stops the plugin supervisor (terminates every supervised plugin process).
4. Closes the database.

There is no forced-kill fallback beyond the shutdown timeout in this version — a handler or a plugin shutdown that hangs past it can delay process exit.

## Flags

| Flag | Default | Meaning |
|---|---|---|
| `--listen` | `127.0.0.1:7331` | Address the HTTP API binds to. |
| `--data-dir` | `./data` | Directory holding the SQLite database (created if missing). |

## What's not here yet

TLS, a config file, and authentication beyond the session mechanism used by [Apps](../apps/index.md) are not part of this phase — see the roadmap in `CLAUDE.md` section 9 (phase 6, "server deployment").
