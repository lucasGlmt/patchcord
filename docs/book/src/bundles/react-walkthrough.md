# Building a Bundle with React

End-to-end walkthrough for the one combination that trips up the two docs it draws from — [`bundle new --template vite`](../cli/commands/bundle.md#new-id) and [`app new`'s "Using a UI framework"](../cli/commands/app.md#using-a-ui-framework-react-vue) — if followed independently rather than in the order below. There is no `--template vite-react`: Vite's own official templates (`npm create vite@latest`) already cover every framework and stay current with upstream, which a copy embedded in this CLI would not.

## 1. Scaffold the bundle

```bash
patchcord bundle new fr.glmtsolutions.invoice-manager --template vite
cd invoice-manager
```

This writes `bundle.yaml` (`app: app/dist`, already correct — a Vite project always builds to `dist/` regardless of template), `workflows/main.yaml`, and a plain Vite + TypeScript project under `app/`. That last part is what gets replaced next.

## 2. Replace `app/` with a React project

**Delete `app/` before running `create-vite`, not after.** `bundle new --template vite` has already written files there (`app/public/patchcord-app.yaml`, `package.json`, …); `create-vite` requires an empty target directory and will otherwise offer to wipe it — silently taking the Patchcord-specific files down with it, which is the confusing failure this page exists to prevent.

```bash
rm -rf app
npm create vite@latest app -- --template react-ts   # or vue-ts, etc. — same recipe either way
```

`bundle.yaml` and `workflows/main.yaml` are untouched by this — only `app/` gets rebuilt.

## 3. Add back the two things `create-vite` knows nothing about

`create-vite`'s templates have no notion of Patchcord, so two things always need adding by hand after step 2, regardless of framework:

**a. `app/public/patchcord-app.yaml`** — Vite copies `public/` into `dist/` verbatim on build, which is what turns `app/dist` into an installable app directory (`bundle.yaml`'s `app: app/dist` expects it there, not in `app/`). The manifest can be as small as:

```yaml
id: fr.glmtsolutions.invoice-manager
version: "0.1.0"
permissions:
  workflows:
    run: []
```

`id`/`version` don't need to match the bundle's own — but reusing the bundle's `id` is the least confusing default (see [App Manifest](../apps/manifest.md) for the full field reference). List any embedded workflow the app needs to call under `permissions.workflows.run` (`fr.glmtsolutions.invoice-manager_workflow` for the scaffolded one above) — an app can only run what its manifest explicitly permits ([Concepts](../apps/concepts.md)).

**b. `base: "./"` in `app/vite.config.ts`** — the agent serves the app under `/apps/<id>/`, a subpath it only learns at install time. Vite's default (`base: "/"`) emits asset URLs rooted at the domain instead (`/assets/index-xxxx.js`), which resolve against the origin, not against `/apps/<id>/index.html` — 404 on every script, and a stylesheet request that comes back as the agent's default 404 response (`Content-Type: text/plain`), which is what a browser's "Refused to apply style … strict MIME checking" console error actually means here. Add it to the `defineConfig({...})` object:

```ts
export default defineConfig({
  base: "./",
  // ...
});
```

`npm run dev`/`vite preview` never reveal this — both serve from the domain root themselves, so the bug only shows up once the build is actually installed and served at `/apps/<id>/`.

If the app calls the API, also add the SDK and a dev-time proxy (`npm run dev` talks to `localhost:5173`, not the agent, so requests to `/v1` need forwarding):

```bash
npm install @glmtsolutions/patchcord-sdk
```

```ts
export default defineConfig({
  base: "./",
  server: {
    proxy: { "/v1": "http://127.0.0.1:7331" },
  },
  // ...
});
```

(See [`app new --template vite`](../cli/commands/app.md#new-id)'s generated `vite.config.ts` for the exact shape this mirrors — it ships both `base` and this proxy out of the box, which is exactly what a React project loses by being scaffolded separately.)

## 4. Build and pack

```bash
cd app && npm install && npm run build && cd ..
patchcord bundle pack ./ --sign-key my-signing-key
```

`bundle pack` validates the embedded app's manifest before archiving anything — if `app/dist/patchcord-app.yaml` is missing (step 3a skipped, or `npm run build` ran before it was added), it fails immediately naming the app path, rather than packing a broken bundle that only fails at install time.

## 5. Iterate with `bundle dev` instead

Packing and signing on every change is unnecessary friction while developing. Skip straight to:

```bash
patchcord bundle dev ./ --watch
```

which installs straight from the source directory — no pack/install round trip — and reinstalls automatically on every subsequent `npm run build` (or `vite build --watch` left running in another terminal). See [`bundle dev`](../cli/commands/bundle.md#dev-dir).

## 6. Verify

```bash
patchcord serve
```

Open `http://127.0.0.1:7331/apps/fr.glmtsolutions.invoice-manager/`. If assets 404 or a stylesheet gets refused for a `text/plain` MIME type, `base: "./"` (step 3b) is the first thing to check — rebuild and reinstall after fixing it, the broken build already installed does not fix itself.

## Troubleshooting

| Symptom | Cause | Fix |
|---|---|---|
| `bundle pack`/`bundle install`: `... patchcord-app.yaml: no such file or directory` | Step 3a skipped, or done after `npm run build` instead of before | Add `app/public/patchcord-app.yaml`, then rebuild (`npm run build`) before packing/installing again |
| Built JS 404s at `/assets/...`; a stylesheet is "refused ... MIME type ('text/plain')" | Step 3b skipped — `base: "./"` missing from `app/vite.config.ts` | Add it, rebuild, reinstall — `npm run dev`/`vite preview` never show this, only the installed app does |
| `create-vite`: target directory not empty / offers to overwrite | `app/` from `bundle new --template vite` still present | `rm -rf app` first (step 2), then run `create-vite` |
