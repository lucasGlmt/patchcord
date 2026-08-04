package bundles

import (
	"context"
	"crypto/ed25519"
	"database/sql"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"

	"github.com/lucasglmt/patchcord/internal/apps"
	"github.com/lucasglmt/patchcord/internal/packaging"
	"github.com/lucasglmt/patchcord/internal/plugins"
	"github.com/lucasglmt/patchcord/internal/runs"
	"github.com/lucasglmt/patchcord/internal/trust"
)

// PackageExtension is the conventional file extension for a bundle package
// produced by Pack (vision document, section 9.3).
const PackageExtension = ".patchcord-bundle"

// Pack archives sourceDir (which must contain a valid bundle.yaml, plus
// whatever app/workflow files it references) into w as a gzip-compressed
// tar stream, plus a checksums.json covering it (see
// internal/packaging.SignedArchive). If key is non-nil, the package is
// also signed — signing a bundle covers its embedded app and workflows
// too, so they are never separately verified again on install (see
// installEmbeddedApp). The result is what InstallPackage (and therefore
// `patchcord bundle install`) expects.
func Pack(sourceDir string, key ed25519.PrivateKey, w io.Writer) error {
	if _, err := LoadManifest(sourceDir); err != nil {
		return err
	}

	return packaging.SignedArchive(sourceDir, key, w)
}

// InstallPackage installs a bundle from a .patchcord-bundle archive (Pack's
// output). It orchestrates, in order:
//
//  1. every "id@version" entry in requires_plugins must already be present
//     in the plugin catalog — a bundle never auto-installs its plugin
//     dependencies (that is the registry/update tasks' job, later in phase
//     7);
//  2. the embedded app (if any) is moved to its permanent location under
//     dataDir/apps/<app-id>/<app-version> and installed via apps.Install,
//     the same choreography apps.InstallPackage uses for a standalone
//     .patchcord-app archive;
//  3. each embedded workflow file is installed via runs.InstallWorkflow —
//     workflow definitions need no on-disk home of their own, they live in
//     the workflow_versions table (ADR-0008).
//
// The package is verified (internal/packaging.Verify) before anything is
// installed: a checksum mismatch or an invalid signature aborts
// unconditionally. requireSignature additionally rejects a package that is
// unsigned, or signed by a key not trusted for its id (internal/trust) —
// when false, InstallPackage still returns the verification outcome so the
// caller can warn about either case instead of failing outright. A bundle's
// signature covers its embedded app and workflows too — they are not
// separately re-verified (see installEmbeddedApp).
//
// A failure partway through (e.g. the app installs but a workflow fails
// validation) is not rolled back: this first pass does not implement
// multi-resource transactions across three independently-catalogued
// resource kinds. The returned error names which step failed.
func InstallPackage(ctx context.Context, db *sql.DB, dataDir, packagePath string, knownActions map[string]struct{}, requireSignature bool) (*Bundle, trust.PolicyResult, error) {
	f, err := os.Open(packagePath)
	if err != nil {
		return nil, trust.PolicyResult{}, fmt.Errorf("open package %q: %w", packagePath, err)
	}
	defer f.Close()

	bundlesDir := filepath.Join(dataDir, "bundles")
	if err := os.MkdirAll(bundlesDir, 0o755); err != nil {
		return nil, trust.PolicyResult{}, fmt.Errorf("create bundles directory %q: %w", bundlesDir, err)
	}

	staging, err := os.MkdirTemp(bundlesDir, ".staging-*")
	if err != nil {
		return nil, trust.PolicyResult{}, fmt.Errorf("create staging directory: %w", err)
	}
	defer os.RemoveAll(staging)

	if err := packaging.Extract(f, staging); err != nil {
		return nil, trust.PolicyResult{}, fmt.Errorf("extract package %q: %w", packagePath, err)
	}

	outcome, err := packaging.Verify(staging)
	if err != nil {
		return nil, trust.PolicyResult{}, fmt.Errorf("verify package %q: %w", packagePath, err)
	}

	manifest, err := LoadManifest(staging)
	if err != nil {
		return nil, trust.PolicyResult{}, err
	}
	manifestSource, err := os.ReadFile(filepath.Join(staging, ManifestFileName))
	if err != nil {
		return nil, trust.PolicyResult{}, fmt.Errorf("read %s: %w", ManifestFileName, err)
	}

	policy, err := trust.CheckPolicy(ctx, db, manifest.ID, outcome, requireSignature)
	if err != nil {
		return nil, policy, err
	}

	if err := checkPluginDependencies(ctx, db, manifest.RequiresPlugins); err != nil {
		return nil, policy, err
	}

	if manifest.App != "" {
		if err := installEmbeddedApp(ctx, db, dataDir, staging, manifest.App); err != nil {
			return nil, policy, fmt.Errorf("install bundle %q app: %w", manifest.ID, err)
		}
	}

	for _, relWorkflow := range manifest.Workflows {
		if err := installEmbeddedWorkflow(ctx, db, staging, relWorkflow, knownActions); err != nil {
			return nil, policy, fmt.Errorf("install bundle %q workflow %q: %w", manifest.ID, relWorkflow, err)
		}
	}

	if err := record(ctx, db, manifest.ID, manifest.Version, string(manifestSource)); err != nil {
		return nil, policy, err
	}

	bundle, err := Get(ctx, db, manifest.ID)
	return bundle, policy, err
}

// checkPluginDependencies fails fast, naming the first unmet dependency, if
// any declared "id@version" plugin is not installed at exactly that
// version.
func checkPluginDependencies(ctx context.Context, db *sql.DB, requires []string) error {
	for _, dep := range requires {
		id, version, err := splitPluginDependency(dep)
		if err != nil {
			return err
		}

		entry, err := plugins.Get(ctx, db, id)
		if errors.Is(err, plugins.ErrNotInstalled) {
			return fmt.Errorf("required plugin %q is not installed", dep)
		}
		if err != nil {
			return fmt.Errorf("check required plugin %q: %w", dep, err)
		}
		if entry.Version != version {
			return fmt.Errorf("required plugin %q is not installed (found version %s)", dep, entry.Version)
		}
	}

	return nil
}

// installEmbeddedApp moves the bundle's embedded app subtree out of staging
// to its permanent location under dataDir/apps/<app-id>/<app-version> and
// installs it — the same two-step apps.InstallPackage uses for a
// standalone .patchcord-app archive, applied here to an already-extracted
// directory instead of a fresh archive.
func installEmbeddedApp(ctx context.Context, db *sql.DB, dataDir, staging, relApp string) error {
	appStagingDir, err := packaging.SafeJoin(staging, relApp)
	if err != nil {
		return fmt.Errorf("app: %w", err)
	}

	appManifest, err := apps.LoadManifest(appStagingDir)
	if err != nil {
		return err
	}

	appTarget := filepath.Join(dataDir, "apps", appManifest.ID, appManifest.Version)
	if err := os.RemoveAll(appTarget); err != nil {
		return fmt.Errorf("clear existing app directory %q: %w", appTarget, err)
	}
	if err := os.MkdirAll(filepath.Dir(appTarget), 0o755); err != nil {
		return fmt.Errorf("create app directory %q: %w", appTarget, err)
	}
	if err := os.Rename(appStagingDir, appTarget); err != nil {
		return fmt.Errorf("move extracted app to %q: %w", appTarget, err)
	}

	if _, err := apps.Install(ctx, db, appTarget); err != nil {
		_ = os.RemoveAll(appTarget)
		return err
	}

	return nil
}

// installEmbeddedWorkflow reads one workflow file out of staging and
// installs it exactly as `workflow install` does.
func installEmbeddedWorkflow(ctx context.Context, db *sql.DB, staging, relWorkflow string, knownActions map[string]struct{}) error {
	path, err := packaging.SafeJoin(staging, relWorkflow)
	if err != nil {
		return err
	}

	source, err := os.ReadFile(path)
	if err != nil {
		return fmt.Errorf("read %q: %w", relWorkflow, err)
	}

	_, err = runs.InstallWorkflow(ctx, db, source, knownActions)
	return err
}
