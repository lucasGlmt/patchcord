# Configuration

There is no configuration file. Everything is a command-line flag; there is no environment variable convention either, except for whatever a connector's own secret references point at (see [Plugins → Connectors → Secrets & Validation](../plugins/connectors/secrets-and-validation.md)).

## `--data-dir`

Present on nearly every command (`serve`, and every subcommand of `plugin`, `connector`, `workflow`, `run`, `app`). Defaults to `./data`. It is the directory holding the agent's SQLite database.

- The database is created and migrated automatically the first time any command touches it — there is no separate `patchcord init` or `migrate` step.
- One-shot commands (everything except `serve`) open this database directly, run their migrations silently, and close it when done. Migration output only appears in `patchcord serve`'s structured logs, never mixed into a one-shot command's output.
- Pointing two different invocations at the same `--data-dir` is safe (SQLite WAL mode), including a running `patchcord serve` and a concurrent `patchcord plugin list` — see [CLI Overview](index.md#a-one-shot-command-does-not-talk-to-a-running-agent) for what "safe" does and does not mean here.

```bash
patchcord plugin list --data-dir ./data
patchcord serve --data-dir ./data
```

## `serve`-only flags

`--listen` (default `127.0.0.1:7331`) — see [Serving the Agent](serve.md).

## Command-specific flags

Some commands take additional flags scoped to their own behavior (e.g. `connector create --type`, `workflow run --input`). These are documented on their own page under [Command Reference](commands/index.md), not here.
