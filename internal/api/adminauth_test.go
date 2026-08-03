package api

import (
	"context"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/lucasglmt/patchcord/internal/auth"
)

func TestWithAdminAuth_OpenByDefault(t *testing.T) {
	db := openMigratedTestDB(t)
	router := NewRouter(Deps{DB: db})

	req := httptest.NewRequest(http.MethodGet, "/v1/workflows", nil)
	rec := httptest.NewRecorder()
	router.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, want %d (body: %s)", rec.Code, http.StatusOK, rec.Body.String())
	}
}

func TestWithAdminAuth_RequiresATokenOnceOneExists(t *testing.T) {
	db := openMigratedTestDB(t)
	plaintext, _, err := auth.CreateToken(context.Background(), db, "ci")
	if err != nil {
		t.Fatalf("CreateToken() error = %v", err)
	}
	router := NewRouter(Deps{DB: db})

	t.Run("no Authorization header is rejected", func(t *testing.T) {
		req := httptest.NewRequest(http.MethodGet, "/v1/workflows", nil)
		rec := httptest.NewRecorder()
		router.ServeHTTP(rec, req)

		if rec.Code != http.StatusUnauthorized {
			t.Fatalf("status = %d, want %d", rec.Code, http.StatusUnauthorized)
		}
	})

	t.Run("an unrecognized bearer token is rejected", func(t *testing.T) {
		req := httptest.NewRequest(http.MethodGet, "/v1/workflows", nil)
		req.Header.Set("Authorization", "Bearer not-a-real-token")
		rec := httptest.NewRecorder()
		router.ServeHTTP(rec, req)

		if rec.Code != http.StatusUnauthorized {
			t.Fatalf("status = %d, want %d", rec.Code, http.StatusUnauthorized)
		}
	})

	t.Run("the admin token reaches the handler", func(t *testing.T) {
		req := httptest.NewRequest(http.MethodGet, "/v1/workflows", nil)
		req.Header.Set("Authorization", "Bearer "+plaintext)
		rec := httptest.NewRecorder()
		router.ServeHTTP(rec, req)

		if rec.Code != http.StatusOK {
			t.Fatalf("status = %d, want %d (body: %s)", rec.Code, http.StatusOK, rec.Body.String())
		}
	})
}

func TestWithAdminAuth_ExemptRoutesStayOpenEvenOnceATokenExists(t *testing.T) {
	db := openMigratedTestDB(t)
	if _, _, err := auth.CreateToken(context.Background(), db, "ci"); err != nil {
		t.Fatalf("CreateToken() error = %v", err)
	}
	installTestApp(t, db, "dashboard")
	router := NewRouter(Deps{DB: db})

	for _, path := range []string{"/v1/system/health", "/v1/openapi.json", "/apps/dashboard/"} {
		t.Run(path, func(t *testing.T) {
			req := httptest.NewRequest(http.MethodGet, path, nil)
			rec := httptest.NewRecorder()
			router.ServeHTTP(rec, req)

			if rec.Code != http.StatusOK {
				t.Fatalf("status = %d, want %d (body: %s)", rec.Code, http.StatusOK, rec.Body.String())
			}
		})
	}
}
