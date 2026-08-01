package runtime

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"log/slog"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/lucasglmt/patchcord/internal/persistence"
	"github.com/lucasglmt/patchcord/internal/plugins"
	"github.com/lucasglmt/patchcord/internal/runs"
	"github.com/lucasglmt/patchcord/migrations"
)

// examplePluginPath is built once in TestMain: the real text-uppercase
// example plugin, used to prove the agent actually launches and stops
// plugins recorded in its catalog, not just a hand-rolled fixture.
var examplePluginPath string

func TestMain(m *testing.M) {
	tmpDir, err := os.MkdirTemp("", "patchcord-runtime-fixtures")
	if err != nil {
		panic(err)
	}
	defer os.RemoveAll(tmpDir)

	examplePluginPath = filepath.Join(tmpDir, "text-uppercase")
	build := exec.Command("go", "build", "-o", examplePluginPath, "../../plugins/examples/text-uppercase")
	if out, err := build.CombinedOutput(); err != nil {
		panic("build example plugin: " + err.Error() + "\n" + string(out))
	}

	os.Exit(m.Run())
}

func testLogger() *slog.Logger {
	return slog.New(slog.NewTextHandler(io.Discard, nil))
}

func TestNewAgent_InvalidListenAddress(t *testing.T) {
	cfg := Config{ListenAddr: "not-a-valid-address", DataDir: t.TempDir()}
	_, err := NewAgent(cfg, testLogger())
	if err == nil {
		t.Fatal("expected an error for an invalid listen address, got nil")
	}
}

func TestAgent_RunServesHealthAndShutsDownOnCancel(t *testing.T) {
	cfg := Config{ListenAddr: "127.0.0.1:0", DataDir: t.TempDir()}
	agent, err := NewAgent(cfg, testLogger())
	if err != nil {
		t.Fatalf("NewAgent() error = %v", err)
	}

	ctx, cancel := context.WithCancel(context.Background())

	runErr := make(chan error, 1)
	go func() {
		runErr <- agent.Run(ctx)
	}()

	healthURL := fmt.Sprintf("http://%s/v1/system/health", agent.Addr())

	var resp *http.Response
	deadline := time.Now().Add(2 * time.Second)
	for time.Now().Before(deadline) {
		resp, err = http.Get(healthURL)
		if err == nil {
			break
		}
		time.Sleep(10 * time.Millisecond)
	}
	if err != nil {
		t.Fatalf("GET %s: %v", healthURL, err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		t.Fatalf("status = %d, want %d", resp.StatusCode, http.StatusOK)
	}

	var body struct {
		Status   string `json:"status"`
		Database string `json:"database"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&body); err != nil {
		t.Fatalf("decode response body: %v", err)
	}
	if body.Status != "ok" || body.Database != "ok" {
		t.Fatalf("body = %+v, want status/database = ok/ok", body)
	}

	cancel()

	select {
	case err := <-runErr:
		if err != nil {
			t.Fatalf("Run() error = %v, want nil after clean shutdown", err)
		}
	case <-time.After(2 * time.Second):
		t.Fatal("Run() did not return within the shutdown timeout")
	}
}

// sseTestWorkflow is a minimal one-step workflow used to exercise the
// agent's /v1/runs/{id}/events endpoint end to end.
const sseTestWorkflow = `
schema_version: 1
id: hello_patchcord
version: 1
trigger:
  type: manual
steps:
  - id: transform
    uses: text.uppercase@1
    with:
      value: "hello"
`

// fakeSSEExecutor resolves any action immediately, without a real plugin —
// this test proves the agent's SSE endpoint reads a run's state as another
// process (here, a second *sql.DB handle standing in for `workflow run`)
// writes it to the shared SQLite file, not that action execution works.
type fakeSSEExecutor struct{}

func (fakeSSEExecutor) ExecuteAction(_ context.Context, _ string, _ map[string]any) (map[string]any, error) {
	return map[string]any{"value": "HELLO"}, nil
}

func TestAgent_StreamsRunEventsOverSSE(t *testing.T) {
	dataDir := t.TempDir()

	setupDB, err := persistence.Open(dataDir)
	if err != nil {
		t.Fatalf("persistence.Open() error = %v", err)
	}
	if err := persistence.Migrate(context.Background(), setupDB, migrations.FS, testLogger()); err != nil {
		t.Fatalf("persistence.Migrate() error = %v", err)
	}
	knownActions := map[string]struct{}{"text.uppercase@1": {}}
	if _, err := runs.InstallWorkflow(context.Background(), setupDB, []byte(sseTestWorkflow), knownActions); err != nil {
		t.Fatalf("InstallWorkflow() error = %v", err)
	}
	if err := setupDB.Close(); err != nil {
		t.Fatalf("close setup database: %v", err)
	}

	cfg := Config{ListenAddr: "127.0.0.1:0", DataDir: dataDir}
	agent, err := NewAgent(cfg, testLogger())
	if err != nil {
		t.Fatalf("NewAgent() error = %v", err)
	}

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	runErr := make(chan error, 1)
	go func() { runErr <- agent.Run(ctx) }()

	healthURL := fmt.Sprintf("http://%s/v1/system/health", agent.Addr())
	deadline := time.Now().Add(2 * time.Second)
	for time.Now().Before(deadline) {
		if resp, err := http.Get(healthURL); err == nil {
			resp.Body.Close()
			break
		}
		time.Sleep(10 * time.Millisecond)
	}

	// Runs a workflow through a second database handle onto the same
	// SQLite file, standing in for a separate `workflow run` process —
	// the agent's HTTP server never executes workflows itself.
	execDB, err := persistence.Open(dataDir)
	if err != nil {
		t.Fatalf("persistence.Open() error = %v", err)
	}
	defer execDB.Close()

	run, err := runs.Execute(context.Background(), execDB, fakeSSEExecutor{}, "hello_patchcord", nil, runs.ExecuteOptions{})
	if err != nil {
		t.Fatalf("Execute() error = %v", err)
	}

	eventsURL := fmt.Sprintf("http://%s/v1/runs/%s/events", agent.Addr(), run.ID)
	resp, err := http.Get(eventsURL)
	if err != nil {
		t.Fatalf("GET %s: %v", eventsURL, err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		t.Fatalf("status = %d, want %d", resp.StatusCode, http.StatusOK)
	}
	if ct := resp.Header.Get("Content-Type"); ct != "text/event-stream" {
		t.Fatalf("Content-Type = %q, want %q", ct, "text/event-stream")
	}

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		t.Fatalf("read SSE body: %v", err)
	}

	for _, want := range []string{"event: run.succeeded", "event: step.succeeded"} {
		if !strings.Contains(string(body), want) {
			t.Fatalf("SSE body does not contain %q; got:\n%s", want, body)
		}
	}

	cancel()

	select {
	case err := <-runErr:
		if err != nil {
			t.Fatalf("Run() error = %v, want nil after clean shutdown", err)
		}
	case <-time.After(2 * time.Second):
		t.Fatal("Run() did not return within the shutdown timeout")
	}
}

func TestAgent_LaunchesInstalledPluginsAndStopsThemOnShutdown(t *testing.T) {
	dataDir := t.TempDir()

	// Install the plugin into the catalog before the agent starts, exactly
	// like `patchcord plugin install` followed by `patchcord serve` would.
	db, err := persistence.Open(dataDir)
	if err != nil {
		t.Fatalf("persistence.Open() error = %v", err)
	}
	if err := persistence.Migrate(context.Background(), db, migrations.FS, testLogger()); err != nil {
		t.Fatalf("persistence.Migrate() error = %v", err)
	}
	if _, err := plugins.Install(context.Background(), db, examplePluginPath); err != nil {
		t.Fatalf("plugins.Install() error = %v", err)
	}
	if err := db.Close(); err != nil {
		t.Fatalf("close setup database: %v", err)
	}

	var logBuf bytes.Buffer
	logger := slog.New(slog.NewTextHandler(&logBuf, nil))

	cfg := Config{ListenAddr: "127.0.0.1:0", DataDir: dataDir}
	agent, err := NewAgent(cfg, logger)
	if err != nil {
		t.Fatalf("NewAgent() error = %v", err)
	}

	if logs := logBuf.String(); !strings.Contains(logs, "plugin launched") || !strings.Contains(logs, "io.patchcord.example-text") {
		t.Fatalf("expected a \"plugin launched\" log entry for io.patchcord.example-text, got:\n%s", logs)
	}

	ctx, cancel := context.WithCancel(context.Background())
	runErr := make(chan error, 1)
	go func() { runErr <- agent.Run(ctx) }()

	// Give the server a brief moment to actually start serving before
	// tearing it down, so shutdown exercises the real running path.
	time.Sleep(50 * time.Millisecond)
	cancel()

	select {
	case err := <-runErr:
		if err != nil {
			t.Fatalf("Run() error = %v, want nil after clean shutdown", err)
		}
	case <-time.After(3 * time.Second):
		t.Fatal("Run() did not return within the shutdown timeout (plugin process may not have been terminated)")
	}
}
