// Package api exposes the agent's public HTTP API.
package api

import (
	"context"
	"encoding/json"
	"net/http"
	"time"
)

const healthCheckTimeout = 2 * time.Second

type healthResponse struct {
	Status   string `json:"status"`
	Database string `json:"database"`
}

func handleHealth(deps Deps) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		body := healthResponse{Status: "ok", Database: "ok"}
		statusCode := http.StatusOK

		ctx, cancel := context.WithTimeout(r.Context(), healthCheckTimeout)
		defer cancel()

		if err := deps.DB.PingContext(ctx); err != nil {
			body.Status = "degraded"
			body.Database = "unreachable"
			statusCode = http.StatusServiceUnavailable
		}

		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(statusCode)
		_ = json.NewEncoder(w).Encode(body)
	}
}
