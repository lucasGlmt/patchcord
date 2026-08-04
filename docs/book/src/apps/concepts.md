# Concepts

## An app never gets the agent's full privileges

Admin authentication ([ADR-0036](../../../adr/0036-authentification-admin-jetons-opt-in.md)) is opt-in — a fresh agent answers every request unauthenticated until an admin token exists — and full, unscoped access is not what an application should hold anyway. Applications get something narrower instead, deliberately built to be additive rather than a workaround for the admin-auth gap that predated it ([ADR-0024](../../../adr/0024-declenchement-asynchrone-workflows-api-http.md)): a **session**, issued for one installed app id, carrying exactly the permissions that app's manifest declared — nothing else, and never the ability to do anything a session-less request couldn't already do while admin auth is off ([ADR-0026](../../../adr/0026-applications-manifeste-hebergement-session-limitee.md)).

```text
patchcord-app.yaml          →  AppPermissions{WorkflowsRun: [...]}
       │  Install                       │  Issue
       ▼                                ▼
   apps table              →      auth.Session{AppID, Permissions, IssuedAt}
```

## What a session actually restricts

Today, exactly one thing: `POST /v1/workflows/{id}/run`. `withRunAuth` (`internal/api/adminauth.go`) wraps that route; `Session.CanRunWorkflow(workflowID)` checks the requested workflow id against `Permissions.WorkflowsRun`. Every other route (listing runs, listing workflows, reading a connector, ...) is admin-gated by `withAdminAuth` instead and never even looks at a session — there is no broader session-based enforcement point yet. This mirrors a pattern already established for plugins: `plugins.CatalogEntry.Permissions` is recorded but unchecked too — declaring a permission ahead of an enforcement point that doesn't exist yet would be validation with nothing to validate.

## Additive, never a new gate — until admin auth is opted into

`withRunAuth` has two modes, tracking the same `auth.AnyTokensExist` flag `withAdminAuth` gates on ([ADR-0036](../../../adr/0036-authentification-admin-jetons-opt-in.md)):

- **No admin token created yet** (the default): a request with no `Authorization` header behaves exactly as it did before sessions existed — `withRunAuth` only checks anything when a bearer token is actually present. A present-but-invalid token is `401`; a valid session requesting a workflow outside its permissions is `403`; a valid, permitted session passes through unchanged.
- **At least one admin token exists**: `POST /v1/workflows/{id}/run` now requires *some* credential — an admin token (full access, no permission check) or a valid app session scoped to that workflow. No credential at all is `401` in this mode, unlike the default one.

Either way, no existing caller of the public API is affected by an application session ever being issued to someone else — a session only ever narrows what its own bearer token can do, never what anyone else's request is allowed.

## Sessions are not durable

`auth.Store` (`internal/auth/session.go`) is an in-memory map guarded by a mutex — a token (`uuid.NewString()`) to `Session{AppID, Permissions, IssuedAt}`. It is not persisted and not revocable: the agent restarts with it empty, and an application whose session was still open must request a new one. This is a deliberate, minimal first slice, not an oversight — see [ADR-0026](../../../adr/0026-applications-manifeste-hebergement-session-limitee.md)'s "conséquences négatives" for what a server, multi-user deployment would need instead (TTL, revocation, persistence).
