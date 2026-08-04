package apps

import (
	"context"
	"crypto/ed25519"
	"database/sql"
	"fmt"
	"io"
	"os"
	"path/filepath"

	"github.com/lucasglmt/patchcord/internal/packaging"
	"github.com/lucasglmt/patchcord/internal/trust"
)

// PackageExtension is the conventional file extension for an application
// package produced by Pack (vision document, section 9.3: ".patchcord-app").
// Install only inspects whether the path is a file or a directory, not this
// extension — it is a naming convention, not part of the format.
const PackageExtension = ".patchcord-app"

// Pack archives sourceDir (which must contain a valid patchcord-app.yaml,
// vision document section 9.3: "Interface web statique et manifeste de
// permissions") into w as a gzip-compressed tar stream, plus a
// checksums.json covering it (see internal/packaging.SignedArchive). If
// key is non-nil, the package is also signed — key == nil (no --sign-key)
// produces a package with integrity data but no provenance. The result is
// what InstallPackage (and therefore `patchcord app install`) expects.
//
// Only regular files and directories are supported; sourceDir must not
// contain symlinks or other special entries.
func Pack(sourceDir string, key ed25519.PrivateKey, w io.Writer) error {
	if _, err := LoadManifest(sourceDir); err != nil {
		return err
	}

	return packaging.SignedArchive(sourceDir, key, w)
}

// InstallPackage installs an application from a .patchcord-app archive
// (Pack's output). Unlike Install, which serves an application straight
// from wherever its source directory happens to live, a package's contents
// are extracted under dataDir/apps/<id>/<version> — a location the agent
// owns for as long as the application stays installed, so the archive
// itself is free to move or disappear afterwards.
//
// The package is verified (internal/packaging.Verify) before anything is
// installed: a checksum mismatch or an invalid signature aborts
// unconditionally. requireSignature additionally rejects a package that is
// unsigned, or signed by a key not trusted for its id (internal/trust) —
// when false, InstallPackage still returns the verification outcome so the
// caller can warn about either case instead of failing outright.
//
// It returns ErrAlreadyExists if an application with the manifest's id is
// already installed, or an error wrapping ErrInvalidManifest if the
// packaged manifest is malformed.
func InstallPackage(ctx context.Context, db *sql.DB, dataDir, packagePath string, requireSignature bool) (*App, trust.PolicyResult, error) {
	f, err := os.Open(packagePath)
	if err != nil {
		return nil, trust.PolicyResult{}, fmt.Errorf("open package %q: %w", packagePath, err)
	}
	defer f.Close()

	appsDir := filepath.Join(dataDir, "apps")
	if err := os.MkdirAll(appsDir, 0o755); err != nil {
		return nil, trust.PolicyResult{}, fmt.Errorf("create apps directory %q: %w", appsDir, err)
	}

	staging, err := os.MkdirTemp(appsDir, ".staging-*")
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

	policy, err := trust.CheckPolicy(ctx, db, manifest.ID, outcome, requireSignature)
	if err != nil {
		return nil, policy, err
	}

	target := filepath.Join(appsDir, manifest.ID, manifest.Version)
	if err := os.RemoveAll(target); err != nil {
		return nil, policy, fmt.Errorf("clear existing app directory %q: %w", target, err)
	}
	if err := os.MkdirAll(filepath.Dir(target), 0o755); err != nil {
		return nil, policy, fmt.Errorf("create app directory %q: %w", target, err)
	}
	if err := os.Rename(staging, target); err != nil {
		return nil, policy, fmt.Errorf("move extracted app to %q: %w", target, err)
	}

	app, err := Install(ctx, db, target)
	if err != nil {
		_ = os.RemoveAll(target)
		return nil, policy, err
	}

	return app, policy, nil
}
