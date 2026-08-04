# Configuration

`patchcord serve` accepts its settings from four layered sources, in increasing order of precedence: a built-in default, a `--config` YAML file, a `PATCHCORD_*` environment variable, then an explicitly passed flag ([ADR-0038](../../../adr/0038-configuration-serveur-fichier-yaml-precedence.md)). A flag you actually typed always wins over everything else; an unset flag falls through to the next source. Every other command (`plugin`, `connector`, `workflow`, `run`, `app`, `secret`) only takes `--data-dir` (and, where relevant, `--secrets-master-key-file`) as plain flags — no file, no environment variable, except where noted below.

## `serve`'s layered settings

| Setting | Flag | Environment variable | Config file key | Default |
|---|---|---|---|---|
| Listen address | `--listen` | `PATCHCORD_LISTEN` | `listen` | `127.0.0.1:7331` |
| Data directory | `--data-dir` | `PATCHCORD_DATA_DIR` | `data_dir` | `./data` |
| Secrets master key file | `--secrets-master-key-file` | `PATCHCORD_SECRETS_MASTER_KEY_FILE` | `secrets_master_key_file` | *(unset — `file` secret references don't resolve)* |

```yaml
# config.yaml
listen: 0.0.0.0:7331
data_dir: /data
```

```bash
patchcord serve --config ./config.yaml
```

A `--config` pointing at a file that doesn't exist fails immediately (`load config file: ...`) — it is never silently skipped. An unknown top-level key in the file (a typo'd `liste:`) is also rejected, the same discipline `workflow.Validate` already applies to workflow YAML.

```bash
# environment variable overrides the file above
PATCHCORD_LISTEN=0.0.0.0:9000 patchcord serve --config ./config.yaml

# an explicit flag overrides everything, including the environment variable
patchcord serve --config ./config.yaml --listen 0.0.0.0:9001
```

`internal/config` (`Load`, `FromEnv`, `Merge`) implements the three non-flag layers; `internal/cli/serve.go` owns the flag layer and the built-in defaults, since only it knows — via cobra's `Flags().Changed()` — whether a flag was actually typed versus left at its default.

## `--data-dir`

Present on nearly every command (`serve`, and every subcommand of `plugin`, `connector`, `workflow`, `run`, `app`). Defaults to `./data`. It is the directory holding the agent's SQLite database. Outside of `serve`, it is a plain flag only — no `PATCHCORD_DATA_DIR` or config file support; a one-shot command's invocation is already explicit enough not to need layered configuration.

- The database is created and migrated automatically the first time any command touches it — there is no separate `patchcord init` or `migrate` step.
- One-shot commands (everything except `serve`) open this database directly, run their migrations silently, and close it when done. Migration output only appears in `patchcord serve`'s structured logs, never mixed into a one-shot command's output.
- Pointing two different invocations at the same `--data-dir` is safe (SQLite WAL mode), including a running `patchcord serve` and a concurrent `patchcord plugin list` — see [CLI Overview](index.md#a-one-shot-command-does-not-talk-to-a-running-agent) for what "safe" does and does not mean here.

```bash
patchcord plugin list --data-dir ./data
patchcord serve --data-dir ./data
```

## `--secrets-master-key-file`

Present on `serve` (layered, see above) and on the commands that resolve a `file` secret reference directly (`connector inspect`, `connector test`, `workflow run`, `secret set --type file`, `secret remove --type file`) — on these, a plain flag only, same convention as `--data-dir` outside of `serve`. Points at the file holding the base64 AES-256 master key `secrets.FileStore` decrypts `<data-dir>/secrets.vault` with (see [Plugins → Connectors → Secrets & Validation](../plugins/connectors/secrets-and-validation.md) and [ADR-0040](../../../adr/0040-secret-providers-keychain-et-fichier-aes.md)). Generate one with `patchcord secret keygen > /path/to/key`. Left unset, `file` secret references simply don't resolve — `env` and `keychain` are unaffected.

## Secret references

Beyond `--secrets-master-key-file`, the only other environment variable convention in the CLI is whatever an `env`-type connector's or workflow trigger's own secret reference points at (see [Plugins → Connectors → Secrets & Validation](../plugins/connectors/secrets-and-validation.md)) — unrelated to `serve`'s own configuration above.

## Command-specific flags

Some commands take additional flags scoped to their own behavior (e.g. `connector create --type`, `workflow run --input`). These are documented on their own page under [Command Reference](commands/index.md), not here.
