import assert from "node:assert/strict";
import http from "node:http";
import type { AddressInfo } from "node:net";
import { test } from "node:test";

import { PatchcordClient } from "../src/client.js";
import type { RunSnapshot } from "../src/types.js";

// startWatchableAgent is a fake agent dedicated to Run.watch(): unlike
// client.test.ts's startFakeAgent (run-level events only), this one emits
// step-level events too, and its final GET /v1/runs/run-1 carries a full
// steps array with input/output — exactly the shape watch()'s last snapshot
// is supposed to pick up that events() itself never carries.
function startWatchableAgent(): Promise<{ baseUrl: string; close: () => Promise<void> }> {
  const server = http.createServer((req, res) => {
    const url = new URL(req.url ?? "/", "http://localhost");

    if (req.method === "POST" && url.pathname === "/v1/workflows/demo/run") {
      res.writeHead(202, { "Content-Type": "application/json" });
      res.end(
        JSON.stringify({
          id: "run-1",
          workflow_id: "demo",
          workflow_version: 1,
          status: "queued",
          created_at: "2026-01-01T00:00:00Z",
        }),
      );
      return;
    }

    if (req.method === "GET" && url.pathname === "/v1/runs/run-1/events") {
      res.writeHead(200, { "Content-Type": "text/event-stream" });
      res.write('event: run.running\ndata: {"run_id":"run-1","status":"running","time":"2026-01-01T00:00:00Z"}\n\n');
      res.write(
        'event: step.running\ndata: {"run_id":"run-1","step_id":"transform","status":"running","time":"2026-01-01T00:00:00Z"}\n\n',
      );
      res.write(
        'event: step.succeeded\ndata: {"run_id":"run-1","step_id":"transform","status":"succeeded","time":"2026-01-01T00:00:01Z"}\n\n',
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
          steps: [
            {
              id: "transform",
              status: "succeeded",
              input: { value: "hi" },
              output: { value: "HI" },
              started_at: "2026-01-01T00:00:00Z",
              finished_at: "2026-01-01T00:00:01Z",
            },
          ],
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

test("Run.watch() merges step and run events into full snapshots, ending with the fetched, authoritative one", async () => {
  const agent = await startWatchableAgent();
  try {
    const client = new PatchcordClient({ baseUrl: agent.baseUrl });
    const run = await client.workflows.run("demo");

    const snapshots: RunSnapshot[] = [];
    for await (const snapshot of run.watch()) {
      snapshots.push(snapshot);
    }

    // 4 raw events in, one extra authoritative snapshot after the stream closes.
    assert.equal(snapshots.length, 5);

    // First event is run-level: status flips, no step known yet.
    assert.deepEqual(snapshots[0], { status: "running", error: undefined, steps: [] });

    // Second event is step-level: the run's status carries over unchanged.
    assert.equal(snapshots[1].status, "running");
    assert.deepEqual(snapshots[1].steps, [{ id: "transform", status: "running", error: undefined }]);

    // Third event updates the same step in place rather than appending.
    assert.equal(snapshots[2].steps.length, 1);
    assert.equal(snapshots[2].steps[0].status, "succeeded");

    // Fourth is the run-level "succeeded" delta — still no input/output yet.
    assert.equal(snapshots[3].status, "succeeded");
    assert.equal(snapshots[3].steps[0].input, undefined);

    // Final snapshot is the re-fetched, authoritative one: input/output populated.
    const final = snapshots[4];
    assert.equal(final.status, "succeeded");
    assert.equal(final.steps.length, 1);
    assert.deepEqual(final.steps[0].input, { value: "hi" });
    assert.deepEqual(final.steps[0].output, { value: "HI" });
    assert.deepEqual(final.outputs, { value: "HI" });
  } finally {
    await agent.close();
  }
});

// startSlowStreamingAgent spaces its event writes out by a few ms each,
// unlike startWatchableAgent's all-at-once burst — enough that Node's fetch
// resolves a separate reader.read() per event instead of coalescing all of
// them into one already-buffered chunk. The abort test below needs that: it
// aborts as soon as the first snapshot arrives, and that only pre-empts
// later events if they weren't already sitting in the client's buffer by
// then.
function startSlowStreamingAgent(): Promise<{ baseUrl: string; close: () => Promise<void> }> {
  const server = http.createServer((req, res) => {
    const url = new URL(req.url ?? "/", "http://localhost");

    if (req.method === "POST" && url.pathname === "/v1/workflows/demo/run") {
      res.writeHead(202, { "Content-Type": "application/json" });
      res.end(
        JSON.stringify({ id: "run-1", workflow_id: "demo", workflow_version: 1, status: "queued", created_at: "2026-01-01T00:00:00Z" }),
      );
      return;
    }

    if (req.method === "GET" && url.pathname === "/v1/runs/run-1/events") {
      res.writeHead(200, { "Content-Type": "text/event-stream" });
      const frames = [
        'event: run.running\ndata: {"run_id":"run-1","status":"running","time":"2026-01-01T00:00:00Z"}\n\n',
        'event: run.succeeded\ndata: {"run_id":"run-1","status":"succeeded","time":"2026-01-01T00:00:01Z"}\n\n',
      ];
      let i = 0;
      const interval = setInterval(() => {
        if (i >= frames.length) {
          clearInterval(interval);
          res.end();
          return;
        }
        res.write(frames[i]);
        i += 1;
      }, 20);
      req.on("close", () => clearInterval(interval));
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

test("Run.watch() honors an AbortSignal, stopping without waiting for the run to finish", async () => {
  const agent = await startSlowStreamingAgent();
  try {
    const client = new PatchcordClient({ baseUrl: agent.baseUrl });
    const run = await client.workflows.run("demo");

    const controller = new AbortController();
    const snapshots: RunSnapshot[] = [];
    await assert.rejects(async () => {
      for await (const snapshot of run.watch({ signal: controller.signal })) {
        snapshots.push(snapshot);
        controller.abort();
      }
    });

    // Stopped after the first snapshot instead of draining both events and
    // going on to the final, re-fetched one.
    assert.equal(snapshots.length, 1);
  } finally {
    await agent.close();
  }
});
