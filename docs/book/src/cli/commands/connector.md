# patchcord connector

Manage connectors — persistent, named configurations for accessing an external system. See [Plugins → Connectors](../../plugins/connectors/index.md) for the underlying model.

## `create <id>`

```bash
patchcord connector create my-postgres \
  --type postgresql.connection@1 \
  --config host=localhost --config database=app --config user=app \
  --secret password=env:PG_PASSWORD
```

- `--type` must match a connector type contributed by an installed plugin's manifest (`patchcord plugin install` first) — validated against the catalog, not just a naming convention ([ADR-0022](../../../../adr/0022-validation-type-connecteur-catalogue-greffons.md)).
- `--config key=value` (repeatable) sets non-secret configuration. All values are strings — there is no `--config-file` yet.
- `--secret name=type:key` (repeatable) sets a secret reference, e.g. `password=env:PG_PASSWORD` or `password=file:PG_PASSWORD`. `env`, `keychain` and `file` are the supported reference types ([Secrets & Validation](../../plugins/connectors/secrets-and-validation.md)) — this never stores the secret's actual value; use [`patchcord secret set`](secret.md) for `keychain`/`file` values.
- Fails if `id` already exists — `create` never overwrites (unlike `plugin install`). To change a connector, `remove` then `create` again.

## `list`

```bash
patchcord connector list
```

Prints one line per connector: `<id>  <type>  <created-at>`.

## `inspect <id>`

```bash
patchcord connector inspect my-postgres
```

Prints the connector's config and, for each secret reference, whether it **currently resolves** (e.g. whether `PG_PASSWORD` is set) — not whether the value is correct. This is not a connectivity test; see `test` below. Pass `--secrets-master-key-file` if any reference is `file`-typed (see [Configuration](../configuration.md)).

## `test <id>`

```bash
patchcord connector test my-postgres
```

Resolves the connector and asks the plugin that declares its type to actually attempt a connection. Unlike `inspect`, this proves the credentials work, not just that they resolve. Prints `OK` or `FAILED: <message>` — a failed connection attempt is a normal result (exit code 0), not a command error. The command itself only fails if no installed plugin declares the connector's type, or that plugin doesn't support connector testing. Pass `--secrets-master-key-file` if any reference is `file`-typed.

## `remove <id>`

```bash
patchcord connector remove my-postgres
```

Deletes the connector. Fails if `id` doesn't exist.
