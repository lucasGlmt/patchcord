# Getting Started

## Install

```bash
npm install @glmtsolutions/patchcord-sdk
```

`patchcord app new --template vite` and `patchcord bundle new --template vite` already declare this dependency in the scaffolded `package.json` — a fresh `npm install` in a generated project is enough, no manual wiring needed.

Requires Node >=18 (for the global `fetch`) if run outside a browser.

### Working inside the monorepo

Code that lives in this repository (`apps/examples/dashboard`, `bundles/examples/*`) consumes the SDK from source instead, as a `file:` dependency, so a local change to `sdk/typescript` is picked up without a publish round trip:

```json
{
  "dependencies": {
    "@glmtsolutions/patchcord-sdk": "file:../../../sdk/typescript"
  }
}
```

Point the path at `sdk/typescript` relative to your `package.json`, then build the SDK once so `dist/` exists:

```bash
cd sdk/typescript
npm install
npm run build
```

## Instantiate the client

```ts
import { PatchcordClient } from "@glmtsolutions/patchcord-sdk";

const client = new PatchcordClient({
  baseUrl: "http://127.0.0.1:7331", // wherever `patchcord serve` is listening
});
```

`baseUrl` is the only required option. See [Building an App with the TS SDK](../apps/building-with-sdk-ts.md) for the `token` option, which limits the client to an installed application's declared permissions.

## Hello world

Assuming the agent has `hello_patchcord` installed (`patchcord workflow install`, or the [reference plugin](../plugins/index.md)'s example workflow):

```ts
const run = await client.workflows.run("hello_patchcord", {
  inputs: { text: "hello" },
});

console.log(`Run ${run.id} started, status: ${(await run.fetch()).status}`);

const result = await run.result(); // waits for a terminal status
console.log(result.status, result.outputs);
```

`workflows.run` starts the run and returns immediately — the agent doesn't wait for any step to execute (`POST /v1/workflows/{id}/run` is asynchronous, [ADR-0024](../../../adr/0024-declenchement-asynchrone-workflows-api-http.md)). Continue to [Actions & Workflows](actions-and-workflows.md) for the rest of the `Run` API, and [Events (SSE)](events.md) to observe a run live instead of just waiting on its result.
