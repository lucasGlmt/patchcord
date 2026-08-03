# Building an App with the TS SDK

An [application](index.md) talks to the agent exclusively through `@patchcord/sdk` (see [SDK TypeScript](../sdk-ts/index.md)) — never through hand-rolled `fetch` calls against `/v1/*`. This page covers the one pattern specific to running *as* an installed application: acquiring a session limited to your own declared permissions — and, just as importantly, when an application legitimately has no use for one.

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

Every request this second client makes sends `Authorization: Bearer <token>`. Only `POST /v1/workflows/{id}/run` accepts an app session — the agent limits that call to the workflow ids the session's app manifest declared under `workflows_run`, rejecting anything else with `403` ([ADR-0026](../../../adr/0026-applications-manifeste-hebergement-session-limitee.md)). Every other route (listing runs, listing workflows, ...) requires a full admin token instead, once one exists ([ADR-0036](../../../adr/0036-authentification-admin-jetons-opt-in.md)) — an app session is intentionally not enough to call them. See [Hosting & Sessions](hosting-and-sessions.md).

## Full example

`apps/examples/greeter/index.html` runs a workflow through exactly this pattern: it fetches an app session for `"greeter"`, then uses the resulting client to trigger `hello_patchcord` and poll its result. Read it alongside [Example Walkthrough](example-walkthrough.md) for the full picture, including the manifest that declares `workflows_run` in the first place.

## An app that deliberately skips sessions

A session's permissions are a fixed allow-list (`workflows_run: [...]`) — a good fit for an application that only ever needs to run one or two specific workflows, like `greeter`. It is a poor fit for an application whose entire purpose is to browse and run **any** installed workflow, since the manifest would need to be kept in sync with the agent's catalog by hand.

`apps/examples/dashboard` (a React + MUI operator dashboard) is exactly that second case: its manifest declares an empty `permissions.workflows.run: []`, and its code never calls `client.apps.createSession` — it builds a plain `new PatchcordClient({ baseUrl })` with no `token`, in `src/App.tsx`. This is not a workaround or a lesser form of access: a request with no `Authorization` header is treated identically to one from a browser tab hitting the agent directly, which already has the same unrestricted access today (see [Hosting & Sessions](hosting-and-sessions.md#sessions) and [Concepts](concepts.md#additive-never-a-new-gate)). Installing the dashboard as a real application only changes where it's served from (`/apps/dashboard/`), never what it's allowed to do.

Use this pattern for an internal or operator-facing tool where session-scoped restriction adds friction without adding safety; use a session (above) for anything meant to run with a deliberately narrower blast radius than the agent's full API.
