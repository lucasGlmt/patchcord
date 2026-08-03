# Connector Model

## HTTP API

`internal/api/connectors.go` mirrors the CLI's CRUD one-for-one: `GET /v1/connectors` (list), `GET /v1/connectors/{id}` (get), `POST /v1/connectors` (create), `DELETE /v1/connectors/{id}` (delete) — same rules as below, same never-a-resolved-secret-value guarantee as `connector inspect`. There is no `PUT`/`PATCH` update endpoint either, for the same reason the CLI has none (see below); a client that wants to change a connector deletes then recreates it under the same id — see [ADR-0034](../../../../adr/0034-connecteurs-catalogue-greffons-http-bindings-dashboard.md) for how the dashboard's "Modifier" action does exactly that, with a warning about the brief window where the connector doesn't exist.

`GET /v1/plugins` lists installed plugins' declared connector types and action ids — enough for a client to build a connector-type picker without reimplementing the CLI's own catalog logic.

## Create rejects duplicates, never overwrites

Unlike `plugin install` (which upserts by ID), `connector create` fails with `ErrAlreadyExists` if the ID is already in use ([ADR-0020](../../../../adr/0020-modele-connecteur-references-secrets-env.md)). A connector ID is a stable reference other workflows point to via `bindings` — an upsert on a command typo could silently repoint a connector already in use elsewhere. There is no `update`; changing a connector means `remove` then `create` again.

## Secrets are never persisted, even encrypted

```go
type Connector struct {
	ID         string
	Type       string
	Config     map[string]any             // non-secret settings
	SecretRefs map[string]secrets.Reference // logical references only
	CreatedAt  time.Time
}
```

`SecretRefs` stores logical references like `{"type":"env","key":"PG_PASSWORD"}`, never the value itself. There is no secrets table. See [Secrets & Validation](secrets-and-validation.md) for how a reference resolves to a value, and only for the duration of one action call.

## `Type` validation

`connectors.Create` takes a `knownTypes` set — the connector types currently declared by installed plugins (`plugins.KnownConnectorTypes`) — and rejects any `Type` not in it ([ADR-0022](../../../../adr/0022-validation-type-connecteur-catalogue-greffons.md)). This means **the plugin declaring a connector type must be installed before you can create a connector of that type**:

```bash
patchcord plugin install bin/plugins/postgresql
patchcord connector create my-postgres --type postgresql.connection@1 --config host=localhost ...
```

The validation is strict, not permissive: an empty `knownTypes` (no plugin installed) rejects every `--type`, it does not disable the check. There is no retroactive revalidation — uninstalling the plugin that declared a type does not flag connectors already created with it.

## Naming convention

`Type` is encouraged, not enforced beyond the catalog check, to follow the same `<name>.<subtype>@<version>` convention as action IDs (e.g. `postgresql.connection@1`, not just `postgresql`).
