# @glmtsolutions/patchcord-sdk

Official TypeScript SDK for [Patchcord](https://github.com/lucasglmt/patchcord)'s public HTTP API — the same API the CLI and the agent's own reference apps are built on (ADR-0005: CLI, API, and every application use the same application layer, never a duplicated one).

## Install

```bash
npm install @glmtsolutions/patchcord-sdk
```

Requires Node >=18 (for the global `fetch`) if run outside a browser.

## Usage

```ts
import { PatchcordClient } from "@glmtsolutions/patchcord-sdk";

const client = new PatchcordClient({
  baseUrl: "http://127.0.0.1:7331", // wherever `patchcord serve` is listening
});

const run = await client.workflows.run("hello_patchcord", {
  inputs: { text: "hello" },
});

const result = await run.result(); // waits for a terminal status
console.log(result.status, result.outputs);
```

For a live progress view instead of just waiting, use `run.watch()` — it yields a fully merged `RunSnapshot` (status, error, steps) after every event instead of raw deltas:

```ts
for await (const snapshot of run.watch()) {
  console.log(snapshot.status, snapshot.steps);
}
```

See the full guide in the docs book: [Getting Started with the TypeScript SDK](https://github.com/lucasglmt/patchcord/blob/main/docs/book/src/sdk-ts/getting-started.md) and [Events (SSE)](https://github.com/lucasglmt/patchcord/blob/main/docs/book/src/sdk-ts/events.md). Building a React UI on top of this? [`@glmtsolutions/patchcord-react`](https://github.com/lucasglmt/patchcord/tree/main/sdk/typescript-react) wraps `watch()` into a `useWorkflowRun` hook.

## Scope

This package covers every `/v1/*` route the agent exposes: workflows, runs (including live events over SSE), connectors, plugins, and application sessions. It does not know about any concrete business service (Patchcord's core never does either — ADR-0004) — connectivity to a specific external system is a plugin's job, not this SDK's.
