package api

import (
	"context"
	"database/sql"
	"log/slog"
	"net/http"

	"github.com/lucasglmt/patchcord/api/agent"
	"github.com/lucasglmt/patchcord/internal/auth"
	"github.com/lucasglmt/patchcord/internal/runs"
	"github.com/lucasglmt/patchcord/internal/secrets"
)

// Deps holds the dependencies the public HTTP API needs to serve requests.
type Deps struct {
	DB *sql.DB
	// Executor runs an action for a workflow step, typically
	// internal/plugins.Supervisor (which satisfies runs.ActionExecutor by
	// duck typing — ADR-0021). Only needed by handlers that trigger a run;
	// left nil, POST /v1/workflows/{id}/run fails clearly rather than
	// panicking.
	Executor runs.ActionExecutor
	// RunCtx is the base context a background-triggered run's runs.Continue
	// call is derived from — never a request's own context, which is
	// cancelled the moment the triggering HTTP response is written, long
	// before the run itself finishes. Defaults to context.Background() when
	// nil, so existing callers that build Deps{DB: db} directly keep
	// working. The agent (internal/runtime) passes a context it cancels
	// during its own shutdown sequence, so an in-flight background run is
	// recorded Cancelled rather than left running against plugins that are
	// about to be torn down.
	RunCtx context.Context
	// Logger receives background-run failures a triggering HTTP request has
	// no way to report back (its response was already sent). Defaults to
	// slog.Default() when nil.
	Logger *slog.Logger
	// Sessions issues and validates the limited sessions installed
	// applications use (vision document, section 15.4). Only dereferenced
	// when a request actually presents an "Authorization: Bearer" header
	// that isn't a valid admin token (withRunAuth) or calls
	// POST /apps/{id}/sessions — left nil, every other existing route keeps
	// working exactly as before this
	// package existed.
	Sessions *auth.Store
	// ConnectorTester attempts a live connection through a resolved
	// connector's plugin, typically the same internal/plugins.Supervisor as
	// Executor (it satisfies both by duck typing). Only needed by
	// POST /connectors/{id}/test — left nil, that one endpoint fails clearly
	// rather than panicking.
	ConnectorTester ConnectorTester
	// Secrets resolves connector and webhook trigger secret references.
	// Defaults to secrets.EnvStore{} when nil, so existing callers that
	// build Deps{DB: db} directly keep resolving "env" references exactly
	// as before secrets.MultiStore existed (ADR-0040).
	Secrets secrets.Store
	// AppsDirectoryListingEnabled turns on GET /apps/, an Apache-style index
	// page listing every installed application with a link to its
	// /apps/{id}/. Defaults to false, so existing callers that build
	// Deps{DB: db} directly keep the pre-ADR-0061 behavior: a plain 404.
	AppsDirectoryListingEnabled bool
}

func (d Deps) secrets() secrets.Store {
	if d.Secrets != nil {
		return d.Secrets
	}
	return secrets.EnvStore{}
}

func (d Deps) runCtx() context.Context {
	if d.RunCtx != nil {
		return d.RunCtx
	}
	return context.Background()
}

func (d Deps) logger() *slog.Logger {
	if d.Logger != nil {
		return d.Logger
	}
	return slog.Default()
}

// NewRouter returns the agent's public HTTP API handler, wiring every route
// behind withAdminAuth (ADR-0036) except four deliberate exceptions:
// GET /v1/system/health (a liveness check has to answer before any caller
// could prove who it is), GET /v1/openapi.json (public API documentation,
// same convention as an authenticated API's docs page), GET /apps/{id}/
// (serves an installed application's own static UI to whichever end user's
// browser loads it — that end user is never expected to hold an admin
// token), and GET /apps/ (an index of the same, opt-in via
// AppsDirectoryListingEnabled — see ADR-0061; it stays unauthenticated for
// the same reason /apps/{id}/ does, since it exposes nothing an end user
// couldn't already reach one /apps/{id}/ URL at a time). Three routes get
// their own dedicated wrapping instead:
// POST /v1/workflows/{id}/run and POST /v1/apps/{id}/sessions (see
// withRunAuth and handleCreateAppSession's doc comment), and
// POST /v1/webhooks/{id} (never admin-gated at all — see
// handleWebhookTrigger's doc comment, ADR-0037).
func NewRouter(deps Deps) http.Handler {
	mux := http.NewServeMux()
	mux.HandleFunc("GET /v1/system/health", handleHealth(deps))
	mux.HandleFunc("GET /v1/workflows", withAdminAuth(deps, handleListWorkflows(deps)))
	mux.HandleFunc("GET /v1/workflows/{id}", withAdminAuth(deps, handleGetWorkflow(deps)))
	mux.HandleFunc("POST /v1/workflows/{id}/run", withRunAuth(deps, handleRunWorkflow(deps)))
	mux.HandleFunc("GET /v1/runs", withAdminAuth(deps, handleListRuns(deps)))
	mux.HandleFunc("GET /v1/runs/{id}", withAdminAuth(deps, handleGetRun(deps)))
	mux.HandleFunc("POST /v1/runs/{id}/cancel", withAdminAuth(deps, handleCancelRun(deps)))
	mux.HandleFunc("GET /v1/runs/{id}/events", withAdminAuth(deps, handleRunEvents(deps)))
	mux.HandleFunc("GET /v1/openapi.json", handleOpenAPISpec())
	mux.HandleFunc("POST /v1/webhooks/{id}", handleWebhookTrigger(deps))
	mux.HandleFunc("GET /v1/apps", withAdminAuth(deps, handleListApps(deps)))
	mux.HandleFunc("POST /v1/apps/{id}/sessions", withAdminAuth(deps, handleCreateAppSession(deps)))
	mux.HandleFunc("GET /apps/", handleAppsDirectory(deps))
	mux.HandleFunc("GET /apps/{id}/", handleServeApp(deps))
	mux.HandleFunc("GET /v1/connectors", withAdminAuth(deps, handleListConnectors(deps)))
	mux.HandleFunc("POST /v1/connectors", withAdminAuth(deps, handleCreateConnector(deps)))
	mux.HandleFunc("GET /v1/connectors/{id}", withAdminAuth(deps, handleGetConnector(deps)))
	mux.HandleFunc("DELETE /v1/connectors/{id}", withAdminAuth(deps, handleDeleteConnector(deps)))
	mux.HandleFunc("POST /v1/connectors/{id}/test", withAdminAuth(deps, handleTestConnector(deps)))
	mux.HandleFunc("GET /v1/plugins", withAdminAuth(deps, handleListPlugins(deps)))
	return withCORS(deps, mux)
}

// handleOpenAPISpec serves the OpenAPI (Swagger 2.0) document generated
// from this package's swag annotations (`make swagger`) — see api/agent's
// doc comment. Static content, so it needs nothing from Deps.
func handleOpenAPISpec() http.HandlerFunc {
	return func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write(agent.Spec)
	}
}

// withCORS wraps handler with a CORS policy that tracks the same opt-in
// flag withAdminAuth already gates on (auth.AnyTokensExist, ADR-0036): with
// no admin token ever created, it stays exactly as permissive as before
// (reflecting any Origin) so a Vite dev server can call this API during
// application development. Once an operator creates the first admin token,
// permissive headers stop going out entirely — a browser then refuses to
// let any cross-origin page read a response even though withAdminAuth would
// otherwise leave "no token presented" ambiguous with "not a browser at
// all". Before ADR-0045, the wildcard was unconditional: a page open in any
// tab could script requests against a freshly installed, not-yet-token'd
// agent and read the JSON back, which is exactly the state ADR-0036 was
// meant to close off. Same-origin callers (curl, the CLI, an app served
// from /apps/{id}/ itself) are unaffected either way — browsers don't
// consult CORS headers for same-origin requests. See ADR-0045.
func withCORS(deps Deps, handler http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		enforced, err := auth.AnyTokensExist(r.Context(), deps.DB)
		// A lookup failure fails closed (no permissive headers): the
		// underlying handler hits the same auth.AnyTokensExist call via
		// withAdminAuth/withRunAuth and reports the real 500 to the caller;
		// this layer just shouldn't hand out cross-origin readability on
		// top of an already-broken auth check.
		if err == nil && !enforced {
			w.Header().Set("Access-Control-Allow-Origin", "*")
			w.Header().Set("Access-Control-Allow-Methods", "GET, POST, DELETE, OPTIONS")
			w.Header().Set("Access-Control-Allow-Headers", "Content-Type, Authorization")
		}

		if r.Method == http.MethodOptions {
			w.WriteHeader(http.StatusNoContent)
			return
		}

		handler.ServeHTTP(w, r)
	})
}
