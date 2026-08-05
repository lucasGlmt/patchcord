# Patchcord

[![CI](https://github.com/lucasGlmt/patchcord/actions/workflows/ci.yml/badge.svg)](https://github.com/lucasGlmt/patchcord/actions/workflows/ci.yml)
[![Release](https://img.shields.io/github/v/release/lucasGlmt/patchcord?include_prereleases)](https://github.com/lucasGlmt/patchcord/releases)
[![License](https://img.shields.io/badge/license-Apache%202.0-blue.svg)](LICENSE)

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

## Installing

Prebuilt binaries for linux/darwin/windows (amd64/arm64) are attached to
every [GitHub Release](https://github.com/lucasGlmt/patchcord/releases).

```bash
# macOS, or Linux via Linuxbrew
brew install lucasGlmt/patchcord/patchcord

# Debian/Ubuntu — download the .deb from the release page, then
sudo dpkg -i patchcord_*_linux_amd64.deb

# Fedora/RHEL — download the .rpm from the release page, then
sudo rpm -i patchcord_*_linux_amd64.rpm
```

There is deliberately no hosted apt/dnf repository or Chocolatey package —
see [ADR-0057](docs/adr/0057-distribution-homebrew-et-paquets-linux.md)
for why.

**macOS**: the binary isn't code-signed (no Apple Developer ID — see
ADR-0057), so Gatekeeper blocks the first run of a `brew`-installed or
directly downloaded copy (`patchcord: rejected` / the process just dies).
Clear the quarantine flag once, after installing:

```bash
xattr -d com.apple.quarantine "$(which patchcord)"
```

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

`make build`/`make build-all` embed a version into the binary (see
Versioning below); a plain `go build` falls back to `dev`.

## Versioning

Patchcord follows [SemVer](https://semver.org/) (`vX.Y.Z`); pre-1.0, any
`0.x` bump may contain breaking changes. `patchcord version` prints the
binary's version, commit and build date; `patchcord --version` prints the
short form; `GET /v1/system/health` reports it as `version` for running
instances.

Releases are cut by pushing a `vX.Y.Z` tag: `make changelog` regenerates
[`CHANGELOG.md`](CHANGELOG.md) from
[Conventional Commits](https://www.conventionalcommits.org/), then
`git tag vX.Y.Z && git push --tags` hands off to CI
([`.github/workflows/release.yml`](.github/workflows/release.yml)), which
cross-compiles and publishes a GitHub Release via
[goreleaser](.goreleaser.yaml). See
[ADR-0056](docs/adr/0056-versionnement-du-binaire-agent.md) for the full
decision.

## License

Apache License 2.0 — see [`LICENSE`](LICENSE).
