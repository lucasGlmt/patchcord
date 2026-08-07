import { Run } from "./run.js";
import type {
  AppSession,
  AppSummary,
  Connector,
  ConnectorTestResult,
  CreateConnectorOptions,
  GetWorkflowOptions,
  HealthStatus,
  ListRunsOptions,
  PluginSummary,
  RunWorkflowOptions,
  SystemMetrics,
  WorkflowDetail,
  WorkflowSummary,
} from "./types.js";
import {
  appSessionFromWire,
  appSummaryFromWire,
  connectorFromWire,
  connectorSecretRefsToWire,
  connectorTestResultFromWire,
  healthStatusFromWire,
  pluginSummaryFromWire,
  runSummaryFromWire,
  systemMetricsFromWire,
  workflowDetailFromWire,
  workflowSummaryFromWire,
  type WireAppSession,
  type WireAppSummary,
  type WireConnector,
  type WireConnectorTestResult,
  type WireHealthStatus,
  type WirePluginSummary,
  type WireRunSummary,
  type WireSystemMetrics,
  type WireWorkflowDetail,
  type WireWorkflowSummary,
} from "./wire.js";

export interface PatchcordClientOptions {
  /** The agent's base URL, e.g. "http://127.0.0.1:7331". */
  baseUrl: string;
  /**
   * An application session token (vision document, section 15.4), as
   * issued by POST /v1/apps/{id}/sessions. When set, every request sends
   * it as "Authorization: Bearer <token>" — the agent then limits what
   * this client may do to that session's application's declared
   * permissions instead of its unrestricted default (ADR-0026). Omit it
   * to call the API exactly as before this option existed.
   */
  token?: string;
  /**
   * Override for the fetch implementation used for every request —
   * mainly for tests. Defaults to the global fetch, present in both
   * browsers and Node >=18.
   */
  fetch?: typeof fetch;
}

/**
 * Client for the Patchcord agent's public HTTP API (vision document,
 * section 10.2), covering every `/v1/*` route the agent implements today:
 * `system.health`/`metrics`, `workflows.list`/`run`,
 * `runs.list`/`get`/`cancel`, and `apps.list`/`createSession`. The vision
 * document's fuller `client.*` surface (plugins/connectors/actions/files/...)
 * has no server-side implementation yet — see internal/api/doc.go — so it
 * isn't wrapped here. GET /metrics (Prometheus text format) is the one
 * exception: it sits outside `/v1/*` by design (ADR-0070) and is meant for
 * direct scraping, not application code — see `system.metrics` for the
 * JSON equivalent this SDK does wrap.
 */
export class PatchcordClient {
  #baseUrl: string;
  #fetch: typeof fetch;
  #token?: string;

  readonly system: {
    /** Reports whether the agent's database is reachable (GET /v1/system/health). */
    health(): Promise<HealthStatus>;
    /**
     * Returns a snapshot of the agent's in-process metrics — run/step
     * transitions and counts, plugin supervision, scheduler activity,
     * connector tests (GET /v1/system/metrics). The same underlying
     * counters are also exposed in Prometheus text format at GET /metrics,
     * outside `/v1/*` and not wrapped by this SDK — that endpoint is for
     * direct scraping by an external Prometheus/Grafana, not application
     * code. See ADR-0070.
     */
    metrics(): Promise<SystemMetrics>;
  };

  readonly workflows: {
    /**
     * Returns every installed workflow version, most recently installed
     * first (GET /v1/workflows). The same workflow id can appear more than
     * once — workflows are immutable once published (ADR-0008).
     */
    list(): Promise<WorkflowSummary[]>;
    /**
     * Returns one workflow version's full definition — its steps and raw
     * YAML source (GET /v1/workflows/{id}). Defaults to the latest
     * installed version; pass `{ version }` for a specific one.
     */
    get(workflowId: string, options?: GetWorkflowOptions): Promise<WorkflowDetail>;
    /**
     * Starts a new run of the latest installed version of workflowId and
     * returns immediately with a Run handle — it does not wait for any
     * step to execute (POST /v1/workflows/{id}/run).
     */
    run(workflowId: string, options?: RunWorkflowOptions): Promise<Run>;
  };

  readonly runs: {
    /**
     * Returns every recorded run, most recently created first
     * (GET /v1/runs), optionally restricted to one workflow id. Unlike
     * runs.get, the returned handles' steps are not populated until
     * .fetch() is called on one of them — the list endpoint omits them.
     */
    list(options?: ListRunsOptions): Promise<Run[]>;
    /** Fetches one run by id, including its steps (GET /v1/runs/{id}). */
    get(runId: string): Promise<Run>;
    /**
     * Marks a queued or running run as cancelled (POST /v1/runs/{id}/cancel).
     * Throws if the run has already reached a terminal status (409).
     */
    cancel(runId: string): Promise<Run>;
  };

  readonly apps: {
    /** Returns every installed application (GET /v1/apps). */
    list(): Promise<AppSummary[]>;
    /**
     * Issues a new session for the named application, limited to its
     * manifest's declared permissions (POST /v1/apps/{id}/sessions). Pass
     * the returned token as `token` to a new PatchcordClient to act within
     * that session.
     */
    createSession(appId: string): Promise<AppSession>;
  };

  readonly connectors: {
    /** Returns every recorded connector, ordered by id (GET /v1/connectors). Never a resolved secret value. */
    list(): Promise<Connector[]>;
    /** Returns one connector by id (GET /v1/connectors/{id}). */
    get(connectorId: string): Promise<Connector>;
    /**
     * Creates a new connector (POST /v1/connectors). `type` must match a
     * connector type declared by an installed plugin's manifest — see
     * PatchcordClient.plugins.list. Throws on a duplicate id (409) or an
     * invalid connector (400: empty id/type, unknown type, unsupported
     * secret reference type).
     */
    create(options: CreateConnectorOptions): Promise<Connector>;
    /**
     * Removes a connector (DELETE /v1/connectors/{id}). There is no update
     * endpoint (ADR-0020) — recreate it with the same id to change it.
     */
    delete(connectorId: string): Promise<void>;
    /**
     * Resolves the connector's configuration and secrets, then asks the
     * installed plugin that declares its type to actually attempt a
     * connection (POST /v1/connectors/{id}/test). A connection attempt that
     * runs but fails is reported as `{ ok: false }`, not a thrown error.
     */
    test(connectorId: string): Promise<ConnectorTestResult>;
  };

  readonly plugins: {
    /** Returns every installed plugin and the connector types/action ids its manifest declares (GET /v1/plugins). */
    list(): Promise<PluginSummary[]>;
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
    this.#token = options.token;

    this.system = {
      health: () => this.#health(),
      metrics: () => this.#metrics(),
    };
    this.workflows = {
      list: () => this.#listWorkflows(),
      get: (workflowId, getOptions) => this.#getWorkflow(workflowId, getOptions),
      run: (workflowId, runOptions) => this.#runWorkflow(workflowId, runOptions),
    };
    this.runs = {
      list: (listOptions) => this.#listRuns(listOptions),
      get: (runId) => this.#getRun(runId),
      cancel: (runId) => this.#cancelRun(runId),
    };
    this.apps = {
      list: () => this.#listApps(),
      createSession: (appId) => this.#createAppSession(appId),
    };
    this.connectors = {
      list: () => this.#listConnectors(),
      get: (connectorId) => this.#getConnector(connectorId),
      create: (options) => this.#createConnector(options),
      delete: (connectorId) => this.#deleteConnector(connectorId),
      test: (connectorId) => this.#testConnector(connectorId),
    };
    this.plugins = {
      list: () => this.#listPlugins(),
    };
  }

  /**
   * Issues one request against the agent's API and decodes its JSON body.
   * Shared by every method above so header/error handling (including the
   * optional app-session bearer token) lives in one place. GET /system/
   * health does not use this — a non-2xx response there still carries a
   * meaningful body (degraded status), not an error to throw.
   */
  async #request<T>(method: string, path: string, body?: unknown): Promise<T> {
    const headers: Record<string, string> = {};
    if (body !== undefined) {
      headers["Content-Type"] = "application/json";
    }
    if (this.#token) {
      headers.Authorization = `Bearer ${this.#token}`;
    }

    const response = await this.#fetch(`${this.#baseUrl}${path}`, {
      method,
      headers,
      body: body !== undefined ? JSON.stringify(body) : undefined,
    });
    if (!response.ok) {
      const text = await response.text().catch(() => "");
      throw new Error(`${method} ${path}: ${response.status} ${response.statusText}${text ? ` — ${text}` : ""}`);
    }
    return (await response.json()) as T;
  }

  /**
   * Like #request, but for an endpoint that responds 204 No Content on
   * success (DELETE /v1/connectors/{id}) — there's no JSON body to decode.
   */
  async #requestNoContent(method: string, path: string): Promise<void> {
    const headers: Record<string, string> = {};
    if (this.#token) {
      headers.Authorization = `Bearer ${this.#token}`;
    }

    const response = await this.#fetch(`${this.#baseUrl}${path}`, { method, headers });
    if (!response.ok) {
      const text = await response.text().catch(() => "");
      throw new Error(`${method} ${path}: ${response.status} ${response.statusText}${text ? ` — ${text}` : ""}`);
    }
  }

  async #health(): Promise<HealthStatus> {
    const response = await this.#fetch(`${this.#baseUrl}/v1/system/health`);
    return healthStatusFromWire((await response.json()) as WireHealthStatus);
  }

  async #metrics(): Promise<SystemMetrics> {
    const wire = await this.#request<WireSystemMetrics>("GET", "/v1/system/metrics");
    return systemMetricsFromWire(wire);
  }

  async #listWorkflows(): Promise<WorkflowSummary[]> {
    const wire = await this.#request<WireWorkflowSummary[]>("GET", "/v1/workflows");
    return wire.map(workflowSummaryFromWire);
  }

  async #getWorkflow(workflowId: string, options?: GetWorkflowOptions): Promise<WorkflowDetail> {
    const query = options?.version ? `?version=${options.version}` : "";
    const wire = await this.#request<WireWorkflowDetail>("GET", `/v1/workflows/${encodeURIComponent(workflowId)}${query}`);
    return workflowDetailFromWire(wire);
  }

  async #runWorkflow(workflowId: string, options?: RunWorkflowOptions): Promise<Run> {
    const wire = await this.#request<WireRunSummary>(
      "POST",
      `/v1/workflows/${encodeURIComponent(workflowId)}/run`,
      { inputs: options?.inputs ?? {}, bindings: options?.bindings ?? {} },
    );
    return new Run(this.#baseUrl, this.#fetch, runSummaryFromWire(wire));
  }

  async #listRuns(options?: ListRunsOptions): Promise<Run[]> {
    const query = options?.workflowId ? `?workflow_id=${encodeURIComponent(options.workflowId)}` : "";
    const wire = await this.#request<WireRunSummary[]>("GET", `/v1/runs${query}`);
    return wire.map((summary) => new Run(this.#baseUrl, this.#fetch, runSummaryFromWire(summary)));
  }

  async #getRun(runId: string): Promise<Run> {
    const wire = await this.#request<WireRunSummary>("GET", `/v1/runs/${encodeURIComponent(runId)}`);
    return new Run(this.#baseUrl, this.#fetch, runSummaryFromWire(wire));
  }

  async #cancelRun(runId: string): Promise<Run> {
    const wire = await this.#request<WireRunSummary>("POST", `/v1/runs/${encodeURIComponent(runId)}/cancel`);
    return new Run(this.#baseUrl, this.#fetch, runSummaryFromWire(wire));
  }

  async #listApps(): Promise<AppSummary[]> {
    const wire = await this.#request<WireAppSummary[]>("GET", "/v1/apps");
    return wire.map(appSummaryFromWire);
  }

  async #createAppSession(appId: string): Promise<AppSession> {
    const wire = await this.#request<WireAppSession>("POST", `/v1/apps/${encodeURIComponent(appId)}/sessions`);
    return appSessionFromWire(wire);
  }

  async #listConnectors(): Promise<Connector[]> {
    const wire = await this.#request<WireConnector[]>("GET", "/v1/connectors");
    return wire.map(connectorFromWire);
  }

  async #getConnector(connectorId: string): Promise<Connector> {
    const wire = await this.#request<WireConnector>("GET", `/v1/connectors/${encodeURIComponent(connectorId)}`);
    return connectorFromWire(wire);
  }

  async #createConnector(options: CreateConnectorOptions): Promise<Connector> {
    const wire = await this.#request<WireConnector>("POST", "/v1/connectors", {
      id: options.id,
      type: options.type,
      config: options.config ?? {},
      secret_refs: connectorSecretRefsToWire(options.secretRefs),
    });
    return connectorFromWire(wire);
  }

  async #deleteConnector(connectorId: string): Promise<void> {
    await this.#requestNoContent("DELETE", `/v1/connectors/${encodeURIComponent(connectorId)}`);
  }

  async #testConnector(connectorId: string): Promise<ConnectorTestResult> {
    const wire = await this.#request<WireConnectorTestResult>("POST", `/v1/connectors/${encodeURIComponent(connectorId)}/test`);
    return connectorTestResultFromWire(wire);
  }

  async #listPlugins(): Promise<PluginSummary[]> {
    const wire = await this.#request<WirePluginSummary[]>("GET", "/v1/plugins");
    return wire.map(pluginSummaryFromWire);
  }
}
