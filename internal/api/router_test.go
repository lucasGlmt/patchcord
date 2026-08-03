package api

import (
	"database/sql"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	_ "modernc.org/sqlite"
)

func openTestDB(t *testing.T) *sql.DB {
	t.Helper()
	// cache=shared: some tests (e.g. handleRunWorkflow's background
	// runs.Continue goroutine) touch the database from more than one
	// goroutine concurrently, which can each get a different pooled
	// connection — an unshared ":memory:" database is private per
	// connection, so a second connection would see an empty schema. A real,
	// file-backed database (as persistence.Open uses in production) doesn't
	// have this problem: the file itself is what's shared.
	db, err := sql.Open("sqlite", "file::memory:?cache=shared&_busy_timeout=5000")
	if err != nil {
		t.Fatalf("open in-memory database: %v", err)
	}
	t.Cleanup(func() { _ = db.Close() })
	return db
}

func closedTestDB(t *testing.T) *sql.DB {
	t.Helper()
	db := openTestDB(t)
	if err := db.Close(); err != nil {
		t.Fatalf("close database: %v", err)
	}
	return db
}

func TestRouter_Health(t *testing.T) {
	tests := []struct {
		name       string
		method     string
		path       string
		db         func(t *testing.T) *sql.DB
		wantStatus int
		wantBody   *healthResponse
	}{
		{
			name:       "GET health returns ok when the database is reachable",
			method:     http.MethodGet,
			path:       "/v1/system/health",
			db:         openTestDB,
			wantStatus: http.StatusOK,
			wantBody:   &healthResponse{Status: "ok", Database: "ok"},
		},
		{
			name:       "GET health returns degraded when the database is unreachable",
			method:     http.MethodGet,
			path:       "/v1/system/health",
			db:         closedTestDB,
			wantStatus: http.StatusServiceUnavailable,
			wantBody:   &healthResponse{Status: "degraded", Database: "unreachable"},
		},
		{
			name:       "POST health is not allowed",
			method:     http.MethodPost,
			path:       "/v1/system/health",
			db:         openTestDB,
			wantStatus: http.StatusMethodNotAllowed,
		},
		{
			name:       "unknown path returns not found",
			method:     http.MethodGet,
			path:       "/v1/unknown",
			db:         openTestDB,
			wantStatus: http.StatusNotFound,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			router := NewRouter(Deps{DB: tt.db(t)})

			req := httptest.NewRequest(tt.method, tt.path, nil)
			rec := httptest.NewRecorder()

			router.ServeHTTP(rec, req)

			if rec.Code != tt.wantStatus {
				t.Fatalf("status = %d, want %d", rec.Code, tt.wantStatus)
			}

			if tt.wantBody != nil {
				var got healthResponse
				if err := json.NewDecoder(rec.Body).Decode(&got); err != nil {
					t.Fatalf("decode response body: %v", err)
				}
				if got != *tt.wantBody {
					t.Fatalf("body = %+v, want %+v", got, *tt.wantBody)
				}
			}
		})
	}
}

// TestRouter_CORSAllowsDelete guards against a regression where a new verb
// (DELETE /v1/connectors/{id}) is wired into the mux but withCORS's
// Access-Control-Allow-Methods list isn't updated to match — invisible to
// every other test here since none of them go through a browser's actual
// CORS preflight enforcement, only this response header.
func TestRouter_CORSAllowsDelete(t *testing.T) {
	router := NewRouter(Deps{DB: openTestDB(t)})

	req := httptest.NewRequest(http.MethodOptions, "/v1/connectors/my_api", nil)
	rec := httptest.NewRecorder()
	router.ServeHTTP(rec, req)

	if rec.Code != http.StatusNoContent {
		t.Fatalf("status = %d, want %d", rec.Code, http.StatusNoContent)
	}
	if allow := rec.Header().Get("Access-Control-Allow-Methods"); !strings.Contains(allow, "DELETE") {
		t.Fatalf("Access-Control-Allow-Methods = %q, want it to contain DELETE", allow)
	}
}

func TestRouter_OpenAPISpec(t *testing.T) {
	router := NewRouter(Deps{DB: openTestDB(t)})

	req := httptest.NewRequest(http.MethodGet, "/v1/openapi.json", nil)
	rec := httptest.NewRecorder()
	router.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, want %d", rec.Code, http.StatusOK)
	}
	if ct := rec.Header().Get("Content-Type"); ct != "application/json" {
		t.Fatalf("Content-Type = %q, want %q", ct, "application/json")
	}

	var spec struct {
		Swagger string         `json:"swagger"`
		Paths   map[string]any `json:"paths"`
	}
	if err := json.NewDecoder(rec.Body).Decode(&spec); err != nil {
		t.Fatalf("decode response body: %v", err)
	}
	if spec.Swagger != "2.0" {
		t.Fatalf("swagger version = %q, want %q", spec.Swagger, "2.0")
	}
	for _, path := range []string{"/system/health", "/workflows/{id}/run", "/runs/{id}", "/runs/{id}/events"} {
		if _, ok := spec.Paths[path]; !ok {
			t.Fatalf("spec paths = %v, want it to contain %q", spec.Paths, path)
		}
	}
}
