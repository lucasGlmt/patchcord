package apps

import (
	"context"
	"database/sql"
	"fmt"
	"io"
	"os"
	"path/filepath"

	"github.com/lucasglmt/patchcord/internal/packaging"
)

// PackageExtension is the conventional file extension for an application
// package produced by Pack (vision document, section 9.3: ".patchcord-app").
// Install only inspects whether the path is a file or a directory, not this
// extension — it is a naming convention, not part of the format.
const PackageExtension = ".patchcord-app"

// Pack archives sourceDir (which must contain a valid patchcord-app.yaml,
// vision document section 9.3: "Interface web statique et manifeste de
// permissions") into w as a gzip-compressed tar stream. The result is what
// InstallPackage (and therefore `patchcord app install`) expects.
//
// Only regular files and directories are supported; sourceDir must not
// contain symlinks or other special entries.
func Pack(sourceDir string, w io.Writer) error {
	if _, err := LoadManifest(sourceDir); err != nil {
		return err
	}

	return packaging.Archive(sourceDir, w)
}

// InstallPackage installs an application from a .patchcord-app archive
// (Pack's output). Unlike Install, which serves an application straight
// from wherever its source directory happens to live, a package's contents
// are extracted under dataDir/apps/<id>/<version> — a location the agent
// owns for as long as the application stays installed, so the archive
// itself is free to move or disappear afterwards.
//
// It returns ErrAlreadyExists if an application with the manifest's id is
// already installed, or an error wrapping ErrInvalidManifest if the
// packaged manifest is malformed.
func InstallPackage(ctx context.Context, db *sql.DB, dataDir, packagePath string) (*App, error) {
	f, err := os.Open(packagePath)
	if err != nil {
		return nil, fmt.Errorf("open package %q: %w", packagePath, err)
	}
	defer f.Close()

	appsDir := filepath.Join(dataDir, "apps")
	if err := os.MkdirAll(appsDir, 0o755); err != nil {
		return nil, fmt.Errorf("create apps directory %q: %w", appsDir, err)
	}

	staging, err := os.MkdirTemp(appsDir, ".staging-*")
	if err != nil {
		return nil, fmt.Errorf("create staging directory: %w", err)
	}
	defer os.RemoveAll(staging)

	if err := packaging.Extract(f, staging); err != nil {
		return nil, fmt.Errorf("extract package %q: %w", packagePath, err)
	}

	manifest, err := LoadManifest(staging)
	if err != nil {
		return nil, err
	}

	target := filepath.Join(appsDir, manifest.ID, manifest.Version)
	if err := os.RemoveAll(target); err != nil {
		return nil, fmt.Errorf("clear existing app directory %q: %w", target, err)
	}
	if err := os.MkdirAll(filepath.Dir(target), 0o755); err != nil {
		return nil, fmt.Errorf("create app directory %q: %w", target, err)
	}
	if err := os.Rename(staging, target); err != nil {
		return nil, fmt.Errorf("move extracted app to %q: %w", target, err)
	}

	app, err := Install(ctx, db, target)
	if err != nil {
		_ = os.RemoveAll(target)
		return nil, err
	}

	return app, nil
}
