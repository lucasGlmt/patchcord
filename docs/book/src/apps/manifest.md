# App Manifest

Every installed application's root directory (or package — see [Hosting & Sessions](hosting-and-sessions.md#installing)) must contain `patchcord-app.yaml`, parsed and validated by `apps.LoadManifest`/`apps.ParseManifest` (`internal/apps/manifest.go`) against the contract published at `api/app/v2/manifest.schema.json`.

```yaml
# apps/examples/greeter/patchcord-app.yaml
id: greeter
version: "0.1.0"
permissions:
  workflows:
    run:
      - hello_patchcord
  connectors:
    use:
      - accounting_mailbox
```

| Field | Required | Meaning |
|---|---|---|
| `id` | yes | The application's unique identifier. Used in its serving URL (`/apps/<id>/`), and as the `app_id` a session issued for it carries. Must not be empty. |
| `version` | yes | Free-form, e.g. a semantic version. Must not be empty. |
| `permissions.workflows.run` | yes (may be an empty list) | The workflow ids this application's sessions are allowed to run. A session issued for this application can never run a workflow outside this list — see [Concepts](concepts.md). Every entry must be non-empty. |
| `permissions.connectors.use` | no (defaults to empty) | The connector ids a run started by this application's sessions is allowed to bind to a workflow step. Checked when a step actually resolves its connector, not upfront when the run starts — see [Concepts](concepts.md). Every entry must be non-empty. |

`ParseManifest` rejects a manifest missing `id` or `version`, or containing an empty string anywhere in `permissions.workflows.run` or `permissions.connectors.use`, returning an error wrapping `apps.ErrInvalidManifest`. A manifest written before `connectors.use` existed (no `permissions.connectors` key at all) still parses — it defaults to an empty list, meaning no connector-bound step can be resolved by that app's sessions (see [ADR-0071](../../../adr/0071-application-permission-connectors-use.md)).

## What's deliberately not here yet

The vision document (§15.4) also sketches top-level `capabilities` (e.g. `file.user_selected.read`, `notification.desktop`) alongside `workflows.run`/`connectors.use`. It doesn't exist in the manifest today: the agent has no enforcement point for it yet (no code path checks a session against an arbitrary capability), and modeling a permission ahead of an enforcement point would be validation with nothing to validate — the same situation plugin permissions are in (`plugins.CatalogEntry.Permissions`, recorded but unchecked). `api/app/v2/manifest.schema.json` documents this explicitly and is versioned so a future revision can add it additively, without breaking `id`/`version`/`permissions.workflows.run`/`permissions.connectors.use` (see [ADR-0026](../../../adr/0026-applications-manifeste-hebergement-session-limitee.md) and [ADR-0071](../../../adr/0071-application-permission-connectors-use.md)).
