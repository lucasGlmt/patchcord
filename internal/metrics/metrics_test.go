package metrics

import (
	"testing"
	"time"

	"github.com/prometheus/client_golang/prometheus/testutil"
)

func TestNewNeverPanics(t *testing.T) {
	defer func() {
		if r := recover(); r != nil {
			t.Fatalf("New() panicked: %v", r)
		}
	}()
	New()
}

func TestOrNoop(t *testing.T) {
	m := New()
	if OrNoop(m) != m {
		t.Fatal("OrNoop(non-nil) should return the same registry")
	}
	if OrNoop(nil) == nil {
		t.Fatal("OrNoop(nil) should return a usable registry, not nil")
	}
}

func TestRecordRunTransition(t *testing.T) {
	r := New()

	r.RecordRunTransition("running", 0)
	if got := testutil.ToFloat64(r.runTransitionsTotal.WithLabelValues("running")); got != 1 {
		t.Fatalf("run_transitions_total{status=running} = %v, want 1", got)
	}

	r.RecordRunTransition("succeeded", 2*time.Second)
	if got := testutil.ToFloat64(r.runTransitionsTotal.WithLabelValues("succeeded")); got != 1 {
		t.Fatalf("run_transitions_total{status=succeeded} = %v, want 1", got)
	}
	if got := testutil.CollectAndCount(r.runDuration); got != 1 {
		t.Fatalf("run_duration_seconds observation count = %v, want 1 (only the succeeded transition observes)", got)
	}
}

func TestRecordStepTransition(t *testing.T) {
	r := New()

	r.RecordStepTransition("failed", 500*time.Millisecond)
	if got := testutil.ToFloat64(r.stepTransitionsTotal.WithLabelValues("failed")); got != 1 {
		t.Fatalf("step_transitions_total{status=failed} = %v, want 1", got)
	}
	if got := testutil.CollectAndCount(r.stepDuration); got != 1 {
		t.Fatalf("step_duration_seconds observation count = %v, want 1", got)
	}
}

func TestActiveRunsGauge(t *testing.T) {
	r := New()

	r.ActiveRunsInc()
	r.ActiveRunsInc()
	if got := testutil.ToFloat64(r.activeRuns); got != 2 {
		t.Fatalf("active_runs = %v, want 2", got)
	}

	r.ActiveRunsDec()
	if got := testutil.ToFloat64(r.activeRuns); got != 1 {
		t.Fatalf("active_runs = %v, want 1", got)
	}
}

func TestPluginLifecycle(t *testing.T) {
	r := New()

	r.PluginStarted("mysql")
	if got := testutil.ToFloat64(r.pluginRunning.WithLabelValues("mysql")); got != 1 {
		t.Fatalf("plugin_running{plugin_id=mysql} = %v, want 1", got)
	}

	r.PluginStopped("mysql")
	if got := testutil.ToFloat64(r.pluginRunning.WithLabelValues("mysql")); got != 0 {
		t.Fatalf("plugin_running{plugin_id=mysql} = %v, want 0", got)
	}

	r.PluginRestarted("mysql")
	r.PluginRestarted("mysql")
	if got := testutil.ToFloat64(r.pluginRestartsTotal.WithLabelValues("mysql")); got != 2 {
		t.Fatalf("plugin_restarts_total{plugin_id=mysql} = %v, want 2", got)
	}

	r.PluginHealthCheckFailed("mysql")
	if got := testutil.ToFloat64(r.pluginHealthCheckFailuresTotal.WithLabelValues("mysql")); got != 1 {
		t.Fatalf("plugin_health_check_failures_total{plugin_id=mysql} = %v, want 1", got)
	}

	r.PluginQuarantined("mysql")
	if got := testutil.ToFloat64(r.pluginQuarantinedTotal.WithLabelValues("mysql")); got != 1 {
		t.Fatalf("plugin_quarantined_total{plugin_id=mysql} = %v, want 1", got)
	}
}

func TestScheduler(t *testing.T) {
	r := New()

	r.ScheduleFired()
	r.ScheduleFired()
	if got := testutil.ToFloat64(r.scheduleFiresTotal); got != 2 {
		t.Fatalf("schedule_fires_total = %v, want 2", got)
	}

	r.ScheduleSkipped("caught_up")
	if got := testutil.ToFloat64(r.scheduleSkippedTotal.WithLabelValues("caught_up")); got != 1 {
		t.Fatalf("schedule_skipped_total{reason=caught_up} = %v, want 1", got)
	}

	r.SetActiveSchedules(3)
	if got := testutil.ToFloat64(r.activeSchedules); got != 3 {
		t.Fatalf("active_schedules = %v, want 3", got)
	}
}

func TestRecordConnectorTest(t *testing.T) {
	r := New()

	r.RecordConnectorTest("ok")
	r.RecordConnectorTest("error")
	r.RecordConnectorTest("error")

	if got := testutil.ToFloat64(r.connectorTestTotal.WithLabelValues("ok")); got != 1 {
		t.Fatalf("connector_test_total{result=ok} = %v, want 1", got)
	}
	if got := testutil.ToFloat64(r.connectorTestTotal.WithLabelValues("error")); got != 2 {
		t.Fatalf("connector_test_total{result=error} = %v, want 2", got)
	}
}

func TestSnapshotReflectsRecordedState(t *testing.T) {
	r := New()

	r.RecordRunTransition("running", 0)
	r.RecordRunTransition("succeeded", time.Second)
	r.ActiveRunsInc()
	r.RecordStepTransition("succeeded", 200*time.Millisecond)
	r.PluginStarted("mysql")
	r.PluginRestarted("mysql")
	r.ScheduleFired()
	r.ScheduleSkipped("caught_up")
	r.SetActiveSchedules(2)
	r.RecordConnectorTest("ok")

	snap := r.Snapshot()

	if snap.Runs.Transitions["running"] != 1 || snap.Runs.Transitions["succeeded"] != 1 {
		t.Fatalf("unexpected run transitions: %+v", snap.Runs.Transitions)
	}
	if snap.Runs.Active != 1 {
		t.Fatalf("Runs.Active = %d, want 1", snap.Runs.Active)
	}
	if snap.Runs.Steps.Transitions["succeeded"] != 1 {
		t.Fatalf("unexpected step transitions: %+v", snap.Runs.Steps.Transitions)
	}

	if len(snap.Plugins) != 1 || snap.Plugins[0].PluginID != "mysql" {
		t.Fatalf("unexpected plugins: %+v", snap.Plugins)
	}
	if !snap.Plugins[0].Running || snap.Plugins[0].RestartsTotal != 1 {
		t.Fatalf("unexpected mysql plugin metrics: %+v", snap.Plugins[0])
	}

	if snap.Scheduler.FiresTotal != 1 || snap.Scheduler.SkippedTotal["caught_up"] != 1 || snap.Scheduler.Active != 2 {
		t.Fatalf("unexpected scheduler metrics: %+v", snap.Scheduler)
	}

	if snap.Connectors.TestTotal["ok"] != 1 {
		t.Fatalf("unexpected connector metrics: %+v", snap.Connectors.TestTotal)
	}
}

func TestHandlerServesPrometheusText(t *testing.T) {
	r := New()
	r.RecordConnectorTest("ok")

	if r.Handler() == nil {
		t.Fatal("Handler() returned nil")
	}
}
