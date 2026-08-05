import { parseEventStream } from "./sse.js";
import type { RunEvent, RunSnapshot, RunStatus, RunStep, RunSummary, WatchRunOptions } from "./types.js";
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
   * The most recently known summary for this run — the one passed to the
   * constructor (from PatchcordClient.workflows.run/runs.list/runs.get)
   * until .fetch() or .cancel() replaces it with a fresher one. Lets a list
   * view (e.g. a runs table) render workflow id, status, and timestamps
   * without an extra GET per row.
   */
  get workflowId(): string {
    return this.#summary.workflowId;
  }

  get workflowVersion(): number {
    return this.#summary.workflowVersion;
  }

  get status(): RunStatus {
    return this.#summary.status;
  }

  get error(): string | undefined {
    return this.#summary.error;
  }

  get createdAt(): string {
    return this.#summary.createdAt;
  }

  get startedAt(): string | undefined {
    return this.#summary.startedAt;
  }

  get finishedAt(): string | undefined {
    return this.#summary.finishedAt;
  }

  /**
   * Streams this run's status changes, and its steps', as they happen
   * (GET /v1/runs/{id}/events). The stream closes once the run reaches a
   * terminal status (internal/runs.WatchRun) — a client connecting after a
   * fast run has already finished still observes each entity's current
   * (final) status once, not the intermediate ones it passed through.
   *
   * Pass `{ signal }` to stop listening early — e.g. a UI component
   * unmounting mid-run — without waiting for the run itself to finish. This
   * only tears down the client's own connection; the run keeps executing on
   * the agent regardless (call .cancel() to actually stop it).
   */
  async *events(options?: WatchRunOptions): AsyncGenerator<RunEvent> {
    const response = await this.#fetch(`${this.#baseUrl}/v1/runs/${this.id}/events`, { signal: options?.signal });
    if (!response.ok) {
      throw new Error(`GET /v1/runs/${this.id}/events: ${response.status} ${response.statusText}`);
    }
    for await (const frame of parseEventStream(response)) {
      yield runEventFromWire(JSON.parse(frame.data) as WireRunEvent);
    }
  }

  /**
   * Streams this run's live state as fully merged snapshots — see
   * RunSnapshot. Internally just a reducer over events(): each iteration
   * upserts the reported step (or the run's own status/error) into a
   * running Map, then yields the whole picture, so a caller never
   * hand-writes that reduction itself. Once events() closes (terminal
   * status reached), watch() does one extra GET /v1/runs/{id} — the same
   * trade its sibling .result() makes — so the final snapshot it yields
   * carries step inputs/outputs, which events() itself never does, before
   * the generator closes for good.
   *
   * Accepts the same `{ signal }` as events(), for the same reason: stop
   * listening from a UI cleanup without cancelling the run.
   */
  async *watch(options?: WatchRunOptions): AsyncGenerator<RunSnapshot> {
    const steps = new Map<string, RunStep>();
    let status: RunStatus = this.#summary.status;
    let error: string | undefined = this.#summary.error;

    for await (const event of this.events(options)) {
      if (event.stepId) {
        steps.set(event.stepId, { ...steps.get(event.stepId), id: event.stepId, status: event.status as RunStep["status"], error: event.error });
      } else {
        status = event.status as RunStatus;
        error = event.error;
      }
      yield { status, error, steps: [...steps.values()] };
    }

    const final = await this.fetch();
    yield { status: final.status, error: final.error, steps: final.steps ?? [...steps.values()], outputs: final.outputs };
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
