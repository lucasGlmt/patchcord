# Developing a Bundle

```bash
patchcord dev ./my-bundle
```

Developing a bundle with an embedded app otherwise needs three commands in three terminals, in order: `patchcord serve`, `patchcord bundle dev --watch`, and the embedded app's own dev server (`npm run dev`). `dev` composes exactly those same three things into one ([ADR-0054](../../../adr/0054-commande-patchcord-dev-unifiant-serve-bundle-dev-et-le-serveur-de-dev-de-lapp-embarquee.md)) — no logic is duplicated, each piece is the same internal service `serve`/`bundle dev` already call.

On startup, in order:

1. Tries to start the agent, with the exact same settings resolution as [`serve`](serve.md) (`--listen`/`--data-dir`/`--config`/`--secrets-master-key-file`, ADR-0038). If `--listen` is already bound — another `patchcord serve` or `patchcord dev` is already running against it — that agent is reused instead of failing: `dev` prints `Agent already running at <listen>, reusing it` and moves on.
2. Installs `<dir>` as a bundle ([`bundles.InstallDir`](commands/bundle.md#dev-dir), exactly what `bundle dev` does) and watches it for changes, reinstalling on every save. An embedded workflow edited without bumping its `version` is auto-installed under the next version instead of being rejected ([ADR-0055](../../../adr/0055-auto-incrementation-de-version-des-workflows-embarques-en-mode-developpement.md)).
3. If the embedded app has a `package.json` declaring a `"dev"` script, runs it — `npm run dev` by default, `--app-dev-cmd` to override (e.g. `--app-dev-cmd "pnpm dev"`), `--no-app-dev` to skip it entirely. Its combined output is prefixed `[app] ` so it stays distinguishable from the agent's own logs and `dev`'s own messages on the same terminal.

`Ctrl+C` stops all three. If any of them fails outright — not counting an install failure while watching, which is only printed, same as `bundle dev --watch` — the other two are stopped too and the error is returned.

## When to use `bundle dev` instead

`dev` is the convenience wrapper; [`bundle dev`](commands/bundle.md#dev-dir) is still the lower-level primitive it's built on, useful when:
- a `patchcord serve` is already managed elsewhere (a shared dev agent, a container) and you only want to iterate on the bundle;
- scripting or CI needs a single reinstall (or a `--watch` loop) without an agent lifecycle or a subprocess attached.

## Flags

| Flag | Default | Meaning |
|---|---|---|
| `--listen` | `127.0.0.1:7331` | Same as [`serve`](serve.md#flags). |
| `--data-dir` | a per-user system directory ([ADR-0052](../../../adr/0052-defaut-data-dir-dossier-standard-du-systeme.md)) | Same as [`serve`](serve.md#flags). |
| `--config` | (none) | Same as [`serve`](serve.md#flags). |
| `--secrets-master-key-file` | (none) | Same as [`serve`](serve.md#flags). |
| `--app-dev-cmd` | `npm run dev` | Command to run the embedded app's dev server, in the directory holding its `package.json` — split on whitespace, no shell involved (no `&&`, no globbing). |
| `--no-app-dev` | `false` | Never start the embedded app's dev server, even if `package.json` declares one. |

## Finding the embedded app's dev server

`dev` looks for a `package.json` declaring a `"dev"` script in `bundle.yaml`'s `app` directory, or — if not found there — its parent. This covers both scaffold templates without extra configuration: the Vite template's `bundle.yaml` points `app` at `app/dist` (only populated after a build), while `package.json` itself lives one level up, at `app/`. The static template has no `package.json` anywhere under `app/`, so no dev server is started for it — `dev` still watches and installs the bundle, it just skips step 3.

```bash
patchcord bundle new io.patchcord.invoice-manager --template vite
cd invoice-manager/app && npm install && npm run build && cd ../..
patchcord dev invoice-manager
```
