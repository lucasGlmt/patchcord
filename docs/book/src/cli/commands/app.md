# patchcord app

Manage installed applications. See [Apps](../../apps/index.md) for the manifest format and hosting model.

## `new <id>`

```bash
patchcord app new greeter
patchcord app new greeter -o my-app --version 1.0.0
```

Scaffolds a minimal application project into `-o/--output` (default: the id's last `.`-separated segment). Fails if the target directory already exists and is not empty.

`--template static` (default) writes a `patchcord-app.yaml`, `index.html` and `AGENTS.md` — ready for `app pack`/`app install` as-is.

`--template vite` writes a Vite + TypeScript project instead (`package.json`, `vite.config.ts`, `tsconfig.json`, `index.html`, `src/main.ts`, `src/vite-env.d.ts`, `AGENTS.md`) — no UI framework opinion, add any npm package you need (React, Vue, MUI, whatever the app calls for). `AGENTS.md` orients a coding agent working in the scaffolded directory — links to the TS SDK docs, the dev loop, and the app manifest's permissions — never packaged by `app pack` (see [App Manifest](../../apps/manifest.md)). It already depends on [`@glmtsolutions/patchcord-sdk`](../../sdk-ts/getting-started.md) and `src/main.ts` instantiates a `PatchcordClient` and checks the agent's health on load, so calling the API is ready out of the box. Unlike the static template, the result is not installable as-is: `patchcord-app.yaml` lives under `public/` so Vite's build copies it into `dist/` alongside the bundled JS — the same convention `apps/examples/dashboard` uses. Build first, then point `app install`/`app dev`/`app pack` at the `dist` subdirectory:

```bash
patchcord app new invoice-manager --template vite
cd invoice-manager && npm install && npm run build
patchcord app install invoice-manager/dist
```

### Using a UI framework (React, Vue, …)

There is no `--template react-ts`/`--template vue-ts` — deliberately: Vite's own official templates (`npm create vite@latest`) already cover every framework combination and stay current with upstream releases, which a copy embedded in this CLI would not. Start from one of those instead, then adapt it to the two things specific to a Patchcord app:

```bash
npm create vite@latest invoice-manager -- --template react-ts   # or vue-ts, etc.
cd invoice-manager
```

1. Move `patchcord-app.yaml` (write one by hand, or copy `apps/examples/*/patchcord-app.yaml`) under `public/` — Vite copies everything there into `dist/` on build, which is what turns the project into an installable app directory.
2. In `vite.config.ts`, add `base: "./"` — the agent serves a built app under `/apps/{id}/`, and Vite's default absolute base (`base: "/"`) emits asset URLs that 404 once installed (see [ADR-0058](../../../adr/0058-base-relatif-pour-les-apps-vite-servies-sous-apps-id.md)). `npm run dev`/`vite preview` never reveal this, since they both serve from the domain root themselves.

Everything else — installing `@glmtsolutions/patchcord-sdk`, proxying `/v1` to the agent in dev (see the generated `vite.config.ts` from `--template vite` for the exact shape) — is the same regardless of framework.

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

## `session create <id>`

```bash
patchcord app session create dashboard --admin-token pcat_...
```

Mints a session for an installed application and writes it to `<static-dir>/patchcord-session.json` (`-o/--output` to write elsewhere) — same origin as the application's own files, so its code can `fetch` it directly instead of calling `client.apps.createSession` itself.

This exists because of a hard constraint, not a stylistic choice: a session lives only in the running agent's memory ([ADR-0026](../../../../adr/0026-applications-manifeste-hebergement-session-limitee.md)), never in the database, so unlike every other `app`/`bundle` command this one genuinely talks to the running agent over HTTP (`--base-url`, default `http://127.0.0.1:7331`) rather than writing to the shared SQLite file. `--admin-token` (or `PATCHCORD_ADMIN_TOKEN`) is sent as `Authorization: Bearer` — required once the agent has any admin token ([ADR-0036](../../../../adr/0036-authentification-admin-jetons-opt-in.md)), since an application's own browser code can never safely mint its own session past that point. See [ADR-0051](../../../../adr/0051-commande-cli-pour-minter-une-session-app-hors-navigateur.md) and [Building an App with the TS SDK](../../apps/building-with-sdk-ts.md#once-the-agent-has-an-admin-token) for the client-side half of this pattern.

Re-run after every rebuild that replaces `static-dir`'s contents (a fresh `vite build`, another `app install`) — the previous run's session file does not survive one.
