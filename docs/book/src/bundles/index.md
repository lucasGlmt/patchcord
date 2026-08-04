# Bundles

A bundle groups an application, its workflows, and its plugin dependencies into one `.patchcord-bundle` package (vision document, section 9.3). It exists to distribute a working slice of Patchcord — "install this dashboard and everything it needs" — as a single file, without duplicating any of the mechanisms that already install an app or a workflow on their own.

Installing a bundle does exactly three things, in order:

1. **Checks plugin dependencies.** Every `id@version` entry in the manifest's `requires_plugins` must already be installed at that exact version. A bundle never installs missing plugins automatically, even with a registry configured (see [ADR-0044](../../../adr/0044-registre-de-packages-et-mise-a-jour-de-bundle.md)'s "explicitement hors scope" section) — install that plugin yourself first. A bundle *can* be updated to a newer version once installed, though: see [`patchcord bundle update`](../cli/commands/bundle.md).
2. **Installs the embedded app**, if the manifest declares one, through the same service `app install` uses.
3. **Installs the embedded workflows**, through the same service `workflow install` uses.

A bundle carries no connector configuration: `internal/connectors` has no file-based export or template mechanism today, so there is nothing a bundle could portably carry beyond a connector's non-secret id and type — a gap left open for a later pass, not forgotten.

See [`patchcord bundle`](../cli/commands/bundle.md) for the manifest format and command reference, and [ADR-0042](../../../adr/0042-formats-de-package-plugin-workflow-bundle.md) for the full design rationale, including what is explicitly out of scope in this first version (automatic dependency installation, multi-resource rollback, connector configuration).

## Example

`bundles/examples/lead-crm/` is Patchcord's reference "first complete business application" (CLAUDE.md §9 phase 7 roadmap): a mini-CRM that submits a lead, enriches it via an HTTP call, and stores it in PostgreSQL, composing an app, two workflows, and two connector-consuming plugins (`http`, `postgresql`) into one bundle. See its own `bundle.yaml` header comment for the full setup sequence — it needs a real PostgreSQL server you control, the same requirement `workflows/examples/postgresql_query_demo.yaml` already documents.
