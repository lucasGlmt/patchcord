# patchcord app

Manage installed applications. See [Apps](../../apps/index.md) for the manifest format and hosting model.

## `install <dir-or-package>`

```bash
patchcord app install apps/examples/greeter
patchcord app install dashboard-0.1.0.patchcord-app
```

Installs an application from either a directory containing a `patchcord-app.yaml` manifest alongside its built static files — the agent serves it straight from that directory — or a `.patchcord-app` package produced by `app pack`, whose contents are extracted under the agent's data directory (so the package file itself does not need to stick around afterwards). The two forms are told apart by `os.Stat` (directory vs. file), never by extension — see [ADR-0027](../../../../adr/0027-app-dev-et-packaging-patchcord-app.md).

`--require-signature` rejects a package that is unsigned or signed by a key not yet trusted for its id (errors immediately if the target turns out to be a directory — nothing to verify there). See [Package Signing & Trust](../package-signing.md).

## `dev <dir>`

```bash
patchcord app dev apps/examples/dashboard
```

Like `install`, but updates the application in place instead of failing when its ID is already installed — the friction that would otherwise mean an `app remove` before every reinstall while iterating. Since the agent always serves an app's files straight off disk, rebuilding `dir`'s contents (e.g. `vite build --watch`) is reflected on the next browser refresh with no further command needed.

## `pack <dir>`

```bash
patchcord app pack apps/examples/dashboard -o dashboard-0.1.0.patchcord-app
patchcord app pack apps/examples/dashboard --sign-key my-signing-key
```

Packs `dir` (which must contain a `patchcord-app.yaml` manifest) into a `.patchcord-app` archive that `app install` can install directly. `-o/--output` defaults to `<id>-<version>.patchcord-app` in the current directory. The result always carries integrity checksums; `--sign-key` additionally signs it — see [Package Signing & Trust](../package-signing.md).

## `list`

```bash
patchcord app list
```

Prints one line per installed app: `<id>  <version>  <permitted-workflows>  <created-at>`.

## `remove <id>`

```bash
patchcord app remove greeter
```

Removes an installed application. Fails if no app with that ID is installed.
