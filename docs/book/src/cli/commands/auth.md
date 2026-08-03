# patchcord auth

Manage admin tokens — full, unscoped bearer credentials for the public HTTP API ([ADR-0036](../../../../adr/0036-authentification-admin-jetons-opt-in.md)). Distinct from an [application session](../../apps/hosting-and-sessions.md), which is limited to one installed app's declared permissions.

A fresh agent answers every request unauthenticated, exactly as before this command existed. Creating the very first admin token switches the entire API — every route except `GET /v1/system/health`, `GET /v1/openapi.json` and `GET /apps/{id}/`'s static content — to requiring a valid one, even for requests coming from `127.0.0.1`. There is no separate flag or config setting for this: whether at least one token exists **is** the setting.

Tokens can only be managed here, over the CLI — there is no HTTP endpoint to create one. The very first token could never pass through an API that would already require one.

## `token create <name>`

```bash
patchcord auth token create ci
```

Prints the plaintext token once, right after creation:

```text
Created admin token "ci" (id 3f9a2e1c-...)

  pcat_kZ3x9mP2qR8vN1wL6yT4uJ7hF0sD5bA...

Save it now — it will not be shown again. Pass it as "Authorization: Bearer <token>".
```

There is no recovery for a lost token — only creating another one and revoking the lost one (`token revoke`). Only its hash is stored; `token list` never shows it again either.

## `token list`

```bash
patchcord auth token list
```

Prints one line per token: `<id>  <name>  <created-at>` — never the plaintext or its hash. Prints a note that the API is fully open when no token has been created yet.

## `token revoke <id>`

```bash
patchcord auth token revoke 3f9a2e1c-...
```

Deletes the token, identified by the `id` `token list` prints (not its `name`, which isn't unique). Revoking the *last* remaining admin token switches the API back to today's default-open behavior — the same state a fresh agent starts in.
