package api

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/lucasglmt/patchcord/internal/connectors"
	"github.com/lucasglmt/patchcord/internal/runs"
)

// blockingExecutor blocks ExecuteAction until release is closed, so a test
// can prove a handler responded before a triggered run actually finished.
type blockingExecutor struct {
	release chan struct{}
}

func (b *blockingExecutor) ExecuteAction(ctx context.Context, _ string, _ map[string]any, _ *connectors.ResolvedConnector) (map[string]any, error) {
	select {
	case <-b.release:
		return map[string]any{"value": "HELLO"}, nil
	case <-ctx.Done():
		return nil, ctx.Err()
	}
}

func TestHandleListWorkflows_EmptyCatalog(t *testing.T) {
	db := openMigratedTestDB(t)
	router := NewRouter(Deps{DB: db})

	req := httptest.NewRequest(http.MethodGet, "/v1/workflows", nil)
	rec := httptest.NewRecorder()
	router.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, want %d", rec.Code, http.StatusOK)
	}

	var got []workflowSummary
	if err := json.NewDecoder(rec.Body).Decode(&got); err != nil {
		t.Fatalf("decode response body: %v", err)
	}
	if len(got) != 0 {
		t.Fatalf("got = %v, want an empty slice", got)
	}
}

func TestHandleListWorkflows_ReturnsInstalledVersions(t *testing.T) {
	db := openMigratedTestDB(t)
	knownActions := map[string]struct{}{"text.uppercase@1": {}}
	if _, err := runs.InstallWorkflow(context.Background(), db, []byte(eventsTestWorkflow), knownActions); err != nil {
		t.Fatalf("InstallWorkflow() error = %v", err)
	}

	router := NewRouter(Deps{DB: db})
	req := httptest.NewRequest(http.MethodGet, "/v1/workflows", nil)
	rec := httptest.NewRecorder()
	router.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, want %d", rec.Code, http.StatusOK)
	}

	var got []workflowSummary
	if err := json.NewDecoder(rec.Body).Decode(&got); err != nil {
		t.Fatalf("decode response body: %v", err)
	}
	if len(got) != 1 {
		t.Fatalf("len(got) = %d, want 1", len(got))
	}
	if got[0].ID != "hello_patchcord" || got[0].Version != 1 {
		t.Fatalf("got[0] = %+v, want id=hello_patchcord version=1", got[0])
	}
}

func TestHandleRunWorkflow_UnknownWorkflowReturnsNotFound(t *testing.T) {
	db := openMigratedTestDB(t)
	router := NewRouter(Deps{DB: db, Executor: fakeExecutor{}})

	req := httptest.NewRequest(http.MethodPost, "/v1/workflows/does-not-exist/run", nil)
	rec := httptest.NewRecorder()
	router.ServeHTTP(rec, req)

	if rec.Code != http.StatusNotFound {
		t.Fatalf("status = %d, want %d", rec.Code, http.StatusNotFound)
	}
}

func TestHandleRunWorkflow_NoExecutorConfiguredReturnsInternalError(t *testing.T) {
	db := openMigratedTestDB(t)
	knownActions := map[string]struct{}{"text.uppercase@1": {}}
	if _, err := runs.InstallWorkflow(context.Background(), db, []byte(eventsTestWorkflow), knownActions); err != nil {
		t.Fatalf("InstallWorkflow() error = %v", err)
	}

	router := NewRouter(Deps{DB: db})

	req := httptest.NewRequest(http.MethodPost, "/v1/workflows/hello_patchcord/run", nil)
	rec := httptest.NewRecorder()
	router.ServeHTTP(rec, req)

	if rec.Code != http.StatusInternalServerError {
		t.Fatalf("status = %d, want %d", rec.Code, http.StatusInternalServerError)
	}
}

func TestHandleRunWorkflow_StartsImmediatelyAndRunsInTheBackground(t *testing.T) {
	db := openMigratedTestDB(t)
	knownActions := map[string]struct{}{"text.uppercase@1": {}}
	if _, err := runs.InstallWorkflow(context.Background(), db, []byte(eventsTestWorkflow), knownActions); err != nil {
		t.Fatalf("InstallWorkflow() error = %v", err)
	}

	executor := &blockingExecutor{release: make(chan struct{})}
	router := NewRouter(Deps{DB: db, Executor: executor})

	body := strings.NewReader(`{"inputs":{"value":"hi"}}`)
	req := httptest.NewRequest(http.MethodPost, "/v1/workflows/hello_patchcord/run", body)
	rec := httptest.NewRecorder()

	done := make(chan struct{})
	go func() {
		router.ServeHTTP(rec, req)
		close(done)
	}()

	select {
	case <-done:
	case <-time.After(2 * time.Second):
		t.Fatal("handler did not return promptly — it must not block on the run's execution")
	}

	if rec.Code != http.StatusAccepted {
		t.Fatalf("status = %d, want %d", rec.Code, http.StatusAccepted)
	}

	var got runSummary
	if err := json.NewDecoder(rec.Body).Decode(&got); err != nil {
		t.Fatalf("decode response body: %v", err)
	}
	if got.ID == "" {
		t.Fatal("response id is empty")
	}
	if got.Status != "running" {
		t.Fatalf("status = %q, want %q", got.Status, "running")
	}
	if got.Inputs["value"] != "hi" {
		t.Fatalf(`Inputs["value"] = %v, want "hi"`, got.Inputs["value"])
	}

	// The background Continue call is still blocked on the executor at this
	// point — confirm the run really hasn't finished yet, then let it
	// proceed and confirm it eventually does.
	run, _, err := runs.GetRun(context.Background(), db, got.ID)
	if err != nil {
		t.Fatalf("GetRun() error = %v", err)
	}
	if run.Status != "running" {
		t.Fatalf("run.Status = %s, want running (must not have finished yet)", run.Status)
	}

	close(executor.release)

	deadline := time.After(2 * time.Second)
	for {
		run, _, err := runs.GetRun(context.Background(), db, got.ID)
		if err != nil {
			t.Fatalf("GetRun() error = %v", err)
		}
		if run.Status == "succeeded" {
			break
		}
		select {
		case <-deadline:
			t.Fatalf("run did not reach succeeded in time, last status = %s", run.Status)
		case <-time.After(10 * time.Millisecond):
		}
	}
}
