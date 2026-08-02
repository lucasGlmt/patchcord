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
    this.#fetch = options.fetch ?? fetch;

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
