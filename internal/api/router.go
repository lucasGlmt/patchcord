package api

import (
	"context"
	"database/sql"
	"log/slog"
	"net/http"

	"github.com/lucasglmt/patchcord/api/agent"
	"github.com/lucasglmt/patchcord/internal/runs"
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

// NewRouter returns the agent's public HTTP API handler.
func NewRouter(deps Deps) http.Handler {
	mux := http.NewServeMux()
	mux.HandleFunc("GET /v1/system/health", handleHealth(deps))
	mux.HandleFunc("GET /v1/workflows", handleListWorkflows(deps))
	mux.HandleFunc("POST /v1/workflows/{id}/run", handleRunWorkflow(deps))
	mux.HandleFunc("GET /v1/runs", handleListRuns(deps))
	mux.HandleFunc("GET /v1/runs/{id}", handleGetRun(deps))
	mux.HandleFunc("POST /v1/runs/{id}/cancel", handleCancelRun(deps))
	mux.HandleFunc("GET /v1/runs/{id}/events", handleRunEvents(deps))
	mux.HandleFunc("GET /v1/openapi.json", handleOpenAPISpec())
	return withCORS(mux)
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

// withCORS wraps handler with a permissive CORS policy — reflecting any
// Origin, allowing GET/POST and a JSON Content-Type, and answering
// preflight OPTIONS requests directly. This exists solely so a Vite dev
// server (a different origin than the agent) can call this API during
// application development; it is NOT a security boundary and must not be
// read as one — see ADR-0024. Real origin restriction and the vision
// document's "sessions limitées" are deferred.
func withCORS(handler http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Access-Control-Allow-Origin", "*")
		w.Header().Set("Access-Control-Allow-Methods", "GET, POST, OPTIONS")
		w.Header().Set("Access-Control-Allow-Headers", "Content-Type")

		if r.Method == http.MethodOptions {
			w.WriteHeader(http.StatusNoContent)
			return
		}

		handler.ServeHTTP(w, r)
	})
}
