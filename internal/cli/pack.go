package cli

import (
	"errors"
	"fmt"
	"io"
	"os"
)

// packToFile stages an archive written by write into a temp file outside
// any directory write might itself read from, then atomically moves that
// temp file into place at out once write succeeds.
//
// Every `pack` command's default output path (<id>-<version>.<ext> in the
// current directory) and any -o/--output the caller chooses can legally
// end up inside the very directory being packed — e.g. `plugin pack .`
// run from the plugin's own directory, or `plugin pack . -o
// name.patchcord-plugin` with no path prefix. Writing straight into out
// in that case makes the archive's own directory walk see the output
// file mid-write, growing while the walk copies it into the tar stream
// and corrupting the archive ("archive/tar: write too long"). Staging
// outside the source directory first also means a failing write never
// leaves a partial or corrupt file at out — a real risk otherwise, since
// a corrupt local archive can silently end up published (e.g. attached to
// a GitHub Release, see ADR-0067) if a caller doesn't notice pack failed.
func packToFile(out string, write func(w io.Writer) error) error {
	tmp, err := os.CreateTemp("", "patchcord-pack-*")
	if err != nil {
		return fmt.Errorf("create temp file: %w", err)
	}
	tmpPath := tmp.Name()
	defer os.Remove(tmpPath) // no-op once the rename/copy below has succeeded

	if err := write(tmp); err != nil {
		_ = tmp.Close()
		return err
	}
	if err := tmp.Close(); err != nil {
		return fmt.Errorf("close temp file: %w", err)
	}

	if err := os.Rename(tmpPath, out); err != nil {
		// os.TempDir() can be on a different filesystem than out's
		// directory (containers, separate mounts, ...), where Rename
		// fails with a cross-device link error — fall back to a copy.
		if copyErr := copyFile(tmpPath, out); copyErr != nil {
			return errors.Join(fmt.Errorf("rename into %q: %w", out, err), copyErr)
		}
	}

	return nil
}

func copyFile(src, dst string) error {
	in, err := os.Open(src)
	if err != nil {
		return err
	}
	defer in.Close()

	out, err := os.Create(dst)
	if err != nil {
		return err
	}
	defer out.Close()

	if _, err := io.Copy(out, in); err != nil {
		return err
	}
	return out.Close()
}
