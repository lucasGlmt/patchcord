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

func TestHandleGetWorkflow_ReturnsStepsAndSource(t *testing.T) {
	db := openMigratedTestDB(t)
	knownActions := map[string]struct{}{"text.uppercase@1": {}}
	if _, err := runs.InstallWorkflow(context.Background(), db, []byte(eventsTestWorkflow), knownActions); err != nil {
		t.Fatalf("InstallWorkflow() error = %v", err)
	}

	router := NewRouter(Deps{DB: db})
	req := httptest.NewRequest(http.MethodGet, "/v1/workflows/hello_patchcord", nil)
	rec := httptest.NewRecorder()
	router.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, want %d, body = %s", rec.Code, http.StatusOK, rec.Body.String())
	}

	var got workflowDetail
	if err := json.NewDecoder(rec.Body).Decode(&got); err != nil {
		t.Fatalf("decode response body: %v", err)
	}
	if got.ID != "hello_patchcord" || got.Version != 1 {
		t.Fatalf("got = %+v, want id=hello_patchcord version=1", got)
	}
	if got.TriggerType != "manual" {
		t.Fatalf("TriggerType = %q, want %q", got.TriggerType, "manual")
	}
	if len(got.Steps) != 1 || got.Steps[0].Uses != "text.uppercase@1" {
		t.Fatalf("Steps = %+v, want one text.uppercase@1 step", got.Steps)
	}
	if !strings.Contains(got.Source, "hello_patchcord") {
		t.Fatalf("Source = %q, want it to contain the raw YAML", got.Source)
	}
}

func TestHandleGetWorkflow_ReturnsDeclaredInputs(t *testing.T) {
	db := openMigratedTestDB(t)
	knownActions := map[string]struct{}{"text.uppercase@1": {}}
	source := `
schema_version: 1
id: greet
version: 1
trigger:
  type: manual
inputs:
  - name: name
    type: string
    required: true
    description: Name to greet.
  - name: shout
    type: boolean
    default: false
steps:
  - id: transform
    uses: text.uppercase@1
    with:
      value: "${{ workflow.inputs.name }}"
`
	if _, err := runs.InstallWorkflow(context.Background(), db, []byte(source), knownActions); err != nil {
		t.Fatalf("InstallWorkflow() error = %v", err)
	}

	router := NewRouter(Deps{DB: db})
	req := httptest.NewRequest(http.MethodGet, "/v1/workflows/greet", nil)
	rec := httptest.NewRecorder()
	router.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, want %d, body = %s", rec.Code, http.StatusOK, rec.Body.String())
	}

	var got workflowDetail
	if err := json.NewDecoder(rec.Body).Decode(&got); err != nil {
		t.Fatalf("decode response body: %v", err)
	}
	if len(got.Inputs) != 2 {
		t.Fatalf("Inputs = %+v, want 2 declared inputs", got.Inputs)
	}
	if got.Inputs[0].Name != "name" || got.Inputs[0].Type != "string" || !got.Inputs[0].Required {
		t.Fatalf("Inputs[0] = %+v, want name=name type=string required=true", got.Inputs[0])
	}
	if got.Inputs[0].Description != "Name to greet." {
		t.Fatalf("Inputs[0].Description = %q, want %q", got.Inputs[0].Description, "Name to greet.")
	}
	if got.Inputs[1].Name != "shout" || got.Inputs[1].Type != "boolean" || got.Inputs[1].Default != false {
		t.Fatalf("Inputs[1] = %+v, want name=shout type=boolean default=false", got.Inputs[1])
	}
}

func TestHandleGetWorkflow_UnknownWorkflowReturnsNotFound(t *testing.T) {
	db := openMigratedTestDB(t)
	router := NewRouter(Deps{DB: db})

	req := httptest.NewRequest(http.MethodGet, "/v1/workflows/does-not-exist", nil)
	rec := httptest.NewRecorder()
	router.ServeHTTP(rec, req)

	if rec.Code != http.StatusNotFound {
		t.Fatalf("status = %d, want %d", rec.Code, http.StatusNotFound)
	}
}

func TestHandleGetWorkflow_RejectsANonIntegerVersion(t *testing.T) {
	db := openMigratedTestDB(t)
	knownActions := map[string]struct{}{"text.uppercase@1": {}}
	if _, err := runs.InstallWorkflow(context.Background(), db, []byte(eventsTestWorkflow), knownActions); err != nil {
		t.Fatalf("InstallWorkflow() error = %v", err)
	}

	router := NewRouter(Deps{DB: db})
	req := httptest.NewRequest(http.MethodGet, "/v1/workflows/hello_patchcord?version=not-a-number", nil)
	rec := httptest.NewRecorder()
	router.ServeHTTP(rec, req)

	if rec.Code != http.StatusBadRequest {
		t.Fatalf("status = %d, want %d", rec.Code, http.StatusBadRequest)
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

const inputsTestWorkflow = `
schema_version: 1
id: greet
version: 1
trigger:
  type: manual
inputs:
  - name: name
    type: string
    required: true
  - name: shout
    type: boolean
    default: true
steps:
  - id: transform
    uses: text.uppercase@1
    with:
      value: "${{ workflow.inputs.name }}"
`

func TestHandleRunWorkflow_RejectsMissingRequiredInputWith400(t *testing.T) {
	db := openMigratedTestDB(t)
	knownActions := map[string]struct{}{"text.uppercase@1": {}}
	if _, err := runs.InstallWorkflow(context.Background(), db, []byte(inputsTestWorkflow), knownActions); err != nil {
		t.Fatalf("InstallWorkflow() error = %v", err)
	}

	router := NewRouter(Deps{DB: db, Executor: fakeExecutor{}})
	req := httptest.NewRequest(http.MethodPost, "/v1/workflows/greet/run", strings.NewReader(`{"inputs":{}}`))
	rec := httptest.NewRecorder()
	router.ServeHTTP(rec, req)

	if rec.Code != http.StatusBadRequest {
		t.Fatalf("status = %d, want %d, body = %s", rec.Code, http.StatusBadRequest, rec.Body.String())
	}
}

func TestHandleRunWorkflow_AppliesDeclaredDefault(t *testing.T) {
	db := openMigratedTestDB(t)
	knownActions := map[string]struct{}{"text.uppercase@1": {}}
	if _, err := runs.InstallWorkflow(context.Background(), db, []byte(inputsTestWorkflow), knownActions); err != nil {
		t.Fatalf("InstallWorkflow() error = %v", err)
	}

	router := NewRouter(Deps{DB: db, Executor: fakeExecutor{}})
	req := httptest.NewRequest(http.MethodPost, "/v1/workflows/greet/run", strings.NewReader(`{"inputs":{"name":"world"}}`))
	rec := httptest.NewRecorder()
	router.ServeHTTP(rec, req)

	if rec.Code != http.StatusAccepted {
		t.Fatalf("status = %d, want %d, body = %s", rec.Code, http.StatusAccepted, rec.Body.String())
	}

	var got runSummary
	if err := json.NewDecoder(rec.Body).Decode(&got); err != nil {
		t.Fatalf("decode response body: %v", err)
	}
	if got.Inputs["shout"] != true {
		t.Fatalf(`Inputs["shout"] = %v, want true (the declared default)`, got.Inputs["shout"])
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
