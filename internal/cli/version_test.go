package cli

import (
	"bytes"
	"strings"
	"testing"

	"github.com/lucasglmt/patchcord/internal/version"
)

func TestVersionCommand_PrintsBuildMetadata(t *testing.T) {
	origVersion, origCommit, origDate := version.Version, version.Commit, version.Date
	t.Cleanup(func() { version.Version, version.Commit, version.Date = origVersion, origCommit, origDate })
	version.Version, version.Commit, version.Date = "0.1.0", "abc1234", "2026-08-05T12:00:00Z"

	root := NewRootCommand()
	out := &bytes.Buffer{}
	root.SetOut(out)
	root.SetArgs([]string{"version"})

	if err := root.Execute(); err != nil {
		t.Fatalf("Execute() error = %v", err)
	}

	want := "0.1.0 (commit abc1234, built 2026-08-05T12:00:00Z)\n"
	if got := out.String(); got != want {
		t.Fatalf("output = %q, want %q", got, want)
	}
}

func TestNewRootCommand_HasVersionFlag(t *testing.T) {
	origVersion := version.Version
	t.Cleanup(func() { version.Version = origVersion })
	version.Version = "0.1.0"

	root := NewRootCommand()
	out := &bytes.Buffer{}
	root.SetOut(out)
	root.SetArgs([]string{"--version"})

	if err := root.Execute(); err != nil {
		t.Fatalf("Execute() error = %v", err)
	}

	if got := out.String(); !strings.Contains(got, "0.1.0") {
		t.Fatalf("--version output = %q, want it to contain %q", got, "0.1.0")
	}
}
