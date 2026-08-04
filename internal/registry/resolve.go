package registry

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"os"
	"path"
	"strings"

	"github.com/lucasglmt/patchcord/internal/packaging"
)

// ErrUnknownVersion is returned by Resolve when the registry that lists a
// package id does not list the requested version.
var ErrUnknownVersion = errors.New("unknown package version")

// Resolved is a package entry found in one configured registry, with
// enough information for Fetch to retrieve its file.
type Resolved struct {
	RegistryName     string
	RegistryLocation string
	ID               string
	Kind             string
	Version          string

	relPath string
}

// Resolve looks up id (at version, or the registry's declared "latest" if
// version is empty) across every configured registry, in the order List
// returns them (added_at, oldest first).
//
// The first registry whose index lists id wins, even if a later registry
// also has it — resolution never mixes sources for one id. Three distinct
// outcomes are not equivalent, though:
//
//   - a registry whose index cannot be read or parsed at all fails Resolve
//     immediately, naming that registry: a broken registry is never
//     silently skipped in favor of a working one further down the list,
//     since that would mask a real configuration mistake;
//   - a registry that is read successfully but simply does not list id is
//     not an error — Resolve moves on to the next configured registry;
//   - once id is found in some registry, that registry is the chosen
//     source: if it does not list the requested version, Resolve returns
//     ErrUnknownVersion immediately rather than searching other registries
//     for that version.
func Resolve(ctx context.Context, db *sql.DB, id, version string) (Resolved, error) {
	registries, err := List(ctx, db)
	if err != nil {
		return Resolved{}, fmt.Errorf("resolve %q: %w", id, err)
	}
	if len(registries) == 0 {
		return Resolved{}, fmt.Errorf("resolve %q: %w (no registry configured — run `patchcord registry add`)", id, ErrNotFound)
	}

	for _, reg := range registries {
		idx, err := loadIndex(ctx, reg.Location)
		if err != nil {
			return Resolved{}, fmt.Errorf("read registry %q (%s): %w", reg.Name, reg.Location, err)
		}

		entry, ok := idx.Packages[id]
		if !ok {
			continue
		}

		wantVersion := version
		if wantVersion == "" {
			wantVersion = entry.Latest
		}

		relPath, ok := entry.Versions[wantVersion]
		if !ok {
			return Resolved{}, fmt.Errorf("registry %q: %w: %q has no version %q", reg.Name, ErrUnknownVersion, id, wantVersion)
		}

		return Resolved{
			RegistryName:     reg.Name,
			RegistryLocation: reg.Location,
			ID:               id,
			Kind:             entry.Kind,
			Version:          wantVersion,
			relPath:          relPath,
		}, nil
	}

	return Resolved{}, fmt.Errorf("%w: %q in any configured registry", ErrNotFound, id)
}

// Fetch downloads/copies the package resolved by Resolve into a new file
// under destDir (created if it does not exist yet) and returns its path.
// The caller owns the result and its parent directory — the same
// os.MkdirTemp + defer os.RemoveAll staging pattern internal/bundles and
// internal/apps already use for a package file before InstallPackage.
func Fetch(ctx context.Context, resolved Resolved, destDir string) (string, error) {
	if err := os.MkdirAll(destDir, 0o755); err != nil {
		return "", fmt.Errorf("fetch %q: create destination directory: %w", resolved.ID, err)
	}

	src, err := openPackageFile(ctx, resolved)
	if err != nil {
		return "", fmt.Errorf("fetch %q: %w", resolved.ID, err)
	}
	defer src.Close()

	out, err := os.CreateTemp(destDir, "package-*")
	if err != nil {
		return "", fmt.Errorf("fetch %q: %w", resolved.ID, err)
	}
	defer out.Close()

	if _, err := io.Copy(out, src); err != nil {
		return "", fmt.Errorf("fetch %q: %w", resolved.ID, err)
	}

	return out.Name(), nil
}

func isHTTPLocation(location string) bool {
	return strings.HasPrefix(location, "http://") || strings.HasPrefix(location, "https://")
}

func loadIndex(ctx context.Context, location string) (Index, error) {
	data, err := readLocation(ctx, location, IndexFileName)
	if err != nil {
		return Index{}, err
	}
	return decodeIndex(data)
}

func openPackageFile(ctx context.Context, resolved Resolved) (io.ReadCloser, error) {
	if isHTTPLocation(resolved.RegistryLocation) {
		if err := validateURLRelPath(resolved.relPath); err != nil {
			return nil, err
		}
		return openHTTP(ctx, resolved.RegistryLocation, resolved.relPath)
	}

	target, err := packaging.SafeJoin(resolved.RegistryLocation, resolved.relPath)
	if err != nil {
		return nil, err
	}
	return os.Open(target)
}

// readLocation reads relPath relative to location, dispatching on an
// http(s):// prefix — the only unambiguous "sniff" the two supported
// schemes allow; a plain location is always treated as a local directory.
func readLocation(ctx context.Context, location, relPath string) ([]byte, error) {
	if isHTTPLocation(location) {
		rc, err := openHTTP(ctx, location, relPath)
		if err != nil {
			return nil, err
		}
		defer rc.Close()
		return io.ReadAll(rc)
	}

	target, err := packaging.SafeJoin(location, relPath)
	if err != nil {
		return nil, err
	}
	return os.ReadFile(target)
}

func openHTTP(ctx context.Context, baseURL, relPath string) (io.ReadCloser, error) {
	target, err := url.JoinPath(baseURL, relPath)
	if err != nil {
		return nil, fmt.Errorf("build URL for %q: %w", relPath, err)
	}

	req, err := http.NewRequestWithContext(ctx, http.MethodGet, target, nil)
	if err != nil {
		return nil, fmt.Errorf("build request for %q: %w", target, err)
	}

	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return nil, fmt.Errorf("fetch %q: %w", target, err)
	}
	if resp.StatusCode != http.StatusOK {
		resp.Body.Close()
		return nil, fmt.Errorf("fetch %q: unexpected status %s", target, resp.Status)
	}

	return resp.Body, nil
}

// validateURLRelPath rejects a relative path that would escape the
// registry's base URL (e.g. "../../etc/passwd" in a malicious or
// corrupted index.json) — the URL-path equivalent of packaging.SafeJoin,
// using "path" (always forward-slash) rather than "filepath" since this
// checks a URL path component, not a filesystem path.
func validateURLRelPath(relPath string) error {
	if relPath == "" || path.IsAbs(relPath) || strings.Contains(relPath, "..") {
		return fmt.Errorf("invalid package path %q in registry index", relPath)
	}
	return nil
}
