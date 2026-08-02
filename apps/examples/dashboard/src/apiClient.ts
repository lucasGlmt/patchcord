// Under `npm run dev`, Vite serves this app on its own port, separate from
// the agent (127.0.0.1:7331 by default) — cross-origin, allowed by the
// agent's provisional CORS (ADR-0024). Once built and installed
// (`patchcord app install apps/examples/dashboard/dist`), the agent serves
// this app itself at /apps/dashboard/ — same origin, so `window.location.
// origin` is the agent's own base URL with zero configuration needed.
// `import.meta.env.DEV` is Vite's own flag for which of the two this is.
//
// No app session anywhere in this app, on purpose — see index.html and
// public/patchcord-app.yaml's comments: this dashboard is an operator's
// tool that lists and runs any installed workflow, which a session's
// fixed permission allow-list (ADR-0026) cannot express. It calls the API
// (App.tsx builds a plain `new PatchcordClient({ baseUrl })`, no `token`)
// with the same unrestricted access a plain browser tab hitting the agent
// directly already has today.
export const defaultBaseUrl = import.meta.env.DEV ? "http://127.0.0.1:7331" : window.location.origin;
