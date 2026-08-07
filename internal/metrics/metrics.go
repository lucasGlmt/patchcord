// Package metrics is the agent's in-process, Prometheus-style metrics
// registry. Every subsystem that wants to record something (runs, the
// plugin supervisor, the scheduler, connector tests) does so through a
// small, intention-revealing method on Registry — never by importing
// client_golang directly — so the exact metric names and label vocabulary
// stay defined in one place.
//
// The same Registry backs two different exposures: a raw Prometheus
// text-exposition endpoint (GET /metrics, see Handler) for an external
// Prometheus/Grafana to scrape, and a JSON snapshot (GET
// /v1/system/metrics, see Snapshot) the TypeScript SDK consumes to build
// dashboards. Metrics are in-memory only and reset on every process
// restart — there is no persistence here, and none is planned; see
// ADR-0070.
package metrics

import (
	"net/http"
	"time"

	"github.com/prometheus/client_golang/prometheus"
	"github.com/prometheus/client_golang/prometheus/collectors"
	"github.com/prometheus/client_golang/prometheus/promhttp"
)

// namespace prefixes every metric this package registers, e.g.
// patchcord_run_transitions_total.
const namespace = "patchcord"

// Registry owns every metric the agent records and the private
// *prometheus.Registry they are registered against. It is never a global
// singleton — like every other shared dependency in this codebase
// (*slog.Logger, *sql.DB), it is built once (New, typically in
// internal/runtime.NewAgent) and passed explicitly into the constructors
// that need it. A private registry, rather than
// prometheus.DefaultRegisterer, also means table-driven tests that build
// several Supervisor/Runner instances in the same test binary each get
// their own Registry and never collide on a duplicate registration.
type Registry struct {
	reg *prometheus.Registry

	runTransitionsTotal  *prometheus.CounterVec
	stepTransitionsTotal *prometheus.CounterVec
	runDuration          *prometheus.HistogramVec
	stepDuration         *prometheus.HistogramVec
	activeRuns           prometheus.Gauge

	pluginRunning                  *prometheus.GaugeVec
	pluginRestartsTotal            *prometheus.CounterVec
	pluginHealthCheckFailuresTotal *prometheus.CounterVec
	pluginQuarantinedTotal         *prometheus.CounterVec

	scheduleFiresTotal   prometheus.Counter
	scheduleSkippedTotal *prometheus.CounterVec
	activeSchedules      prometheus.Gauge

	connectorTestTotal *prometheus.CounterVec
}

// durationBuckets covers a workflow step or run's plausible duration range:
// sub-second action calls up to a multi-minute long-running step. Wider and
// coarser than client_golang's DefBuckets (which tops out at 10s) since a
// workflow step commonly calls out to a slow external system.
var durationBuckets = []float64{.05, .1, .25, .5, 1, 2.5, 5, 10, 30, 60, 120, 300, 600}

// New builds a Registry with every metric registered under the "patchcord"
// namespace, plus the standard Go runtime/process collectors (memory, GC,
// file descriptors, ...) client_golang provides for free — generic process
// health, not business-specific, so registering them here does not touch
// the core's non-negotiable #3 (no concrete business service in internal/).
// Registration can only fail on a programming bug (e.g. two metrics
// declared with the same name), so New panics rather than returning an
// error a caller could plausibly ignore — exactly like an invalid regexp
// literal would; go test ./... catches it immediately.
func New() *Registry {
	reg := prometheus.NewRegistry()

	r := &Registry{
		reg: reg,

		runTransitionsTotal: prometheus.NewCounterVec(prometheus.CounterOpts{
			Namespace: namespace,
			Name:      "run_transitions_total",
			Help:      "Total number of run status transitions, by the status transitioned to.",
		}, []string{"status"}),
		stepTransitionsTotal: prometheus.NewCounterVec(prometheus.CounterOpts{
			Namespace: namespace,
			Name:      "step_transitions_total",
			Help:      "Total number of step status transitions, by the status transitioned to.",
		}, []string{"status"}),
		runDuration: prometheus.NewHistogramVec(prometheus.HistogramOpts{
			Namespace: namespace,
			Name:      "run_duration_seconds",
			Help:      "Duration of a run from started to a terminal status, by that terminal status.",
			Buckets:   durationBuckets,
		}, []string{"status"}),
		stepDuration: prometheus.NewHistogramVec(prometheus.HistogramOpts{
			Namespace: namespace,
			Name:      "step_duration_seconds",
			Help:      "Duration of a step from running to a terminal status, by that terminal status.",
			Buckets:   durationBuckets,
		}, []string{"status"}),
		activeRuns: prometheus.NewGauge(prometheus.GaugeOpts{
			Namespace: namespace,
			Name:      "active_runs",
			Help:      "Number of runs currently in the running state.",
		}),

		pluginRunning: prometheus.NewGaugeVec(prometheus.GaugeOpts{
			Namespace: namespace,
			Name:      "plugin_running",
			Help:      "Whether a plugin is currently running (1) or not (0), by plugin id.",
		}, []string{"plugin_id"}),
		pluginRestartsTotal: prometheus.NewCounterVec(prometheus.CounterOpts{
			Namespace: namespace,
			Name:      "plugin_restarts_total",
			Help:      "Total number of restart attempts made for a plugin after a crash or failed health check, by plugin id.",
		}, []string{"plugin_id"}),
		pluginHealthCheckFailuresTotal: prometheus.NewCounterVec(prometheus.CounterOpts{
			Namespace: namespace,
			Name:      "plugin_health_check_failures_total",
			Help:      "Total number of failed periodic health checks, by plugin id.",
		}, []string{"plugin_id"}),
		pluginQuarantinedTotal: prometheus.NewCounterVec(prometheus.CounterOpts{
			Namespace: namespace,
			Name:      "plugin_quarantined_total",
			Help:      "Total number of times a plugin was quarantined after exhausting its restart attempts, by plugin id.",
		}, []string{"plugin_id"}),

		scheduleFiresTotal: prometheus.NewCounter(prometheus.CounterOpts{
			Namespace: namespace,
			Name:      "schedule_fires_total",
			Help:      "Total number of scheduled runs the scheduler has fired.",
		}),
		scheduleSkippedTotal: prometheus.NewCounterVec(prometheus.CounterOpts{
			Namespace: namespace,
			Name:      "schedule_skipped_total",
			Help:      "Total number of due schedule occurrences the scheduler decided not to fire, by reason.",
		}, []string{"reason"}),
		activeSchedules: prometheus.NewGauge(prometheus.GaugeOpts{
			Namespace: namespace,
			Name:      "active_schedules",
			Help:      "Number of workflows currently registered with a schedule trigger.",
		}),

		connectorTestTotal: prometheus.NewCounterVec(prometheus.CounterOpts{
			Namespace: namespace,
			Name:      "connector_test_total",
			Help:      "Total number of connector test attempts, by result (ok, failed, error).",
		}, []string{"result"}),
	}

	reg.MustRegister(
		r.runTransitionsTotal,
		r.stepTransitionsTotal,
		r.runDuration,
		r.stepDuration,
		r.activeRuns,
		r.pluginRunning,
		r.pluginRestartsTotal,
		r.pluginHealthCheckFailuresTotal,
		r.pluginQuarantinedTotal,
		r.scheduleFiresTotal,
		r.scheduleSkippedTotal,
		r.activeSchedules,
		r.connectorTestTotal,
		collectors.NewGoCollector(),
		collectors.NewProcessCollector(collectors.ProcessCollectorOpts{}),
	)

	return r
}

// OrNoop returns m, or a freshly built, privately held Registry when m is
// nil — the same nil-safe-default convention used throughout this codebase
// for *slog.Logger (see Deps.logger in internal/api). Resolving nil exactly
// once at construction time means every recording method below is always
// safe to call unconditionally, with no nil check scattered across call
// sites in internal/runs, internal/plugins or internal/scheduler.
func OrNoop(m *Registry) *Registry {
	if m != nil {
		return m
	}
	return New()
}

// Handler returns the Prometheus text-exposition HTTP handler for this
// registry's metrics — mounted at GET /metrics by internal/api.
func (r *Registry) Handler() http.Handler {
	return promhttp.HandlerFor(r.reg, promhttp.HandlerOpts{})
}

// RecordRunTransition records a run transitioning to status. duration is
// the run's total elapsed time and is only observed into the duration
// histogram when non-zero — a non-terminal transition (e.g. into
// "running") has no meaningful duration yet, so callers pass zero for
// those.
func (r *Registry) RecordRunTransition(status string, duration time.Duration) {
	r.runTransitionsTotal.WithLabelValues(status).Inc()
	if duration > 0 {
		r.runDuration.WithLabelValues(status).Observe(duration.Seconds())
	}
}

// RecordStepTransition records a step transitioning to status, following
// the same duration convention as RecordRunTransition.
func (r *Registry) RecordStepTransition(status string, duration time.Duration) {
	r.stepTransitionsTotal.WithLabelValues(status).Inc()
	if duration > 0 {
		r.stepDuration.WithLabelValues(status).Observe(duration.Seconds())
	}
}

// ActiveRunsInc records one more run entering the running state.
func (r *Registry) ActiveRunsInc() {
	r.activeRuns.Inc()
}

// ActiveRunsDec records one run leaving the running state for a terminal
// one.
func (r *Registry) ActiveRunsDec() {
	r.activeRuns.Dec()
}

// PluginStarted records pluginID as currently running — called both after
// its initial launch and after a successful restart.
func (r *Registry) PluginStarted(pluginID string) {
	r.pluginRunning.WithLabelValues(pluginID).Set(1)
}

// PluginStopped records pluginID as no longer running — called on crash
// and on a failed health check, before a restart is attempted.
func (r *Registry) PluginStopped(pluginID string) {
	r.pluginRunning.WithLabelValues(pluginID).Set(0)
}

// PluginRestarted records one restart attempt for pluginID — incremented
// when the attempt is made, regardless of whether the relaunch itself then
// succeeds, since "how many restarts has this plugin needed" is the
// operationally interesting signal.
func (r *Registry) PluginRestarted(pluginID string) {
	r.pluginRestartsTotal.WithLabelValues(pluginID).Inc()
}

// PluginHealthCheckFailed records one failed periodic health check for
// pluginID.
func (r *Registry) PluginHealthCheckFailed(pluginID string) {
	r.pluginHealthCheckFailuresTotal.WithLabelValues(pluginID).Inc()
}

// PluginQuarantined records pluginID as quarantined after exhausting its
// restart attempts.
func (r *Registry) PluginQuarantined(pluginID string) {
	r.pluginQuarantinedTotal.WithLabelValues(pluginID).Inc()
}

// ScheduleFired records the scheduler deciding to fire a due schedule.
func (r *Registry) ScheduleFired() {
	r.scheduleFiresTotal.Inc()
}

// ScheduleSkipped records the scheduler deciding not to fire a due
// schedule occurrence, e.g. because it caught up on a backlog under the
// "skip" on_missed policy.
func (r *Registry) ScheduleSkipped(reason string) {
	r.scheduleSkippedTotal.WithLabelValues(reason).Inc()
}

// SetActiveSchedules sets the current number of workflows registered with
// a schedule trigger.
func (r *Registry) SetActiveSchedules(n int) {
	r.activeSchedules.Set(float64(n))
}

// RecordConnectorTest records one connector test attempt's outcome: "ok"
// (the connector answered successfully), "failed" (it was reached but
// reported itself unhealthy) or "error" (the test could not even be
// attempted, e.g. no running plugin declares that connector type).
func (r *Registry) RecordConnectorTest(result string) {
	r.connectorTestTotal.WithLabelValues(result).Inc()
}
