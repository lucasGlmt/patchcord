import assert from "node:assert/strict";
import { test } from "node:test";

import { parseEventStream, type SSEFrame } from "../src/sse.js";

function responseFromString(body: string | null): Response {
  return new Response(body);
}

async function collect(response: Response): Promise<SSEFrame[]> {
  const frames: SSEFrame[] = [];
  for await (const frame of parseEventStream(response)) {
    frames.push(frame);
  }
  return frames;
}

test("parseEventStream yields event/data frames separated by a blank line", async () => {
  const body = 'event: run.running\ndata: {"a":1}\n\nevent: run.succeeded\ndata: {"a":2}\n\n';
  const frames = await collect(responseFromString(body));
  assert.deepEqual(frames, [
    { event: "run.running", data: '{"a":1}' },
    { event: "run.succeeded", data: '{"a":2}' },
  ]);
});

test('parseEventStream defaults to "message" when no event: field is present', async () => {
  const frames = await collect(responseFromString("data: hello\n\n"));
  assert.deepEqual(frames, [{ event: "message", data: "hello" }]);
});

test("parseEventStream joins multiple data: lines with a newline", async () => {
  const frames = await collect(responseFromString("data: line1\ndata: line2\n\n"));
  assert.deepEqual(frames, [{ event: "message", data: "line1\nline2" }]);
});

test("parseEventStream ignores an incomplete trailing frame with no blank-line terminator", async () => {
  const frames = await collect(responseFromString("event: run.running\ndata: partial"));
  assert.deepEqual(frames, []);
});

test("parseEventStream yields nothing for a response with no body", async () => {
  const frames = await collect(responseFromString(null));
  assert.deepEqual(frames, []);
});
