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
| `--secrets-master-key-file` | (none) | Path to the file holding the base64 AES-256 master key for the `file` secret store. Also settable via `PATCHCORD_SECRETS_MASTER_KEY_FILE` or a `--config` file's `secrets_master_key_file` key. Left unset, `file` secret references don't resolve. See [Configuration](configuration.md) and [ADR-0040](../../../adr/0040-secret-providers-keychain-et-fichier-aes.md). |
| `--config` | (none) | Path to a YAML file providing `listen`/`data_dir`/`secrets_master_key_file` — the lowest-precedence source; a flag or environment variable always overrides it. See [Configuration](configuration.md). |

## Admin authentication

The public API answers every request unauthenticated until an admin token exists — see [`patchcord auth`](commands/auth.md) and [ADR-0036](../../../adr/0036-authentification-admin-jetons-opt-in.md). Create one before binding `--listen` to anything beyond `127.0.0.1`:

```bash
patchcord auth token create ci --data-dir ./data
patchcord serve --listen 0.0.0.0:7331 --data-dir ./data
```

## Docker

```bash
docker compose up --build
```

`Dockerfile` (repo root) is a multi-stage build: `CGO_ENABLED=0` against `golang:1.25` (`modernc.org/sqlite` is pure Go, no libc needed at runtime), copied into `gcr.io/distroless/static-debian12` — no shell, no package manager, ~19 MB (`docker images` after a build). `docker-compose.yml` mirrors the vision document's own example (section 13.3): binds `./data` and `./bin/plugins` into the container, publishes `7331`.

The image bakes in `/etc/patchcord/config.yaml` (`listen: 0.0.0.0:7331`, `data_dir: /data`) as its default `--config` — deliberately different from the CLI's own bare `127.0.0.1`-only default, since a container needs to be reachable from outside itself to be useful at all (this is a Docker packaging choice, not a change to `serve`'s own defaults, and not the local-vs-server branching CLAUDE.md's non-negotiable #2 forbids — the *binary* still defaults to `127.0.0.1` everywhere, only this *image*'s launch command differs, the same way an official Postgres or Nginx image's baked-in config differs from each project's own upstream default). Override it by mounting your own file over that path (matching the vision document's `--config=/data/config.yaml` exactly), or with `PATCHCORD_LISTEN`/`PATCHCORD_DATA_DIR`/`--listen`/`--data-dir` — all of which still take precedence ([ADR-0038](../../../adr/0038-configuration-serveur-fichier-yaml-precedence.md)).

No plugin is baked into the image — the core never bundles a concrete integration (non-negotiable #3). Build one for the container's OS/arch and mount it in:

```bash
GOOS=linux GOARCH=$(go env GOARCH) make build-plugins
docker compose up -d
docker compose exec patchcord patchcord plugin install /plugins/text --data-dir /data
docker compose restart patchcord   # picks up the newly installed plugin
```

See [ADR-0039](../../../adr/0039-image-docker-multi-stage-distroless.md) for the full set of packaging decisions (base image, why nothing is baked in beyond the binary and the default config, why `./bin/plugins` rather than `./plugins`).

### Secrets in a container

A `keychain` secret reference (see [Secrets & Validation](../plugins/connectors/secrets-and-validation.md)) typically fails to resolve in a container — no Secret Service daemon runs in a headless Linux image. Use `file` instead:

```bash
patchcord secret keygen > ./data/secrets.key
docker compose exec patchcord patchcord secret set --type file PG_PASSWORD \
  --data-dir /data --secrets-master-key-file /data/secrets.key
```

...then pass `--secrets-master-key-file /data/secrets.key` (or `PATCHCORD_SECRETS_MASTER_KEY_FILE`) to the `patchcord serve` command the container runs, so the running agent can resolve `file:PG_PASSWORD` references too.

## TLS

`serve` never terminates TLS itself — `internal/api.NewRouter` is always served over plain HTTP, on `127.0.0.1` or `0.0.0.0` alike. The vision document is explicit that Patchcord runs *behind* a reverse proxy for TLS (§13.4), not that it grows a certificate stack of its own ([ADR-0041](../../../adr/0041-tls-via-reverse-proxy.md)).

`docker-compose.tls.yml` (repo root) is the reference example: [Caddy](https://caddyserver.com) terminates TLS, obtaining and renewing a Let's Encrypt certificate automatically via ACME, and reverse-proxies to the `patchcord` service over the internal Docker network — `patchcord` itself publishes nothing to the host, only `expose: [7331]` inside the compose network. Requires a domain whose DNS already points at this host, and ports 80/443 reachable from the internet (the ACME HTTP-01 challenge needs both):

```bash
PATCHCORD_DOMAIN=agent.example.com make docker-run-tls
# equivalent to:
PATCHCORD_DOMAIN=agent.example.com docker compose -f docker-compose.tls.yml up --build
```

The Caddyfile driving this (`docker/Caddyfile`) is three lines — Caddy's whole pitch is that automatic HTTPS needs no more than that:

```caddyfile
{$PATCHCORD_DOMAIN} {
	reverse_proxy patchcord:7331
}
```

No public domain, or a fully offline/internal deployment? Put any TLS-terminating reverse proxy you already run (nginx, Traefik, an internal load balancer with your own CA) in front of `patchcord serve` the same way — proxy to its plain-HTTP listen address, nothing in the agent itself needs to know TLS is involved.

**Create an admin token before exposing `serve` behind a public domain** — see [Admin authentication](#admin-authentication) above. TLS protects the connection in transit; it does nothing to stop an unauthenticated request from reaching an API that still defaults to open ([ADR-0036](../../../adr/0036-authentification-admin-jetons-opt-in.md)).
