# Hosting & Sessions

## Installing

```bash
patchcord app install apps/examples/greeter        # from a directory
patchcord app install dashboard-0.1.0.patchcord-app # from a package
```

`patchcord app install <dir-or-package>` (`internal/cli/app.go`) accepts either form, distinguished with `os.Stat` — a directory or a file, never by extension:

- **A directory** containing `patchcord-app.yaml` (see [App Manifest](manifest.md)) alongside the app's built static files. `apps.Install` (`internal/apps/apps.go`) records `static_dir` as that directory's absolute path — the agent serves it straight from there, so the directory must keep existing for as long as the app stays installed.
- **A `.patchcord-app` package**, produced by `app pack` (below). `apps.InstallPackage` (`internal/apps/package.go`) extracts its contents under `<data-dir>/apps/<id>/<version>/` — a location the agent owns — then installs from there exactly as it would a directory. The original package file can be deleted afterwards; the agent now has its own copy.

Either way, `Install` fails with `apps.ErrAlreadyExists` if the manifest's `id` is already installed — there is no update, only `app remove` then `app install` again (see [ADR-0026](../../../adr/0026-applications-manifeste-hebergement-session-limitee.md); `app dev`, below, is the one exception).

## Packaging

```bash
patchcord app pack apps/examples/dashboard/dist -o dashboard.patchcord-app
```

`patchcord app pack <dir>` (`apps.Pack`) archives a directory — which must contain a valid manifest — into a gzip-compressed tar stream, the `.patchcord-app` format (vision document, §9.3: "interface web statique et manifeste de permissions"). `-o/--output` defaults to `<id>-<version>.patchcord-app` in the current directory. Only regular files and directories are supported — a symlink or other special entry fails the pack rather than silently producing an incomplete archive. Extraction rejects any entry whose path would escape the destination directory (a "zip slip"), since a package is untrusted input ([ADR-0027](../../../adr/0027-app-dev-et-packaging-patchcord-app.md)).

## Developing

```bash
patchcord app dev apps/examples/dashboard/dist
```

`patchcord app dev <dir>` (`apps.InstallOrUpdate`) is `app install` for a directory, except that installing over an already-installed id **updates it in place** instead of failing — no `app remove` needed between iterations. This works because the agent always serves `static_dir` straight off disk on every request (`handleServeApp`, below uses `http.FileServer`, not a cache or a copy): once an app is registered, rebuilding its directory's contents (e.g. `vite build --watch`) is reflected on the next browser refresh with no further agent involvement. `app dev` removes the friction, not the rebuild step — the agent does not run a bundler or provide JavaScript hot-module-replacement itself (that stays the job of the frontend tool's own dev server, run separately); see [ADR-0027](../../../adr/0027-app-dev-et-packaging-patchcord-app.md).

## Static hosting

```text
GET /apps/{id}/  →  http.StripPrefix("/apps/{id}/", http.FileServer(http.Dir(app.StaticDir)))
```

`handleServeApp` (`internal/api/apps.go`) resolves the installed app by id and serves its `static_dir` with a per-app `http.FileServer` — the first use of a `FileServer` in the core (everything else served a single embedded file). Open `http://127.0.0.1:7331/apps/<id>/` once `patchcord serve` is running and the app is installed.

## Sessions

```bash
curl -X POST http://127.0.0.1:7331/v1/apps/greeter/sessions
# {"token":"...", "app_id":"greeter", "workflows_run":["hello_patchcord"], "issued_at":"..."}
```

`POST /v1/apps/{id}/sessions` (`handleCreateAppSession`) issues a token via `auth.Store.Issue`, carrying exactly the permissions from the app's manifest — see [Concepts](concepts.md) for what that restricts and what it doesn't. Present it as `Authorization: Bearer <token>` on `POST /v1/workflows/{id}/run`; every other route ignores it. Requesting a session itself requires an admin token once one exists ([ADR-0036](../../../adr/0036-authentification-admin-jetons-opt-in.md)) — before then, no credential is required, consistent with the agent having no admin authentication yet (ADR-0026).

In application code, prefer the TypeScript SDK over calling this endpoint by hand — see [Building an App with the TS SDK](building-with-sdk-ts.md#acquiring-a-session).
