package cli

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/lucasglmt/patchcord/internal/apps"
)

func TestMintAppSession_SendsTheGivenAdminTokenAndReturnsIt(t *testing.T) {
	var gotAuth, gotPath string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotAuth = r.Header.Get("Authorization")
		gotPath = r.URL.Path
		w.WriteHeader(http.StatusCreated)
		_ = json.NewEncoder(w).Encode(map[string]string{"token": "sess-abc"})
	}))
	defer srv.Close()

	token, err := mintAppSession(context.Background(), srv.URL, "io.patchcord.crm", "admin-tkn")
	if err != nil {
		t.Fatalf("mintAppSession() error = %v", err)
	}
	if token != "sess-abc" {
		t.Fatalf("token = %q, want %q", token, "sess-abc")
	}
	if gotAuth != "Bearer admin-tkn" {
		t.Fatalf("Authorization header = %q, want %q", gotAuth, "Bearer admin-tkn")
	}
	if gotPath != "/v1/apps/io.patchcord.crm/sessions" {
		t.Fatalf("path = %q, want %q", gotPath, "/v1/apps/io.patchcord.crm/sessions")
	}
}

func TestMintAppSession_OmitsTheHeaderWhenNoAdminTokenIsGiven(t *testing.T) {
	var gotAuth string
	sawAuth := false
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotAuth, sawAuth = r.Header.Get("Authorization"), r.Header.Get("Authorization") != ""
		w.WriteHeader(http.StatusCreated)
		_ = json.NewEncoder(w).Encode(map[string]string{"token": "sess-abc"})
	}))
	defer srv.Close()

	if _, err := mintAppSession(context.Background(), srv.URL, "greeter", ""); err != nil {
		t.Fatalf("mintAppSession() error = %v", err)
	}
	if sawAuth {
		t.Fatalf("Authorization header = %q, want none", gotAuth)
	}
}

func TestMintAppSession_SurfacesANonCreatedStatus(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		http.Error(w, "admin auth: missing bearer token", http.StatusUnauthorized)
	}))
	defer srv.Close()

	_, err := mintAppSession(context.Background(), srv.URL, "greeter", "")
	if err == nil {
		t.Fatal("expected an error for a 401 response, got nil")
	}
	if !strings.Contains(err.Error(), "401") || !strings.Contains(err.Error(), "missing bearer token") {
		t.Fatalf("error = %v, want it to mention the status and body", err)
	}
}

func TestMintAppSession_SurfacesAnUnreachableAgent(t *testing.T) {
	_, err := mintAppSession(context.Background(), "http://127.0.0.1:1", "greeter", "")
	if err == nil {
		t.Fatal("expected an error for an unreachable agent, got nil")
	}
}

func TestAppSessionCreateCommand_WritesTheSessionNextToTheAppsStaticFiles(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusCreated)
		_ = json.NewEncoder(w).Encode(map[string]string{"token": "sess-xyz"})
	}))
	defer srv.Close()

	dataDir := t.TempDir()
	db, err := openDataStore(dataDir)
	if err != nil {
		t.Fatalf("openDataStore() error = %v", err)
	}
	appDir := newTestAppDir(t, "io.patchcord.crm")
	if _, err := apps.Install(context.Background(), db, appDir); err != nil {
		t.Fatalf("apps.Install() error = %v", err)
	}
	db.Close()

	cmd := newAppSessionCreateCommand()
	cmd.SetArgs([]string{"io.patchcord.crm", "--data-dir", dataDir, "--base-url", srv.URL})
	cmd.SetContext(context.Background())
	if err := cmd.Execute(); err != nil {
		t.Fatalf("Execute() error = %v", err)
	}

	sessionPath := filepath.Join(appDir, sessionFileName)
	raw, err := os.ReadFile(sessionPath)
	if err != nil {
		t.Fatalf("read %s: %v", sessionPath, err)
	}
	var got struct {
		Token string `json:"token"`
	}
	if err := json.Unmarshal(raw, &got); err != nil {
		t.Fatalf("unmarshal session file: %v", err)
	}
	if got.Token != "sess-xyz" {
		t.Fatalf("token = %q, want %q", got.Token, "sess-xyz")
	}
}

func TestAppSessionCreateCommand_RejectsAnUninstalledApp(t *testing.T) {
	cmd := newAppSessionCreateCommand()
	cmd.SetArgs([]string{"does-not-exist", "--data-dir", t.TempDir()})
	cmd.SetContext(context.Background())

	if err := cmd.Execute(); err == nil {
		t.Fatal("expected an error for an app that was never installed, got nil")
	}
}

func TestAppSessionCreateCommand_WritesToAnExplicitOutputPath(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusCreated)
		_ = json.NewEncoder(w).Encode(map[string]string{"token": "sess-out"})
	}))
	defer srv.Close()

	dataDir := t.TempDir()
	db, err := openDataStore(dataDir)
	if err != nil {
		t.Fatalf("openDataStore() error = %v", err)
	}
	appDir := newTestAppDir(t, "io.patchcord.crm")
	if _, err := apps.Install(context.Background(), db, appDir); err != nil {
		t.Fatalf("apps.Install() error = %v", err)
	}
	db.Close()

	out := filepath.Join(t.TempDir(), "session.json")
	cmd := newAppSessionCreateCommand()
	cmd.SetArgs([]string{"io.patchcord.crm", "--data-dir", dataDir, "--base-url", srv.URL, "--output", out})
	cmd.SetContext(context.Background())
	if err := cmd.Execute(); err != nil {
		t.Fatalf("Execute() error = %v", err)
	}

	if _, err := os.Stat(out); err != nil {
		t.Fatalf("%s missing: %v", out, err)
	}
}
