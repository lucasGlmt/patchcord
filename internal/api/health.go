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

// @Summary      Check agent health
// @Description  Reports whether the agent's database is reachable.
// @Tags         system
// @Produce      json
// @Success      200  {object}  healthResponse
// @Failure      503  {object}  healthResponse
// @Router       /system/health [get]
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
