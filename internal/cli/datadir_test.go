package cli

import (
	"bytes"
	"context"
	"os"
	"path/filepath"
	"testing"

	"github.com/spf13/cobra"
)

// newDataDirTestCommand builds a minimal cobra.Command carrying only a
// --data-dir flag, parses args against it, and returns the flag's final
// value alongside the command itself — enough for resolveDataDir's callers
// (cmd.Flags().Changed("data-dir")) to see exactly what a real command
// would see after cobra's own flag parsing.
func newDataDirTestCommand(t *testing.T, args []string) (*cobra.Command, string) {
	t.Helper()

	var dataDir string
	cmd := &cobra.Command{Use: "test", RunE: func(*cobra.Command, []string) error { return nil }}
	cmd.Flags().StringVar(&dataDir, "data-dir", defaultDataDir, "")
	cmd.SetArgs(args)
	cmd.SetContext(context.Background())
	if err := cmd.Execute(); err != nil {
		t.Fatalf("Execute() error = %v", err)
	}
	return cmd, dataDir
}

func TestResolveDataDir(t *testing.T) {
	t.Run("falls back to the built-in default when neither flag nor env is set", func(t *testing.T) {
		cmd, dataDir := newDataDirTestCommand(t, nil)

		if got := resolveDataDir(cmd, dataDir); got != defaultDataDir {
			t.Fatalf("resolveDataDir() = %q, want the built-in default %q", got, defaultDataDir)
		}
	})

	t.Run("PATCHCORD_DATA_DIR overrides the default when --data-dir is not passed", func(t *testing.T) {
		t.Setenv("PATCHCORD_DATA_DIR", "/env/data")
		cmd, dataDir := newDataDirTestCommand(t, nil)

		if got := resolveDataDir(cmd, dataDir); got != "/env/data" {
			t.Fatalf("resolveDataDir() = %q, want the environment variable's value", got)
		}
	})

	t.Run("an explicit --data-dir overrides PATCHCORD_DATA_DIR", func(t *testing.T) {
		t.Setenv("PATCHCORD_DATA_DIR", "/env/data")
		cmd, dataDir := newDataDirTestCommand(t, []string{"--data-dir", "/flag/data"})

		if got := resolveDataDir(cmd, dataDir); got != "/flag/data" {
			t.Fatalf("resolveDataDir() = %q, want the flag's value", got)
		}
	})

	t.Run("an unset environment variable leaves the default untouched", func(t *testing.T) {
		cmd, dataDir := newDataDirTestCommand(t, []string{"--data-dir", defaultDataDir})

		if got := resolveDataDir(cmd, dataDir); got != defaultDataDir {
			t.Fatalf("resolveDataDir() = %q, want the built-in default %q", got, defaultDataDir)
		}
	})
}

// TestDataDirCommands_HonorEnvironmentVariable proves the widened
// precedence (ADR-0049) end to end for one representative command in every
// family that takes --data-dir, not just resolveDataDir in isolation:
// PATCHCORD_DATA_DIR alone, with no --data-dir flag at all, must be enough
// to point the command at the right database.
func TestDataDirCommands_HonorEnvironmentVariable(t *testing.T) {
	envDir := t.TempDir()
	t.Setenv("PATCHCORD_DATA_DIR", envDir)

	cmd := newPluginListCommand()
	cmd.SetArgs(nil)
	cmd.SetContext(context.Background())

	var out bytes.Buffer
	cmd.SetOut(&out)

	if err := cmd.Execute(); err != nil {
		t.Fatalf("Execute() error = %v", err)
	}

	if _, err := os.Stat(filepath.Join(envDir, "patchcord.db")); err != nil {
		t.Fatalf("expected the database to be created under PATCHCORD_DATA_DIR (%s): %v", envDir, err)
	}
}
