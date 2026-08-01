# Patchcord

> The extensible runtime for integrations, workflows and intelligent apps.
> **Connect anything. Automate everything. Build on top.**

Patchcord is a local-first, universal execution agent written in Go. It loads
and supervises independent plugins, manages connectors, executes atomic
actions, and orchestrates versioned, declarative workflows — while exposing a
stable public API consumed by a CLI, a TypeScript SDK, and third-party
applications.

> Patchcord Agent is the fundamental product. Plugins provide capabilities.
> Workflows provide orchestration. Applications provide the experience.

## Status

Early stage — Phase 1 (Core minimal) of the roadmap. No stable release yet.
See [`docs/PATCHCORD_VISION_ARCHITECTURE.md`](docs/PATCHCORD_VISION_ARCHITECTURE.md)
for the full product and architecture vision, and [`docs/adr/`](docs/adr/)
for the architecture decision records.

## Repository layout

This is a monorepo, with boundaries treated as if each component already
lived in its own repository:

```text
cmd/patchcord/     entry point of the main binary
internal/          core runtime — never imported by plugins/, sdk/, or apps/
api/               public contracts (OpenAPI/Protobuf) for the agent, plugin
                   protocol, workflow format, and app manifests
sdk/go-plugin/     official Go SDK for writing a plugin
sdk/typescript/    official TypeScript SDK for building applications
plugins/examples/  example plugins
apps/examples/     example applications
docs/              vision document and architecture decision records
migrations/        database schema migrations
```

## Non-negotiables

- The core never imports a concrete business integration (no Gmail, OpenAI,
  Postgres driver, etc. inside `internal/`) — capabilities arrive only
  through the plugin protocol.
- A plugin depends only on the public protocol and `sdk/go-plugin` — never on
  `internal/`.
- Public boundaries (client API, plugin protocol, package/workflow formats)
  are versioned contracts (Protobuf / JSON Schema / OpenAPI).
- The CLI, applications, and dashboards all call the same internal services
  as the public API — never duplicated logic.
- The cloud is always optional; no core feature requires a remote account.

See [`CLAUDE.md`](CLAUDE.md) for the full set of contribution rules.

## Building

```bash
go build ./...
go vet ./...
go test ./...
```

## License

Apache License 2.0 — see [`LICENSE`](LICENSE).
