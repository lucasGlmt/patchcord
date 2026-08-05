package cli

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestNewRootCommand_HasMCPServeSubcommand(t *testing.T) {
	root := NewRootCommand()

	serve, _, err := root.Find([]string{"mcp", "serve"})
	if err != nil {
		t.Fatalf("Find(mcp serve) error = %v", err)
	}
	if serve.Name() != "serve" {
		t.Fatalf("found command %q, want %q", serve.Name(), "serve")
	}

	dataDirFlag := serve.Flags().Lookup("data-dir")
	if dataDirFlag == nil {
		t.Fatal("mcp serve command has no --data-dir flag")
	}
	if dataDirFlag.DefValue != defaultDataDir {
		t.Fatalf("--data-dir default = %q, want %q", dataDirFlag.DefValue, defaultDataDir)
	}
}

// TestMCPServeCommand_FailsFastOnUnusableDataDir exercises the one part of
// `mcp serve` a test can safely call Execute() on: everything up to
// server.Run(ctx, &mcp.StdioTransport{}), which blocks reading the test
// process's own stdin and can't be exercised here without a real MCP
// client on the other end (see internal/mcpserver's in-memory-transport
// tests for that). Pointing --data-dir at a regular file, not a
// directory, makes persistence.Open's os.MkdirAll fail immediately —
// proving the command wires --data-dir/openDataStore correctly before
// ever reaching the transport, the same "force an early failure" approach
// serve_test.go already uses for `serve`.
func TestMCPServeCommand_FailsFastOnUnusableDataDir(t *testing.T) {
	notADir := filepath.Join(t.TempDir(), "not-a-directory")
	if err := os.WriteFile(notADir, []byte("x"), 0o644); err != nil {
		t.Fatalf("write file: %v", err)
	}

	cmd := newMCPServeCommand()
	cmd.SetArgs([]string{"--data-dir", notADir})
	cmd.SetContext(context.Background())

	err := cmd.Execute()
	if err == nil {
		t.Fatal("expected an error for a --data-dir that isn't a directory, got nil")
	}
	if !strings.Contains(err.Error(), "open database") {
		t.Fatalf("error = %q, want it to mention database opening", err.Error())
	}
}
