package cli

import (
	"bytes"
	"crypto/rand"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"testing"

	"github.com/lucasglmt/patchcord/internal/packaging"
)

func TestPackToFile_WritesBytesAndRenamesIntoPlace(t *testing.T) {
	out := filepath.Join(t.TempDir(), "package.bin")

	if err := packToFile(out, func(w io.Writer) error {
		_, err := w.Write([]byte("archive bytes"))
		return err
	}); err != nil {
		t.Fatalf("packToFile() error = %v", err)
	}

	got, err := os.ReadFile(out)
	if err != nil {
		t.Fatalf("read out: %v", err)
	}
	if string(got) != "archive bytes" {
		t.Fatalf("out contents = %q, want %q", got, "archive bytes")
	}
}

func TestPackToFile_WriteFailure_LeavesNoFileAtOut(t *testing.T) {
	out := filepath.Join(t.TempDir(), "package.bin")

	err := packToFile(out, func(w io.Writer) error {
		return errors.New("boom")
	})
	if err == nil || err.Error() != "boom" {
		t.Fatalf("packToFile() error = %v, want %q", err, "boom")
	}

	if _, err := os.Stat(out); !os.IsNotExist(err) {
		t.Fatalf("Stat(out) error = %v, want a not-exist error — a failed write must never leave a partial file behind", err)
	}
}

// TestPackToFile_OutputPathInsideArchivedDirectory_DoesNotCorruptArchive is
// a regression test: writing an archive straight into an output path that
// lives inside the very directory being archived used to corrupt the
// archive, because the directory walk backing the write would see the
// output file growing mid-walk as it was written, and copy that growth
// into the tar stream — overflowing the size a tar header had already
// declared for that entry ("archive/tar: write too long"). This hit real
// users running `plugin pack .` (or any pack command's default output
// path, <id>-<version>.<ext> in the current directory) from inside the
// directory being packed.
//
// Enough incompressible padding is included, sorted before the output
// file's name, to force gzip to actually flush bytes to disk before the
// walk reaches the output entry — required to reproduce the original
// failure deterministically; a small archive never trips it, since
// nothing has hit disk yet by the time the walk gets there.
func TestPackToFile_OutputPathInsideArchivedDirectory_DoesNotCorruptArchive(t *testing.T) {
	sourceDir := t.TempDir()

	padding := make([]byte, 200_000)
	for i := 0; i < 50; i++ {
		if _, err := rand.Read(padding); err != nil {
			t.Fatalf("generate padding: %v", err)
		}
		name := filepath.Join(sourceDir, fmt.Sprintf("padding-%02d.bin", i))
		if err := os.WriteFile(name, padding, 0o644); err != nil {
			t.Fatalf("write padding file: %v", err)
		}
	}

	out := filepath.Join(sourceDir, "zzz-package.tar.gz")
	if err := packToFile(out, func(w io.Writer) error {
		return packaging.Archive(sourceDir, w)
	}); err != nil {
		t.Fatalf("packToFile() error = %v, want the output path living inside the archived directory to work", err)
	}

	body, err := os.ReadFile(out)
	if err != nil {
		t.Fatalf("read out: %v", err)
	}

	dest := t.TempDir()
	if err := packaging.Extract(bytes.NewReader(body), dest); err != nil {
		t.Fatalf("extract packed archive: %v (archive was corrupted)", err)
	}
	if _, err := os.Stat(filepath.Join(dest, "zzz-package.tar.gz")); !os.IsNotExist(err) {
		t.Fatalf("extracted archive contains its own output file — it must never include itself")
	}
}
