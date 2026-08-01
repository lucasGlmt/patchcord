package cli

import (
	"context"
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
