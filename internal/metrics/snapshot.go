package metrics

import (
	"sort"

	dto "github.com/prometheus/client_model/go"
)

// Snapshot is a point-in-time, JSON-friendly view of everything a Registry
// has recorded — the shape internal/api's GET /v1/system/metrics handler
// serializes. It deliberately carries less detail than the Prometheus text
// exposition (no histogram buckets): a JSON snapshot is for a dashboard
// widget or a quick status check, not for the same kind of querying an
// external Prometheus server does over the text endpoint.
type Snapshot struct {
	Runs       RunMetrics       `json:"runs"`
	Plugins    []PluginMetrics  `json:"plugins"`
	Scheduler  SchedulerMetrics `json:"scheduler"`
	Connectors ConnectorMetrics `json:"connectors"`
}

// RunMetrics summarizes run and step transitions recorded so far.
type RunMetrics struct {
	// Transitions maps a run status (e.g. "running", "succeeded", "failed",
	// "cancelled") to how many times a run has transitioned into it.
	Transitions map[string]uint64 `json:"transitions"`
	// Active is the number of runs currently in the running state.
	Active int64       `json:"active"`
	Steps  StepMetrics `json:"steps"`
}

// StepMetrics summarizes step transitions recorded so far.
type StepMetrics struct {
	// Transitions maps a step status to how many times a step has
	// transitioned into it.
	Transitions map[string]uint64 `json:"transitions"`
}

// PluginMetrics summarizes one plugin's supervision history. A plugin only
// appears here once it has been observed at least once by the Supervisor
// (typically: launched) — a plugin that has never successfully started
// does not have an entry.
type PluginMetrics struct {
	PluginID                 string `json:"plugin_id"`
	Running                  bool   `json:"running"`
	RestartsTotal            uint64 `json:"restarts_total"`
	HealthCheckFailuresTotal uint64 `json:"health_check_failures_total"`
	QuarantinedTotal         uint64 `json:"quarantined_total"`
}

// SchedulerMetrics summarizes the scheduler's activity so far.
type SchedulerMetrics struct {
	FiresTotal uint64 `json:"fires_total"`
	// SkippedTotal maps a skip reason (e.g. "caught_up") to how many times
	// a due occurrence was skipped for it.
	SkippedTotal map[string]uint64 `json:"skipped_total"`
	// Active is the number of workflows currently registered with a
	// schedule trigger.
	Active int64 `json:"active"`
}

// ConnectorMetrics summarizes connector test attempts recorded so far.
type ConnectorMetrics struct {
	// TestTotal maps a test result ("ok", "failed", "error") to how many
	// attempts ended with it.
	TestTotal map[string]uint64 `json:"test_total"`
}

// Snapshot gathers this registry's current values into a Snapshot. It goes
// through the same prometheus.Gatherer interface promhttp's text handler
// uses internally — client_golang deliberately does not expose a "read the
// current value" method on a live Counter/Gauge/HistogramVec, since its
// instrumentation API is write-only by design.
func (r *Registry) Snapshot() Snapshot {
	families, _ := r.reg.Gather() // Gather can only fail on a broken collector, a programming bug New already panics on.

	running := gaugeMapByLabel(families, "patchcord_plugin_running", "plugin_id")
	restarts := counterMapByLabel(families, "patchcord_plugin_restarts_total", "plugin_id")
	healthFailures := counterMapByLabel(families, "patchcord_plugin_health_check_failures_total", "plugin_id")
	quarantined := counterMapByLabel(families, "patchcord_plugin_quarantined_total", "plugin_id")

	pluginIDs := make(map[string]struct{})
	for id := range running {
		pluginIDs[id] = struct{}{}
	}
	for id := range restarts {
		pluginIDs[id] = struct{}{}
	}
	for id := range healthFailures {
		pluginIDs[id] = struct{}{}
	}
	for id := range quarantined {
		pluginIDs[id] = struct{}{}
	}

	plugins := make([]PluginMetrics, 0, len(pluginIDs))
	for id := range pluginIDs {
		plugins = append(plugins, PluginMetrics{
			PluginID:                 id,
			Running:                  running[id] == 1,
			RestartsTotal:            restarts[id],
			HealthCheckFailuresTotal: healthFailures[id],
			QuarantinedTotal:         quarantined[id],
		})
	}
	sort.Slice(plugins, func(i, j int) bool { return plugins[i].PluginID < plugins[j].PluginID })

	return Snapshot{
		Runs: RunMetrics{
			Transitions: counterMapByLabel(families, "patchcord_run_transitions_total", "status"),
			Active:      int64(gaugeValue(families, "patchcord_active_runs")),
			Steps: StepMetrics{
				Transitions: counterMapByLabel(families, "patchcord_step_transitions_total", "status"),
			},
		},
		Plugins: plugins,
		Scheduler: SchedulerMetrics{
			FiresTotal:   uint64(counterValue(families, "patchcord_schedule_fires_total")),
			SkippedTotal: counterMapByLabel(families, "patchcord_schedule_skipped_total", "reason"),
			Active:       int64(gaugeValue(families, "patchcord_active_schedules")),
		},
		Connectors: ConnectorMetrics{
			TestTotal: counterMapByLabel(families, "patchcord_connector_test_total", "result"),
		},
	}
}

func familyByName(families []*dto.MetricFamily, name string) *dto.MetricFamily {
	for _, f := range families {
		if f.GetName() == name {
			return f
		}
	}
	return nil
}

func labelValue(m *dto.Metric, name string) string {
	for _, lp := range m.GetLabel() {
		if lp.GetName() == name {
			return lp.GetValue()
		}
	}
	return ""
}

// counterMapByLabel reads familyName's metric family (a CounterVec with a
// single label labelName) into a map of that label's value to the
// counter's current count. Missing family/metrics yield an empty map, not
// a nil one, so callers (and their JSON encoding) never need a nil check.
func counterMapByLabel(families []*dto.MetricFamily, familyName, labelName string) map[string]uint64 {
	result := make(map[string]uint64)
	f := familyByName(families, familyName)
	if f == nil {
		return result
	}
	for _, m := range f.GetMetric() {
		result[labelValue(m, labelName)] = uint64(m.GetCounter().GetValue())
	}
	return result
}

// gaugeMapByLabel is counterMapByLabel's counterpart for a GaugeVec.
func gaugeMapByLabel(families []*dto.MetricFamily, familyName, labelName string) map[string]float64 {
	result := make(map[string]float64)
	f := familyByName(families, familyName)
	if f == nil {
		return result
	}
	for _, m := range f.GetMetric() {
		result[labelValue(m, labelName)] = m.GetGauge().GetValue()
	}
	return result
}

// counterValue reads a label-less Counter's current value.
func counterValue(families []*dto.MetricFamily, familyName string) float64 {
	f := familyByName(families, familyName)
	if f == nil || len(f.GetMetric()) == 0 {
		return 0
	}
	return f.GetMetric()[0].GetCounter().GetValue()
}

// gaugeValue reads a label-less Gauge's current value.
func gaugeValue(families []*dto.MetricFamily, familyName string) float64 {
	f := familyByName(families, familyName)
	if f == nil || len(f.GetMetric()) == 0 {
		return 0
	}
	return f.GetMetric()[0].GetGauge().GetValue()
}
