package bundles

import (
	"context"
	"crypto/ed25519"
	"database/sql"
	"errors"
	"fmt"
	"io"
	"io/fs"
	"os"
	"path/filepath"

	"github.com/lucasglmt/patchcord/internal/apps"
	"github.com/lucasglmt/patchcord/internal/packaging"
	"github.com/lucasglmt/patchcord/internal/plugins"
	"github.com/lucasglmt/patchcord/internal/runs"
	"github.com/lucasglmt/patchcord/internal/trust"
	"github.com/lucasglmt/patchcord/internal/workflow"
)

// PackageExtension is the conventional file extension for a bundle package
// produced by Pack (vision document, section 9.3).
const PackageExtension = ".patchcord-bundle"

// Pack archives sourceDir's bundle.yaml plus exactly the app/workflow files
// it references into w as a gzip-compressed tar stream, plus a
// checksums.json covering it (see internal/packaging.SignedArchive). If key
// is non-nil, the package is also signed — signing a bundle covers its
// embedded app and workflows too, so they are never separately verified
// again on install (see installEmbeddedApp). The result is what
// InstallPackage (and therefore `patchcord bundle install`) expects.
//
// Pack never walks sourceDir itself: only the manifest's declared App
// subtree and Workflows files are staged and archived. sourceDir routinely
// holds things bundle.yaml does not declare — a Vite app's node_modules
// (which packaging.Archive would otherwise reject outright the moment it
// hit a symlink such as node_modules/.bin/esbuild), a .git directory,
// editor state — none of which belong in the package.
func Pack(sourceDir string, key ed25519.PrivateKey, w io.Writer) error {
	manifest, err := LoadManifest(sourceDir)
	if err != nil {
		return err
	}

	staging, err := os.MkdirTemp("", "patchcord-bundle-pack-*")
	if err != nil {
		return fmt.Errorf("create staging directory: %w", err)
	}
	defer os.RemoveAll(staging)

	if err := stageBundleContent(sourceDir, staging, manifest); err != nil {
		return fmt.Errorf("pack bundle %q: %w", sourceDir, err)
	}

	return packaging.SignedArchive(staging, key, w)
}

// stageBundleContent copies exactly what bundle.yaml declares — the
// manifest itself, its embedded app subtree (if any), and its embedded
// workflow files — out of sourceDir into staging, preserving their
// relative paths so staging ends up laid out exactly as InstallPackage
// expects.
func stageBundleContent(sourceDir, staging string, manifest *Manifest) error {
	if err := copyFile(filepath.Join(sourceDir, ManifestFileName), filepath.Join(staging, ManifestFileName)); err != nil {
		return fmt.Errorf("stage %s: %w", ManifestFileName, err)
	}

	if manifest.App != "" {
		if err := copyDir(filepath.Join(sourceDir, manifest.App), filepath.Join(staging, manifest.App)); err != nil {
			return fmt.Errorf("stage app %q: %w", manifest.App, err)
		}
	}

	for _, relWorkflow := range manifest.Workflows {
		if err := copyFile(filepath.Join(sourceDir, relWorkflow), filepath.Join(staging, relWorkflow)); err != nil {
			return fmt.Errorf("stage workflow %q: %w", relWorkflow, err)
		}
	}

	return nil
}

// copyFile copies a single regular file from src to dst, creating dst's
// parent directory as needed.
func copyFile(src, dst string) error {
	data, err := os.ReadFile(src)
	if err != nil {
		return err
	}
	if err := os.MkdirAll(filepath.Dir(dst), 0o755); err != nil {
		return err
	}
	return os.WriteFile(dst, data, 0o644)
}

// copyDir recursively copies srcDir's regular files and directories to
// dstDir. Like packaging.Archive, any other entry type (a symlink, most
// commonly — e.g. node_modules/.bin's shims) fails loudly rather than being
// silently skipped or followed.
func copyDir(srcDir, dstDir string) error {
	return filepath.WalkDir(srcDir, func(path string, d fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		rel, err := filepath.Rel(srcDir, path)
		if err != nil {
			return err
		}
		target := filepath.Join(dstDir, rel)

		if d.IsDir() {
			return os.MkdirAll(target, 0o755)
		}

		info, err := d.Info()
		if err != nil {
			return err
		}
		if !info.Mode().IsRegular() {
			return fmt.Errorf("%s: unsupported file type", filepath.ToSlash(rel))
		}

		return copyFile(path, target)
	})
}

// InstallPackage installs a bundle from a .patchcord-bundle archive (Pack's
// output). It orchestrates, in order:
//
//  1. every "id@version" entry in requires_plugins must already be present
//     in the plugin catalog — a bundle never auto-installs its plugin
//     dependencies (see ADR-0044: this stays deferred even once a
//     registry exists);
//  2. the embedded app (if any) is moved to its permanent location under
//     dataDir/apps/<app-id>/<app-version> and installed via
//     apps.InstallOrUpdate — unlike apps.InstallPackage's own standalone
//     .patchcord-app flow (which stays strict, see apps.Install), a
//     bundle install has always been upsert-by-design at the top level
//     (record(), below): re-running `bundle install`/`bundle update` on an
//     already-installed bundle id must succeed and replace its embedded
//     app in place, not fail with apps.ErrAlreadyExists (ADR-0044);
//  3. each embedded workflow file is installed via installWorkflowIfChanged
//     — workflow definitions need no on-disk home of their own, they live
//     in the workflow_versions table (ADR-0008), and re-running install/
//     update with an unchanged workflow is a no-op rather than a rejection
//     (only redeclaring a version with genuinely different content still
//     hits ADR-0008's immutability rule).
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

// InstallDir installs a bundle straight from a source directory instead of
// a packaged .patchcord-bundle archive — no Pack/Extract round trip, no
// checksum, no signature: dir is local, unsigned-by-design source under
// active development. It backs `patchcord bundle dev` the same way
// apps.InstallOrUpdate backs `patchcord app dev`.
//
// Unlike InstallPackage's installEmbeddedApp, the embedded app (if any) is
// installed in place via apps.InstallOrUpdate pointed straight at
// dir/manifest.App — never moved under dataDir/apps — so it ends up served
// live off dir exactly as `app dev` serves an app: rebuilding it (e.g.
// `vite build --watch`) needs no further agent involvement, and no dataDir
// argument is needed here at all.
//
// Each embedded workflow goes through installWorkflowForDev (ADR-0055),
// not the strict installWorkflowIfChanged InstallPackage uses: redeclaring
// an already-installed version with byte-identical content is a silent
// no-op (as before, and for the same reason — `bundle dev --watch`
// reinstalls every embedded workflow on every change under dir, including
// ones a given save never touched), but redeclaring it with genuinely
// different content — edited the workflow's body, forgot to bump
// `version:` — is installed under the next unused version instead of
// rejected, so a save is never a hard failure. The source file is never
// rewritten; ADR-0008 immutability stays intact for InstallPackage
// (`bundle install`/`update`) and `workflow install`, which are unaffected
// by this.
//
// requires_plugins is enforced exactly as InstallPackage enforces it: a
// missing dependency is not installed automatically.
func InstallDir(ctx context.Context, db *sql.DB, dir string, knownActions map[string]struct{}) (*Bundle, error) {
	manifest, err := LoadManifest(dir)
	if err != nil {
		return nil, err
	}

	if err := checkPluginDependencies(ctx, db, manifest.RequiresPlugins); err != nil {
		return nil, err
	}

	if manifest.App != "" {
		if _, err := apps.InstallOrUpdate(ctx, db, filepath.Join(dir, manifest.App)); err != nil {
			return nil, fmt.Errorf("install bundle %q app: %w", manifest.ID, err)
		}
	}

	for _, relWorkflow := range manifest.Workflows {
		path := filepath.Join(dir, relWorkflow)
		source, err := os.ReadFile(path)
		if err != nil {
			return nil, fmt.Errorf("install bundle %q workflow %q: %w", manifest.ID, relWorkflow, err)
		}
		if _, err := installWorkflowForDev(ctx, db, source, knownActions); err != nil {
			return nil, fmt.Errorf("install bundle %q workflow %q: %w", manifest.ID, relWorkflow, err)
		}
	}

	manifestSource, err := os.ReadFile(filepath.Join(dir, ManifestFileName))
	if err != nil {
		return nil, fmt.Errorf("read %s: %w", ManifestFileName, err)
	}
	if err := record(ctx, db, manifest.ID, manifest.Version, string(manifestSource)); err != nil {
		return nil, err
	}

	return Get(ctx, db, manifest.ID)
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
// installs it via apps.InstallOrUpdate, so that re-installing/updating a
// bundle whose app id is already recorded replaces it in place instead of
// failing with apps.ErrAlreadyExists (ADR-0044) — consistent with
// record()'s own always-upsert behavior for the bundle's provenance row.
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

	if _, err := apps.InstallOrUpdate(ctx, db, appTarget); err != nil {
		_ = os.RemoveAll(appTarget)
		return err
	}

	return nil
}

// installEmbeddedWorkflow reads one workflow file out of staging and
// installs it exactly as `workflow install` does (through
// installWorkflowIfChanged, so re-running `bundle install`/`update` on an
// unchanged package is a no-op rather than an ADR-0008 rejection).
func installEmbeddedWorkflow(ctx context.Context, db *sql.DB, staging, relWorkflow string, knownActions map[string]struct{}) error {
	path, err := packaging.SafeJoin(staging, relWorkflow)
	if err != nil {
		return err
	}

	source, err := os.ReadFile(path)
	if err != nil {
		return fmt.Errorf("read %q: %w", relWorkflow, err)
	}

	_, err = installWorkflowIfChanged(ctx, db, source, knownActions)
	return err
}

// installWorkflowIfChanged installs source via runs.InstallWorkflow, except
// when the exact same (id, version) is already installed with
// byte-identical content: then it is a silent no-op instead of hitting
// ADR-0008's immutability rejection.
//
// This is what makes reinstalling a bundle safe to call repeatedly without
// having touched a workflow at all: `bundle dev --watch` reinstalls the
// whole bundle on every change under dir (internal/cli/bundle.go), even
// one that has nothing to do with a given embedded workflow (e.g. the
// embedded app's own build output), so a strict InstallWorkflow call would
// fail on every such reinstall once a workflow version is first recorded.
// runs.InstallWorkflow itself stays strict — `workflow install`, called
// directly by a developer naming a specific file, should keep flagging an
// unbumped version as a likely mistake. Used by InstallPackage
// (`bundle install`/`update`), where the same strictness applies: a
// package a developer chose to publish should keep flagging an unbumped
// version as a likely mistake. InstallDir (`bundle dev`/`patchcord dev`)
// uses installWorkflowForDev below instead.
func installWorkflowIfChanged(ctx context.Context, db *sql.DB, source []byte, knownActions map[string]struct{}) (*workflow.Definition, error) {
	def, err := workflow.Parse(source)
	if err != nil {
		return nil, err
	}

	existing, err := runs.WorkflowSource(ctx, db, def.ID, def.Version)
	if err == nil && existing == string(source) {
		return def, nil
	}
	if err != nil && !errors.Is(err, runs.ErrWorkflowNotFound) {
		return nil, fmt.Errorf("check existing workflow %s version %d: %w", def.ID, def.Version, err)
	}

	return runs.InstallWorkflow(ctx, db, source, knownActions)
}

// installWorkflowForDev is installWorkflowIfChanged's counterpart for
// InstallDir (`bundle dev`/`patchcord dev`, ADR-0055): byte-identical
// content at the declared version is still a silent no-op, and an unseen
// version still installs normally, but content that *differs* from an
// already-recorded version — the "edited the workflow, forgot to bump
// `version:`" case installWorkflowIfChanged rejects under ADR-0008 — is
// installed under the next unused version instead of failing. source is
// never rewritten on disk: only the workflow_versions row's version number
// differs from what the file declares. Every caller that resolves "the"
// workflow without pinning an explicit version (runs.LatestWorkflow,
// WorkflowSource's version 0, a manual/schedule/webhook trigger) already
// picks the highest installed version, so the auto-assigned version is
// transparent downstream.
//
// This is deliberately not used by InstallPackage (`bundle
// install`/`update`) or `workflow install`, which stay on
// installWorkflowIfChanged/runs.InstallWorkflow: installing a package a
// developer chose to publish should keep flagging an unbumped version as a
// likely mistake, with no dev-mode exception.
func installWorkflowForDev(ctx context.Context, db *sql.DB, source []byte, knownActions map[string]struct{}) (*workflow.Definition, error) {
	def, err := workflow.Parse(source)
	if err != nil {
		return nil, err
	}

	existing, err := runs.WorkflowSource(ctx, db, def.ID, def.Version)
	if err == nil && existing == string(source) {
		return def, nil
	}
	if err != nil && !errors.Is(err, runs.ErrWorkflowNotFound) {
		return nil, fmt.Errorf("check existing workflow %s version %d: %w", def.ID, def.Version, err)
	}
	if err != nil { // errors.Is(err, runs.ErrWorkflowNotFound): a normal first install.
		return runs.InstallWorkflow(ctx, db, source, knownActions)
	}

	// err == nil and existing != string(source): the declared version is
	// already recorded with different content — auto-assign the next one.
	next, err := runs.NextWorkflowVersion(ctx, db, def.ID)
	if err != nil {
		return nil, err
	}

	return runs.InstallWorkflowAtVersion(ctx, db, source, next, knownActions)
}
