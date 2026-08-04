# Introduction

Patchcord is a local-first, universal execution agent written in Go. It loads and supervises **plugins** (independent processes, never statically linked), manages **connectors** (persistent access configurations to external systems), executes **actions** (atomic operations), and orchestrates **workflows** (declarative, versioned, immutable once published). It exposes a stable public API (HTTP/SSE/WebSocket) consumed by a CLI, a TypeScript SDK, and third-party applications.

Guiding principle:

> The core provides the mechanisms. Plugins provide the capabilities. Workflows provide the orchestration. Applications provide the experience.

## Vocabulary

These terms are used with a precise meaning throughout the API, the CLI, and this documentation. Do not read them as synonyms for generic terms like "module" or "job".

| Term | Definition |
|---|---|
| **Plugin** | An independent process, launched and supervised by the agent, communicating over RPC. Never loaded in the core's memory. |
| **Connector** | A persistent access configuration to an external system (e.g. a PostgreSQL connection). Not an action. |
| **Action** | An atomic operation executable by a workflow (e.g. `postgresql.query@1`). Declares inputs, outputs, capabilities, timeout, and known errors. |
| **Workflow** | A declarative orchestration of actions, versioned and immutable once published, serialized as YAML/JSON. |
| **Application** | A client of the agent (web, desktop, ...) using only the public API and the TypeScript SDK. Never runs with full admin privileges. |
| **Run** | An execution instance of a workflow. States: `queued`, `running`, `succeeded`, `failed`, `cancelled`. |
| **Package** | A distributable archive of a plugin (`.patchcord-plugin`) or application (`.patchcord-app`) that `plugin install`/`app install` can install directly, produced by `plugin pack`/`app pack`. A workflow's package (`.patchcord-workflow`) is just its plain YAML file — no archive involved. |
| **Bundle** | A `.patchcord-bundle` package that groups an application, its workflows, and its plugin dependencies into one installable unit. See [Bundles](bundles/index.md). |

## How this book is organized

This book covers the parts of Patchcord you build against or operate day to day:

- **[CLI](cli/index.md)** — installing, configuring, and running the `patchcord` binary, and the full command reference.
- **[Plugins](plugins/index.md)** — the plugin protocol, writing a plugin with the Go SDK, and the connector model plugins expose.
- **[Workflows](workflows/index.md)** — the workflow format, run lifecycle, triggering, and real-time events.
- **[SDK TypeScript](sdk-ts/index.md)** — the official TypeScript client for the public API.
- **[Apps](apps/index.md)** — building and hosting applications on top of the agent.
- **[Bundles](bundles/index.md)** — grouping an app, its workflows, and its plugin dependencies into one installable package.

## What this book is not

This book documents *how to use* Patchcord's public surfaces. It does not cover *why* the architecture is shaped this way — for that, see [`PATCHCORD_VISION_ARCHITECTURE.md`](../../PATCHCORD_VISION_ARCHITECTURE.md) and the [Architecture Decision Records](../../adr/) in `docs/adr/`. When a page here needs to justify a design choice, it links to the relevant ADR instead of repeating the reasoning.
