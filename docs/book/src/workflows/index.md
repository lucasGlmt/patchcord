# Workflows Overview

A workflow is a declarative orchestration of [actions](../plugins/concepts.md): a sequence of steps, each invoking one action contributed by an installed plugin, with a step's output available to any later step via a `${{ ... }}` expression. Workflows are versioned and immutable once published ([ADR-0008](../../../adr/0008-workflows-publies-immuables.md)) — installing a workflow never edits an existing version in place, it adds a new one.

The engine lives in `internal/workflow/` (parsing, validation, expressions, the `Run`/`Step` state machines) and has no knowledge of persistence or of how actions actually execute. `internal/runs/` is the run manager built on top of it: it persists workflow versions, runs and steps, and orchestrates execution against an `ActionExecutor` (`internal/plugins.Supervisor` in practice).

## Where to go next

- [Concepts](concepts.md) — versioning and immutability.
- [Workflow Format](format.md) — the YAML structure, validation, and the expression language.
- [Runs](runs.md) — the `Run` and `Step` state machines.
- [Triggering Workflows](triggering.md) — manual (CLI/HTTP), `schedule` and `webhook` triggers.
- [Events](events.md) — watching a run's progress in real time.
- [Timeouts & Cancellation](timeouts-and-cancellation.md) — how a step is bounded and how a run stops early.
