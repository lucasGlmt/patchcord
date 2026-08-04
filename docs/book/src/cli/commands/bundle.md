# patchcord bundle

Manage bundles: packages that group an application, its workflows, and its plugin dependencies (vision document, section 9.3). See [ADR-0042](../../../../adr/0042-formats-de-package-plugin-workflow-bundle.md) for the manifest format and what installing a bundle actually does under the hood — it delegates entirely to `app install` and `workflow install`'s underlying services, never duplicating their logic.

## `install <path>`

```bash
patchcord bundle install my-bundle-1.0.0.patchcord-bundle
```

Installs a `.patchcord-bundle` package produced by `bundle pack`. Every entry in the manifest's `requires_plugins` (`id@version`) must already be installed at that exact version — install does not fetch missing plugin dependencies automatically. The embedded app (if any) and workflows are installed exactly as `app install` and `workflow install` would. A failure partway through (e.g. the app installs but a workflow fails validation) is not rolled back — the error names which step failed.

`--require-signature` rejects a package that is unsigned or signed by a key not yet trusted for its id — this covers the embedded app and workflows too, they are not separately re-verified. See [Package Signing & Trust](../package-signing.md).

## `pack <dir>`

```bash
patchcord bundle pack ./my-bundle -o my-bundle-1.0.0.patchcord-bundle
patchcord bundle pack ./my-bundle --sign-key my-signing-key
```

Packs `dir` — which must contain a `bundle.yaml` manifest, plus the app directory and workflow files it references — into a `.patchcord-bundle` archive. `-o/--output` defaults to `<id>-<version>.patchcord-bundle` in the current directory. The result always carries integrity checksums; `--sign-key` additionally signs it (covering the embedded app and workflows) — see [Package Signing & Trust](../package-signing.md).

`bundle.yaml`:

```yaml
id: io.patchcord.example-bundle
version: "1.0.0"
app: app                        # optional; relative path, patchcord-app.yaml at its root
workflows:
  - workflows/main.yaml
requires_plugins:
  - io.patchcord.example-text@1.0.0
```

## `list`

```bash
patchcord bundle list
```

Prints one line per installed bundle: `<id>  <version>  <installed-at>`.

## `inspect <id>`

```bash
patchcord bundle inspect io.patchcord.example-bundle
```

Prints the bundle's ID, version, install timestamp, and its raw `bundle.yaml` source. This is a provenance record of what was declared — it does not re-derive what actually got installed; use `app list`/`workflow list` for that.
