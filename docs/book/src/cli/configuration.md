# Configuration

`patchcord serve` accepts its settings from four layered sources, in increasing order of precedence: a built-in default, a `--config` YAML file, a `PATCHCORD_*` environment variable, then an explicitly passed flag ([ADR-0038](../../../adr/0038-configuration-serveur-fichier-yaml-precedence.md)). A flag you actually typed always wins over everything else; an unset flag falls through to the next source. Every other command (`plugin`, `connector`, `workflow`, `run`, `app`, `bundle`, `auth`, `trust`, `registry`, `secret`) takes `--data-dir` as a plain flag too, and also honors `PATCHCORD_DATA_DIR` — flag beats environment variable beats the built-in default ([ADR-0049](../../../adr/0049-patchcord-data-dir-etendue-aux-commandes-ponctuelles.md)). They do not read a `--config` file, unlike `serve` — that layer stays scoped to `serve` alone.

## `serve`'s layered settings

| Setting | Flag | Environment variable | Config file key | Default |
|---|---|---|---|---|
| Listen address | `--listen` | `PATCHCORD_LISTEN` | `listen` | `127.0.0.1:7331` |
| Data directory | `--data-dir` | `PATCHCORD_DATA_DIR` | `data_dir` | a per-user system directory ([ADR-0052](../../../adr/0052-defaut-data-dir-dossier-standard-du-systeme.md)) |
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

Present on nearly every command (`serve`, and every subcommand of `plugin`, `connector`, `workflow`, `run`, `app`, `bundle`, `auth`, `trust`, `registry`, `secret`). It is the directory holding the agent's SQLite database. Precedence is the same everywhere: an explicit `--data-dir` beats `PATCHCORD_DATA_DIR` beats the built-in default ([ADR-0049](../../../adr/0049-patchcord-data-dir-etendue-aux-commandes-ponctuelles.md)) — only `serve` additionally accepts it through a `--config` file, per the layered settings above.

Left unset, the built-in default is a per-user system directory, the same regardless of which directory a command happens to be run from ([ADR-0052](../../../adr/0052-defaut-data-dir-dossier-standard-du-systeme.md)): `~/Library/Application Support/patchcord` on macOS, `$XDG_DATA_HOME/patchcord` (or `~/.local/share/patchcord`) on Linux/BSD, `%LOCALAPPDATA%\patchcord` on Windows. Every command run by the same user resolves the same database by default, without exporting anything — set `--data-dir`/`PATCHCORD_DATA_DIR` explicitly whenever you want an isolated database instead (e.g. one bundle under active development, or a test run).

- The database is created and migrated automatically the first time any command touches it — there is no separate `patchcord init` or `migrate` step.
- One-shot commands (everything except `serve`) open this database directly, run their migrations silently, and close it when done. Migration output only appears in `patchcord serve`'s structured logs, never mixed into a one-shot command's output.
- Pointing two different invocations at the same `--data-dir` is safe (SQLite WAL mode), including a running `patchcord serve` and a concurrent `patchcord plugin list` — see [CLI Overview](index.md#a-one-shot-command-does-not-talk-to-a-running-agent) for what "safe" does and does not mean here.

```bash
# no --data-dir needed for these two to share the same database — both
# resolve the same built-in per-user default
patchcord plugin list
patchcord serve

# isolate a bundle under active development in its own database instead
export PATCHCORD_DATA_DIR=./bundle-under-dev/data
patchcord bundle dev ./bundle-under-dev --watch
patchcord plugin list
```

## `--secrets-master-key-file`

Present on `serve` (layered, see above) and on the commands that resolve a `file` secret reference directly (`connector inspect`, `connector test`, `workflow run`, `secret set --type file`, `secret remove --type file`) — on these, a plain flag only, same convention as `--data-dir` outside of `serve`. Points at the file holding the base64 AES-256 master key `secrets.FileStore` decrypts `<data-dir>/secrets.vault` with (see [Plugins → Connectors → Secrets & Validation](../plugins/connectors/secrets-and-validation.md) and [ADR-0040](../../../adr/0040-secret-providers-keychain-et-fichier-aes.md)). Generate one with `patchcord secret keygen > /path/to/key`. Left unset, `file` secret references simply don't resolve — `env` and `keychain` are unaffected.

## Secret references

Beyond `--secrets-master-key-file`, the only other environment variable convention in the CLI is whatever an `env`-type connector's or workflow trigger's own secret reference points at (see [Plugins → Connectors → Secrets & Validation](../plugins/connectors/secrets-and-validation.md)) — unrelated to `serve`'s own configuration above.

## Command-specific flags

Some commands take additional flags scoped to their own behavior (e.g. `connector create --type`, `workflow run --input`). These are documented on their own page under [Command Reference](commands/index.md), not here.
