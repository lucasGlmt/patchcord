package api

import (
	"context"
	"database/sql"
	"io"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/lucasglmt/patchcord/internal/persistence"
	"github.com/lucasglmt/patchcord/internal/runs"
	"github.com/lucasglmt/patchcord/migrations"
)

// openMigratedTestDB returns an in-memory database with the full schema
// applied — handleRunEvents needs the runs/run_steps tables that the plain
// openTestDB helper (used by the health checks, which only ping) doesn't
// set up.
func openMigratedTestDB(t *testing.T) *sql.DB {
	t.Helper()

	db := openTestDB(t)
	logger := slog.New(slog.NewTextHandler(io.Discard, nil))
	if err := persistence.Migrate(context.Background(), db, migrations.FS, logger); err != nil {
		t.Fatalf("persistence.Migrate() error = %v", err)
	}
	return db
}

const eventsTestWorkflow = `
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

// fakeExecutor resolves any action immediately, without launching a real
// plugin — this test only exercises the HTTP/SSE surface, not execution.
type fakeExecutor struct{}

func (fakeExecutor) ExecuteAction(_ context.Context, _ string, _ map[string]any) (map[string]any, error) {
	return map[string]any{"value": "HELLO"}, nil
}

func TestHandleRunEvents_UnknownRunReturnsNotFound(t *testing.T) {
	db := openMigratedTestDB(t)
	router := NewRouter(Deps{DB: db})

	req := httptest.NewRequest(http.MethodGet, "/v1/runs/does-not-exist/events", nil)
	rec := httptest.NewRecorder()
	router.ServeHTTP(rec, req)

	if rec.Code != http.StatusNotFound {
		t.Fatalf("status = %d, want %d", rec.Code, http.StatusNotFound)
	}
}

func TestHandleRunEvents_StreamsRunAndStepEventsAsSSE(t *testing.T) {
	db := openMigratedTestDB(t)

	knownActions := map[string]struct{}{"text.uppercase@1": {}}
	if _, err := runs.InstallWorkflow(context.Background(), db, []byte(eventsTestWorkflow), knownActions); err != nil {
		t.Fatalf("InstallWorkflow() error = %v", err)
	}

	run, err := runs.Execute(context.Background(), db, fakeExecutor{}, "hello_patchcord", nil, runs.ExecuteOptions{})
	if err != nil {
		t.Fatalf("Execute() error = %v", err)
	}

	router := NewRouter(Deps{DB: db})
	req := httptest.NewRequest(http.MethodGet, "/v1/runs/"+run.ID+"/events", nil)
	rec := httptest.NewRecorder()
	router.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, want %d", rec.Code, http.StatusOK)
	}
	if ct := rec.Header().Get("Content-Type"); ct != "text/event-stream" {
		t.Fatalf("Content-Type = %q, want %q", ct, "text/event-stream")
	}

	body := rec.Body.String()
	for _, want := range []string{
		"event: run.succeeded",
		"event: step.succeeded",
		`"run_id":"` + run.ID + `"`,
		`"step_id":"transform"`,
	} {
		if !strings.Contains(body, want) {
			t.Fatalf("SSE body does not contain %q; got:\n%s", want, body)
		}
	}
}
