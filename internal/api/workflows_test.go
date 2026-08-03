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
	"github.com/lucasglmt/patchcord/internal/plugins"
	"github.com/lucasglmt/patchcord/internal/runs"
	"github.com/lucasglmt/patchcord/internal/scheduler"
)

func TestBindingName(t *testing.T) {
	tests := []struct {
		name     string
		expr     string
		wantName string
		wantOK   bool
	}{
		{name: "simple binding expression", expr: "${{ bindings.db }}", wantName: "db", wantOK: true},
		{name: "no surrounding whitespace", expr: "${{bindings.db}}", wantName: "db", wantOK: true},
		{name: "extra inner whitespace", expr: "${{   bindings.provider   }}", wantName: "provider", wantOK: true},
		{name: "underscored name", expr: "${{ bindings.my_conn_1 }}", wantName: "my_conn_1", wantOK: true},
		{name: "empty connector", expr: "", wantOK: false},
		{name: "literal connector id", expr: "my_connector", wantOK: false},
		{name: "workflow input expression", expr: "${{ workflow.inputs.provider }}", wantOK: false},
		{name: "step output expression", expr: "${{ steps.pick.outputs.id }}", wantOK: false},
		{name: "malformed expression", expr: "${{ bindings.db", wantOK: false},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			gotName, gotOK := bindingName(tt.expr)
			if gotOK != tt.wantOK {
				t.Fatalf("bindingName(%q) ok = %v, want %v", tt.expr, gotOK, tt.wantOK)
			}
			if gotOK && gotName != tt.wantName {
				t.Fatalf("bindingName(%q) name = %q, want %q", tt.expr, gotName, tt.wantName)
			}
		})
	}
}

func TestBindingConnectorType(t *testing.T) {
	catalog := []plugins.CatalogEntry{
		{PluginID: "io.patchcord.postgresql", Connectors: []string{"postgresql.connection@1"}, Actions: []string{"postgresql.query@1"}},
		{PluginID: "io.patchcord.multi", Connectors: []string{"multi.a@1", "multi.b@1"}, Actions: []string{"multi.do@1"}},
	}

	tests := []struct {
		name     string
		uses     string
		wantType string
		wantOK   bool
	}{
		{name: "single connector type is inferred", uses: "postgresql.query@1", wantType: "postgresql.connection@1", wantOK: true},
		{name: "no owning plugin is not inferred", uses: "smtp.send@1", wantOK: false},
		{name: "plugin with more than one connector type is ambiguous", uses: "multi.do@1", wantOK: false},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			gotType, gotOK := bindingConnectorType(tt.uses, catalog)
			if gotOK != tt.wantOK {
				t.Fatalf("bindingConnectorType(%q) ok = %v, want %v", tt.uses, gotOK, tt.wantOK)
			}
			if gotOK && gotType != tt.wantType {
				t.Fatalf("bindingConnectorType(%q) type = %q, want %q", tt.uses, gotType, tt.wantType)
			}
		})
	}
}

const bindingTestWorkflow = `
schema_version: 1
id: query_demo
version: 1
trigger:
  type: manual
steps:
  - id: run_query
    uses: postgresql.query@1
    connector: "${{ bindings.db }}"
    with:
      query: "select 1"
`

func TestHandleGetWorkflow_InfersConnectorTypeForABinding(t *testing.T) {
	db := openMigratedTestDB(t)
	insertTestPlugin(t, db, "io.patchcord.postgresql", []string{"postgresql.connection@1"}, []string{"postgresql.query@1"})

	knownActions := map[string]struct{}{"postgresql.query@1": {}}
	if _, err := runs.InstallWorkflow(context.Background(), db, []byte(bindingTestWorkflow), knownActions); err != nil {
		t.Fatalf("InstallWorkflow() error = %v", err)
	}

	router := NewRouter(Deps{DB: db})
	req := httptest.NewRequest(http.MethodGet, "/v1/workflows/query_demo", nil)
	rec := httptest.NewRecorder()
	router.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, want %d, body = %s", rec.Code, http.StatusOK, rec.Body.String())
	}

	var got workflowDetail
	if err := json.NewDecoder(rec.Body).Decode(&got); err != nil {
		t.Fatalf("decode response body: %v", err)
	}

	if len(got.Steps) != 1 || got.Steps[0].BindingName != "db" || got.Steps[0].ConnectorType != "postgresql.connection@1" {
		t.Fatalf("Steps = %+v, want one step with binding_name=db connector_type=postgresql.connection@1", got.Steps)
	}
	if len(got.Bindings) != 1 || got.Bindings[0] != (workflowBindingDetail{Name: "db", ConnectorType: "postgresql.connection@1"}) {
		t.Fatalf("Bindings = %+v, want [{db postgresql.connection@1}]", got.Bindings)
	}
}

func TestHandleGetWorkflow_LeavesConnectorTypeEmptyWhenNoPluginIsInstalled(t *testing.T) {
	db := openMigratedTestDB(t)

	knownActions := map[string]struct{}{"postgresql.query@1": {}}
	if _, err := runs.InstallWorkflow(context.Background(), db, []byte(bindingTestWorkflow), knownActions); err != nil {
		t.Fatalf("InstallWorkflow() error = %v", err)
	}

	router := NewRouter(Deps{DB: db})
	req := httptest.NewRequest(http.MethodGet, "/v1/workflows/query_demo", nil)
	rec := httptest.NewRecorder()
	router.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, want %d, body = %s", rec.Code, http.StatusOK, rec.Body.String())
	}

	var got workflowDetail
	if err := json.NewDecoder(rec.Body).Decode(&got); err != nil {
		t.Fatalf("decode response body: %v", err)
	}

	if len(got.Steps) != 1 || got.Steps[0].BindingName != "db" || got.Steps[0].ConnectorType != "" {
		t.Fatalf("Steps = %+v, want one step with binding_name=db connector_type=\"\"", got.Steps)
	}
	if len(got.Bindings) != 1 || got.Bindings[0] != (workflowBindingDetail{Name: "db"}) {
		t.Fatalf("Bindings = %+v, want [{db }]", got.Bindings)
	}
}

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

const scheduledTestWorkflow = `
schema_version: 1
id: nightly_report
version: 1
trigger:
  type: schedule
  cron: "*/5 * * * *"
steps:
  - id: step1
    uses: text.uppercase@1
    with:
      value: hello
`

func TestHandleGetWorkflow_ReturnsScheduleTriggerDetails(t *testing.T) {
	db := openMigratedTestDB(t)
	knownActions := map[string]struct{}{"text.uppercase@1": {}}
	def, err := runs.InstallWorkflow(context.Background(), db, []byte(scheduledTestWorkflow), knownActions)
	if err != nil {
		t.Fatalf("InstallWorkflow() error = %v", err)
	}
	// handleGetWorkflow doesn't schedule anything itself — only the CLI's
	// `workflow install` calls scheduler.Sync (see internal/cli/workflow.go)
	// — so the test does the same before exercising the handler.
	if err := scheduler.Sync(context.Background(), db, def); err != nil {
		t.Fatalf("scheduler.Sync() error = %v", err)
	}

	router := NewRouter(Deps{DB: db})
	req := httptest.NewRequest(http.MethodGet, "/v1/workflows/nightly_report", nil)
	rec := httptest.NewRecorder()
	router.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, want %d, body = %s", rec.Code, http.StatusOK, rec.Body.String())
	}

	var got workflowDetail
	if err := json.NewDecoder(rec.Body).Decode(&got); err != nil {
		t.Fatalf("decode response body: %v", err)
	}
	if got.TriggerType != "schedule" {
		t.Fatalf("TriggerType = %q, want %q", got.TriggerType, "schedule")
	}
	if got.TriggerCron != "*/5 * * * *" {
		t.Fatalf("TriggerCron = %q, want %q", got.TriggerCron, "*/5 * * * *")
	}
	if got.TriggerOnMissed != "skip" {
		t.Fatalf("TriggerOnMissed = %q, want default %q", got.TriggerOnMissed, "skip")
	}
	if got.NextRunAt == nil || !got.NextRunAt.After(time.Now()) {
		t.Fatalf("NextRunAt = %v, want a time in the future", got.NextRunAt)
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
