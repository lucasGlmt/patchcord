package api

import (
	"net/http"
)

// systemMetricsResponse is the JSON snapshot of the agent's in-process
// metrics registry (GET /v1/system/metrics) — the same counters and gauges
// GET /metrics exposes in Prometheus text format, reshaped for a
// dashboard or application to consume without a Prometheus text parser.
// See ADR-0070.
type systemMetricsResponse struct {
	Runs       runMetricsResponse       `json:"runs"`
	Plugins    []pluginMetricsResponse  `json:"plugins"`
	Scheduler  schedulerMetricsResponse `json:"scheduler"`
	Connectors connectorMetricsResponse `json:"connectors"`
}

type runMetricsResponse struct {
	Transitions map[string]uint64   `json:"transitions"`
	Active      int64               `json:"active"`
	Steps       stepMetricsResponse `json:"steps"`
}

type stepMetricsResponse struct {
	Transitions map[string]uint64 `json:"transitions"`
}

type pluginMetricsResponse struct {
	PluginID                 string `json:"plugin_id"`
	Running                  bool   `json:"running"`
	RestartsTotal            uint64 `json:"restarts_total"`
	HealthCheckFailuresTotal uint64 `json:"health_check_failures_total"`
	QuarantinedTotal         uint64 `json:"quarantined_total"`
}

type schedulerMetricsResponse struct {
	FiresTotal   uint64            `json:"fires_total"`
	SkippedTotal map[string]uint64 `json:"skipped_total"`
	Active       int64             `json:"active"`
}

type connectorMetricsResponse struct {
	TestTotal map[string]uint64 `json:"test_total"`
}

// @Summary      Get a metrics snapshot
// @Description  Returns a JSON snapshot of the agent's in-process metrics registry (run/step transitions and durations, plugin supervision, scheduler activity, connector tests) — the same data GET /metrics exposes in Prometheus text format, reshaped for a dashboard or application. See ADR-0070.
// @Tags         system
// @Produce      json
// @Success      200  {object}  systemMetricsResponse
// @Security     BearerAuth
// @Router       /system/metrics [get]
func handleSystemMetrics(deps Deps) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		snap := deps.metrics().Snapshot()

		plugins := make([]pluginMetricsResponse, 0, len(snap.Plugins))
		for _, p := range snap.Plugins {
			plugins = append(plugins, pluginMetricsResponse{
				PluginID:                 p.PluginID,
				Running:                  p.Running,
				RestartsTotal:            p.RestartsTotal,
				HealthCheckFailuresTotal: p.HealthCheckFailuresTotal,
				QuarantinedTotal:         p.QuarantinedTotal,
			})
		}

		writeJSON(w, http.StatusOK, systemMetricsResponse{
			Runs: runMetricsResponse{
				Transitions: snap.Runs.Transitions,
				Active:      snap.Runs.Active,
				Steps:       stepMetricsResponse{Transitions: snap.Runs.Steps.Transitions},
			},
			Plugins: plugins,
			Scheduler: schedulerMetricsResponse{
				FiresTotal:   snap.Scheduler.FiresTotal,
				SkippedTotal: snap.Scheduler.SkippedTotal,
				Active:       snap.Scheduler.Active,
			},
			Connectors: connectorMetricsResponse{
				TestTotal: snap.Connectors.TestTotal,
			},
		})
	}
}

// handlePrometheusMetrics serves GET /metrics: the agent's in-process
// metrics in Prometheus text-exposition format, for an external
// Prometheus/Grafana server to scrape. Deliberately outside the /v1 prefix
// every JSON route in this package uses, and — like handleAppsDirectory —
// deliberately undocumented in the generated OpenAPI spec (api/agent,
// @BasePath /v1): it is not a JSON resource under that base path, and
// Prometheus scrapers and the wider ecosystem (Grafana Agent,
// kube-prometheus-stack auto-discovery, ...) universally expect /metrics
// at the root regardless. See ADR-0070.
func handlePrometheusMetrics(deps Deps) http.HandlerFunc {
	handler := deps.metrics().Handler()
	return func(w http.ResponseWriter, r *http.Request) {
		handler.ServeHTTP(w, r)
	}
}
