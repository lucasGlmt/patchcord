package api

import (
	"context"
	"database/sql"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/lucasglmt/patchcord/internal/connectors"
	"github.com/lucasglmt/patchcord/internal/plugins"
)

// fakeConnectorTester is an in-memory ConnectorTester: it never launches a
// real plugin process, mirroring fakeExecutor's role for handleRunWorkflow.
type fakeConnectorTester struct {
	ok      bool
	message string
	err     error
}

func (f fakeConnectorTester) TestConnector(_ context.Context, _ *connectors.ResolvedConnector) (bool, string, error) {
	return f.ok, f.message, f.err
}

// insertTestPlugin records a plugin catalog entry directly in the database
// — the same shape plugins.Install would record after a real handshake —
// so a test can exercise connector-type validation and the plugins listing
// endpoint without launching an actual plugin process. It only names ids,
// not full descriptors (description, JSON Schema, ADR-0062): the tests that
// call it only care about identity and binding, not documentation content.
func insertTestPlugin(t *testing.T, db *sql.DB, pluginID string, connectorTypes, actions []string) {
	t.Helper()

	connectorDescs := make([]plugins.ConnectorDescriptor, len(connectorTypes))
	for i, typ := range connectorTypes {
		connectorDescs[i] = plugins.ConnectorDescriptor{Type: typ}
	}
	actionDescs := make([]plugins.ActionDescriptor, len(actions))
	for i, id := range actions {
		actionDescs[i] = plugins.ActionDescriptor{ID: id}
	}

	connectorsJSON, err := json.Marshal(connectorDescs)
	if err != nil {
		t.Fatalf("marshal connectors: %v", err)
	}
	actionsJSON, err := json.Marshal(actionDescs)
	if err != nil {
		t.Fatalf("marshal actions: %v", err)
	}

	_, err = db.ExecContext(context.Background(), `
		INSERT INTO plugins (plugin_id, version, executable_path, protocol_version, connectors, actions)
		VALUES (?, '1.0.0', '/dev/null', 1, ?, ?)
	`, pluginID, string(connectorsJSON), string(actionsJSON))
	if err != nil {
		t.Fatalf("insert test plugin: %v", err)
	}
}

func TestHandleListConnectors_EmptyCatalog(t *testing.T) {
	db := openMigratedTestDB(t)
	router := NewRouter(Deps{DB: db})

	req := httptest.NewRequest(http.MethodGet, "/v1/connectors", nil)
	rec := httptest.NewRecorder()
	router.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, want %d", rec.Code, http.StatusOK)
	}

	var got []connectorSummary
	if err := json.NewDecoder(rec.Body).Decode(&got); err != nil {
		t.Fatalf("decode response body: %v", err)
	}
	if len(got) != 0 {
		t.Fatalf("got = %v, want an empty slice", got)
	}
}

func TestHandleCreateConnector_RecordsAConnector(t *testing.T) {
	db := openMigratedTestDB(t)
	insertTestPlugin(t, db, "io.patchcord.http", []string{"http.request@1"}, []string{"http.get@1"})

	router := NewRouter(Deps{DB: db})

	body := `{"id":"my_api","type":"http.request@1","config":{"base_url":"https://example.com"},"secret_refs":{"api_key":{"type":"env","key":"DEMO_API_KEY"}}}`
	req := httptest.NewRequest(http.MethodPost, "/v1/connectors", strings.NewReader(body))
	rec := httptest.NewRecorder()
	router.ServeHTTP(rec, req)

	if rec.Code != http.StatusCreated {
		t.Fatalf("status = %d, want %d, body = %s", rec.Code, http.StatusCreated, rec.Body.String())
	}

	var got connectorSummary
	if err := json.NewDecoder(rec.Body).Decode(&got); err != nil {
		t.Fatalf("decode response body: %v", err)
	}
	if got.ID != "my_api" || got.Type != "http.request@1" {
		t.Fatalf("got = %+v, want id=my_api type=http.request@1", got)
	}
	if got.Config["base_url"] != "https://example.com" {
		t.Fatalf("Config[base_url] = %v, want %q", got.Config["base_url"], "https://example.com")
	}
	if got.SecretRefs["api_key"] != (connectorSecretRef{Type: "env", Key: "DEMO_API_KEY"}) {
		t.Fatalf("SecretRefs[api_key] = %+v, want {env DEMO_API_KEY}", got.SecretRefs["api_key"])
	}
}

func TestHandleCreateConnector_UnknownTypeReturns400(t *testing.T) {
	db := openMigratedTestDB(t)
	router := NewRouter(Deps{DB: db})

	body := `{"id":"my_api","type":"smtp.connection@1"}`
	req := httptest.NewRequest(http.MethodPost, "/v1/connectors", strings.NewReader(body))
	rec := httptest.NewRecorder()
	router.ServeHTTP(rec, req)

	if rec.Code != http.StatusBadRequest {
		t.Fatalf("status = %d, want %d, body = %s", rec.Code, http.StatusBadRequest, rec.Body.String())
	}
}

func TestHandleCreateConnector_DuplicateIDReturns409(t *testing.T) {
	db := openMigratedTestDB(t)
	insertTestPlugin(t, db, "io.patchcord.http", []string{"http.request@1"}, nil)
	router := NewRouter(Deps{DB: db})

	body := `{"id":"my_api","type":"http.request@1"}`
	first := httptest.NewRequest(http.MethodPost, "/v1/connectors", strings.NewReader(body))
	router.ServeHTTP(httptest.NewRecorder(), first)

	second := httptest.NewRequest(http.MethodPost, "/v1/connectors", strings.NewReader(body))
	rec := httptest.NewRecorder()
	router.ServeHTTP(rec, second)

	if rec.Code != http.StatusConflict {
		t.Fatalf("status = %d, want %d, body = %s", rec.Code, http.StatusConflict, rec.Body.String())
	}
}

func TestHandleGetConnector_UnknownIDReturns404(t *testing.T) {
	db := openMigratedTestDB(t)
	router := NewRouter(Deps{DB: db})

	req := httptest.NewRequest(http.MethodGet, "/v1/connectors/does-not-exist", nil)
	rec := httptest.NewRecorder()
	router.ServeHTTP(rec, req)

	if rec.Code != http.StatusNotFound {
		t.Fatalf("status = %d, want %d", rec.Code, http.StatusNotFound)
	}
}

func TestHandleDeleteConnector_RemovesIt(t *testing.T) {
	db := openMigratedTestDB(t)
	insertTestPlugin(t, db, "io.patchcord.http", []string{"http.request@1"}, nil)
	router := NewRouter(Deps{DB: db})

	create := httptest.NewRequest(http.MethodPost, "/v1/connectors", strings.NewReader(`{"id":"my_api","type":"http.request@1"}`))
	router.ServeHTTP(httptest.NewRecorder(), create)

	del := httptest.NewRequest(http.MethodDelete, "/v1/connectors/my_api", nil)
	rec := httptest.NewRecorder()
	router.ServeHTTP(rec, del)
	if rec.Code != http.StatusNoContent {
		t.Fatalf("delete status = %d, want %d", rec.Code, http.StatusNoContent)
	}

	get := httptest.NewRequest(http.MethodGet, "/v1/connectors/my_api", nil)
	rec = httptest.NewRecorder()
	router.ServeHTTP(rec, get)
	if rec.Code != http.StatusNotFound {
		t.Fatalf("get after delete status = %d, want %d", rec.Code, http.StatusNotFound)
	}
}

func TestHandleDeleteConnector_UnknownIDReturns404(t *testing.T) {
	db := openMigratedTestDB(t)
	router := NewRouter(Deps{DB: db})

	req := httptest.NewRequest(http.MethodDelete, "/v1/connectors/does-not-exist", nil)
	rec := httptest.NewRecorder()
	router.ServeHTTP(rec, req)

	if rec.Code != http.StatusNotFound {
		t.Fatalf("status = %d, want %d", rec.Code, http.StatusNotFound)
	}
}

func TestHandleTestConnector_NoTesterConfiguredReturns500(t *testing.T) {
	db := openMigratedTestDB(t)
	insertTestPlugin(t, db, "io.patchcord.http", []string{"http.request@1"}, nil)
	router := NewRouter(Deps{DB: db})

	create := httptest.NewRequest(http.MethodPost, "/v1/connectors", strings.NewReader(`{"id":"my_api","type":"http.request@1"}`))
	router.ServeHTTP(httptest.NewRecorder(), create)

	req := httptest.NewRequest(http.MethodPost, "/v1/connectors/my_api/test", nil)
	rec := httptest.NewRecorder()
	router.ServeHTTP(rec, req)

	if rec.Code != http.StatusInternalServerError {
		t.Fatalf("status = %d, want %d", rec.Code, http.StatusInternalServerError)
	}
}

func TestHandleTestConnector_ReportsTheTesterResult(t *testing.T) {
	db := openMigratedTestDB(t)
	insertTestPlugin(t, db, "io.patchcord.http", []string{"http.request@1"}, nil)
	router := NewRouter(Deps{DB: db, ConnectorTester: fakeConnectorTester{ok: false, message: "connection refused"}})

	create := httptest.NewRequest(http.MethodPost, "/v1/connectors", strings.NewReader(`{"id":"my_api","type":"http.request@1"}`))
	router.ServeHTTP(httptest.NewRecorder(), create)

	req := httptest.NewRequest(http.MethodPost, "/v1/connectors/my_api/test", nil)
	rec := httptest.NewRecorder()
	router.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, want %d, body = %s", rec.Code, http.StatusOK, rec.Body.String())
	}

	var got connectorTestResponse
	if err := json.NewDecoder(rec.Body).Decode(&got); err != nil {
		t.Fatalf("decode response body: %v", err)
	}
	if got.OK || got.Message != "connection refused" {
		t.Fatalf("got = %+v, want ok=false message=%q", got, "connection refused")
	}
}

// deadlineObservingConnectorTester records whether the ctx it was called
// with carried a deadline, without ever blocking — proving
// handleTestConnector bounds the call (ADR-0018-style) without an actual
// multi-second test.
type deadlineObservingConnectorTester struct {
	hadDeadline *bool
}

func (d deadlineObservingConnectorTester) TestConnector(ctx context.Context, _ *connectors.ResolvedConnector) (bool, string, error) {
	_, ok := ctx.Deadline()
	*d.hadDeadline = ok
	return true, "", nil
}

// TestHandleTestConnector_BoundsTheCallWithADeadline guards against a
// connector pointed at an unreachable host hanging the request forever: a
// request's own context carries no deadline by itself
// (net/http.Server.Serve wires request contexts to server shutdown, not to
// a per-request timeout), so handleTestConnector must add one itself
// before calling the tester.
func TestHandleTestConnector_BoundsTheCallWithADeadline(t *testing.T) {
	db := openMigratedTestDB(t)
	insertTestPlugin(t, db, "io.patchcord.http", []string{"http.request@1"}, nil)

	var hadDeadline bool
	router := NewRouter(Deps{DB: db, ConnectorTester: deadlineObservingConnectorTester{hadDeadline: &hadDeadline}})

	create := httptest.NewRequest(http.MethodPost, "/v1/connectors", strings.NewReader(`{"id":"my_api","type":"http.request@1"}`))
	router.ServeHTTP(httptest.NewRecorder(), create)

	req := httptest.NewRequest(http.MethodPost, "/v1/connectors/my_api/test", nil)
	rec := httptest.NewRecorder()
	router.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, want %d, body = %s", rec.Code, http.StatusOK, rec.Body.String())
	}
	if !hadDeadline {
		t.Fatal("ConnectorTester.TestConnector was called with a ctx that had no deadline")
	}
}

func TestHandleTestConnector_UnknownIDReturns404(t *testing.T) {
	db := openMigratedTestDB(t)
	router := NewRouter(Deps{DB: db, ConnectorTester: fakeConnectorTester{ok: true}})

	req := httptest.NewRequest(http.MethodPost, "/v1/connectors/does-not-exist/test", nil)
	rec := httptest.NewRecorder()
	router.ServeHTTP(rec, req)

	if rec.Code != http.StatusNotFound {
		t.Fatalf("status = %d, want %d", rec.Code, http.StatusNotFound)
	}
}
