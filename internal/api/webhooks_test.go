package api

import (
	"context"
	"database/sql"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/lucasglmt/patchcord/internal/auth"
	"github.com/lucasglmt/patchcord/internal/runs"
	"github.com/lucasglmt/patchcord/internal/workflow"
)

const webhookTestWorkflow = `
schema_version: 1
id: webhook_demo
version: 1
trigger:
  type: webhook
  secret_ref:
    type: env
    key: WEBHOOK_DEMO_TOKEN
inputs:
  - name: name
    type: string
    required: true
steps:
  - id: transform
    uses: text.uppercase@1
    with:
      value: "${{ workflow.inputs.name }}"
`

func TestHandleWebhookTrigger(t *testing.T) {
	knownActions := map[string]workflow.KnownAction{"text.uppercase@1": {}}

	t.Run("unknown workflow returns not found", func(t *testing.T) {
		db := openMigratedTestDB(t)
		router := NewRouter(Deps{DB: db})

		req := httptest.NewRequest(http.MethodPost, "/v1/webhooks/does-not-exist", nil)
		rec := httptest.NewRecorder()
		router.ServeHTTP(rec, req)

		if rec.Code != http.StatusNotFound {
			t.Fatalf("status = %d, want %d", rec.Code, http.StatusNotFound)
		}
	})

	t.Run("a workflow without a webhook trigger returns not found", func(t *testing.T) {
		db := openMigratedTestDB(t)
		if _, err := runs.InstallWorkflow(context.Background(), db, []byte(eventsTestWorkflow), knownActions); err != nil {
			t.Fatalf("InstallWorkflow() error = %v", err)
		}
		router := NewRouter(Deps{DB: db})

		req := httptest.NewRequest(http.MethodPost, "/v1/webhooks/hello_patchcord", nil)
		rec := httptest.NewRecorder()
		router.ServeHTTP(rec, req)

		if rec.Code != http.StatusNotFound {
			t.Fatalf("status = %d, want %d", rec.Code, http.StatusNotFound)
		}
	})

	t.Run("a missing token header is rejected", func(t *testing.T) {
		t.Setenv("WEBHOOK_DEMO_TOKEN", "s3cr3t")
		db := openMigratedTestDB(t)
		if _, err := runs.InstallWorkflow(context.Background(), db, []byte(webhookTestWorkflow), knownActions); err != nil {
			t.Fatalf("InstallWorkflow() error = %v", err)
		}
		router := NewRouter(Deps{DB: db, Executor: fakeExecutor{}})

		req := httptest.NewRequest(http.MethodPost, "/v1/webhooks/webhook_demo", nil)
		rec := httptest.NewRecorder()
		router.ServeHTTP(rec, req)

		if rec.Code != http.StatusUnauthorized {
			t.Fatalf("status = %d, want %d", rec.Code, http.StatusUnauthorized)
		}
	})

	t.Run("an incorrect token header is rejected", func(t *testing.T) {
		t.Setenv("WEBHOOK_DEMO_TOKEN", "s3cr3t")
		db := openMigratedTestDB(t)
		if _, err := runs.InstallWorkflow(context.Background(), db, []byte(webhookTestWorkflow), knownActions); err != nil {
			t.Fatalf("InstallWorkflow() error = %v", err)
		}
		router := NewRouter(Deps{DB: db, Executor: fakeExecutor{}})

		req := httptest.NewRequest(http.MethodPost, "/v1/webhooks/webhook_demo", nil)
		req.Header.Set(webhookTokenHeader, "wrong")
		rec := httptest.NewRecorder()
		router.ServeHTTP(rec, req)

		if rec.Code != http.StatusUnauthorized {
			t.Fatalf("status = %d, want %d", rec.Code, http.StatusUnauthorized)
		}
	})

	t.Run("an unresolvable secret_ref returns an internal error", func(t *testing.T) {
		// WEBHOOK_DEMO_TOKEN deliberately left unset.
		db := openMigratedTestDB(t)
		if _, err := runs.InstallWorkflow(context.Background(), db, []byte(webhookTestWorkflow), knownActions); err != nil {
			t.Fatalf("InstallWorkflow() error = %v", err)
		}
		router := NewRouter(Deps{DB: db, Executor: fakeExecutor{}})

		req := httptest.NewRequest(http.MethodPost, "/v1/webhooks/webhook_demo", nil)
		req.Header.Set(webhookTokenHeader, "anything")
		rec := httptest.NewRecorder()
		router.ServeHTTP(rec, req)

		if rec.Code != http.StatusInternalServerError {
			t.Fatalf("status = %d, want %d (body: %s)", rec.Code, http.StatusInternalServerError, rec.Body.String())
		}
	})

	t.Run("a non-object JSON body is rejected", func(t *testing.T) {
		t.Setenv("WEBHOOK_DEMO_TOKEN", "s3cr3t")
		db := openMigratedTestDB(t)
		if _, err := runs.InstallWorkflow(context.Background(), db, []byte(webhookTestWorkflow), knownActions); err != nil {
			t.Fatalf("InstallWorkflow() error = %v", err)
		}
		router := NewRouter(Deps{DB: db, Executor: fakeExecutor{}})

		req := httptest.NewRequest(http.MethodPost, "/v1/webhooks/webhook_demo", strings.NewReader(`[1,2,3]`))
		req.Header.Set(webhookTokenHeader, "s3cr3t")
		rec := httptest.NewRecorder()
		router.ServeHTTP(rec, req)

		if rec.Code != http.StatusBadRequest {
			t.Fatalf("status = %d, want %d", rec.Code, http.StatusBadRequest)
		}
	})

	t.Run("a body missing a required input is rejected", func(t *testing.T) {
		t.Setenv("WEBHOOK_DEMO_TOKEN", "s3cr3t")
		db := openMigratedTestDB(t)
		if _, err := runs.InstallWorkflow(context.Background(), db, []byte(webhookTestWorkflow), knownActions); err != nil {
			t.Fatalf("InstallWorkflow() error = %v", err)
		}
		router := NewRouter(Deps{DB: db, Executor: fakeExecutor{}})

		req := httptest.NewRequest(http.MethodPost, "/v1/webhooks/webhook_demo", strings.NewReader(`{}`))
		req.Header.Set(webhookTokenHeader, "s3cr3t")
		rec := httptest.NewRecorder()
		router.ServeHTTP(rec, req)

		if rec.Code != http.StatusBadRequest {
			t.Fatalf("status = %d, want %d (body: %s)", rec.Code, http.StatusBadRequest, rec.Body.String())
		}
	})

	t.Run("the raw JSON body becomes the run's inputs directly", func(t *testing.T) {
		t.Setenv("WEBHOOK_DEMO_TOKEN", "s3cr3t")
		db := openMigratedTestDB(t)
		if _, err := runs.InstallWorkflow(context.Background(), db, []byte(webhookTestWorkflow), knownActions); err != nil {
			t.Fatalf("InstallWorkflow() error = %v", err)
		}
		router := NewRouter(Deps{DB: db, Executor: fakeExecutor{}})

		req := httptest.NewRequest(http.MethodPost, "/v1/webhooks/webhook_demo", strings.NewReader(`{"name":"world"}`))
		req.Header.Set(webhookTokenHeader, "s3cr3t")
		rec := httptest.NewRecorder()
		router.ServeHTTP(rec, req)

		if rec.Code != http.StatusAccepted {
			t.Fatalf("status = %d, want %d (body: %s)", rec.Code, http.StatusAccepted, rec.Body.String())
		}

		var got runSummary
		if err := json.NewDecoder(rec.Body).Decode(&got); err != nil {
			t.Fatalf("decode response body: %v", err)
		}
		if got.Inputs["name"] != "world" {
			t.Fatalf(`Inputs["name"] = %v, want "world"`, got.Inputs["name"])
		}

		waitForWebhookRunToFinish(t, db, got.ID)
	})

	t.Run("works even once an admin token exists — never admin-gated", func(t *testing.T) {
		t.Setenv("WEBHOOK_DEMO_TOKEN", "s3cr3t")
		db := openMigratedTestDB(t)
		if _, err := runs.InstallWorkflow(context.Background(), db, []byte(webhookTestWorkflow), knownActions); err != nil {
			t.Fatalf("InstallWorkflow() error = %v", err)
		}
		if _, _, err := auth.CreateToken(context.Background(), db, "ci"); err != nil {
			t.Fatalf("CreateToken() error = %v", err)
		}
		router := NewRouter(Deps{DB: db, Executor: fakeExecutor{}})

		req := httptest.NewRequest(http.MethodPost, "/v1/webhooks/webhook_demo", strings.NewReader(`{"name":"world"}`))
		req.Header.Set(webhookTokenHeader, "s3cr3t")
		rec := httptest.NewRecorder()
		router.ServeHTTP(rec, req)

		if rec.Code != http.StatusAccepted {
			t.Fatalf("status = %d, want %d (body: %s)", rec.Code, http.StatusAccepted, rec.Body.String())
		}

		var got runSummary
		if err := json.NewDecoder(rec.Body).Decode(&got); err != nil {
			t.Fatalf("decode response body: %v", err)
		}
		waitForWebhookRunToFinish(t, db, got.ID)
	})
}

// waitForWebhookRunToFinish polls until runID reaches a terminal status —
// same reasoning as apps_test.go's waitForRunToFinish (lets the background
// runs.Continue goroutine finish before t.Cleanup closes the shared-cache
// in-memory database out from under it), but takes the id directly since
// this test already decoded the response body once to assert on Inputs.
func waitForWebhookRunToFinish(t *testing.T, db *sql.DB, runID string) {
	t.Helper()

	deadline := time.After(2 * time.Second)
	for {
		run, _, err := runs.GetRun(context.Background(), db, runID)
		if err != nil {
			t.Fatalf("GetRun() error = %v", err)
		}
		if run.Status == "succeeded" || run.Status == "failed" {
			return
		}
		select {
		case <-deadline:
			t.Fatalf("run did not reach a terminal status in time, last status = %s", run.Status)
		case <-time.After(10 * time.Millisecond):
		}
	}
}
