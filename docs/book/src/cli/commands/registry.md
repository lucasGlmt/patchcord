# patchcord registry

Manage configured package registries (ADR-0044). A registry is a name pointing at either a local directory or an `http(s)://` URL serving a static `index.json` plus package files — no bespoke registry server, no authentication. `bundle install <ref>`/`bundle update` resolve an `id` or `id@version` reference against every configured registry, in the order they were added; the first registry whose index lists the id wins.

## `add <name> <location>`

```bash
patchcord registry add local ./my-registry
patchcord registry add company https://packages.example.internal
```

Configures `location` under `name`. Re-adding the same name updates its location instead of failing.

## `list`

```bash
patchcord registry list
```

Prints one line per configured registry: `<name>  <location>  <added-at>`.

## `remove <name>`

```bash
patchcord registry remove local
```

Removes a configured registry. Fails if `name` was never configured.

## `index.json` format

Every registry, local or remote, must serve one `index.json` at its root:

```json
{
  "schemaVersion": 1,
  "packages": {
    "io.patchcord.example-bundle": {
      "kind": "bundle",
      "latest": "1.1.0",
      "versions": {
        "1.0.0": "packages/example-bundle-1.0.0.patchcord-bundle",
        "1.1.0": "packages/example-bundle-1.1.0.patchcord-bundle"
      }
    }
  }
}
```

`kind` is one of `plugin`, `app`, `workflow`, `bundle` (the same package vocabulary as [`patchcord bundle`](bundle.md)'s manifest format). `versions` paths are relative to the registry's own location (its directory, or its base URL). `latest` is a plain declaration by whoever maintains the index — it is never inferred by comparing version strings, since versions are opaque strings everywhere in Patchcord (no semantic-version comparison).

Resolution never mixes sources for one id: once a registry's index lists the requested id, that registry is the chosen source for it — a version missing from that same index is an error, not a reason to look elsewhere. A registry whose index cannot be read at all fails resolution immediately, naming that registry, rather than silently falling through to the next one.

See [ADR-0044](../../../../adr/0044-registre-de-packages-et-mise-a-jour-de-bundle.md) for the full design rationale, including what is explicitly out of scope in this first version (registry-resolved `plugin install`/`app install`, trust-key distribution via a registry, caching).
