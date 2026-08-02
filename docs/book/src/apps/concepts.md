# Concepts

## An app never gets the agent's full privileges

Every request the public API serves today has no admin authentication at all ([ADR-0024](../../../adr/0024-declenchement-asynchrone-workflows-api-http.md), `internal/api/router.go`'s `withCORS` doc comment). Applications get something narrower, deliberately built to be additive rather than a workaround for that gap: a **session**, issued for one installed app id, carrying exactly the permissions that app's manifest declared — nothing else, and never the ability to do anything a session-less request couldn't already do ([ADR-0026](../../../adr/0026-applications-manifeste-hebergement-session-limitee.md)).

```text
patchcord-app.yaml          →  AppPermissions{WorkflowsRun: [...]}
       │  Install                       │  Issue
       ▼                                ▼
   apps table              →      auth.Session{AppID, Permissions, IssuedAt}
```

## What a session actually restricts

Today, exactly one thing: `POST /v1/workflows/{id}/run`. `withOptionalAppSession` (`internal/api/apps.go`) wraps that route; `Session.CanRunWorkflow(workflowID)` checks the requested workflow id against `Permissions.WorkflowsRun`. Every other route (listing runs, listing workflows, reading a connector, ...) behaves identically whether or not a token is presented — there is no broader enforcement point yet. This mirrors a pattern already established for plugins: `plugins.CatalogEntry.Permissions` is recorded but unchecked too — declaring a permission ahead of an enforcement point that doesn't exist yet would be validation with nothing to validate.

## Additive, never a new gate

A request with no `Authorization` header behaves exactly as it did before sessions existed — `withOptionalAppSession` only checks anything when a bearer token is actually present. A present-but-invalid token is `401`; a valid token requesting a workflow outside its permissions is `403`; a valid, permitted token passes through to the handler unchanged. No existing, unauthenticated caller of the public API is affected by an application session ever being issued to someone else.

## Sessions are not durable

`auth.Store` (`internal/auth/session.go`) is an in-memory map guarded by a mutex — a token (`uuid.NewString()`) to `Session{AppID, Permissions, IssuedAt}`. It is not persisted and not revocable: the agent restarts with it empty, and an application whose session was still open must request a new one. This is a deliberate, minimal first slice, not an oversight — see [ADR-0026](../../../adr/0026-applications-manifeste-hebergement-session-limitee.md)'s "conséquences négatives" for what a server, multi-user deployment would need instead (TTL, revocation, persistence).
