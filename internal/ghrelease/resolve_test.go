package ghrelease

import (
	"context"
	"fmt"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestParseRef(t *testing.T) {
	tests := []struct {
		name    string
		arg     string
		wantOK  bool
		wantRef Ref
	}{
		{"bare owner and repo", "github.com/owner/repo", true, Ref{Owner: "owner", Repo: "repo"}},
		{"pinned semver tag", "github.com/owner/repo@v1.2.0", true, Ref{Owner: "owner", Repo: "repo", Tag: "v1.2.0"}},
		{"tag containing a slash", "github.com/owner/repo@some/weird-tag", true, Ref{Owner: "owner", Repo: "repo", Tag: "some/weird-tag"}},
		{"wrong host", "gitlab.com/owner/repo", false, Ref{}},
		{"missing repo segment", "github.com/owner", false, Ref{}},
		{"extra path segment", "github.com/owner/repo/extra", false, Ref{}},
		{"local path", "./local/path", false, Ref{}},
		{"empty string", "", false, Ref{}},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			ref, ok := ParseRef(tt.arg)
			if ok != tt.wantOK {
				t.Fatalf("ParseRef(%q) ok = %v, want %v", tt.arg, ok, tt.wantOK)
			}
			if ok && ref != tt.wantRef {
				t.Fatalf("ParseRef(%q) = %+v, want %+v", tt.arg, ref, tt.wantRef)
			}
		})
	}
}

// releaseServer builds an httptest.Server serving a single GitHub release
// at both releases/latest and releases/tags/<tag_name>, plus the assets'
// browser_download_url pointed back at the same server, so a test never
// touches the real network.
func releaseServer(t *testing.T, tagName string, assets []asset, assetBodies map[string]string) *httptest.Server {
	t.Helper()

	var mux http.ServeMux
	mux.HandleFunc("/repos/owner/repo/releases/latest", func(w http.ResponseWriter, r *http.Request) {
		writeRelease(w, tagName, assets)
	})
	mux.HandleFunc("/repos/owner/repo/releases/tags/"+tagName, func(w http.ResponseWriter, r *http.Request) {
		writeRelease(w, tagName, assets)
	})
	for name, body := range assetBodies {
		body := body
		mux.HandleFunc("/download/"+name, func(w http.ResponseWriter, r *http.Request) {
			_, _ = w.Write([]byte(body))
		})
	}

	return httptest.NewServer(&mux)
}

func writeRelease(w http.ResponseWriter, tagName string, assets []asset) {
	w.Header().Set("Content-Type", "application/json")
	fmt.Fprint(w, `{"tag_name":`+quote(tagName)+`,"assets":[`)
	for i, a := range assets {
		if i > 0 {
			fmt.Fprint(w, ",")
		}
		fmt.Fprintf(w, `{"name":%s,"browser_download_url":%s,"size":%d}`, quote(a.Name), quote(a.BrowserDownloadURL), a.Size)
	}
	fmt.Fprint(w, `]}`)
}

func quote(s string) string {
	return `"` + strings.ReplaceAll(s, `"`, `\"`) + `"`
}

func TestResolve_LatestRelease_ReturnsTagAndMatchingAsset(t *testing.T) {
	server := releaseServer(t, "v1.2.0", []asset{
		{Name: "source.tar.gz"},
		{Name: "plugin-1.2.0.patchcord-plugin"},
	}, nil)
	defer server.Close()

	resolved, err := Resolve(context.Background(), Ref{Owner: "owner", Repo: "repo"}, ".patchcord-plugin", Options{APIBaseURL: server.URL})
	if err != nil {
		t.Fatalf("Resolve() error = %v", err)
	}
	if resolved.Tag != "v1.2.0" {
		t.Fatalf("Tag = %q, want %q", resolved.Tag, "v1.2.0")
	}
	if resolved.Asset.Name != "plugin-1.2.0.patchcord-plugin" {
		t.Fatalf("Asset.Name = %q, want %q", resolved.Asset.Name, "plugin-1.2.0.patchcord-plugin")
	}
}

func TestResolve_PinnedTag_HitsTagsEndpoint(t *testing.T) {
	server := releaseServer(t, "v0.9.0", []asset{
		{Name: "plugin-0.9.0.patchcord-plugin"},
	}, nil)
	defer server.Close()

	resolved, err := Resolve(context.Background(), Ref{Owner: "owner", Repo: "repo", Tag: "v0.9.0"}, ".patchcord-plugin", Options{APIBaseURL: server.URL})
	if err != nil {
		t.Fatalf("Resolve() error = %v", err)
	}
	if resolved.Tag != "v0.9.0" {
		t.Fatalf("Tag = %q, want %q", resolved.Tag, "v0.9.0")
	}
}

func TestResolve_NoMatchingAsset_ReturnsClearError(t *testing.T) {
	server := releaseServer(t, "v1.0.0", []asset{{Name: "source.tar.gz"}}, nil)
	defer server.Close()

	_, err := Resolve(context.Background(), Ref{Owner: "owner", Repo: "repo"}, ".patchcord-plugin", Options{APIBaseURL: server.URL})
	if err == nil {
		t.Fatal("expected an error for a release with no matching asset, got nil")
	}
	if !strings.Contains(err.Error(), "no .patchcord-plugin asset") {
		t.Fatalf("error = %q, want it to mention the missing asset", err.Error())
	}
	if !strings.Contains(err.Error(), "plugin pack") {
		t.Fatalf("error = %q, want it to tell the author how to publish one", err.Error())
	}
}

func TestResolve_MultipleMatchingAssets_ReturnsClearError(t *testing.T) {
	server := releaseServer(t, "v1.0.0", []asset{
		{Name: "linux.patchcord-plugin"},
		{Name: "windows.patchcord-plugin"},
	}, nil)
	defer server.Close()

	_, err := Resolve(context.Background(), Ref{Owner: "owner", Repo: "repo"}, ".patchcord-plugin", Options{APIBaseURL: server.URL})
	if err == nil {
		t.Fatal("expected an error for a release with ambiguous assets, got nil")
	}
	if !strings.Contains(err.Error(), "linux.patchcord-plugin") || !strings.Contains(err.Error(), "windows.patchcord-plugin") {
		t.Fatalf("error = %q, want it to name both ambiguous assets", err.Error())
	}
}

func TestResolve_RepositoryOrReleaseNotFound_Returns404Error(t *testing.T) {
	var mux http.ServeMux
	mux.HandleFunc("/", func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusNotFound)
	})
	server := httptest.NewServer(&mux)
	defer server.Close()

	_, err := Resolve(context.Background(), Ref{Owner: "owner", Repo: "repo"}, ".patchcord-plugin", Options{APIBaseURL: server.URL})
	if err == nil {
		t.Fatal("expected an error for a 404, got nil")
	}
	if !strings.Contains(err.Error(), "not found") {
		t.Fatalf("error = %q, want it to mention \"not found\"", err.Error())
	}
}

func TestResolve_NonOKStatus_IncludesStatusAndBodyInError(t *testing.T) {
	var mux http.ServeMux
	mux.HandleFunc("/", func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusInternalServerError)
		_, _ = w.Write([]byte("something broke"))
	})
	server := httptest.NewServer(&mux)
	defer server.Close()

	_, err := Resolve(context.Background(), Ref{Owner: "owner", Repo: "repo"}, ".patchcord-plugin", Options{APIBaseURL: server.URL})
	if err == nil {
		t.Fatal("expected an error for a 500, got nil")
	}
	if !strings.Contains(err.Error(), "500") || !strings.Contains(err.Error(), "something broke") {
		t.Fatalf("error = %q, want it to include the status and body", err.Error())
	}
}

func TestResolve_RateLimited_SuggestsGitHubToken(t *testing.T) {
	var mux http.ServeMux
	mux.HandleFunc("/", func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("X-RateLimit-Remaining", "0")
		w.WriteHeader(http.StatusForbidden)
		_, _ = w.Write([]byte("API rate limit exceeded"))
	})
	server := httptest.NewServer(&mux)
	defer server.Close()

	_, err := Resolve(context.Background(), Ref{Owner: "owner", Repo: "repo"}, ".patchcord-plugin", Options{APIBaseURL: server.URL})
	if err == nil {
		t.Fatal("expected an error for a rate-limited response, got nil")
	}
	if !strings.Contains(err.Error(), "--github-token") || !strings.Contains(err.Error(), "GITHUB_TOKEN") {
		t.Fatalf("error = %q, want it to suggest --github-token/GITHUB_TOKEN", err.Error())
	}
}

func TestResolve_SendsAuthorizationHeaderWhenTokenConfigured(t *testing.T) {
	var gotAuth string
	var mux http.ServeMux
	mux.HandleFunc("/", func(w http.ResponseWriter, r *http.Request) {
		gotAuth = r.Header.Get("Authorization")
		writeRelease(w, "v1.0.0", []asset{{Name: "plugin.patchcord-plugin"}})
	})
	server := httptest.NewServer(&mux)
	defer server.Close()

	if _, err := Resolve(context.Background(), Ref{Owner: "owner", Repo: "repo"}, ".patchcord-plugin", Options{APIBaseURL: server.URL, Token: "test-token"}); err != nil {
		t.Fatalf("Resolve() error = %v", err)
	}
	if gotAuth != "Bearer test-token" {
		t.Fatalf("Authorization header = %q, want %q", gotAuth, "Bearer test-token")
	}
}

func TestResolve_NoAuthorizationHeaderWhenTokenUnset(t *testing.T) {
	var gotAuth string
	seen := false
	var mux http.ServeMux
	mux.HandleFunc("/", func(w http.ResponseWriter, r *http.Request) {
		gotAuth = r.Header.Get("Authorization")
		seen = true
		writeRelease(w, "v1.0.0", []asset{{Name: "plugin.patchcord-plugin"}})
	})
	server := httptest.NewServer(&mux)
	defer server.Close()

	if _, err := Resolve(context.Background(), Ref{Owner: "owner", Repo: "repo"}, ".patchcord-plugin", Options{APIBaseURL: server.URL}); err != nil {
		t.Fatalf("Resolve() error = %v", err)
	}
	if !seen {
		t.Fatal("handler was never called")
	}
	if gotAuth != "" {
		t.Fatalf("Authorization header = %q, want empty", gotAuth)
	}
}

func TestResolve_SendsGitHubAPIVersionHeaders(t *testing.T) {
	var gotAccept, gotVersion string
	var mux http.ServeMux
	mux.HandleFunc("/", func(w http.ResponseWriter, r *http.Request) {
		gotAccept = r.Header.Get("Accept")
		gotVersion = r.Header.Get("X-GitHub-Api-Version")
		writeRelease(w, "v1.0.0", []asset{{Name: "plugin.patchcord-plugin"}})
	})
	server := httptest.NewServer(&mux)
	defer server.Close()

	if _, err := Resolve(context.Background(), Ref{Owner: "owner", Repo: "repo"}, ".patchcord-plugin", Options{APIBaseURL: server.URL}); err != nil {
		t.Fatalf("Resolve() error = %v", err)
	}
	if gotAccept != "application/vnd.github+json" {
		t.Fatalf("Accept header = %q, want %q", gotAccept, "application/vnd.github+json")
	}
	if gotVersion != "2022-11-28" {
		t.Fatalf("X-GitHub-Api-Version header = %q, want %q", gotVersion, "2022-11-28")
	}
}

func TestDownload_WritesExactAssetBytesIntoDestDir(t *testing.T) {
	const body = "fake .patchcord-plugin archive bytes"
	var mux http.ServeMux
	mux.HandleFunc("/download/plugin.patchcord-plugin", func(w http.ResponseWriter, r *http.Request) {
		_, _ = w.Write([]byte(body))
	})
	server := httptest.NewServer(&mux)
	defer server.Close()

	resolved := Resolved{
		Ref: Ref{Owner: "owner", Repo: "repo"},
		Tag: "v1.0.0",
		Asset: Asset{
			Name:               "plugin.patchcord-plugin",
			BrowserDownloadURL: server.URL + "/download/plugin.patchcord-plugin",
		},
	}

	destDir := t.TempDir()
	path, err := Download(context.Background(), resolved, destDir, Options{})
	if err != nil {
		t.Fatalf("Download() error = %v", err)
	}
	if filepath.Dir(path) != destDir {
		t.Fatalf("Download() path = %q, want it under %q", path, destDir)
	}

	got, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("read downloaded file: %v", err)
	}
	if string(got) != body {
		t.Fatalf("downloaded bytes = %q, want %q", got, body)
	}
}

func TestDownload_NonOKStatus_ReturnsError(t *testing.T) {
	var mux http.ServeMux
	mux.HandleFunc("/download/plugin.patchcord-plugin", func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusNotFound)
	})
	server := httptest.NewServer(&mux)
	defer server.Close()

	resolved := Resolved{
		Asset: Asset{
			Name:               "plugin.patchcord-plugin",
			BrowserDownloadURL: server.URL + "/download/plugin.patchcord-plugin",
		},
	}

	if _, err := Download(context.Background(), resolved, t.TempDir(), Options{}); err == nil {
		t.Fatal("expected an error for a 404 asset download, got nil")
	}
}
