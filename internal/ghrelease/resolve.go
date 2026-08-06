package ghrelease

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"os"
	"strings"
	"time"
)

// defaultAPIBaseURL is GitHub's public REST API. Overridable via
// Options.APIBaseURL, for tests only.
const defaultAPIBaseURL = "https://api.github.com"

// maxErrorBodySnippet bounds how much of a non-2xx response body is read
// into an error message.
const maxErrorBodySnippet = 2048

// Options configures GitHub API access and the HTTP transport. The zero
// value talks to the real, unauthenticated github.com API with a
// 30s-timeout client — everything here exists to make GitHub fakeable in
// tests and to let a caller supply a token.
type Options struct {
	// APIBaseURL overrides https://api.github.com. Tests only.
	APIBaseURL string

	// Token, sent as "Authorization: Bearer <Token>" on the GitHub API
	// call (never on the asset download), raises GitHub's unauthenticated
	// rate limit (60 req/hr/IP). Optional; never required for a public
	// repository. Not a private-repo mechanism — see ADR-0067.
	Token string

	// HTTPClient overrides the client used for both the API call and the
	// asset download. Tests only; defaults to &http.Client{Timeout: 30 *
	// time.Second}.
	HTTPClient *http.Client
}

// Asset is one release asset as reported by the GitHub API.
type Asset struct {
	Name               string
	BrowserDownloadURL string
	Size               int64
}

// Resolved pins a Ref to one concrete release tag and exactly one release
// asset whose name ends in the assetSuffix given to Resolve.
type Resolved struct {
	Ref   Ref
	Tag   string // always concrete, even when Ref.Tag was ""
	Asset Asset
}

// release is the subset of GitHub's release JSON this package needs.
type release struct {
	TagName string  `json:"tag_name"`
	Assets  []asset `json:"assets"`
}

type asset struct {
	Name               string `json:"name"`
	BrowserDownloadURL string `json:"browser_download_url"`
	Size               int64  `json:"size"`
}

// Resolve calls GitHub's Releases API for ref (releases/latest when
// ref.Tag == "", releases/tags/<ref.Tag> otherwise) and returns the single
// release asset whose name ends in assetSuffix (e.g. plugins.PackageExtension,
// passed in by the caller rather than imported, so this package stays
// usable for other package kinds later without depending on any of them).
//
// Fails with a clear, actionable error if: the repository/release/tag does
// not exist (GitHub 404 — the API cannot distinguish "no such repo" from
// "no such release", so the message covers both plainly); the API returns
// any other non-2xx status (status + response body snippet included, with
// a dedicated message when the response indicates rate limiting); the
// release has zero assets ending in assetSuffix; or it has more than one
// (ambiguous — the caller is told to publish a single one, this pass does
// not support disambiguation).
func Resolve(ctx context.Context, ref Ref, assetSuffix string, opts Options) (Resolved, error) {
	rel, err := fetchRelease(ctx, ref, opts)
	if err != nil {
		return Resolved{}, err
	}

	a, err := pickAsset(rel.Assets, assetSuffix)
	if err != nil {
		return Resolved{}, fmt.Errorf("github.com/%s/%s@%s: %w", ref.Owner, ref.Repo, rel.TagName, err)
	}

	return Resolved{
		Ref: ref,
		Tag: rel.TagName,
		Asset: Asset{
			Name:               a.Name,
			BrowserDownloadURL: a.BrowserDownloadURL,
			Size:               a.Size,
		},
	}, nil
}

// Download streams resolved.Asset's bytes into a new file under destDir
// (created if missing) and returns its path — same shape as
// internal/registry.Fetch. The asset's BrowserDownloadURL, as returned by
// the GitHub API, is fetched directly with a plain GET (redirects followed
// by the default http.Client behavior); this is also what makes the
// download side trivially fakeable in tests without any extra "download
// base URL" option — a test's fake /releases/... handler just points
// BrowserDownloadURL at that same httptest.Server.
func Download(ctx context.Context, resolved Resolved, destDir string, opts Options) (string, error) {
	if err := os.MkdirAll(destDir, 0o755); err != nil {
		return "", fmt.Errorf("create download directory: %w", err)
	}

	req, err := http.NewRequestWithContext(ctx, http.MethodGet, resolved.Asset.BrowserDownloadURL, nil)
	if err != nil {
		return "", fmt.Errorf("build asset download request: %w", err)
	}

	resp, err := httpClient(opts).Do(req)
	if err != nil {
		return "", fmt.Errorf("download %s: %w", resolved.Asset.Name, err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return "", fmt.Errorf("download %s: unexpected status %s", resolved.Asset.Name, resp.Status)
	}

	out, err := os.CreateTemp(destDir, "package-*")
	if err != nil {
		return "", fmt.Errorf("create download file: %w", err)
	}
	defer out.Close()

	if _, err := io.Copy(out, resp.Body); err != nil {
		_ = os.Remove(out.Name())
		return "", fmt.Errorf("download %s: %w", resolved.Asset.Name, err)
	}

	return out.Name(), nil
}

func fetchRelease(ctx context.Context, ref Ref, opts Options) (release, error) {
	endpoint := apiBaseURL(opts) + "/repos/" + url.PathEscape(ref.Owner) + "/" + url.PathEscape(ref.Repo) + "/releases/latest"
	if ref.Tag != "" {
		endpoint = apiBaseURL(opts) + "/repos/" + url.PathEscape(ref.Owner) + "/" + url.PathEscape(ref.Repo) + "/releases/tags/" + url.PathEscape(ref.Tag)
	}

	req, err := http.NewRequestWithContext(ctx, http.MethodGet, endpoint, nil)
	if err != nil {
		return release{}, fmt.Errorf("build GitHub API request: %w", err)
	}
	req.Header.Set("Accept", "application/vnd.github+json")
	req.Header.Set("X-GitHub-Api-Version", "2022-11-28")
	if opts.Token != "" {
		req.Header.Set("Authorization", "Bearer "+opts.Token)
	}

	resp, err := httpClient(opts).Do(req)
	if err != nil {
		return release{}, fmt.Errorf("call GitHub API for github.com/%s/%s: %w", ref.Owner, ref.Repo, err)
	}
	defer resp.Body.Close()

	if resp.StatusCode == http.StatusNotFound {
		return release{}, fmt.Errorf("github.com/%s/%s: repository or release not found (tag=%q) — confirm the repository is public and has a matching release", ref.Owner, ref.Repo, ref.Tag)
	}

	if resp.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(io.LimitReader(resp.Body, maxErrorBodySnippet))
		if isRateLimited(resp) {
			return release{}, fmt.Errorf("github.com/%s/%s: GitHub API rate limit exceeded (status %s) — pass --github-token or set GITHUB_TOKEN to raise the unauthenticated limit", ref.Owner, ref.Repo, resp.Status)
		}
		return release{}, fmt.Errorf("github.com/%s/%s: GitHub API returned %s: %s", ref.Owner, ref.Repo, resp.Status, strings.TrimSpace(string(body)))
	}

	var rel release
	if err := json.NewDecoder(resp.Body).Decode(&rel); err != nil {
		return release{}, fmt.Errorf("decode GitHub API response for github.com/%s/%s: %w", ref.Owner, ref.Repo, err)
	}

	return rel, nil
}

// isRateLimited reports whether resp looks like a GitHub rate-limit
// response: a 403 or 429 with the standard X-RateLimit-Remaining: 0
// header GitHub sets on that response, per its documented rate-limiting
// contract.
func isRateLimited(resp *http.Response) bool {
	if resp.StatusCode != http.StatusForbidden && resp.StatusCode != http.StatusTooManyRequests {
		return false
	}
	return resp.Header.Get("X-RateLimit-Remaining") == "0"
}

func pickAsset(assets []asset, assetSuffix string) (asset, error) {
	var matches []asset
	for _, a := range assets {
		if strings.HasSuffix(a.Name, assetSuffix) {
			matches = append(matches, a)
		}
	}

	switch len(matches) {
	case 0:
		return asset{}, fmt.Errorf("release has no %s asset — plugin authors publish one by running `plugin pack` and attaching the result to the GitHub Release", assetSuffix)
	case 1:
		return matches[0], nil
	default:
		names := make([]string, len(matches))
		for i, a := range matches {
			names[i] = a.Name
		}
		return asset{}, fmt.Errorf("release has %d %s assets (%s) — ambiguous, not supported yet", len(matches), assetSuffix, strings.Join(names, ", "))
	}
}

func httpClient(opts Options) *http.Client {
	if opts.HTTPClient != nil {
		return opts.HTTPClient
	}
	return &http.Client{Timeout: 30 * time.Second}
}

func apiBaseURL(opts Options) string {
	if opts.APIBaseURL != "" {
		return opts.APIBaseURL
	}
	return defaultAPIBaseURL
}
