package plugins

import (
	"bytes"
	"context"
	"database/sql"
	"log/slog"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"testing"
	"time"

	"google.golang.org/grpc/health/grpc_health_v1"

	"github.com/lucasglmt/patchcord/internal/connectors"
	"github.com/lucasglmt/patchcord/internal/metrics"
)

// testSupervisorConfig keeps timings short so crash/health-check/restart
// scenarios settle quickly without being so tight the test flakes on a
// loaded machine.
func testSupervisorConfig() SupervisorConfig {
	return SupervisorConfig{
		HealthCheckInterval: 50 * time.Millisecond,
		HealthCheckTimeout:  200 * time.Millisecond,
		MaxRestarts:         2,
		RestartDelay:        50 * time.Millisecond,
	}
}

// syncBuffer is an io.Writer safe for the concurrent access a Supervisor's
// background goroutines and a test observing its logs both need.
type syncBuffer struct {
	mu  sync.Mutex
	buf bytes.Buffer
}

func (b *syncBuffer) Write(p []byte) (int, error) {
	b.mu.Lock()
	defer b.mu.Unlock()
	return b.buf.Write(p)
}

func (b *syncBuffer) String() string {
	b.mu.Lock()
	defer b.mu.Unlock()
	return b.buf.String()
}

// seedCatalog inserts a catalog entry directly, bypassing Install's own
// validation launch — necessary here because that extra launch would
// consume a fixture's "crash once" marker or inflate its launch counter
// before the Supervisor ever gets to it.
func seedCatalog(t *testing.T, db *sql.DB, id, path string) {
	t.Helper()
	if err := upsertCatalogEntry(context.Background(), db, &CatalogEntry{
		PluginID:        id,
		Version:         "0.0.1",
		ExecutablePath:  path,
		ProtocolVersion: 1,
	}); err != nil {
		t.Fatalf("seed catalog entry: %v", err)
	}
}

func waitForCondition(t *testing.T, timeout time.Duration, cond func() bool) {
	t.Helper()
	deadline := time.Now().Add(timeout)
	for time.Now().Before(deadline) {
		if cond() {
			return
		}
		time.Sleep(10 * time.Millisecond)
	}
	t.Fatalf("condition not met within %s", timeout)
}

func launchCount(t *testing.T, counterFile string) int {
	t.Helper()
	data, err := os.ReadFile(counterFile)
	if err != nil {
		if os.IsNotExist(err) {
			return 0
		}
		t.Fatalf("read launch counter file: %v", err)
	}
	return len(data)
}

func stopSupervisor(t *testing.T, sup *Supervisor) {
	t.Helper()
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()
	sup.Stop(ctx)
}

func TestSupervisor_StartLaunchesAndStopsAHealthyPlugin(t *testing.T) {
	db := openCatalogTestDB(t)
	seedCatalog(t, db, "io.patchcord.fake", fakePluginPath)

	logger := slog.New(slog.NewTextHandler(&syncBuffer{}, nil))
	m := metrics.New()
	sup := NewSupervisor(testSupervisorConfig(), logger, m)

	if err := sup.Start(context.Background(), db); err != nil {
		t.Fatalf("Start() error = %v", err)
	}

	sup.mu.Lock()
	rp := sup.running["io.patchcord.fake"]
	sup.mu.Unlock()
	if rp == nil {
		t.Fatal("expected the plugin to be running after Start()")
	}
	if snap := m.Snapshot(); len(snap.Plugins) != 1 || snap.Plugins[0].PluginID != "io.patchcord.fake" || !snap.Plugins[0].Running {
		t.Fatalf("metrics snapshot plugins = %+v, want one running io.patchcord.fake entry", snap.Plugins)
	}

	healthCtx, cancel := context.WithTimeout(context.Background(), time.Second)
	resp, err := rp.proc.HealthClient.Check(healthCtx, &grpc_health_v1.HealthCheckRequest{})
	cancel()
	if err != nil {
		t.Fatalf("Check() error = %v", err)
	}
	if resp.GetStatus() != grpc_health_v1.HealthCheckResponse_SERVING {
		t.Fatalf("status = %v, want SERVING", resp.GetStatus())
	}

	stopSupervisor(t, sup)

	sup.mu.Lock()
	remaining := len(sup.running)
	sup.mu.Unlock()
	if remaining != 0 {
		t.Fatalf("running plugins after Stop() = %d, want 0", remaining)
	}
}

func TestSupervisor_ExecuteAction(t *testing.T) {
	db := openCatalogTestDB(t)
	if _, err := Install(context.Background(), db, examplePluginPath); err != nil {
		t.Fatalf("Install() error = %v", err)
	}

	logger := slog.New(slog.NewTextHandler(&syncBuffer{}, nil))
	sup := NewSupervisor(testSupervisorConfig(), logger, metrics.New())
	if err := sup.Start(context.Background(), db); err != nil {
		t.Fatalf("Start() error = %v", err)
	}
	defer stopSupervisor(t, sup)

	output, err := sup.ExecuteAction(context.Background(), "text.uppercase@1", map[string]any{"value": "hello"}, nil)
	if err != nil {
		t.Fatalf("ExecuteAction() error = %v", err)
	}
	if output["value"] != "HELLO" {
		t.Fatalf(`output["value"] = %v, want %q`, output["value"], "HELLO")
	}

	if _, err := sup.ExecuteAction(context.Background(), "unknown.action@1", nil, nil); err == nil {
		t.Fatal("expected an error for an action no running plugin contributes, got nil")
	}
}

func TestSupervisor_TestConnector(t *testing.T) {
	t.Run("returns an error when no running plugin declares the connector's type", func(t *testing.T) {
		db := openCatalogTestDB(t)
		seedCatalog(t, db, "io.patchcord.fake", fakePluginPath)

		logger := slog.New(slog.NewTextHandler(&syncBuffer{}, nil))
		sup := NewSupervisor(testSupervisorConfig(), logger, metrics.New())
		if err := sup.Start(context.Background(), db); err != nil {
			t.Fatalf("Start() error = %v", err)
		}
		defer stopSupervisor(t, sup)

		_, _, err := sup.TestConnector(context.Background(), &connectors.ResolvedConnector{Type: "unknown.connection@1"})
		if err == nil {
			t.Fatal("expected an error for a connector type no running plugin declares, got nil")
		}
	})

	t.Run("requires a connector", func(t *testing.T) {
		db := openCatalogTestDB(t)

		logger := slog.New(slog.NewTextHandler(&syncBuffer{}, nil))
		sup := NewSupervisor(testSupervisorConfig(), logger, metrics.New())
		if err := sup.Start(context.Background(), db); err != nil {
			t.Fatalf("Start() error = %v", err)
		}
		defer stopSupervisor(t, sup)

		if _, _, err := sup.TestConnector(context.Background(), nil); err == nil {
			t.Fatal("expected an error for a nil connector, got nil")
		}
	})

	t.Run("routes to the plugin that declares the connector's type", func(t *testing.T) {
		t.Setenv("FAKE_PLUGIN_CONNECTOR_TYPE", "fake.connection@1")
		t.Setenv("FAKE_PLUGIN_CONNECTOR_TEST_MODE", "ok")

		db := openCatalogTestDB(t)
		if _, err := Install(context.Background(), db, fakePluginPath); err != nil {
			t.Fatalf("Install() error = %v", err)
		}

		logger := slog.New(slog.NewTextHandler(&syncBuffer{}, nil))
		m := metrics.New()
		sup := NewSupervisor(testSupervisorConfig(), logger, m)
		if err := sup.Start(context.Background(), db); err != nil {
			t.Fatalf("Start() error = %v", err)
		}
		defer stopSupervisor(t, sup)

		ok, _, err := sup.TestConnector(context.Background(), &connectors.ResolvedConnector{Type: "fake.connection@1"})
		if err != nil {
			t.Fatalf("TestConnector() error = %v", err)
		}
		if !ok {
			t.Fatal("ok = false, want true")
		}
		if got := m.Snapshot().Connectors.TestTotal["ok"]; got != 1 {
			t.Fatalf("connector_test_total{result=ok} = %d, want 1", got)
		}
	})

	t.Run("surfaces a failed test without an error", func(t *testing.T) {
		t.Setenv("FAKE_PLUGIN_CONNECTOR_TYPE", "fake.connection@1")
		t.Setenv("FAKE_PLUGIN_CONNECTOR_TEST_MODE", "fail")

		db := openCatalogTestDB(t)
		if _, err := Install(context.Background(), db, fakePluginPath); err != nil {
			t.Fatalf("Install() error = %v", err)
		}

		logger := slog.New(slog.NewTextHandler(&syncBuffer{}, nil))
		m := metrics.New()
		sup := NewSupervisor(testSupervisorConfig(), logger, m)
		if err := sup.Start(context.Background(), db); err != nil {
			t.Fatalf("Start() error = %v", err)
		}
		defer stopSupervisor(t, sup)

		ok, message, err := sup.TestConnector(context.Background(), &connectors.ResolvedConnector{Type: "fake.connection@1"})
		if err != nil {
			t.Fatalf("TestConnector() error = %v, want nil (a failed test is a legitimate result)", err)
		}
		if ok {
			t.Fatal("ok = true, want false")
		}
		if message != "boom" {
			t.Fatalf("message = %q, want %q", message, "boom")
		}
		if got := m.Snapshot().Connectors.TestTotal["failed"]; got != 1 {
			t.Fatalf("connector_test_total{result=failed} = %d, want 1", got)
		}
	})

	t.Run("surfaces Unimplemented as an error for a plugin that doesn't support testing", func(t *testing.T) {
		t.Setenv("FAKE_PLUGIN_CONNECTOR_TYPE", "fake.connection@1")

		db := openCatalogTestDB(t)
		if _, err := Install(context.Background(), db, fakePluginPath); err != nil {
			t.Fatalf("Install() error = %v", err)
		}

		logger := slog.New(slog.NewTextHandler(&syncBuffer{}, nil))
		m := metrics.New()
		sup := NewSupervisor(testSupervisorConfig(), logger, m)
		if err := sup.Start(context.Background(), db); err != nil {
			t.Fatalf("Start() error = %v", err)
		}
		defer stopSupervisor(t, sup)

		if _, _, err := sup.TestConnector(context.Background(), &connectors.ResolvedConnector{Type: "fake.connection@1"}); err == nil {
			t.Fatal("expected an error for a plugin that does not support connector testing, got nil")
		}
		if got := m.Snapshot().Connectors.TestTotal["error"]; got != 1 {
			t.Fatalf("connector_test_total{result=error} = %d, want 1", got)
		}
	})
}

func TestSupervisor_RestartsACrashedPluginThenStaysUp(t *testing.T) {
	counterFile := filepath.Join(t.TempDir(), "launches")
	markerFile := filepath.Join(t.TempDir(), "crashed-once")
	t.Setenv("FAKE_PLUGIN_MODE", "crash-once")
	t.Setenv("FAKE_PLUGIN_MARKER_FILE", markerFile)
	t.Setenv("FAKE_PLUGIN_LAUNCH_COUNTER_FILE", counterFile)

	db := openCatalogTestDB(t)
	seedCatalog(t, db, "io.patchcord.fake", fakePluginPath)

	logs := &syncBuffer{}
	logger := slog.New(slog.NewTextHandler(logs, nil))
	// A health check interval well beyond the fixture's ~150ms crash delay
	// keeps this test isolated to the Exited()-detected crash path: a tick
	// racing the crash could otherwise report the same failure as a health
	// check error instead, which is equally correct behavior but not what
	// this test is about (see TestSupervisor_RestartsAndQuarantinesAnUnhealthyPlugin
	// for the health-check-detected path).
	cfg := testSupervisorConfig()
	cfg.HealthCheckInterval = time.Second
	m := metrics.New()
	sup := NewSupervisor(cfg, logger, m)

	if err := sup.Start(context.Background(), db); err != nil {
		t.Fatalf("Start() error = %v", err)
	}
	defer stopSupervisor(t, sup)

	waitForCondition(t, 3*time.Second, func() bool {
		return launchCount(t, counterFile) >= 2
	})
	waitForCondition(t, 2*time.Second, func() bool {
		sup.mu.Lock()
		defer sup.mu.Unlock()
		return sup.running["io.patchcord.fake"] != nil
	})

	logStr := logs.String()
	if !strings.Contains(logStr, "plugin crashed") {
		t.Fatalf("logs = %q, want a \"plugin crashed\" entry", logStr)
	}
	if !strings.Contains(logStr, "restarting plugin") {
		t.Fatalf("logs = %q, want a \"restarting plugin\" entry", logStr)
	}
	if strings.Contains(logStr, "quarantined") {
		t.Fatalf("logs = %q, plugin should not have been quarantined", logStr)
	}

	// PluginStarted is recorded inside launchAndHandshake, strictly before
	// restart() adds the plugin back to s.running — so by the time the
	// waitForCondition above observes it in s.running, the metrics gauge
	// has already been set. Still poll rather than read it once: the
	// health-check ticker (1s interval) can otherwise race a slow test
	// run and briefly flip it while this goroutine is scheduled out.
	waitForCondition(t, 2*time.Second, func() bool {
		snap := m.Snapshot()
		return len(snap.Plugins) == 1 && snap.Plugins[0].PluginID == "io.patchcord.fake" && snap.Plugins[0].Running
	})

	snap := m.Snapshot()
	if snap.Plugins[0].RestartsTotal == 0 {
		t.Fatal("expected plugin_restarts_total to have recorded at least one restart attempt")
	}
	if snap.Plugins[0].QuarantinedTotal != 0 {
		t.Fatalf("plugin_quarantined_total = %d, want 0 (plugin should not have been quarantined)", snap.Plugins[0].QuarantinedTotal)
	}
}

func TestSupervisor_QuarantinesAfterRepeatedCrashes(t *testing.T) {
	counterFile := filepath.Join(t.TempDir(), "launches")
	t.Setenv("FAKE_PLUGIN_MODE", "always-crash")
	t.Setenv("FAKE_PLUGIN_LAUNCH_COUNTER_FILE", counterFile)

	db := openCatalogTestDB(t)
	seedCatalog(t, db, "io.patchcord.fake", fakePluginPath)

	logs := &syncBuffer{}
	logger := slog.New(slog.NewTextHandler(logs, nil))
	cfg := testSupervisorConfig()
	m := metrics.New()
	sup := NewSupervisor(cfg, logger, m)

	if err := sup.Start(context.Background(), db); err != nil {
		t.Fatalf("Start() error = %v", err)
	}
	defer stopSupervisor(t, sup)

	waitForCondition(t, 5*time.Second, func() bool {
		return strings.Contains(logs.String(), "plugin quarantined after repeated failures")
	})

	// Give any further (incorrect) restart attempt a chance to happen, then
	// confirm the launch count stopped growing.
	time.Sleep(300 * time.Millisecond)

	wantLaunches := cfg.MaxRestarts + 1 // the initial launch, plus MaxRestarts retries
	if got := launchCount(t, counterFile); got != wantLaunches {
		t.Fatalf("launch count = %d, want exactly %d (no relaunch after quarantine)", got, wantLaunches)
	}

	sup.mu.Lock()
	_, stillRunning := sup.running["io.patchcord.fake"]
	sup.mu.Unlock()
	if stillRunning {
		t.Fatal("expected the plugin to be removed from the running set after quarantine")
	}

	snap := m.Snapshot()
	if len(snap.Plugins) != 1 || snap.Plugins[0].PluginID != "io.patchcord.fake" {
		t.Fatalf("metrics snapshot plugins = %+v, want exactly one io.patchcord.fake entry", snap.Plugins)
	}
	if snap.Plugins[0].Running {
		t.Fatal("expected the plugin to be recorded as not running once quarantined")
	}
	if snap.Plugins[0].QuarantinedTotal != 1 {
		t.Fatalf("plugin_quarantined_total = %d, want 1", snap.Plugins[0].QuarantinedTotal)
	}
	if int(snap.Plugins[0].RestartsTotal) != cfg.MaxRestarts {
		t.Fatalf("plugin_restarts_total = %d, want %d (one attempt per retry before quarantine)", snap.Plugins[0].RestartsTotal, cfg.MaxRestarts)
	}
}

func TestSupervisor_RestartsAndQuarantinesAnUnhealthyPlugin(t *testing.T) {
	t.Setenv("FAKE_PLUGIN_MODE", "unhealthy")

	db := openCatalogTestDB(t)
	seedCatalog(t, db, "io.patchcord.fake", fakePluginPath)

	logs := &syncBuffer{}
	logger := slog.New(slog.NewTextHandler(logs, nil))
	m := metrics.New()
	sup := NewSupervisor(testSupervisorConfig(), logger, m)

	if err := sup.Start(context.Background(), db); err != nil {
		t.Fatalf("Start() error = %v", err)
	}
	defer stopSupervisor(t, sup)

	waitForCondition(t, 5*time.Second, func() bool {
		return strings.Contains(logs.String(), "plugin quarantined after repeated failures")
	})

	if snap := m.Snapshot(); len(snap.Plugins) != 1 || snap.Plugins[0].HealthCheckFailuresTotal == 0 {
		t.Fatalf("metrics snapshot plugins = %+v, want plugin_health_check_failures_total > 0", snap.Plugins)
	}

	logStr := logs.String()
	if !strings.Contains(logStr, "plugin health check failed") {
		t.Fatalf("logs = %q, want a \"plugin health check failed\" entry", logStr)
	}
}
