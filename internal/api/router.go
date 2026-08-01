package api

import (
	"database/sql"
	"net/http"
)

// Deps holds the dependencies the public HTTP API needs to serve requests.
type Deps struct {
	DB *sql.DB
}

// NewRouter returns the agent's public HTTP API handler.
func NewRouter(deps Deps) http.Handler {
	mux := http.NewServeMux()
	mux.HandleFunc("GET /v1/system/health", handleHealth(deps))
	mux.HandleFunc("GET /v1/runs/{id}/events", handleRunEvents(deps))
	return mux
}
