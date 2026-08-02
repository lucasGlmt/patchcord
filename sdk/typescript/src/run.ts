import { parseEventStream } from "./sse.js";
import type { RunEvent, RunSummary } from "./types.js";
import { runEventFromWire, runSummaryFromWire, type WireRunEvent, type WireRunSummary } from "./wire.js";

/**
 * One triggered execution of a workflow, as returned by
 * PatchcordClient.workflows.run. Mirrors the vision document's SDK example
 * (section 10.2): call .events() to observe it live, or .result() to wait
 * for it to finish and get its final summary.
 */
export class Run {
  readonly id: string;

  #baseUrl: string;
  #fetch: typeof fetch;
  #summary: RunSummary;

  constructor(baseUrl: string, fetchImpl: typeof fetch, summary: RunSummary) {
    this.id = summary.id;
    this.#baseUrl = baseUrl;
    this.#fetch = fetchImpl;
    this.#summary = summary;
  }

  /**
   * Streams this run's status changes, and its steps', as they happen
   * (GET /v1/runs/{id}/events). The stream closes once the run reaches a
   * terminal status (internal/runs.WatchRun) — a client connecting after a
   * fast run has already finished still observes each entity's current
   * (final) status once, not the intermediate ones it passed through.
   */
  async *events(): AsyncGenerator<RunEvent> {
    const response = await this.#fetch(`${this.#baseUrl}/v1/runs/${this.id}/events`);
    if (!response.ok) {
      throw new Error(`GET /v1/runs/${this.id}/events: ${response.status} ${response.statusText}`);
    }
    for await (const frame of parseEventStream(response)) {
      yield runEventFromWire(JSON.parse(frame.data) as WireRunEvent);
    }
  }

  /**
   * Waits for this run to reach a terminal status — by draining events()
   * until it closes, which is exactly what a terminal status means here —
   * then returns its authoritative final summary (one extra
   * GET /v1/runs/{id}, since events() only ever carries a status string,
   * never the run's outputs).
   */
  async result(): Promise<RunSummary> {
    for await (const _event of this.events()) {
      // Draining is enough: events() closing means the run is done.
    }
    return this.fetch();
  }

  /** Fetches this run's current summary without waiting for completion. */
  async fetch(): Promise<RunSummary> {
    const response = await this.#fetch(`${this.#baseUrl}/v1/runs/${this.id}`);
    if (!response.ok) {
      throw new Error(`GET /v1/runs/${this.id}: ${response.status} ${response.statusText}`);
    }
    this.#summary = runSummaryFromWire((await response.json()) as WireRunSummary);
    return this.#summary;
  }
}
