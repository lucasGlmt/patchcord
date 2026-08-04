package plugins

import (
	"context"
	"crypto/ed25519"
	"database/sql"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"runtime"

	"github.com/lucasglmt/patchcord/internal/packaging"
	"github.com/lucasglmt/patchcord/internal/trust"
)

// PackageExtension is the conventional file extension for a plugin package
// produced by Pack (vision document, section 9.1: ".patchcord-plugin").
// Install only distinguishes a package from a raw executable by sniffing
// its content, not this extension — see internal/cli/plugin.go.
const PackageExtension = ".patchcord-plugin"

// Pack archives sourceDir (which must contain a valid manifest.json, vision
// document section 9.1) into w as a gzip-compressed tar stream, plus a
// checksums.json covering it (see internal/packaging.SignedArchive). If key
// is non-nil, the package is also signed — key == nil (no --sign-key)
// produces a package with integrity data but no provenance. The result is
// what InstallPackage (and therefore `patchcord plugin install`) expects.
//
// Only regular files and directories are supported; sourceDir must not
// contain symlinks or other special entries.
func Pack(sourceDir string, key ed25519.PrivateKey, w io.Writer) error {
	if _, err := LoadPackageManifest(sourceDir); err != nil {
		return err
	}

	return packaging.SignedArchive(sourceDir, key, w)
}

// InstallPackage installs a plugin from a .patchcord-plugin archive (Pack's
// output). Its contents are extracted under dataDir/plugins/<id>/<version>
// — a location the agent owns for as long as the plugin stays installed —
// then the executable matching the current platform (runtime.GOOS+"-"+
// runtime.GOARCH) is selected, made executable, and handed to the existing
// Install, which launches it, completes the handshake, and records it in
// the catalog exactly as it does for a raw executable path today.
//
// The package is verified (internal/packaging.Verify) before anything is
// installed: a checksum mismatch or an invalid signature aborts
// unconditionally. requireSignature additionally rejects a package that is
// unsigned, or signed by a key not trusted for its id (internal/trust) —
// when false, InstallPackage still returns the verification outcome so the
// caller can warn about either case instead of failing outright.
//
// It returns an error wrapping ErrInvalidPackageManifest if the packaged
// manifest is malformed, or a plain error if the package declares no
// executable for the current platform or requires a protocol version this
// agent does not support.
func InstallPackage(ctx context.Context, db *sql.DB, dataDir, packagePath string, requireSignature bool) (*CatalogEntry, trust.PolicyResult, error) {
	f, err := os.Open(packagePath)
	if err != nil {
		return nil, trust.PolicyResult{}, fmt.Errorf("open package %q: %w", packagePath, err)
	}
	defer f.Close()

	pluginsDir := filepath.Join(dataDir, "plugins")
	if err := os.MkdirAll(pluginsDir, 0o755); err != nil {
		return nil, trust.PolicyResult{}, fmt.Errorf("create plugins directory %q: %w", pluginsDir, err)
	}

	staging, err := os.MkdirTemp(pluginsDir, ".staging-*")
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

	manifest, err := LoadPackageManifest(staging)
	if err != nil {
		return nil, trust.PolicyResult{}, err
	}
	if manifest.ProtocolVersion > CurrentProtocolVersion {
		return nil, trust.PolicyResult{}, fmt.Errorf("plugin package requires protocol version %d, agent supports up to %d",
			manifest.ProtocolVersion, CurrentProtocolVersion)
	}

	policy, err := trust.CheckPolicy(ctx, db, manifest.ID, outcome, requireSignature)
	if err != nil {
		return nil, policy, err
	}

	platform := runtime.GOOS + "-" + runtime.GOARCH
	relExecutable, ok := manifest.Executables[platform]
	if !ok {
		return nil, policy, fmt.Errorf("plugin package %q has no executable for platform %q", manifest.ID, platform)
	}

	stagedExecutable, err := packaging.SafeJoin(staging, relExecutable)
	if err != nil {
		return nil, policy, fmt.Errorf("executables[%q]: %w", platform, err)
	}
	if _, err := os.Stat(stagedExecutable); err != nil {
		return nil, policy, fmt.Errorf("declared executable %q not found in package: %w", relExecutable, err)
	}
	// The archive is untrusted input: Extract never preserves a header's
	// file mode (internal/packaging.Extract), so the one file this agent
	// will actually run must be made executable explicitly.
	if err := os.Chmod(stagedExecutable, 0o755); err != nil {
		return nil, policy, fmt.Errorf("make executable %q runnable: %w", relExecutable, err)
	}

	target := filepath.Join(pluginsDir, manifest.ID, manifest.Version)
	if err := os.RemoveAll(target); err != nil {
		return nil, policy, fmt.Errorf("clear existing plugin directory %q: %w", target, err)
	}
	if err := os.MkdirAll(filepath.Dir(target), 0o755); err != nil {
		return nil, policy, fmt.Errorf("create plugin directory %q: %w", target, err)
	}
	if err := os.Rename(staging, target); err != nil {
		return nil, policy, fmt.Errorf("move extracted plugin to %q: %w", target, err)
	}

	entry, err := Install(ctx, db, filepath.Join(target, relExecutable))
	if err != nil {
		_ = os.RemoveAll(target)
		return nil, policy, err
	}

	return entry, policy, nil
}
