package apps

import (
	"archive/tar"
	"compress/gzip"
	"context"
	"database/sql"
	"fmt"
	"io"
	"io/fs"
	"os"
	"path/filepath"
	"strings"
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

	gz := gzip.NewWriter(w)
	tw := tar.NewWriter(gz)

	walkErr := filepath.WalkDir(sourceDir, func(path string, d fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		rel, err := filepath.Rel(sourceDir, path)
		if err != nil {
			return err
		}
		if rel == "." {
			return nil
		}

		info, err := d.Info()
		if err != nil {
			return err
		}
		if !info.Mode().IsRegular() && !d.IsDir() {
			return fmt.Errorf("pack app: %s: unsupported file type", rel)
		}

		header, err := tar.FileInfoHeader(info, "")
		if err != nil {
			return err
		}
		header.Name = filepath.ToSlash(rel)
		if d.IsDir() {
			header.Name += "/"
		}
		if err := tw.WriteHeader(header); err != nil {
			return err
		}
		if d.IsDir() {
			return nil
		}

		f, err := os.Open(path)
		if err != nil {
			return err
		}
		defer f.Close()

		_, err = io.Copy(tw, f)
		return err
	})
	if walkErr != nil {
		return fmt.Errorf("pack app %q: %w", sourceDir, walkErr)
	}
	if err := tw.Close(); err != nil {
		return fmt.Errorf("pack app %q: %w", sourceDir, err)
	}
	if err := gz.Close(); err != nil {
		return fmt.Errorf("pack app %q: %w", sourceDir, err)
	}

	return nil
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

	if err := extractPackage(f, staging); err != nil {
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

// extractPackage extracts a gzip-compressed tar stream (as produced by
// Pack) into destDir, which must already exist.
func extractPackage(r io.Reader, destDir string) error {
	gz, err := gzip.NewReader(r)
	if err != nil {
		return fmt.Errorf("read package: %w", err)
	}
	defer gz.Close()

	tr := tar.NewReader(gz)
	for {
		header, err := tr.Next()
		if err == io.EOF {
			break
		}
		if err != nil {
			return fmt.Errorf("read package: %w", err)
		}

		target, err := safeJoin(destDir, header.Name)
		if err != nil {
			return err
		}

		switch header.Typeflag {
		case tar.TypeDir:
			if err := os.MkdirAll(target, 0o755); err != nil {
				return err
			}
		case tar.TypeReg:
			if err := os.MkdirAll(filepath.Dir(target), 0o755); err != nil {
				return err
			}
			if err := writeExtractedFile(target, tr); err != nil {
				return err
			}
		default:
			return fmt.Errorf("unsupported entry type in %q", header.Name)
		}
	}

	return nil
}

func writeExtractedFile(target string, r io.Reader) error {
	f, err := os.OpenFile(target, os.O_CREATE|os.O_WRONLY|os.O_TRUNC, 0o644)
	if err != nil {
		return err
	}
	defer f.Close()

	_, err = io.Copy(f, r)
	return err
}

// safeJoin joins destDir and name, rejecting any entry (typically one
// using "../" components, a "zip slip") whose resolved path would escape
// destDir. Package archives are untrusted input — see the security review
// requirements in CLAUDE.md section (OWASP top 10: path traversal).
func safeJoin(destDir, name string) (string, error) {
	target := filepath.Join(destDir, filepath.FromSlash(name))
	if target != destDir && !strings.HasPrefix(target, destDir+string(os.PathSeparator)) {
		return "", fmt.Errorf("entry %q escapes destination directory", name)
	}
	return target, nil
}
