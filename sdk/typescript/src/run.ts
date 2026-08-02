import { parseEventStream } from "./sse.js";
import type { RunEvent, RunSummary } from "./types.js";
import { runEventFromWire, runSummaryFromWire, type WireRunEvent, type WireRunSummary } from "./wire.js";

/**
 * One triggered execution of a workflow, as returned by
 * PatchcordClient.workflows.run, PatchcordClient.runs.get, and
 * PatchcordClient.runs.list. Mirrors the vision document's SDK example
 * (section 10.2): call .events() to observe it live, .result() to wait for
 * it to finish and get its final summary, or .cancel() to stop it early.
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

  /**
   * Marks this run cancelled, if it is still queued or running
   * (POST /v1/runs/{id}/cancel). Does not interrupt a step already
   * executing in-process — it only flips the recorded status; an in-flight
   * step stops at its next persistence checkpoint. Throws if the run has
   * already reached a terminal status (409).
   */
  async cancel(): Promise<RunSummary> {
    const response = await this.#fetch(`${this.#baseUrl}/v1/runs/${this.id}/cancel`, { method: "POST" });
    if (!response.ok) {
      const text = await response.text().catch(() => "");
      throw new Error(
        `POST /v1/runs/${this.id}/cancel: ${response.status} ${response.statusText}${text ? ` — ${text}` : ""}`,
      );
    }
    this.#summary = runSummaryFromWire((await response.json()) as WireRunSummary);
    return this.#summary;
  }
}
