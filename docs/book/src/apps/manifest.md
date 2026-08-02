# App Manifest

Every installed application's root directory (or package — see [Hosting & Sessions](hosting-and-sessions.md#installing)) must contain `patchcord-app.yaml`, parsed and validated by `apps.LoadManifest`/`apps.ParseManifest` (`internal/apps/manifest.go`) against the contract published at `api/app/v1/manifest.schema.json`.

```yaml
# apps/examples/greeter/patchcord-app.yaml
id: greeter
version: "0.1.0"
permissions:
  workflows:
    run:
      - hello_patchcord
```

| Field | Required | Meaning |
|---|---|---|
| `id` | yes | The application's unique identifier. Used in its serving URL (`/apps/<id>/`), and as the `app_id` a session issued for it carries. Must not be empty. |
| `version` | yes | Free-form, e.g. a semantic version. Must not be empty. |
| `permissions.workflows.run` | yes (may be an empty list) | The workflow ids this application's sessions are allowed to run. A session issued for this application can never run a workflow outside this list — see [Concepts](concepts.md). Every entry must be non-empty. |

`ParseManifest` rejects a manifest missing `id` or `version`, or containing an empty string anywhere in `permissions.workflows.run`, returning an error wrapping `apps.ErrInvalidManifest`.

## What's deliberately not here yet

The vision document (§15.4) sketches a richer permission model — `connectors.use` and top-level `capabilities` (e.g. `file.user_selected.read`) alongside `workflows.run`. Neither exists in the manifest today: the agent has no enforcement point for them yet (no code path checks a session against a connector or a capability), and modeling permissions ahead of an enforcement point would be validation with nothing to validate — the same situation plugin permissions are in (`plugins.CatalogEntry.Permissions`, recorded but unchecked). `api/app/v1/manifest.schema.json` documents this explicitly and is versioned so a future revision can add both fields additively, without breaking `id`/`version`/`permissions.workflows.run` (see [ADR-0026](../../../adr/0026-applications-manifeste-hebergement-session-limitee.md)).
