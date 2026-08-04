# Command Reference

Every command below accepts `--data-dir` (default `./data`); see [Configuration](../configuration.md). Each takes no arguments beyond what's listed and prints plain, tab-separated or `key: value` text to stdout — there is no `--output json` yet.

| Group | Subcommands |
|---|---|
| [`plugin`](plugin.md) | `new <id>`, `install <path>`, `pack <dir>`, `list`, `inspect <plugin-id>`, `uninstall <plugin-id>` |
| [`connector`](connector.md) | `create <id>`, `list`, `inspect <id>`, `test <id>`, `remove <id>` |
| [`workflow`](workflow.md) | `new <id>`, `install <path.yaml>`, `list`, `validate <path.yaml>`, `export <workflow-id>`, `run <workflow-id>` |
| [`run`](run.md) | `list`, `inspect <run-id>`, `logs <run-id>`, `cancel <run-id>` |
| [`app`](app.md) | `new <id>`, `install <dir-or-package>`, `dev <dir>`, `pack <dir>`, `list`, `remove <id>` |
| [`bundle`](bundle.md) | `new <id>`, `install <path>`, `pack <dir>`, `list`, `inspect <id>` |
| [`key`](key.md) | `generate` |
| [`trust`](trust.md) | `add <id> <pubkey-path>`, `list [id]`, `remove <id> <pubkey-path>` |
| [`auth`](auth.md) | `token create <name>`, `token list`, `token revoke <id>` |
