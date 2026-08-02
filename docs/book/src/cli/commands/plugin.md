# patchcord plugin

Manage the plugin catalog. See [Plugins](../../plugins/index.md) for what a plugin is, and [ADR-0015](../../../../adr/0015-catalogue-greffons-effet-au-redemarrage.md) for why these commands never talk to a running `patchcord serve` — a change here takes effect the next time the agent (or `workflow run` / `connector test`) starts.

## `install <path>`

```bash
patchcord plugin install bin/plugins/text
```

Launches the executable at `path`, performs the protocol handshake, and records it in the catalog (replacing any existing entry with the same plugin ID — this is an upsert, unlike `connector create`). Prints the negotiated actions, connectors, and permissions.

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
