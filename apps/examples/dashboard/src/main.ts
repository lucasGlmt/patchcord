import { PatchcordClient } from "@patchcord/sdk";
import "./style.css";

const app = document.querySelector<HTMLDivElement>("#app");
if (!app) {
  throw new Error("#app element not found");
}

app.innerHTML = `
  <h1>Patchcord Dashboard (example)</h1>
  <form id="run-form">
    <label for="base-url">Agent base URL</label>
    <input id="base-url" value="http://127.0.0.1:7331" />

    <label for="workflow-id">Workflow id</label>
    <input id="workflow-id" value="hello_patchcord" />

    <label for="inputs">Inputs (JSON)</label>
    <textarea id="inputs">{}</textarea>

    <label for="bindings">Bindings (JSON)</label>
    <textarea id="bindings">{}</textarea>

    <button type="submit">Run</button>
  </form>

  <p class="status" id="status"></p>
  <h2>Events</h2>
  <pre id="log"></pre>
  <h2>Result</h2>
  <pre id="result"></pre>
`;

const form = document.querySelector<HTMLFormElement>("#run-form")!;
const statusEl = document.querySelector<HTMLParagraphElement>("#status")!;
const logEl = document.querySelector<HTMLPreElement>("#log")!;
const resultEl = document.querySelector<HTMLPreElement>("#result")!;

function appendLog(line: string): void {
  logEl.textContent += `${line}\n`;
}

function parseJSONField(id: string): Record<string, unknown> {
  const raw = document.querySelector<HTMLTextAreaElement>(`#${id}`)!.value.trim();
  if (raw === "") {
    return {};
  }
  return JSON.parse(raw) as Record<string, unknown>;
}

// fetchAppSession asks the agent for a session limited to this
// application's own declared permissions (patchcord-app.yaml,
// POST /v1/apps/dashboard/sessions — vision document, section 15.4;
// ADR-0026), so this page runs the workflow through the same restricted
// path a real installed application would, rather than the agent's
// unrestricted default. Returns undefined — not an error — when the app
// isn't installed under that id yet (e.g. this page opened straight from
// `npm run dev` without `patchcord app install` first), so local
// development keeps working exactly as it did before app sessions existed.
async function fetchAppSession(baseUrl: string): Promise<string | undefined> {
  try {
    const response = await fetch(`${baseUrl}/v1/apps/dashboard/sessions`, { method: "POST" });
    if (!response.ok) {
      return undefined;
    }
    const body = (await response.json()) as { token: string };
    return body.token;
  } catch {
    return undefined;
  }
}

form.addEventListener("submit", (event) => {
  event.preventDefault();
  void runWorkflow();
});

async function runWorkflow(): Promise<void> {
  logEl.textContent = "";
  resultEl.textContent = "";
  statusEl.textContent = "";
  statusEl.classList.remove("error");

  const baseUrl = document.querySelector<HTMLInputElement>("#base-url")!.value.trim();
  const workflowId = document.querySelector<HTMLInputElement>("#workflow-id")!.value.trim();

  let inputs: Record<string, unknown>;
  let bindings: Record<string, unknown>;
  try {
    inputs = parseJSONField("inputs");
    bindings = parseJSONField("bindings") as Record<string, string>;
  } catch (err) {
    statusEl.textContent = `Invalid JSON: ${(err as Error).message}`;
    statusEl.classList.add("error");
    return;
  }

  const token = await fetchAppSession(baseUrl);
  if (token) {
    appendLog(`Using a limited app session (installed as "dashboard").`);
  }
  const client = new PatchcordClient({ baseUrl, token });

  try {
    statusEl.textContent = "Starting…";
    const run = await client.workflows.run(workflowId, { inputs, bindings: bindings as Record<string, string> });
    statusEl.textContent = `Run ${run.id} — running`;

    for await (const runEvent of run.events()) {
      const label = runEvent.stepId ? `step:${runEvent.stepId}` : "run";
      appendLog(`[${runEvent.time}] ${label} → ${runEvent.status}${runEvent.error ? ` (${runEvent.error})` : ""}`);
    }

    const finalRun = await run.fetch();
    statusEl.textContent = `Run ${finalRun.id} — ${finalRun.status}`;
    resultEl.textContent = JSON.stringify(finalRun, null, 2);
  } catch (err) {
    statusEl.textContent = `Error: ${(err as Error).message}`;
    statusEl.classList.add("error");
  }
}
