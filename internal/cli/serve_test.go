package cli

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestServeCommand_InvalidListenAddressFailsFast(t *testing.T) {
	cmd := newServeCommand()
	cmd.SetArgs([]string{"--listen", "not-a-valid-address", "--data-dir", t.TempDir()})
	cmd.SetContext(context.Background())

	err := cmd.Execute()
	if err == nil {
		t.Fatal("expected an error for an invalid --listen address, got nil")
	}
	if !strings.Contains(err.Error(), "create agent") {
		t.Fatalf("error = %q, want it to mention agent creation", err.Error())
	}
}

// writeServeConfigFile writes a minimal config.yaml declaring listen, so
// the precedence tests below can tell whether it was actually used —
// binding a deliberately invalid address makes NewAgent fail with an error
// that echoes back exactly which address it tried (see runtime.NewAgent's
// "bind listen address %q" wrapping), which is enough to prove which
// source's value actually made it through without ever letting the server
// start listening.
func writeServeConfigFile(t *testing.T, listen string) string {
	t.Helper()
	path := filepath.Join(t.TempDir(), "config.yaml")
	if err := os.WriteFile(path, []byte("listen: "+listen+"\n"), 0o644); err != nil {
		t.Fatalf("write config file: %v", err)
	}
	return path
}

func TestServeCommand_ConfigPrecedence(t *testing.T) {
	t.Run("a config file's listen is used when nothing else sets it", func(t *testing.T) {
		configPath := writeServeConfigFile(t, "file-address")

		cmd := newServeCommand()
		cmd.SetArgs([]string{"--config", configPath, "--data-dir", t.TempDir()})
		cmd.SetContext(context.Background())

		err := cmd.Execute()
		if err == nil || !strings.Contains(err.Error(), `"file-address"`) {
			t.Fatalf("error = %v, want it to mention the config file's listen address", err)
		}
	})

	t.Run("an environment variable overrides the config file", func(t *testing.T) {
		configPath := writeServeConfigFile(t, "file-address")
		t.Setenv("PATCHCORD_LISTEN", "env-address")

		cmd := newServeCommand()
		cmd.SetArgs([]string{"--config", configPath, "--data-dir", t.TempDir()})
		cmd.SetContext(context.Background())

		err := cmd.Execute()
		if err == nil || !strings.Contains(err.Error(), `"env-address"`) {
			t.Fatalf("error = %v, want it to mention the environment variable's listen address", err)
		}
	})

	t.Run("an explicit flag overrides both the environment variable and the config file", func(t *testing.T) {
		configPath := writeServeConfigFile(t, "file-address")
		t.Setenv("PATCHCORD_LISTEN", "env-address")

		cmd := newServeCommand()
		cmd.SetArgs([]string{"--config", configPath, "--listen", "flag-address", "--data-dir", t.TempDir()})
		cmd.SetContext(context.Background())

		err := cmd.Execute()
		if err == nil || !strings.Contains(err.Error(), `"flag-address"`) {
			t.Fatalf("error = %v, want it to mention the flag's listen address", err)
		}
	})

	t.Run("a missing config file fails before even trying to create the agent", func(t *testing.T) {
		cmd := newServeCommand()
		cmd.SetArgs([]string{"--config", filepath.Join(t.TempDir(), "does-not-exist.yaml"), "--data-dir", t.TempDir()})
		cmd.SetContext(context.Background())

		err := cmd.Execute()
		if err == nil || !strings.Contains(err.Error(), "load config file") {
			t.Fatalf("error = %v, want it to mention config file loading", err)
		}
	})
}
