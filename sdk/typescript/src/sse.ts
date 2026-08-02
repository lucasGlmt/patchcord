// A minimal text/event-stream parser over a fetch Response, used instead of
// the browser's EventSource API so the same code works identically in a
// browser and under Node (EventSource isn't a universal global, but fetch's
// Response.body — a WHATWG ReadableStream — is, on both). It only
// understands "event:"/"data:" fields, framed by a blank line, exactly what
// internal/api/events.go emits — not the full SSE spec (no "id:"/"retry:"
// reconnection support): a client that wants to resume simply issues a
// fresh GET, which is enough for how Patchcord's runs.WatchRun already
// behaves (it replays each entity's current status to a newly connecting
// client).

export interface SSEFrame {
  event: string;
  data: string;
}

export async function* parseEventStream(response: Response): AsyncGenerator<SSEFrame> {
  if (!response.body) {
    return;
  }

  const reader = response.body.getReader();
  const decoder = new TextDecoder();
  let buffer = "";

  try {
    while (true) {
      const { done, value } = await reader.read();
      if (done) {
        break;
      }
      buffer += decoder.decode(value, { stream: true });

      let boundary: number;
      while ((boundary = buffer.indexOf("\n\n")) !== -1) {
        const rawFrame = buffer.slice(0, boundary);
        buffer = buffer.slice(boundary + 2);
        const frame = parseFrame(rawFrame);
        if (frame) {
          yield frame;
        }
      }
    }
  } finally {
    reader.releaseLock();
  }
}

function parseFrame(raw: string): SSEFrame | null {
  let event = "message";
  const dataLines: string[] = [];

  for (const line of raw.split("\n")) {
    if (line.startsWith("event:")) {
      event = line.slice("event:".length).trim();
    } else if (line.startsWith("data:")) {
      dataLines.push(line.slice("data:".length).trim());
    }
  }

  if (dataLines.length === 0) {
    return null;
  }
  return { event, data: dataLines.join("\n") };
}
