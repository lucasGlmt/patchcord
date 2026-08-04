# patchcord plugin

Manage the plugin catalog. See [Plugins](../../plugins/index.md) for what a plugin is, and [ADR-0015](../../../../adr/0015-catalogue-greffons-effet-au-redemarrage.md) for why these commands never talk to a running `patchcord serve` — a change here takes effect the next time the agent (or `workflow run` / `connector test`) starts.

## `install <path>`

```bash
patchcord plugin install bin/plugins/text
patchcord plugin install text-1.0.0.patchcord-plugin
```

Installs a plugin from either a raw executable path, or a `.patchcord-plugin` package produced by `plugin pack`. The two are told apart by sniffing the file's content (gzip magic bytes), never by extension — see [ADR-0042](../../../../adr/0042-formats-de-package-plugin-workflow-bundle.md). For a package, the executable matching the current platform (`GOOS-GOARCH`, e.g. `darwin-arm64`) is extracted under the agent's data directory; for a raw path, the executable is used in place. Either way, the plugin is launched, the protocol handshake is performed, and it is recorded in the catalog (replacing any existing entry with the same plugin ID — this is an upsert, unlike `connector create`). Prints the negotiated actions, connectors, and permissions.

`--require-signature` rejects a package that is unsigned or signed by a key not yet trusted for its id (errors immediately if `path` turns out to be a raw executable — nothing to verify there). See [Package Signing & Trust](../package-signing.md).

## `pack <dir>`

```bash
patchcord plugin pack ./my-plugin -o my-plugin-1.0.0.patchcord-plugin
patchcord plugin pack ./my-plugin --sign-key my-signing-key
```

Packs `dir` — which must contain a `manifest.json` (id, version, protocol version, declared permissions, and one executable path per supported platform) plus the executables themselves under `binaries/<platform>/` — into a `.patchcord-plugin` archive that `plugin install` can install directly. `pack` only archives what is already built; producing the per-platform executables (cross-compiling with `GOOS`/`GOARCH`) is left to the plugin's own build tooling. `-o/--output` defaults to `<id>-<version>.patchcord-plugin` in the current directory. The result always carries integrity checksums; `--sign-key` additionally signs it — see [Package Signing & Trust](../package-signing.md).

## `list`

```bash
patchcord plugin list
```

Prints one line per installed plugin: `<plugin-id>  <version>  <executable-path>`.

## `inspect <plugin-id>`

```bash
patchcord plugin inspect io.patchcord.example-text
```

Prints the plugin's ID, version, executable path, negotiated protocol version, contributed actions, connectors, permissions, and install timestamp.

## `uninstall <plugin-id>`

```bash
patchcord plugin uninstall io.patchcord.example-text
```

Removes the plugin from the catalog. Fails if no plugin with that ID is installed.
