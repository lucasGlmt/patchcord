# Connector Model

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
