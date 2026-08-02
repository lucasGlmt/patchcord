# Connectors

A connector is a persistent, named configuration for accessing an external system — not an action, not a plugin. It's a CRUD resource of its own (`internal/connectors`), created with `patchcord connector create` and consumed by a workflow step that binds it to an action call.

```
Connector{ID, Type, Config, SecretRefs, CreatedAt}
```

- **`ID`** is a user-chosen slug (like a workflow's ID), not a generated UUID.
- **`Type`** must match a connector type an installed plugin declares in its manifest (e.g. `postgresql.connection@1`), validated against the plugin catalog at creation time ([ADR-0022](../../../../adr/0022-validation-type-connecteur-catalogue-greffons.md)).
- **`Config`** holds non-secret settings (host, port, base URL, ...).
- **`SecretRefs`** holds logical references to secret values, by name — never a secret's actual value.

## The three operations, and why they're separate

| Question | Command | What it actually checks |
|---|---|---|
| "Is this reference resolvable?" | [`connector inspect`](../../cli/commands/connector.md#inspect-id) | Whether each `SecretRefs` entry currently resolves (e.g. an env var is set) — not whether the value is *correct*. |
| "Does this connection actually work?" | [`connector test`](../../cli/commands/connector.md#test-id) | Delegates to the plugin declaring the connector's type, which attempts a real connection. |
| "How does an action use it?" | (a workflow step's `connector:` field) | See [Secrets & Validation](secrets-and-validation.md) for how a bound connector reaches a plugin process. |

These three stayed deliberately separate across three ADRs ([0020](../../../../adr/0020-modele-connecteur-references-secrets-env.md), [0021](../../../../adr/0021-binding-connecteur-workflow-protocole.md), [0022](../../../../adr/0022-validation-type-connecteur-catalogue-greffons.md), [0023](../../../../adr/0023-protocole-test-connecteur.md)) rather than being designed all at once, each building only what the next concrete use case (a real connector-consuming plugin) demanded.

## Read next

- [Connector Model](model.md) — the data model and how secrets are referenced, never inlined.
- [Secrets & Validation](secrets-and-validation.md) — how a bound connector's config and secrets reach a plugin, and the type-validation rule.
- [Testing a Connector](testing.md) — the `TestConnector` RPC and what it does and doesn't prove.
