import { Run } from "./run.js";
import type { RunWorkflowOptions } from "./types.js";
import { runSummaryFromWire, type WireRunSummary } from "./wire.js";

export interface PatchcordClientOptions {
  /** The agent's base URL, e.g. "http://127.0.0.1:7331". */
  baseUrl: string;
  /**
   * Override for the fetch implementation used for every request —
   * mainly for tests. Defaults to the global fetch, present in both
   * browsers and Node >=18.
   */
  fetch?: typeof fetch;
}

/**
 * Client for the Patchcord agent's public HTTP API (vision document,
 * section 10.2). This first version only covers `workflows.run` — the
 * example the vision document itself gives — not the full `client.*`
 * surface (plugins/connectors/actions/apps/files/...) described there.
 */
export class PatchcordClient {
  #baseUrl: string;
  #fetch: typeof fetch;

  readonly workflows: {
    /**
     * Starts a new run of the latest installed version of workflowId and
     * returns immediately with a Run handle — it does not wait for any
     * step to execute (POST /v1/workflows/{id}/run).
     */
    run(workflowId: string, options?: RunWorkflowOptions): Promise<Run>;
  };

  constructor(options: PatchcordClientOptions) {
    this.#baseUrl = options.baseUrl.replace(/\/+$/, "");
    // fetch is a Web API method: it checks that its receiver (`this`) is
    // the global object, so storing the bare reference and later calling
    // it as `this.#fetch(...)` (where `this` is the PatchcordClient
    // instance, not window) throws "Illegal invocation" in a real browser.
    // Node's fetch doesn't enforce this, which is why it never showed up
    // in this SDK's own (Node-based) test suite. Bind it explicitly.
    this.#fetch = options.fetch ?? fetch.bind(globalThis);

    this.workflows = {
      run: (workflowId, runOptions) => this.#runWorkflow(workflowId, runOptions),
    };
  }

  async #runWorkflow(workflowId: string, options?: RunWorkflowOptions): Promise<Run> {
    const response = await this.#fetch(`${this.#baseUrl}/v1/workflows/${encodeURIComponent(workflowId)}/run`, {
      method: "POST",
      headers: { "Content-Type": "application/json" },
      body: JSON.stringify({
        inputs: options?.inputs ?? {},
        bindings: options?.bindings ?? {},
      }),
    });
    if (!response.ok) {
      const text = await response.text().catch(() => "");
      throw new Error(
        `POST /v1/workflows/${workflowId}/run: ${response.status} ${response.statusText}${text ? ` — ${text}` : ""}`,
      );
    }

    const wire = (await response.json()) as WireRunSummary;
    return new Run(this.#baseUrl, this.#fetch, runSummaryFromWire(wire));
  }
}
