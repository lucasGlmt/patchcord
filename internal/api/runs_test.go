package api

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/lucasglmt/patchcord/internal/runs"
)

func TestHandleGetRun_UnknownRunReturnsNotFound(t *testing.T) {
	db := openMigratedTestDB(t)
	router := NewRouter(Deps{DB: db})

	req := httptest.NewRequest(http.MethodGet, "/v1/runs/does-not-exist", nil)
	rec := httptest.NewRecorder()
	router.ServeHTTP(rec, req)

	if rec.Code != http.StatusNotFound {
		t.Fatalf("status = %d, want %d", rec.Code, http.StatusNotFound)
	}
}

func TestHandleGetRun_ReturnsTheRunAndItsSteps(t *testing.T) {
	db := openMigratedTestDB(t)
	knownActions := map[string]struct{}{"text.uppercase@1": {}}
	if _, err := runs.InstallWorkflow(context.Background(), db, []byte(eventsTestWorkflow), knownActions); err != nil {
		t.Fatalf("InstallWorkflow() error = %v", err)
	}

	run, err := runs.Execute(context.Background(), db, fakeExecutor{}, "hello_patchcord", nil, nil, runs.ExecuteOptions{})
	if err != nil {
		t.Fatalf("Execute() error = %v", err)
	}

	router := NewRouter(Deps{DB: db})
	req := httptest.NewRequest(http.MethodGet, "/v1/runs/"+run.ID, nil)
	rec := httptest.NewRecorder()
	router.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, want %d", rec.Code, http.StatusOK)
	}

	var got runSummary
	if err := json.NewDecoder(rec.Body).Decode(&got); err != nil {
		t.Fatalf("decode response body: %v", err)
	}
	if got.ID != run.ID {
		t.Fatalf("ID = %q, want %q", got.ID, run.ID)
	}
	if got.Status != "succeeded" {
		t.Fatalf("Status = %q, want %q", got.Status, "succeeded")
	}
	if got.Outputs["value"] != "HELLO" {
		t.Fatalf(`Outputs["value"] = %v, want "HELLO"`, got.Outputs["value"])
	}
	if len(got.Steps) != 1 {
		t.Fatalf("len(Steps) = %d, want 1", len(got.Steps))
	}
	if got.Steps[0].ID != "transform" || got.Steps[0].Status != "succeeded" {
		t.Fatalf("Steps[0] = %+v, want id=transform status=succeeded", got.Steps[0])
	}
}

func TestHandleListRuns(t *testing.T) {
	db := openMigratedTestDB(t)
	knownActions := map[string]struct{}{"text.uppercase@1": {}}
	if _, err := runs.InstallWorkflow(context.Background(), db, []byte(eventsTestWorkflow), knownActions); err != nil {
		t.Fatalf("InstallWorkflow() error = %v", err)
	}

	first, err := runs.Execute(context.Background(), db, fakeExecutor{}, "hello_patchcord", nil, nil, runs.ExecuteOptions{})
	if err != nil {
		t.Fatalf("Execute() error = %v", err)
	}
	second, err := runs.Execute(context.Background(), db, fakeExecutor{}, "hello_patchcord", nil, nil, runs.ExecuteOptions{})
	if err != nil {
		t.Fatalf("Execute() error = %v", err)
	}

	router := NewRouter(Deps{DB: db})

	t.Run("lists every run", func(t *testing.T) {
		req := httptest.NewRequest(http.MethodGet, "/v1/runs", nil)
		rec := httptest.NewRecorder()
		router.ServeHTTP(rec, req)

		if rec.Code != http.StatusOK {
			t.Fatalf("status = %d, want %d", rec.Code, http.StatusOK)
		}
		var got []runSummary
		if err := json.NewDecoder(rec.Body).Decode(&got); err != nil {
			t.Fatalf("decode response body: %v", err)
		}
		if len(got) != 2 {
			t.Fatalf("len(got) = %d, want 2", len(got))
		}
		// created_at has only second resolution in SQLite, so two runs
		// created back to back in a test may tie — check the set, not a
		// strict order between them.
		gotIDs := map[string]bool{got[0].ID: true, got[1].ID: true}
		if !gotIDs[first.ID] || !gotIDs[second.ID] {
			t.Fatalf("got ids = %v, want them to contain %q and %q", gotIDs, first.ID, second.ID)
		}
	})

	t.Run("filters by workflow_id", func(t *testing.T) {
		req := httptest.NewRequest(http.MethodGet, "/v1/runs?workflow_id=does-not-exist", nil)
		rec := httptest.NewRecorder()
		router.ServeHTTP(rec, req)

		if rec.Code != http.StatusOK {
			t.Fatalf("status = %d, want %d", rec.Code, http.StatusOK)
		}
		var got []runSummary
		if err := json.NewDecoder(rec.Body).Decode(&got); err != nil {
			t.Fatalf("decode response body: %v", err)
		}
		if len(got) != 0 {
			t.Fatalf("got = %v, want an empty slice for an unknown workflow_id", got)
		}
	})
}

func TestHandleCancelRun_UnknownRunReturnsNotFound(t *testing.T) {
	db := openMigratedTestDB(t)
	router := NewRouter(Deps{DB: db})

	req := httptest.NewRequest(http.MethodPost, "/v1/runs/does-not-exist/cancel", nil)
	rec := httptest.NewRecorder()
	router.ServeHTTP(rec, req)

	if rec.Code != http.StatusNotFound {
		t.Fatalf("status = %d, want %d", rec.Code, http.StatusNotFound)
	}
}

func TestHandleCancelRun_AlreadyFinishedReturnsConflict(t *testing.T) {
	db := openMigratedTestDB(t)
	knownActions := map[string]struct{}{"text.uppercase@1": {}}
	if _, err := runs.InstallWorkflow(context.Background(), db, []byte(eventsTestWorkflow), knownActions); err != nil {
		t.Fatalf("InstallWorkflow() error = %v", err)
	}
	run, err := runs.Execute(context.Background(), db, fakeExecutor{}, "hello_patchcord", nil, nil, runs.ExecuteOptions{})
	if err != nil {
		t.Fatalf("Execute() error = %v", err)
	}

	router := NewRouter(Deps{DB: db})
	req := httptest.NewRequest(http.MethodPost, "/v1/runs/"+run.ID+"/cancel", nil)
	rec := httptest.NewRecorder()
	router.ServeHTTP(rec, req)

	if rec.Code != http.StatusConflict {
		t.Fatalf("status = %d, want %d", rec.Code, http.StatusConflict)
	}
}

func TestHandleCancelRun_CancelsAQueuedRun(t *testing.T) {
	db := openMigratedTestDB(t)
	knownActions := map[string]struct{}{"text.uppercase@1": {}}
	if _, err := runs.InstallWorkflow(context.Background(), db, []byte(eventsTestWorkflow), knownActions); err != nil {
		t.Fatalf("InstallWorkflow() error = %v", err)
	}
	_, run, _, err := runs.Start(context.Background(), db, "hello_patchcord", nil)
	if err != nil {
		t.Fatalf("Start() error = %v", err)
	}

	router := NewRouter(Deps{DB: db})
	req := httptest.NewRequest(http.MethodPost, "/v1/runs/"+run.ID+"/cancel", nil)
	rec := httptest.NewRecorder()
	router.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, want %d", rec.Code, http.StatusOK)
	}
	var got runSummary
	if err := json.NewDecoder(rec.Body).Decode(&got); err != nil {
		t.Fatalf("decode response body: %v", err)
	}
	if got.Status != "cancelled" {
		t.Fatalf("Status = %q, want %q", got.Status, "cancelled")
	}
}
