package api

import (
	"database/sql"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	_ "modernc.org/sqlite"
)

func openTestDB(t *testing.T) *sql.DB {
	t.Helper()
	db, err := sql.Open("sqlite", "file::memory:")
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
