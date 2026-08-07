package api

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/lucasglmt/patchcord/internal/auth"
	"github.com/lucasglmt/patchcord/internal/metrics"
)

func TestHandleSystemMetrics_ReturnsASnapshot(t *testing.T) {
	db := openMigratedTestDB(t)
	m := metrics.New()
	m.RecordConnectorTest("ok")
	m.PluginStarted("io.patchcord.fake")

	router := NewRouter(Deps{DB: db, Metrics: m})

	req := httptest.NewRequest(http.MethodGet, "/v1/system/metrics", nil)
	rec := httptest.NewRecorder()
	router.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, want %d (body: %s)", rec.Code, http.StatusOK, rec.Body.String())
	}
	if ct := rec.Header().Get("Content-Type"); ct != "application/json" {
		t.Fatalf("Content-Type = %q, want %q", ct, "application/json")
	}

	var got systemMetricsResponse
	if err := json.NewDecoder(rec.Body).Decode(&got); err != nil {
		t.Fatalf("decode response body: %v", err)
	}
	if got.Connectors.TestTotal["ok"] != 1 {
		t.Fatalf("Connectors.TestTotal[ok] = %d, want 1", got.Connectors.TestTotal["ok"])
	}
	if len(got.Plugins) != 1 || got.Plugins[0].PluginID != "io.patchcord.fake" || !got.Plugins[0].Running {
		t.Fatalf("Plugins = %+v, want one running io.patchcord.fake entry", got.Plugins)
	}
}

func TestHandleSystemMetrics_DefaultsToAnEmptyRegistryWhenNoneConfigured(t *testing.T) {
	db := openMigratedTestDB(t)
	router := NewRouter(Deps{DB: db})

	req := httptest.NewRequest(http.MethodGet, "/v1/system/metrics", nil)
	rec := httptest.NewRecorder()
	router.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, want %d (body: %s)", rec.Code, http.StatusOK, rec.Body.String())
	}

	var got systemMetricsResponse
	if err := json.NewDecoder(rec.Body).Decode(&got); err != nil {
		t.Fatalf("decode response body: %v", err)
	}
	if len(got.Plugins) != 0 {
		t.Fatalf("Plugins = %+v, want none", got.Plugins)
	}
}

func TestHandlePrometheusMetrics_ServesTextExposition(t *testing.T) {
	db := openMigratedTestDB(t)
	m := metrics.New()
	m.RecordConnectorTest("ok")

	router := NewRouter(Deps{DB: db, Metrics: m})

	req := httptest.NewRequest(http.MethodGet, "/metrics", nil)
	rec := httptest.NewRecorder()
	router.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, want %d (body: %s)", rec.Code, http.StatusOK, rec.Body.String())
	}
	if ct := rec.Header().Get("Content-Type"); !strings.HasPrefix(ct, "text/plain") {
		t.Fatalf("Content-Type = %q, want a text/plain prefix", ct)
	}
	if body := rec.Body.String(); !strings.Contains(body, "patchcord_connector_test_total") {
		t.Fatalf("body does not contain the expected metric name: %s", body)
	}
}

func TestMetricsRoutes_RequireATokenOnceOneExists(t *testing.T) {
	db := openMigratedTestDB(t)
	plaintext, _, err := auth.CreateToken(context.Background(), db, "ci")
	if err != nil {
		t.Fatalf("CreateToken() error = %v", err)
	}
	router := NewRouter(Deps{DB: db})

	for _, path := range []string{"/v1/system/metrics", "/metrics"} {
		t.Run(path, func(t *testing.T) {
			req := httptest.NewRequest(http.MethodGet, path, nil)
			rec := httptest.NewRecorder()
			router.ServeHTTP(rec, req)
			if rec.Code != http.StatusUnauthorized {
				t.Fatalf("status without a token = %d, want %d", rec.Code, http.StatusUnauthorized)
			}

			req = httptest.NewRequest(http.MethodGet, path, nil)
			req.Header.Set("Authorization", "Bearer "+plaintext)
			rec = httptest.NewRecorder()
			router.ServeHTTP(rec, req)
			if rec.Code != http.StatusOK {
				t.Fatalf("status with the admin token = %d, want %d (body: %s)", rec.Code, http.StatusOK, rec.Body.String())
			}
		})
	}
}
