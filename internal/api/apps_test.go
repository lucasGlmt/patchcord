package api

import (
	"bytes"
	"context"
	"database/sql"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/lucasglmt/patchcord/internal/apps"
	"github.com/lucasglmt/patchcord/internal/auth"
	"github.com/lucasglmt/patchcord/internal/connectors"
	"github.com/lucasglmt/patchcord/internal/runs"
	"github.com/lucasglmt/patchcord/internal/workflow"
)

// installTestApp writes a minimal application (manifest + index.html) to a
// temporary directory and installs it, returning the recorded app.
func installTestApp(t *testing.T, db *sql.DB, id string, workflowsRun ...string) *apps.App {
	t.Helper()

	dir := t.TempDir()
	content := "id: " + id + "\nversion: \"0.1.0\"\npermissions:\n  workflows:\n    run:\n"
	for _, w := range workflowsRun {
		content += "      - " + w + "\n"
	}
	if err := os.WriteFile(filepath.Join(dir, apps.ManifestFileName), []byte(content), 0o644); err != nil {
		t.Fatalf("write manifest: %v", err)
	}
	if err := os.WriteFile(filepath.Join(dir, "index.html"), []byte("<h1>"+id+"</h1>"), 0o644); err != nil {
		t.Fatalf("write index.html: %v", err)
	}

	app, err := apps.Install(context.Background(), db, dir)
	if err != nil {
		t.Fatalf("apps.Install() error = %v", err)
	}
	return app
}

// installTestAppWithConnectors is installTestApp plus a declared
// connectors.use permission (ADR-0071).
func installTestAppWithConnectors(t *testing.T, db *sql.DB, id string, workflowsRun []string, connectorsUse []string) *apps.App {
	t.Helper()

	dir := t.TempDir()
	content := "id: " + id + "\nversion: \"0.1.0\"\npermissions:\n  workflows:\n    run:\n"
	for _, w := range workflowsRun {
		content += "      - " + w + "\n"
	}
	content += "  connectors:\n    use:\n"
	for _, c := range connectorsUse {
		content += "      - " + c + "\n"
	}
	if err := os.WriteFile(filepath.Join(dir, apps.ManifestFileName), []byte(content), 0o644); err != nil {
		t.Fatalf("write manifest: %v", err)
	}
	if err := os.WriteFile(filepath.Join(dir, "index.html"), []byte("<h1>"+id+"</h1>"), 0o644); err != nil {
		t.Fatalf("write index.html: %v", err)
	}

	app, err := apps.Install(context.Background(), db, dir)
	if err != nil {
		t.Fatalf("apps.Install() error = %v", err)
	}
	return app
}

func TestHandleListApps_EmptyCatalog(t *testing.T) {
	db := openMigratedTestDB(t)
	router := NewRouter(Deps{DB: db})

	req := httptest.NewRequest(http.MethodGet, "/v1/apps", nil)
	rec := httptest.NewRecorder()
	router.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, want %d", rec.Code, http.StatusOK)
	}

	var got []appSummary
	if err := json.NewDecoder(rec.Body).Decode(&got); err != nil {
		t.Fatalf("decode response body: %v", err)
	}
	if len(got) != 0 {
		t.Fatalf("got = %v, want an empty slice", got)
	}
}

func TestHandleListApps_ReturnsInstalledApps(t *testing.T) {
	db := openMigratedTestDB(t)
	installTestApp(t, db, "dashboard", "hello_patchcord")

	router := NewRouter(Deps{DB: db})
	req := httptest.NewRequest(http.MethodGet, "/v1/apps", nil)
	rec := httptest.NewRecorder()
	router.ServeHTTP(rec, req)

	var got []appSummary
	if err := json.NewDecoder(rec.Body).Decode(&got); err != nil {
		t.Fatalf("decode response body: %v", err)
	}
	if len(got) != 1 || got[0].ID != "dashboard" {
		t.Fatalf("got = %+v, want one app with id=dashboard", got)
	}
	if len(got[0].WorkflowsRun) != 1 || got[0].WorkflowsRun[0] != "hello_patchcord" {
		t.Fatalf("got[0].WorkflowsRun = %v, want [hello_patchcord]", got[0].WorkflowsRun)
	}
}

func TestHandleCreateAppSession_UnknownAppReturnsNotFound(t *testing.T) {
	db := openMigratedTestDB(t)
	router := NewRouter(Deps{DB: db, Sessions: auth.NewStore()})

	req := httptest.NewRequest(http.MethodPost, "/v1/apps/does-not-exist/sessions", nil)
	rec := httptest.NewRecorder()
	router.ServeHTTP(rec, req)

	if rec.Code != http.StatusNotFound {
		t.Fatalf("status = %d, want %d", rec.Code, http.StatusNotFound)
	}
}

func TestHandleCreateAppSession_IssuesATokenScopedToPermissions(t *testing.T) {
	db := openMigratedTestDB(t)
	installTestApp(t, db, "dashboard", "hello_patchcord")
	router := NewRouter(Deps{DB: db, Sessions: auth.NewStore()})

	req := httptest.NewRequest(http.MethodPost, "/v1/apps/dashboard/sessions", nil)
	rec := httptest.NewRecorder()
	router.ServeHTTP(rec, req)

	if rec.Code != http.StatusCreated {
		t.Fatalf("status = %d, want %d", rec.Code, http.StatusCreated)
	}

	var got appSessionResponse
	if err := json.NewDecoder(rec.Body).Decode(&got); err != nil {
		t.Fatalf("decode response body: %v", err)
	}
	if got.Token == "" {
		t.Fatal("response token is empty")
	}
	if got.AppID != "dashboard" {
		t.Fatalf("AppID = %q, want %q", got.AppID, "dashboard")
	}
	if len(got.WorkflowsRun) != 1 || got.WorkflowsRun[0] != "hello_patchcord" {
		t.Fatalf("WorkflowsRun = %v, want [hello_patchcord]", got.WorkflowsRun)
	}
}

func TestHandleCreateAppSession_RequiresAdminTokenOnceOneExists(t *testing.T) {
	db := openMigratedTestDB(t)
	installTestApp(t, db, "dashboard", "hello_patchcord")
	if _, _, err := auth.CreateToken(context.Background(), db, "ci"); err != nil {
		t.Fatalf("CreateToken() error = %v", err)
	}
	router := NewRouter(Deps{DB: db, Sessions: auth.NewStore()})

	t.Run("no Authorization header is rejected", func(t *testing.T) {
		req := httptest.NewRequest(http.MethodPost, "/v1/apps/dashboard/sessions", nil)
		rec := httptest.NewRecorder()
		router.ServeHTTP(rec, req)

		if rec.Code != http.StatusUnauthorized {
			t.Fatalf("status = %d, want %d", rec.Code, http.StatusUnauthorized)
		}
	})

	t.Run("a valid admin token is accepted", func(t *testing.T) {
		plaintext, _, err := auth.CreateToken(context.Background(), db, "second")
		if err != nil {
			t.Fatalf("CreateToken() error = %v", err)
		}
		req := httptest.NewRequest(http.MethodPost, "/v1/apps/dashboard/sessions", nil)
		req.Header.Set("Authorization", "Bearer "+plaintext)
		rec := httptest.NewRecorder()
		router.ServeHTTP(rec, req)

		if rec.Code != http.StatusCreated {
			t.Fatalf("status = %d, want %d (body: %s)", rec.Code, http.StatusCreated, rec.Body.String())
		}
	})
}

func TestHandleServeApp(t *testing.T) {
	t.Run("serves the app's static files", func(t *testing.T) {
		db := openMigratedTestDB(t)
		installTestApp(t, db, "dashboard")
		router := NewRouter(Deps{DB: db})

		// Not /apps/dashboard/index.html: net/http's file server redirects
		// an explicit "index.html" request to its directory's URL, to avoid
		// two URLs serving the same content — the trailing-slash form below
		// is also the exact shape the vision document uses (section 10.3).
		req := httptest.NewRequest(http.MethodGet, "/apps/dashboard/", nil)
		rec := httptest.NewRecorder()
		router.ServeHTTP(rec, req)

		if rec.Code != http.StatusOK {
			t.Fatalf("status = %d, want %d", rec.Code, http.StatusOK)
		}
		if rec.Body.String() != "<h1>dashboard</h1>" {
			t.Fatalf("body = %q, want %q", rec.Body.String(), "<h1>dashboard</h1>")
		}
	})

	t.Run("returns not found for an unknown app", func(t *testing.T) {
		db := openMigratedTestDB(t)
		router := NewRouter(Deps{DB: db})

		req := httptest.NewRequest(http.MethodGet, "/apps/does-not-exist/index.html", nil)
		rec := httptest.NewRecorder()
		router.ServeHTTP(rec, req)

		if rec.Code != http.StatusNotFound {
			t.Fatalf("status = %d, want %d", rec.Code, http.StatusNotFound)
		}
	})
}

func TestHandleAppsDirectory(t *testing.T) {
	t.Run("disabled returns not found, matching pre-ADR-0061 behavior", func(t *testing.T) {
		db := openMigratedTestDB(t)
		installTestApp(t, db, "dashboard")
		router := NewRouter(Deps{DB: db})

		req := httptest.NewRequest(http.MethodGet, "/apps/", nil)
		rec := httptest.NewRecorder()
		router.ServeHTTP(rec, req)

		if rec.Code != http.StatusNotFound {
			t.Fatalf("status = %d, want %d", rec.Code, http.StatusNotFound)
		}
	})

	t.Run("enabled with no apps installed lists none", func(t *testing.T) {
		db := openMigratedTestDB(t)
		router := NewRouter(Deps{DB: db, AppsDirectoryListingEnabled: true})

		req := httptest.NewRequest(http.MethodGet, "/apps/", nil)
		rec := httptest.NewRecorder()
		router.ServeHTTP(rec, req)

		if rec.Code != http.StatusOK {
			t.Fatalf("status = %d, want %d", rec.Code, http.StatusOK)
		}
		if ct := rec.Header().Get("Content-Type"); !strings.HasPrefix(ct, "text/html") {
			t.Fatalf("Content-Type = %q, want text/html prefix", ct)
		}
		if strings.Contains(rec.Body.String(), "/apps/dashboard/") {
			t.Fatalf("body unexpectedly links to an app: %s", rec.Body.String())
		}
	})

	t.Run("enabled with apps installed links to each one", func(t *testing.T) {
		db := openMigratedTestDB(t)
		installTestApp(t, db, "dashboard")
		installTestApp(t, db, "notes")
		router := NewRouter(Deps{DB: db, AppsDirectoryListingEnabled: true})

		req := httptest.NewRequest(http.MethodGet, "/apps/", nil)
		rec := httptest.NewRecorder()
		router.ServeHTTP(rec, req)

		if rec.Code != http.StatusOK {
			t.Fatalf("status = %d, want %d", rec.Code, http.StatusOK)
		}
		body := rec.Body.String()
		if !strings.Contains(body, `href="/apps/dashboard/"`) {
			t.Fatalf("body does not link to /apps/dashboard/: %s", body)
		}
		if !strings.Contains(body, `href="/apps/notes/"`) {
			t.Fatalf("body does not link to /apps/notes/: %s", body)
		}
	})

	t.Run("an individual app is still served, not shadowed by the new route", func(t *testing.T) {
		db := openMigratedTestDB(t)
		installTestApp(t, db, "dashboard")
		router := NewRouter(Deps{DB: db, AppsDirectoryListingEnabled: true})

		req := httptest.NewRequest(http.MethodGet, "/apps/dashboard/", nil)
		rec := httptest.NewRecorder()
		router.ServeHTTP(rec, req)

		if rec.Code != http.StatusOK {
			t.Fatalf("status = %d, want %d", rec.Code, http.StatusOK)
		}
		if rec.Body.String() != "<h1>dashboard</h1>" {
			t.Fatalf("body = %q, want %q", rec.Body.String(), "<h1>dashboard</h1>")
		}
	})
}

func TestWithRunAuth(t *testing.T) {
	t.Run("no Authorization header reaches the handler unrestricted while no admin token exists", func(t *testing.T) {
		db := openMigratedTestDB(t)
		knownActions := map[string]workflow.KnownAction{"text.uppercase@1": {}}
		if _, err := runs.InstallWorkflow(context.Background(), db, []byte(eventsTestWorkflow), knownActions); err != nil {
			t.Fatalf("InstallWorkflow() error = %v", err)
		}
		router := NewRouter(Deps{DB: db, Executor: fakeExecutor{}, Sessions: auth.NewStore()})

		req := httptest.NewRequest(http.MethodPost, "/v1/workflows/hello_patchcord/run", nil)
		rec := httptest.NewRecorder()
		router.ServeHTTP(rec, req)

		if rec.Code != http.StatusAccepted {
			t.Fatalf("status = %d, want %d (body: %s)", rec.Code, http.StatusAccepted, rec.Body.String())
		}
		waitForRunToFinish(t, db, rec.Body)
	})

	t.Run("an invalid token is rejected even while no admin token exists", func(t *testing.T) {
		db := openMigratedTestDB(t)
		knownActions := map[string]workflow.KnownAction{"text.uppercase@1": {}}
		if _, err := runs.InstallWorkflow(context.Background(), db, []byte(eventsTestWorkflow), knownActions); err != nil {
			t.Fatalf("InstallWorkflow() error = %v", err)
		}
		router := NewRouter(Deps{DB: db, Executor: fakeExecutor{}, Sessions: auth.NewStore()})

		req := httptest.NewRequest(http.MethodPost, "/v1/workflows/hello_patchcord/run", nil)
		req.Header.Set("Authorization", "Bearer not-a-real-token")
		rec := httptest.NewRecorder()
		router.ServeHTTP(rec, req)

		if rec.Code != http.StatusUnauthorized {
			t.Fatalf("status = %d, want %d", rec.Code, http.StatusUnauthorized)
		}
	})

	t.Run("a valid token not permitted to run the workflow is forbidden", func(t *testing.T) {
		db := openMigratedTestDB(t)
		knownActions := map[string]workflow.KnownAction{"text.uppercase@1": {}}
		if _, err := runs.InstallWorkflow(context.Background(), db, []byte(eventsTestWorkflow), knownActions); err != nil {
			t.Fatalf("InstallWorkflow() error = %v", err)
		}
		app := installTestApp(t, db, "dashboard" /* no workflows.run permission */)
		sessions := auth.NewStore()
		session := sessions.Issue(*app)
		router := NewRouter(Deps{DB: db, Executor: fakeExecutor{}, Sessions: sessions})

		req := httptest.NewRequest(http.MethodPost, "/v1/workflows/hello_patchcord/run", nil)
		req.Header.Set("Authorization", "Bearer "+session.Token)
		rec := httptest.NewRecorder()
		router.ServeHTTP(rec, req)

		if rec.Code != http.StatusForbidden {
			t.Fatalf("status = %d, want %d (body: %s)", rec.Code, http.StatusForbidden, rec.Body.String())
		}
	})

	t.Run("a valid token permitted to run the workflow reaches the handler", func(t *testing.T) {
		db := openMigratedTestDB(t)
		knownActions := map[string]workflow.KnownAction{"text.uppercase@1": {}}
		if _, err := runs.InstallWorkflow(context.Background(), db, []byte(eventsTestWorkflow), knownActions); err != nil {
			t.Fatalf("InstallWorkflow() error = %v", err)
		}
		app := installTestApp(t, db, "dashboard", "hello_patchcord")
		sessions := auth.NewStore()
		session := sessions.Issue(*app)
		router := NewRouter(Deps{DB: db, Executor: fakeExecutor{}, Sessions: sessions})

		req := httptest.NewRequest(http.MethodPost, "/v1/workflows/hello_patchcord/run", nil)
		req.Header.Set("Authorization", "Bearer "+session.Token)
		rec := httptest.NewRecorder()
		router.ServeHTTP(rec, req)

		if rec.Code != http.StatusAccepted {
			t.Fatalf("status = %d, want %d (body: %s)", rec.Code, http.StatusAccepted, rec.Body.String())
		}
		waitForRunToFinish(t, db, rec.Body)
	})

	t.Run("once an admin token exists, no Authorization header is rejected", func(t *testing.T) {
		db := openMigratedTestDB(t)
		knownActions := map[string]workflow.KnownAction{"text.uppercase@1": {}}
		if _, err := runs.InstallWorkflow(context.Background(), db, []byte(eventsTestWorkflow), knownActions); err != nil {
			t.Fatalf("InstallWorkflow() error = %v", err)
		}
		if _, _, err := auth.CreateToken(context.Background(), db, "ci"); err != nil {
			t.Fatalf("CreateToken() error = %v", err)
		}
		router := NewRouter(Deps{DB: db, Executor: fakeExecutor{}, Sessions: auth.NewStore()})

		req := httptest.NewRequest(http.MethodPost, "/v1/workflows/hello_patchcord/run", nil)
		rec := httptest.NewRecorder()
		router.ServeHTTP(rec, req)

		if rec.Code != http.StatusUnauthorized {
			t.Fatalf("status = %d, want %d", rec.Code, http.StatusUnauthorized)
		}
	})

	t.Run("once an admin token exists, that admin token reaches the handler", func(t *testing.T) {
		db := openMigratedTestDB(t)
		knownActions := map[string]workflow.KnownAction{"text.uppercase@1": {}}
		if _, err := runs.InstallWorkflow(context.Background(), db, []byte(eventsTestWorkflow), knownActions); err != nil {
			t.Fatalf("InstallWorkflow() error = %v", err)
		}
		plaintext, _, err := auth.CreateToken(context.Background(), db, "ci")
		if err != nil {
			t.Fatalf("CreateToken() error = %v", err)
		}
		router := NewRouter(Deps{DB: db, Executor: fakeExecutor{}, Sessions: auth.NewStore()})

		req := httptest.NewRequest(http.MethodPost, "/v1/workflows/hello_patchcord/run", nil)
		req.Header.Set("Authorization", "Bearer "+plaintext)
		rec := httptest.NewRecorder()
		router.ServeHTTP(rec, req)

		if rec.Code != http.StatusAccepted {
			t.Fatalf("status = %d, want %d (body: %s)", rec.Code, http.StatusAccepted, rec.Body.String())
		}
		waitForRunToFinish(t, db, rec.Body)
	})

	t.Run("once an admin token exists, a scoped app session still reaches the handler", func(t *testing.T) {
		db := openMigratedTestDB(t)
		knownActions := map[string]workflow.KnownAction{"text.uppercase@1": {}}
		if _, err := runs.InstallWorkflow(context.Background(), db, []byte(eventsTestWorkflow), knownActions); err != nil {
			t.Fatalf("InstallWorkflow() error = %v", err)
		}
		if _, _, err := auth.CreateToken(context.Background(), db, "ci"); err != nil {
			t.Fatalf("CreateToken() error = %v", err)
		}
		app := installTestApp(t, db, "dashboard", "hello_patchcord")
		sessions := auth.NewStore()
		session := sessions.Issue(*app)
		router := NewRouter(Deps{DB: db, Executor: fakeExecutor{}, Sessions: sessions})

		req := httptest.NewRequest(http.MethodPost, "/v1/workflows/hello_patchcord/run", nil)
		req.Header.Set("Authorization", "Bearer "+session.Token)
		rec := httptest.NewRecorder()
		router.ServeHTTP(rec, req)

		if rec.Code != http.StatusAccepted {
			t.Fatalf("status = %d, want %d (body: %s)", rec.Code, http.StatusAccepted, rec.Body.String())
		}
		waitForRunToFinish(t, db, rec.Body)
	})

	t.Run("once an admin token exists, an unrecognized bearer token is rejected", func(t *testing.T) {
		db := openMigratedTestDB(t)
		knownActions := map[string]workflow.KnownAction{"text.uppercase@1": {}}
		if _, err := runs.InstallWorkflow(context.Background(), db, []byte(eventsTestWorkflow), knownActions); err != nil {
			t.Fatalf("InstallWorkflow() error = %v", err)
		}
		if _, _, err := auth.CreateToken(context.Background(), db, "ci"); err != nil {
			t.Fatalf("CreateToken() error = %v", err)
		}
		router := NewRouter(Deps{DB: db, Executor: fakeExecutor{}, Sessions: auth.NewStore()})

		req := httptest.NewRequest(http.MethodPost, "/v1/workflows/hello_patchcord/run", nil)
		req.Header.Set("Authorization", "Bearer not-a-real-token")
		rec := httptest.NewRecorder()
		router.ServeHTTP(rec, req)

		if rec.Code != http.StatusUnauthorized {
			t.Fatalf("status = %d, want %d", rec.Code, http.StatusUnauthorized)
		}
	})
}

// connectorBoundTestWorkflow binds its only step to "${{ bindings.demo }}"
// — used by TestWithRunAuth_RestrictsConnectors.
const connectorBoundTestWorkflow = `
schema_version: 1
id: connector_bound
version: 1
trigger:
  type: manual
steps:
  - id: only
    uses: text.uppercase@1
    connector: "${{ bindings.demo }}"
    with:
      value: "hello"
`

func TestWithRunAuth_RestrictsConnectors(t *testing.T) {
	installConnector := func(t *testing.T, db *sql.DB) {
		t.Helper()
		if _, err := connectors.Create(context.Background(), db, "my_conn", "postgresql.connection@1", map[string]any{}, nil, map[string]struct{}{"postgresql.connection@1": {}}); err != nil {
			t.Fatalf("connectors.Create() error = %v", err)
		}
	}

	t.Run("a session without connectors.use fails a connector-bound run", func(t *testing.T) {
		db := openMigratedTestDB(t)
		knownActions := map[string]workflow.KnownAction{"text.uppercase@1": {}}
		if _, err := runs.InstallWorkflow(context.Background(), db, []byte(connectorBoundTestWorkflow), knownActions); err != nil {
			t.Fatalf("InstallWorkflow() error = %v", err)
		}
		installConnector(t, db)
		app := installTestAppWithConnectors(t, db, "dashboard", []string{"connector_bound"}, nil)
		sessions := auth.NewStore()
		session := sessions.Issue(*app)
		router := NewRouter(Deps{DB: db, Executor: fakeExecutor{}, Sessions: sessions})

		req := httptest.NewRequest(http.MethodPost, "/v1/workflows/connector_bound/run", strings.NewReader(`{"bindings":{"demo":"my_conn"}}`))
		req.Header.Set("Authorization", "Bearer "+session.Token)
		rec := httptest.NewRecorder()
		router.ServeHTTP(rec, req)

		if rec.Code != http.StatusAccepted {
			t.Fatalf("status = %d, want %d (body: %s)", rec.Code, http.StatusAccepted, rec.Body.String())
		}
		run := waitForRunToFinish(t, db, rec.Body)
		if run.Status != workflow.RunFailed {
			t.Fatalf("run status = %s, want %s (connector not declared in connectors.use)", run.Status, workflow.RunFailed)
		}
	})

	t.Run("a session with the connector in connectors.use succeeds", func(t *testing.T) {
		db := openMigratedTestDB(t)
		knownActions := map[string]workflow.KnownAction{"text.uppercase@1": {}}
		if _, err := runs.InstallWorkflow(context.Background(), db, []byte(connectorBoundTestWorkflow), knownActions); err != nil {
			t.Fatalf("InstallWorkflow() error = %v", err)
		}
		installConnector(t, db)
		app := installTestAppWithConnectors(t, db, "dashboard", []string{"connector_bound"}, []string{"my_conn"})
		sessions := auth.NewStore()
		session := sessions.Issue(*app)
		router := NewRouter(Deps{DB: db, Executor: fakeExecutor{}, Sessions: sessions})

		req := httptest.NewRequest(http.MethodPost, "/v1/workflows/connector_bound/run", strings.NewReader(`{"bindings":{"demo":"my_conn"}}`))
		req.Header.Set("Authorization", "Bearer "+session.Token)
		rec := httptest.NewRecorder()
		router.ServeHTTP(rec, req)

		if rec.Code != http.StatusAccepted {
			t.Fatalf("status = %d, want %d (body: %s)", rec.Code, http.StatusAccepted, rec.Body.String())
		}
		run := waitForRunToFinish(t, db, rec.Body)
		if run.Status != workflow.RunSucceeded {
			t.Fatalf("run status = %s, want %s", run.Status, workflow.RunSucceeded)
		}
	})

	t.Run("an admin token bypasses connectors.use entirely", func(t *testing.T) {
		db := openMigratedTestDB(t)
		knownActions := map[string]workflow.KnownAction{"text.uppercase@1": {}}
		if _, err := runs.InstallWorkflow(context.Background(), db, []byte(connectorBoundTestWorkflow), knownActions); err != nil {
			t.Fatalf("InstallWorkflow() error = %v", err)
		}
		installConnector(t, db)
		plaintext, _, err := auth.CreateToken(context.Background(), db, "ci")
		if err != nil {
			t.Fatalf("CreateToken() error = %v", err)
		}
		router := NewRouter(Deps{DB: db, Executor: fakeExecutor{}, Sessions: auth.NewStore()})

		req := httptest.NewRequest(http.MethodPost, "/v1/workflows/connector_bound/run", strings.NewReader(`{"bindings":{"demo":"my_conn"}}`))
		req.Header.Set("Authorization", "Bearer "+plaintext)
		rec := httptest.NewRecorder()
		router.ServeHTTP(rec, req)

		if rec.Code != http.StatusAccepted {
			t.Fatalf("status = %d, want %d (body: %s)", rec.Code, http.StatusAccepted, rec.Body.String())
		}
		run := waitForRunToFinish(t, db, rec.Body)
		if run.Status != workflow.RunSucceeded {
			t.Fatalf("run status = %s, want %s (admin token must not be restricted)", run.Status, workflow.RunSucceeded)
		}
	})

	t.Run("an app with no connectors.use permission still runs a workflow with no connector-bound step", func(t *testing.T) {
		// Regression guard: apps that only ever declared workflows.run
		// (pre-ADR-0071) must keep working for workflows that never bind a
		// connector in the first place.
		db := openMigratedTestDB(t)
		knownActions := map[string]workflow.KnownAction{"text.uppercase@1": {}}
		if _, err := runs.InstallWorkflow(context.Background(), db, []byte(eventsTestWorkflow), knownActions); err != nil {
			t.Fatalf("InstallWorkflow() error = %v", err)
		}
		app := installTestApp(t, db, "dashboard", "hello_patchcord")
		sessions := auth.NewStore()
		session := sessions.Issue(*app)
		router := NewRouter(Deps{DB: db, Executor: fakeExecutor{}, Sessions: sessions})

		req := httptest.NewRequest(http.MethodPost, "/v1/workflows/hello_patchcord/run", nil)
		req.Header.Set("Authorization", "Bearer "+session.Token)
		rec := httptest.NewRecorder()
		router.ServeHTTP(rec, req)

		if rec.Code != http.StatusAccepted {
			t.Fatalf("status = %d, want %d (body: %s)", rec.Code, http.StatusAccepted, rec.Body.String())
		}
		run := waitForRunToFinish(t, db, rec.Body)
		if run.Status != workflow.RunSucceeded {
			t.Fatalf("run status = %s, want %s", run.Status, workflow.RunSucceeded)
		}
	})
}

// waitForRunToFinish decodes a runSummary from body and polls until the run
// it names reaches a terminal status, returning it. Other tests in this
// package (e.g. TestHandleRunWorkflow_StartsImmediatelyAndRunsInTheBackground)
// avoid this by asserting on the final status directly; here it exists
// only to let handleRunWorkflow's background runs.Continue goroutine
// finish and release its database connection before t.Cleanup closes db
// — otherwise a later subtest opening a *sql.DB with the same shared-cache
// in-memory URI can race with it and see stale state. Most callers ignore
// the return value; ADR-0071's connector-permission tests use it to assert
// on the run's terminal status.
func waitForRunToFinish(t *testing.T, db *sql.DB, body *bytes.Buffer) *runs.Run {
	t.Helper()

	var summary runSummary
	if err := json.NewDecoder(body).Decode(&summary); err != nil {
		t.Fatalf("decode response body: %v", err)
	}

	deadline := time.After(2 * time.Second)
	for {
		run, _, err := runs.GetRun(context.Background(), db, summary.ID)
		if err != nil {
			t.Fatalf("GetRun() error = %v", err)
		}
		if run.Status == "succeeded" || run.Status == "failed" {
			return run
		}
		select {
		case <-deadline:
			t.Fatalf("run did not reach a terminal status in time, last status = %s", run.Status)
		case <-time.After(10 * time.Millisecond):
		}
	}
}
