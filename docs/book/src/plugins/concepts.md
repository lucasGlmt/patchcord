# Concepts

## Why a separate process

Native Go plugins (`plugin` package, `.so`) would tie a plugin's version tightly to the core binary, not survive a plugin crash, force every plugin to be written in Go, and require recompilation or strict binary compatibility on every core change. Patchcord plugins are external processes instead ([ADR-0002](../../../adr/0002-greffons-processus-externes.md)):

- A plugin crash is contained by the [Supervisor](supervision.md) and never propagates to the agent.
- A plugin can be written in any language able to speak the [protocol](protocol.md) — not just Go.
- A plugin's lifecycle (install, update, uninstall) is independent of the core's — no agent recompilation.
- Compatibility is negotiated explicitly at [handshake](protocol.md#handshake) time.

The tradeoff: inter-process communication costs more than an in-process function call, and the [Plugin Supervisor](supervision.md) has to own real operational complexity (launch, health checks, restart, quarantine) that a native plugin wouldn't need.

## Plugin, action, connector — not synonyms

These three are easy to conflate but distinct (full definitions in the [Introduction](../introduction.md#vocabulary)):

- A **plugin** is the process. It contributes one or more actions and, optionally, one or more connector types.
- An **action** (e.g. `postgresql.query@1`) is one atomic, callable operation a plugin contributes.
- A **connector** is a persistent, named *configuration* (host, credentials, ...) that an action can be bound to at run time. A plugin *declares* which connector types its actions accept ([Connectors](connectors/index.md)); it does not itself hold any particular connector instance.

`postgresql` illustrates all three at once: it is one plugin, contributing two actions (`postgresql.query@1`, `postgresql.execute@1`) that both require a connector of type `postgresql.connection@1` — a type declared by the plugin, but instantiated by the user via `patchcord connector create`.
