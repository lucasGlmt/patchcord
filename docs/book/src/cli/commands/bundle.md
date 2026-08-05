# patchcord bundle

Manage bundles: packages that group an application, its workflows, and its plugin dependencies (vision document, section 9.3). See [ADR-0042](../../../../adr/0042-formats-de-package-plugin-workflow-bundle.md) for the manifest format and what installing a bundle actually does under the hood — it delegates entirely to `app install` and `workflow install`'s underlying services, never duplicating their logic.

## `new <id>`

```bash
patchcord bundle new io.patchcord.example-bundle
patchcord bundle new io.patchcord.example-bundle -o my-bundle --version 1.0.0
```

Scaffolds a minimal `bundle.yaml` into `-o/--output` (default: the id's last `.`-separated segment), plus an embedded app and workflow (`workflows/main.yaml`). `requires_plugins` starts empty: there is no way to know ahead of time what plugin your bundle should depend on, add entries to `bundle.yaml` yourself. The scaffolded workflow uses `text.uppercase@1` (the reference plugin) as a working example, the same convention [`workflow new`](workflow.md) uses. Fails if the target directory already exists and is not empty.

`--template static` (default) writes the embedded app as a plain `app/` directory (`patchcord-app.yaml` + `index.html`) — ready for `bundle pack`/`bundle install` as-is, same as [`app new`](app.md)'s static template.

`--template vite` writes the embedded app as a Vite + TypeScript project under `app/` instead (same layout as [`app new --template vite`](app.md)) — no UI framework opinion, add any npm package the app needs. `bundle.yaml`'s `app` field already points at `app/dist`, but that directory only exists once the project has been built at least once:

```bash
patchcord bundle new io.patchcord.invoice-manager --template vite
cd invoice-manager/app && npm install && npm run build
patchcord bundle dev invoice-manager --watch
```

From there, `bundle dev --watch` reinstalls automatically on every subsequent `npm run build` (or `vite build --watch`) — see [`dev`](#dev-dir) below. This is the "all in one" path: the app ends up served by the same agent, at `/apps/<id>/`, packaged together with its workflows and plugin dependencies — unlike a standalone app such as `apps/examples/dashboard`, which is installed and served on its own, outside any bundle.

## `install <path-or-ref>`

```bash
patchcord bundle install my-bundle-1.0.0.patchcord-bundle
patchcord bundle install io.patchcord.example-bundle
patchcord bundle install io.patchcord.example-bundle@1.0.0
```

Installs a `.patchcord-bundle` package produced by `bundle pack`. Every entry in the manifest's `requires_plugins` (`id@version`) must already be installed at that exact version — install does not fetch missing plugin dependencies automatically. The embedded app (if any) and workflows are installed exactly as `app install` and `workflow install` would. A failure partway through (e.g. the app installs but a workflow fails validation) is not rolled back — the error names which step failed.

If the argument does not name an existing local file, it is resolved instead as a registry reference — a bare `id` (the registry's declared latest version) or `id@version` — against every configured registry. See [`patchcord registry`](registry.md). Re-installing an already-installed bundle id, from either a local path or a registry reference, updates it in place — `bundle update` (below) is the more convenient way to do that by id alone.

`--require-signature` rejects a package that is unsigned or signed by a key not yet trusted for its id — this covers the embedded app and workflows too, they are not separately re-verified. See [Package Signing & Trust](../package-signing.md).

## `update <id>[@version]`

```bash
patchcord bundle update io.patchcord.example-bundle
patchcord bundle update io.patchcord.example-bundle@1.2.0
```

Resolves `id`'s latest version (or the pinned `@version`, if given) against every configured registry, and installs it exactly as `bundle install` would — but only if the resolved version differs from the one currently installed. `id` must already be installed (`bundle update` errors, pointing at `bundle install`, otherwise). If the resolved version matches what's installed, this is a no-op: `<id> is already up to date (<version>)`. On an actual update, prints `Updated <id>: <old> -> <new>`.

## `dev <dir>`

```bash
patchcord bundle dev ./my-bundle
patchcord bundle dev ./my-bundle --watch
```

Installs straight from `dir` — no `bundle pack`/`bundle install` round trip, no checksum, no signature (`dir` is local, unsigned-by-design source under active development). The embedded app (if any) is installed in place exactly as [`app dev`](app.md) would: served live from `dir`'s app subdirectory, never copied or moved, so rebuilding it (e.g. `vite build --watch`) needs no further agent involvement. Each embedded workflow still goes through `workflow install`'s own rule: a published version is immutable ([ADR-0008](../../../../adr/0008-workflows-publies-immuables.md)), so editing a workflow's body requires bumping its `version` field before the next `bundle dev` picks up the change. `requires_plugins` is still enforced — a missing dependency is not installed automatically.

`--watch` keeps the command running, reinstalling on every change under `dir` (debounced ~300ms to coalesce a build tool's burst of writes into one reinstall) until interrupted with Ctrl+C. An install failure while watching (most commonly: a workflow edited without bumping its version) is printed but does not stop the watch — fix it and save again. Hidden directories (`.git` and similar) are never watched.

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
