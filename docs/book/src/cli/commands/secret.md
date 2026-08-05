# patchcord secret

Write and delete secret values in the `keychain` and `file` stores directly — the two [reference types](../../plugins/connectors/secrets-and-validation.md) beyond `env` a connector's `--secret name=type:key` or a workflow's `webhook` trigger can point at ([ADR-0040](../../../../adr/0040-secret-providers-keychain-et-fichier-aes.md)).

Like `connector` and `auth token`, these commands touch the OS keychain or the file vault directly — never over HTTP.

A secret set here is independent of any one connector: the same `file:PG_PASSWORD` key can be referenced by several connectors' `--secret` flags. There is no `secret list` — the OS keychain library this build uses has no portable way to list all entries under a service, and listing only the `file` vault's keys would break the symmetry between the two adapters.

## `keygen`

```bash
patchcord secret keygen > ./secrets.key
```

Prints a new random base64 AES-256 master key on stdout, once, with no side effect — redirect it yourself to the file `--secrets-master-key-file` (or `PATCHCORD_SECRETS_MASTER_KEY_FILE`) points at. Needed only for the `file` store; `keychain` needs no master key.

There is no recovery for a lost key — only generating another one. Every secret already encrypted under the old key becomes unreadable.

## `set <key> --type <keychain|file>`

```bash
printf '%s' "$PG_PASSWORD" | patchcord secret set --type file PG_PASSWORD \
  --secrets-master-key-file ./secrets.key

printf '%s' "$OPENAI_API_KEY" | patchcord secret set --type keychain OPENAI_API_KEY
```

Reads the value from **stdin**, never a flag — a flag would leak the value into the shell history and the process list (`ps`). `--secrets-master-key-file` is required when `--type file`; unused for `--type keychain`.

Overwrites any existing value under the same `key`.

## `remove <key> --type <keychain|file>`

```bash
patchcord secret remove --type file PG_PASSWORD --secrets-master-key-file ./secrets.key
```

Deletes the stored value. Fails if `key` was never set — a typo'd key silently no-op'ing would be a worse failure mode than an explicit error.

## `--data-dir` and `--secrets-master-key-file`

Both `set` and `remove` take `--data-dir` (default: a per-user system directory, [ADR-0052](../../../../adr/0052-defaut-data-dir-dossier-standard-du-systeme.md); only relevant to `--type file`, since that's where `secrets.vault` lives) and `--secrets-master-key-file` (required for `--type file`) as plain flags — same convention as `connector`/`workflow run`, see [Configuration](../configuration.md).
