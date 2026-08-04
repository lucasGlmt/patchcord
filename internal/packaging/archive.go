// Package packaging holds the tar.gz archive primitives shared by every
// installable Patchcord package format (.patchcord-app, .patchcord-plugin,
// .patchcord-bundle): archiving a validated source directory, and safely
// extracting an untrusted archive back onto disk. Format-specific concerns
// — manifest parsing, where extracted files end up under dataDir, what
// happens after extraction — stay in each format's own package
// (internal/apps, internal/plugins, internal/bundles).
package packaging

import (
	"archive/tar"
	"compress/gzip"
	"fmt"
	"io"
	"io/fs"
	"os"
	"path/filepath"
	"strings"
)

// Archive walks sourceDir and writes it to w as a gzip-compressed tar
// stream. Only regular files and directories are supported; any other
// entry type (symlink, device, ...) fails the archive rather than
// producing a silently incomplete one.
func Archive(sourceDir string, w io.Writer) error {
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
			return fmt.Errorf("archive: %s: unsupported file type", rel)
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
		return fmt.Errorf("archive %q: %w", sourceDir, walkErr)
	}
	if err := tw.Close(); err != nil {
		return fmt.Errorf("archive %q: %w", sourceDir, err)
	}
	if err := gz.Close(); err != nil {
		return fmt.Errorf("archive %q: %w", sourceDir, err)
	}

	return nil
}

// Extract extracts a gzip-compressed tar stream (as produced by Archive)
// into destDir, which must already exist. Every entry is written with mode
// 0o644 regardless of what the archive's header claims — an archive is
// untrusted input, so its declared file mode (which could set unusual bits
// such as setuid) is never trusted. Callers that need an extracted file to
// be executable (a plugin binary, for instance) must chmod it explicitly
// after Extract returns.
func Extract(r io.Reader, destDir string) error {
	gz, err := gzip.NewReader(r)
	if err != nil {
		return fmt.Errorf("read archive: %w", err)
	}
	defer gz.Close()

	tr := tar.NewReader(gz)
	for {
		header, err := tr.Next()
		if err == io.EOF {
			break
		}
		if err != nil {
			return fmt.Errorf("read archive: %w", err)
		}

		target, err := SafeJoin(destDir, header.Name)
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

// SafeJoin joins destDir and name, rejecting any entry (typically one
// using "../" components, a "zip slip") whose resolved path would escape
// destDir. Archives are untrusted input — see the security review
// requirements in CLAUDE.md (OWASP top 10: path traversal).
func SafeJoin(destDir, name string) (string, error) {
	target := filepath.Join(destDir, filepath.FromSlash(name))
	if target != destDir && !strings.HasPrefix(target, destDir+string(os.PathSeparator)) {
		return "", fmt.Errorf("entry %q escapes destination directory", name)
	}
	return target, nil
}
