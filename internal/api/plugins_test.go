package api

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
)

func TestHandleListPlugins_EmptyCatalog(t *testing.T) {
	db := openMigratedTestDB(t)
	router := NewRouter(Deps{DB: db})

	req := httptest.NewRequest(http.MethodGet, "/v1/plugins", nil)
	rec := httptest.NewRecorder()
	router.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, want %d", rec.Code, http.StatusOK)
	}

	var got []pluginSummary
	if err := json.NewDecoder(rec.Body).Decode(&got); err != nil {
		t.Fatalf("decode response body: %v", err)
	}
	if len(got) != 0 {
		t.Fatalf("got = %v, want an empty slice", got)
	}
}

func TestHandleListPlugins_ReturnsInstalledPlugins(t *testing.T) {
	db := openMigratedTestDB(t)
	insertTestPlugin(t, db, "io.patchcord.postgresql", []string{"postgresql.connection@1"}, []string{"postgresql.query@1"})

	router := NewRouter(Deps{DB: db})
	req := httptest.NewRequest(http.MethodGet, "/v1/plugins", nil)
	rec := httptest.NewRecorder()
	router.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, want %d", rec.Code, http.StatusOK)
	}

	var got []pluginSummary
	if err := json.NewDecoder(rec.Body).Decode(&got); err != nil {
		t.Fatalf("decode response body: %v", err)
	}
	if len(got) != 1 {
		t.Fatalf("len(got) = %d, want 1", len(got))
	}
	if got[0].ID != "io.patchcord.postgresql" {
		t.Fatalf("ID = %q, want %q", got[0].ID, "io.patchcord.postgresql")
	}
	if len(got[0].Connectors) != 1 || got[0].Connectors[0] != "postgresql.connection@1" {
		t.Fatalf("Connectors = %v, want [postgresql.connection@1]", got[0].Connectors)
	}
	if len(got[0].Actions) != 1 || got[0].Actions[0] != "postgresql.query@1" {
		t.Fatalf("Actions = %v, want [postgresql.query@1]", got[0].Actions)
	}
}
