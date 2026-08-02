# Apps Overview

An application is a client of the agent per the [vocabulary](../introduction.md#vocabulary): built with Vite, React, Flutter, Electron, plain HTML — anything that can call the public API — and using only that API and the [TypeScript SDK](../sdk-ts/index.md). It never receives the agent's full privileges: it declares the permissions it needs in a manifest and, at runtime, holds a session limited to exactly those (vision document, §7.6, §15.4; [ADR-0026](../../../adr/0026-applications-manifeste-hebergement-session-limitee.md)).

`internal/apps` manages installed applications; `internal/auth` issues and validates their sessions; the agent serves an installed app's static files itself, at `/apps/<id>/` — see [Hosting & Sessions](hosting-and-sessions.md).

## Example applications

`apps/examples/` contains three reference applications, each proving a different part of the surface:

| Example | What it demonstrates |
|---|---|
| [`greeter`](example-walkthrough.md) | The minimal case: a single static `index.html`, no build step, one permitted workflow (`hello_patchcord`). Proves exactly what `patchcord app install` itself proves. |
| `dashboard` | An operator dashboard built with React, MUI and the TypeScript SDK: browse installed workflows and their steps, browse installed applications, and trigger a run with a live, step-by-step view of its progress. Deliberately calls the API without an app session — see [Building an App with the TS SDK](building-with-sdk-ts.md#an-app-that-deliberately-skips-sessions). |
| `users_list` | Another static, no-build-step example, permitted to run `http_api_task` — a second data point for the "plain HTML" end of the spectrum alongside `greeter`. |

## Where to go next

- [Concepts](concepts.md) — the session-limited hosting model.
- [App Manifest](manifest.md) — `patchcord-app.yaml` field by field.
- [Hosting & Sessions](hosting-and-sessions.md) — installing, packaging, and how a session's permissions are enforced.
- [Building an App with the TS SDK](building-with-sdk-ts.md) — acquiring a session from application code.
- [Example Walkthrough](example-walkthrough.md) — install and run `greeter` end to end.
