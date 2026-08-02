# patchcord app

Manage installed applications. See [Apps](../../apps/index.md) for the manifest format and hosting model.

## `install <dir>`

```bash
patchcord app install apps/examples/greeter
```

`dir` must contain a `patchcord-app.yaml` manifest alongside the application's built static files. The agent serves the application straight from `dir` — there is no packaging or copy step in this version.

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
