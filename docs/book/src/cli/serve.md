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
| `--listen` | `127.0.0.1:7331` | Address the HTTP API binds to. Also settable via `PATCHCORD_LISTEN` or a `--config` file's `listen` key — see [Configuration](configuration.md). |
| `--data-dir` | `./data` | Directory holding the SQLite database (created if missing). Also settable via `PATCHCORD_DATA_DIR` or a `--config` file's `data_dir` key. |
| `--config` | (none) | Path to a YAML file providing `listen`/`data_dir` — the lowest-precedence source; a flag or environment variable always overrides it. See [Configuration](configuration.md). |

## Admin authentication

The public API answers every request unauthenticated until an admin token exists — see [`patchcord auth`](commands/auth.md) and [ADR-0036](../../../adr/0036-authentification-admin-jetons-opt-in.md). Create one before binding `--listen` to anything beyond `127.0.0.1`:

```bash
patchcord auth token create ci --data-dir ./data
patchcord serve --listen 0.0.0.0:7331 --data-dir ./data
```

## What's not here yet

TLS termination and Docker packaging are not part of this phase yet — see the roadmap in `CLAUDE.md` section 9 (phase 6, "server deployment").
