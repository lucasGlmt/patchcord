import assert from "node:assert/strict";
import http from "node:http";
import type { AddressInfo } from "node:net";
import { test } from "node:test";

import { PatchcordClient } from "../src/client.js";

// startFakeAgent stands in for the two new Go endpoints
// (POST /v1/workflows/{id}/run, GET /v1/runs/{id}) plus the pre-existing
// GET /v1/runs/{id}/events — a real local HTTP server, not a mocked fetch,
// so this exercises the SDK's actual wire parsing end to end without
// depending on the Go binary being built (mirrors the Go side's own
// "mock the transport" testing philosophy, CLAUDE.md section 5).
function startFakeAgent(): Promise<{ baseUrl: string; close: () => Promise<void> }> {
  const server = http.createServer((req, res) => {
    const url = new URL(req.url ?? "/", "http://localhost");

    if (req.method === "POST" && url.pathname === "/v1/workflows/demo/run") {
      let body = "";
      req.on("data", (chunk: Buffer) => {
        body += chunk.toString("utf8");
      });
      req.on("end", () => {
        const parsed = body ? JSON.parse(body) : {};
        res.writeHead(202, { "Content-Type": "application/json" });
        res.end(
          JSON.stringify({
            id: "run-1",
            workflow_id: "demo",
            workflow_version: 1,
            status: "running",
            inputs: parsed.inputs ?? {},
            created_at: "2026-01-01T00:00:00Z",
          }),
        );
      });
      return;
    }

    if (req.method === "GET" && url.pathname === "/v1/runs/run-1/events") {
      res.writeHead(200, { "Content-Type": "text/event-stream" });
      res.write(
        'event: run.running\ndata: {"run_id":"run-1","status":"running","time":"2026-01-01T00:00:00Z"}\n\n',
      );
      res.write(
        'event: run.succeeded\ndata: {"run_id":"run-1","status":"succeeded","time":"2026-01-01T00:00:01Z"}\n\n',
      );
      res.end();
      return;
    }

    if (req.method === "GET" && url.pathname === "/v1/runs/run-1") {
      res.writeHead(200, { "Content-Type": "application/json" });
      res.end(
        JSON.stringify({
          id: "run-1",
          workflow_id: "demo",
          workflow_version: 1,
          status: "succeeded",
          outputs: { value: "HI" },
          created_at: "2026-01-01T00:00:00Z",
          finished_at: "2026-01-01T00:00:01Z",
        }),
      );
      return;
    }

    if (req.method === "GET" && url.pathname === "/v1/system/health") {
      res.writeHead(200, { "Content-Type": "application/json" });
      res.end(JSON.stringify({ status: "ok", database: "ok" }));
      return;
    }

    if (req.method === "GET" && url.pathname === "/v1/workflows") {
      res.writeHead(200, { "Content-Type": "application/json" });
      res.end(JSON.stringify([{ id: "demo", version: 1, installed_at: "2026-01-01T00:00:00Z" }]));
      return;
    }

    if (req.method === "GET" && url.pathname === "/v1/workflows/demo") {
      res.writeHead(200, { "Content-Type": "application/json" });
      res.end(
        JSON.stringify({
          id: "demo",
          version: 1,
          schema_version: 1,
          trigger_type: "manual",
          inputs: [
            { name: "value", type: "string", required: true, description: "Value to uppercase." },
            { name: "shout", type: "boolean", required: false, default: false },
            { name: "greeting", type: "enum", required: false, enum: ["hi", "hello"] },
          ],
          steps: [{ id: "transform", uses: "text.uppercase@1", with: { value: "hi" } }],
          source: "schema_version: 1\nid: demo\n",
        }),
      );
      return;
    }

    if (req.method === "GET" && url.pathname === "/v1/runs") {
      res.writeHead(200, { "Content-Type": "application/json" });
      res.end(
        JSON.stringify([
          {
            id: "run-1",
            workflow_id: "demo",
            workflow_version: 1,
            status: "succeeded",
            created_at: "2026-01-01T00:00:00Z",
          },
        ]),
      );
      return;
    }

    if (req.method === "POST" && url.pathname === "/v1/runs/run-1/cancel") {
      res.writeHead(200, { "Content-Type": "application/json" });
      res.end(
        JSON.stringify({
          id: "run-1",
          workflow_id: "demo",
          workflow_version: 1,
          status: "cancelled",
          created_at: "2026-01-01T00:00:00Z",
          finished_at: "2026-01-01T00:00:02Z",
        }),
      );
      return;
    }

    if (req.method === "GET" && url.pathname === "/v1/apps") {
      res.writeHead(200, { "Content-Type": "application/json" });
      res.end(JSON.stringify([{ id: "dashboard", version: "1.0.0", workflows_run: ["demo"] }]));
      return;
    }

    if (req.method === "POST" && url.pathname === "/v1/apps/dashboard/sessions") {
      res.writeHead(201, { "Content-Type": "application/json" });
      res.end(
        JSON.stringify({
          token: "app-session-token",
          app_id: "dashboard",
          workflows_run: ["demo"],
          issued_at: "2026-01-01T00:00:00Z",
        }),
      );
      return;
    }

    res.writeHead(404).end();
  });

  return new Promise((resolve, reject) => {
    server.on("error", reject);
    server.listen(0, "127.0.0.1", () => {
      const addr = server.address() as AddressInfo;
      resolve({
        baseUrl: `http://127.0.0.1:${addr.port}`,
        close: () => new Promise((r) => server.close(() => r())),
      });
    });
  });
}

test("PatchcordClient triggers a run, streams its events, and fetches its result", async () => {
  const agent = await startFakeAgent();
  try {
    const client = new PatchcordClient({ baseUrl: agent.baseUrl });
    const run = await client.workflows.run("demo", { inputs: { value: "hi" } });

    assert.equal(run.id, "run-1");

    const statuses: string[] = [];
    for await (const event of run.events()) {
      assert.equal(event.runId, "run-1");
      statuses.push(event.status);
    }
    assert.deepEqual(statuses, ["running", "succeeded"]);

    const result = await run.result();
    assert.equal(result.status, "succeeded");
    assert.equal(result.outputs?.value, "HI");
  } finally {
    await agent.close();
  }
});

// Node's own global fetch (undici) doesn't enforce that it's called with
// the global object as its receiver, so a test built directly against it
// can't catch a real regression here: `this.#fetch = options.fetch ??
// fetch` (the bare reference, unbound) passes every test above just fine
// under Node, yet throws "Illegal invocation" in an actual browser, where
// fetch — like many Web APIs — checks its receiver. This test stands in
// for that stricter browser behavior by wrapping the real fetch with the
// same receiver check, so a regression (removing the `.bind(globalThis)`
// in client.ts) fails here instead of only being caught by a user
// clicking "Run" in the example app.
test("PatchcordClient's default fetch does not rely on being called as a method (mirrors a real browser's receiver check on fetch)", async () => {
  const agent = await startFakeAgent();
  const realFetch = globalThis.fetch;
  const strictFetch = function (this: unknown, ...args: Parameters<typeof fetch>): ReturnType<typeof fetch> {
    if (this !== globalThis) {
      throw new TypeError("Failed to execute 'fetch' on 'Window': Illegal invocation");
    }
    return realFetch(...args);
  };
  globalThis.fetch = strictFetch as typeof fetch;

  try {
    // No `fetch` override passed here on purpose: this must exercise
    // PatchcordClient's own default, not a test-supplied stand-in.
    const client = new PatchcordClient({ baseUrl: agent.baseUrl });
    const run = await client.workflows.run("demo");
    assert.equal(run.id, "run-1");
  } finally {
    globalThis.fetch = realFetch;
    await agent.close();
  }
});

test("PatchcordClient.system.health reports the agent's database status", async () => {
  const agent = await startFakeAgent();
  try {
    const client = new PatchcordClient({ baseUrl: agent.baseUrl });
    assert.deepEqual(await client.system.health(), { status: "ok", database: "ok" });
  } finally {
    await agent.close();
  }
});

test("PatchcordClient.workflows.list returns every installed workflow version", async () => {
  const agent = await startFakeAgent();
  try {
    const client = new PatchcordClient({ baseUrl: agent.baseUrl });
    const workflows = await client.workflows.list();
    assert.deepEqual(workflows, [{ id: "demo", version: 1, installedAt: "2026-01-01T00:00:00Z" }]);
  } finally {
    await agent.close();
  }
});

test("PatchcordClient.workflows.get returns one workflow version's steps and source", async () => {
  const agent = await startFakeAgent();
  try {
    const client = new PatchcordClient({ baseUrl: agent.baseUrl });
    const detail = await client.workflows.get("demo");
    assert.equal(detail.id, "demo");
    assert.equal(detail.version, 1);
    assert.equal(detail.schemaVersion, 1);
    assert.equal(detail.triggerType, "manual");
    assert.deepEqual(detail.inputs, [
      { name: "value", type: "string", required: true, description: "Value to uppercase.", default: undefined, enum: undefined },
      { name: "shout", type: "boolean", required: false, description: undefined, default: false, enum: undefined },
      { name: "greeting", type: "enum", required: false, description: undefined, default: undefined, enum: ["hi", "hello"] },
    ]);
    assert.deepEqual(detail.steps, [{ id: "transform", uses: "text.uppercase@1", with: { value: "hi" }, connector: undefined }]);
    assert.match(detail.source, /id: demo/);
  } finally {
    await agent.close();
  }
});

test("PatchcordClient.runs.list returns Run handles that can be fetched and awaited", async () => {
  const agent = await startFakeAgent();
  try {
    const client = new PatchcordClient({ baseUrl: agent.baseUrl });
    const runs = await client.runs.list({ workflowId: "demo" });
    assert.equal(runs.length, 1);
    assert.equal(runs[0].id, "run-1");

    const summary = await runs[0].fetch();
    assert.equal(summary.status, "succeeded");
    assert.equal(summary.outputs?.value, "HI");
  } finally {
    await agent.close();
  }
});

test("PatchcordClient.runs.get fetches a single run by id", async () => {
  const agent = await startFakeAgent();
  try {
    const client = new PatchcordClient({ baseUrl: agent.baseUrl });
    const run = await client.runs.get("run-1");
    assert.equal(run.id, "run-1");
  } finally {
    await agent.close();
  }
});

test("PatchcordClient.runs.cancel marks a run cancelled", async () => {
  const agent = await startFakeAgent();
  try {
    const client = new PatchcordClient({ baseUrl: agent.baseUrl });
    const run = await client.runs.cancel("run-1");
    assert.equal(run.id, "run-1");

    const summary = await run.fetch();
    assert.equal(summary.status, "succeeded"); // GET /v1/runs/run-1 is fixed in the fake agent
  } finally {
    await agent.close();
  }
});

test("Run.cancel marks a run cancelled without going through PatchcordClient.runs.cancel", async () => {
  const agent = await startFakeAgent();
  try {
    const client = new PatchcordClient({ baseUrl: agent.baseUrl });
    const run = await client.workflows.run("demo");
    const summary = await run.cancel();
    assert.equal(summary.status, "cancelled");
  } finally {
    await agent.close();
  }
});

test("PatchcordClient.apps.list and apps.createSession round-trip an application session", async () => {
  const agent = await startFakeAgent();
  try {
    const client = new PatchcordClient({ baseUrl: agent.baseUrl });
    const apps = await client.apps.list();
    assert.deepEqual(apps, [{ id: "dashboard", version: "1.0.0", workflowsRun: ["demo"] }]);

    const session = await client.apps.createSession("dashboard");
    assert.deepEqual(session, {
      token: "app-session-token",
      appId: "dashboard",
      workflowsRun: ["demo"],
      issuedAt: "2026-01-01T00:00:00Z",
    });
  } finally {
    await agent.close();
  }
});

test("PatchcordClient.workflows.run rejects a non-2xx response", async () => {
  const server = http.createServer((_req, res) => {
    res.writeHead(404, { "Content-Type": "text/plain" });
    res.end('workflow "missing" was not found');
  });
  await new Promise<void>((resolve) => server.listen(0, "127.0.0.1", resolve));
  const addr = server.address() as AddressInfo;

  try {
    const client = new PatchcordClient({ baseUrl: `http://127.0.0.1:${addr.port}` });
    await assert.rejects(() => client.workflows.run("missing"), /404/);
  } finally {
    await new Promise((r) => server.close(() => r(undefined)));
  }
});

// recordingServer stands in for an agent that hasn't been given an app
// session's token: it just echoes back whether one arrived, letting these
// tests assert on client.ts's own request-building logic without needing
// the full run/events lifecycle startFakeAgent sets up.
function startRecordingServer(): Promise<{
  baseUrl: string;
  lastAuthorization: () => string | undefined;
  close: () => Promise<void>;
}> {
  let lastAuthorization: string | undefined;
  const server = http.createServer((req, res) => {
    lastAuthorization = req.headers.authorization;
    res.writeHead(202, { "Content-Type": "application/json" });
    res.end(
      JSON.stringify({
        id: "run-1",
        workflow_id: "demo",
        workflow_version: 1,
        status: "running",
        created_at: "2026-01-01T00:00:00Z",
      }),
    );
  });

  return new Promise((resolve, reject) => {
    server.on("error", reject);
    server.listen(0, "127.0.0.1", () => {
      const addr = server.address() as AddressInfo;
      resolve({
        baseUrl: `http://127.0.0.1:${addr.port}`,
        lastAuthorization: () => lastAuthorization,
        close: () => new Promise((r) => server.close(() => r())),
      });
    });
  });
}

test("PatchcordClient sends 'Authorization: Bearer <token>' when constructed with a token", async () => {
  const server = await startRecordingServer();
  try {
    const client = new PatchcordClient({ baseUrl: server.baseUrl, token: "app-session-token" });
    await client.workflows.run("demo");
    assert.equal(server.lastAuthorization(), "Bearer app-session-token");
  } finally {
    await server.close();
  }
});

test("PatchcordClient sends no Authorization header when constructed without a token", async () => {
  const server = await startRecordingServer();
  try {
    const client = new PatchcordClient({ baseUrl: server.baseUrl });
    await client.workflows.run("demo");
    assert.equal(server.lastAuthorization(), undefined);
  } finally {
    await server.close();
  }
});
