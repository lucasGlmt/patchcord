import assert from "node:assert/strict";
import http from "node:http";
import type { AddressInfo } from "node:net";
import { afterEach, test } from "node:test";

import { PatchcordClient } from "@glmtsolutions/patchcord-sdk";
import { act, cleanup, renderHook, waitFor } from "@testing-library/react";

import { useWorkflowRun } from "../src/useWorkflowRun.js";

afterEach(cleanup);

// startFakeAgent stands in for the endpoints useWorkflowRun drives: run a
// workflow, stream its events, cancel it, and re-fetch its final summary —
// a real local HTTP server (as sdk/typescript's own tests do), not a mocked
// fetch, so this exercises the hook against the SDK's real wire parsing.
function startFakeAgent(): Promise<{
  baseUrl: string;
  cancelCalls: () => number;
  close: () => Promise<void>;
}> {
  let cancelCalls = 0;

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
      res.write('event: run.running\ndata: {"run_id":"run-1","status":"running","time":"2026-01-01T00:00:00Z"}\n\n');
      res.write(
        'event: step.succeeded\ndata: {"run_id":"run-1","step_id":"transform","status":"succeeded","time":"2026-01-01T00:00:01Z"}\n\n',
      );
      res.write('event: run.succeeded\ndata: {"run_id":"run-1","status":"succeeded","time":"2026-01-01T00:00:01Z"}\n\n');
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
          steps: [{ id: "transform", status: "succeeded", input: { value: "hi" }, output: { value: "HI" } }],
        }),
      );
      return;
    }

    if (req.method === "POST" && url.pathname === "/v1/runs/run-1/cancel") {
      cancelCalls += 1;
      res.writeHead(200, { "Content-Type": "application/json" });
      res.end(
        JSON.stringify({
          id: "run-1",
          workflow_id: "demo",
          workflow_version: 1,
          status: "cancelled",
          created_at: "2026-01-01T00:00:00Z",
          finished_at: "2026-01-01T00:00:01Z",
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
        cancelCalls: () => cancelCalls,
        close: () => new Promise((r) => server.close(() => r())),
      });
    });
  });
}

test("useWorkflowRun runs a workflow to completion, tracking live steps then the final outputs", async () => {
  const agent = await startFakeAgent();
  try {
    const client = new PatchcordClient({ baseUrl: agent.baseUrl });
    const { result } = renderHook(() => useWorkflowRun(client, "demo"));

    assert.equal(result.current.phase, "idle");

    act(() => {
      result.current.start({ inputs: { value: "hi" } });
    });
    assert.equal(result.current.phase, "running");

    await waitFor(() => assert.equal(result.current.phase, "succeeded"));

    assert.deepEqual(result.current.outputs, { value: "HI" });
    assert.equal(result.current.steps.length, 1);
    assert.equal(result.current.steps[0].id, "transform");
    assert.equal(result.current.steps[0].status, "succeeded");
    assert.equal(result.current.error, undefined);
    assert.equal(result.current.run?.id, "run-1");
  } finally {
    await agent.close();
  }
});

test("useWorkflowRun.cancel() calls Run.cancel() on the in-flight run", async () => {
  const agent = await startFakeAgent();
  try {
    const client = new PatchcordClient({ baseUrl: agent.baseUrl });
    const { result } = renderHook(() => useWorkflowRun(client, "demo"));

    act(() => {
      result.current.start();
    });
    await waitFor(() => assert.ok(result.current.run));

    act(() => {
      result.current.cancel();
    });

    await waitFor(() => assert.equal(agent.cancelCalls(), 1));
  } finally {
    await agent.close();
  }
});

test("useWorkflowRun.reset() returns to idle and discards the previous run's outcome", async () => {
  const agent = await startFakeAgent();
  try {
    const client = new PatchcordClient({ baseUrl: agent.baseUrl });
    const { result } = renderHook(() => useWorkflowRun(client, "demo"));

    act(() => {
      result.current.start();
    });
    await waitFor(() => assert.equal(result.current.phase, "succeeded"));

    act(() => {
      result.current.reset();
    });

    assert.equal(result.current.phase, "idle");
    assert.equal(result.current.run, undefined);
    assert.deepEqual(result.current.steps, []);
    assert.equal(result.current.outputs, undefined);
  } finally {
    await agent.close();
  }
});
