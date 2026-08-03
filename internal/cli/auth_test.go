package cli

import (
	"bytes"
	"context"
	"strings"
	"testing"
)

func TestNewRootCommand_HasAuthTokenSubcommands(t *testing.T) {
	root := NewRootCommand()

	for _, name := range []string{"create", "list", "revoke"} {
		t.Run(name, func(t *testing.T) {
			cmd, _, err := root.Find([]string{"auth", "token", name})
			if err != nil {
				t.Fatalf("Find(auth token %s) error = %v", name, err)
			}
			if cmd.Name() != name {
				t.Fatalf("found command %q, want %q", cmd.Name(), name)
			}
		})
	}
}

func TestAuthTokenCreateCommand_PrintsThePlaintextOnce(t *testing.T) {
	dataDir := t.TempDir()

	cmd := newAuthTokenCreateCommand()
	var out bytes.Buffer
	cmd.SetOut(&out)
	cmd.SetArgs([]string{"ci", "--data-dir", dataDir})
	cmd.SetContext(context.Background())

	if err := cmd.Execute(); err != nil {
		t.Fatalf("Execute() error = %v", err)
	}

	if !strings.Contains(out.String(), "pcat_") {
		t.Fatalf("output = %q, want it to contain the plaintext token", out.String())
	}
	if !strings.Contains(out.String(), "ci") {
		t.Fatalf("output = %q, want it to contain the token's name", out.String())
	}
}

func TestAuthTokenListCommand(t *testing.T) {
	dataDir := t.TempDir()

	t.Run("reports none when no token has been created", func(t *testing.T) {
		cmd := newAuthTokenListCommand()
		var out bytes.Buffer
		cmd.SetOut(&out)
		cmd.SetArgs([]string{"--data-dir", dataDir})
		cmd.SetContext(context.Background())

		if err := cmd.Execute(); err != nil {
			t.Fatalf("Execute() error = %v", err)
		}
		if !strings.Contains(out.String(), "No admin token created") {
			t.Fatalf("output = %q, want a message saying no token exists", out.String())
		}
	})

	create := newAuthTokenCreateCommand()
	var createOut bytes.Buffer
	create.SetOut(&createOut)
	create.SetArgs([]string{"ci", "--data-dir", dataDir})
	create.SetContext(context.Background())
	if err := create.Execute(); err != nil {
		t.Fatalf("create Execute() error = %v", err)
	}

	t.Run("lists a created token by name, never its plaintext", func(t *testing.T) {
		cmd := newAuthTokenListCommand()
		var out bytes.Buffer
		cmd.SetOut(&out)
		cmd.SetArgs([]string{"--data-dir", dataDir})
		cmd.SetContext(context.Background())

		if err := cmd.Execute(); err != nil {
			t.Fatalf("Execute() error = %v", err)
		}
		if !strings.Contains(out.String(), "ci") {
			t.Fatalf("output = %q, want it to contain the token's name", out.String())
		}
		if strings.Contains(out.String(), "pcat_") {
			t.Fatalf("output = %q, want it to never contain a plaintext token", out.String())
		}
	})
}

func TestAuthTokenRevokeCommand(t *testing.T) {
	dataDir := t.TempDir()

	t.Run("rejects an unknown id", func(t *testing.T) {
		cmd := newAuthTokenRevokeCommand()
		cmd.SetArgs([]string{"does-not-exist", "--data-dir", dataDir})
		cmd.SetContext(context.Background())

		err := cmd.Execute()
		if err == nil {
			t.Fatal("expected an error for an unknown token id, got nil")
		}
		if !strings.Contains(err.Error(), "was not found") {
			t.Fatalf("error = %q, want it to say the token was not found", err.Error())
		}
	})

	create := newAuthTokenCreateCommand()
	var createOut bytes.Buffer
	create.SetOut(&createOut)
	create.SetArgs([]string{"ci", "--data-dir", dataDir})
	create.SetContext(context.Background())
	if err := create.Execute(); err != nil {
		t.Fatalf("create Execute() error = %v", err)
	}

	list := newAuthTokenListCommand()
	var listOut bytes.Buffer
	list.SetOut(&listOut)
	list.SetArgs([]string{"--data-dir", dataDir})
	list.SetContext(context.Background())
	if err := list.Execute(); err != nil {
		t.Fatalf("list Execute() error = %v", err)
	}
	id := strings.Fields(listOut.String())[0]

	t.Run("revokes an existing token", func(t *testing.T) {
		cmd := newAuthTokenRevokeCommand()
		var out bytes.Buffer
		cmd.SetOut(&out)
		cmd.SetArgs([]string{id, "--data-dir", dataDir})
		cmd.SetContext(context.Background())

		if err := cmd.Execute(); err != nil {
			t.Fatalf("Execute() error = %v", err)
		}

		list := newAuthTokenListCommand()
		var out2 bytes.Buffer
		list.SetOut(&out2)
		list.SetArgs([]string{"--data-dir", dataDir})
		list.SetContext(context.Background())
		if err := list.Execute(); err != nil {
			t.Fatalf("list Execute() error = %v", err)
		}
		if !strings.Contains(out2.String(), "No admin token created") {
			t.Fatalf("output after revoke = %q, want no token left", out2.String())
		}
	})
}
