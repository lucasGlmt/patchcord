# Installation

There is no published binary or package yet. Build from source.

## Build the agent

```bash
go build -o bin/patchcord ./cmd/patchcord
# or, equivalently:
make build
```

This produces `bin/patchcord`.

## Build the agent and every example plugin

```bash
make build-all
```

This runs `make build` plus `make build-plugins`, which builds each directory under `plugins/examples/` into `bin/plugins/<name>` (e.g. `bin/plugins/text`, `bin/plugins/postgresql`). You need these if you plan to follow [Writing a Plugin in Go](../plugins/writing-a-plugin-go.md) or run a workflow that uses one of the example plugins.

## Requirements

- Go (a version satisfying `go.mod`).
- No Node.js or other runtime is required to build or run `patchcord` itself — only `sdk/typescript` and the app examples under `apps/examples/` need Node.

## Verify

```bash
bin/patchcord --help
```
