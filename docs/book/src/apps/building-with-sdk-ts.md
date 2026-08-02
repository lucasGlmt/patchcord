# Building an App with the TS SDK

An [application](index.md) talks to the agent exclusively through `@patchcord/sdk` (see [SDK TypeScript](../sdk-ts/index.md)) — never through hand-rolled `fetch` calls against `/v1/*`. `apps/examples/dashboard` is the fuller reference; this page covers the one pattern specific to running *as* an installed application: acquiring a session limited to your own declared permissions.

## Acquiring a session

An app's manifest (`patchcord-app.yaml`) declares which workflows its sessions may run (see [App Manifest](manifest.md)). At runtime, ask the agent for a session token scoped to those permissions with `client.apps.createSession`:

```ts
import { PatchcordClient } from "@patchcord/sdk";

async function fetchAppSessionToken(baseUrl: string, appId: string): Promise<string | undefined> {
  try {
    const session = await new PatchcordClient({ baseUrl }).apps.createSession(appId);
    return session.token;
  } catch {
    // Not installed under this id yet (e.g. running via `npm run dev`
    // before `patchcord app install`) — fall back to the agent's
    // unrestricted default rather than failing local development.
    return undefined;
  }
}
```

Then construct the client you actually use for the rest of the page with that token:

```ts
const token = await fetchAppSessionToken(baseUrl, "dashboard");
const client = new PatchcordClient({ baseUrl, token });

const run = await client.workflows.run("hello_patchcord", { inputs: { text: "hi" } });
```

Every request this second client makes sends `Authorization: Bearer <token>`. Only `POST /v1/workflows/{id}/run` currently checks it — the agent limits that call to the workflow ids the session's app manifest declared under `workflows_run`, rejecting anything else with `403` ([ADR-0026](../../../adr/0026-applications-manifeste-hebergement-session-limitee.md)). Every other route (listing runs, listing workflows, ...) behaves the same with or without a token today, since the agent has no broader authentication story yet — see [Hosting & Sessions](hosting-and-sessions.md).

## Full example

`apps/examples/dashboard/src/main.ts` runs a workflow through exactly this pattern: it fetches an app session for `"dashboard"`, falls back to no token if the app isn't installed, then uses the resulting client to trigger a run and stream its [events](../sdk-ts/events.md). Read it alongside [Example Walkthrough](example-walkthrough.md) for the full picture, including the manifest that declares `workflows_run` in the first place.
