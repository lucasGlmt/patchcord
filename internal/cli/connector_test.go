package cli

import (
	"bytes"
	"context"
	"strings"
	"testing"
)

func TestNewRootCommand_HasConnectorSubcommands(t *testing.T) {
	root := NewRootCommand()

	for _, name := range []string{"create", "list", "inspect", "remove"} {
		t.Run(name, func(t *testing.T) {
			cmd, _, err := root.Find([]string{"connector", name})
			if err != nil {
				t.Fatalf("Find(connector %s) error = %v", name, err)
			}
			if cmd.Name() != name {
				t.Fatalf("found command %q, want %q", cmd.Name(), name)
			}
		})
	}
}

func TestConnectorCreateCommand_RejectsAnInvalidSecretReference(t *testing.T) {
	cmd := newConnectorCreateCommand()
	cmd.SetArgs([]string{
		"my_api", "--type", "http.request@1",
		"--secret", "api_key=not-a-valid-reference",
		"--data-dir", t.TempDir(),
	})
	cmd.SetContext(context.Background())

	err := cmd.Execute()
	if err == nil {
		t.Fatal("expected an error for an invalid secret reference, got nil")
	}
	if !strings.Contains(err.Error(), "type:key") {
		t.Fatalf("error = %q, want it to explain the expected type:key format", err.Error())
	}
}

func TestConnectorListCommand_EmptyCatalog(t *testing.T) {
	cmd := newConnectorListCommand()
	cmd.SetArgs([]string{"--data-dir", t.TempDir()})
	cmd.SetContext(context.Background())

	var out bytes.Buffer
	cmd.SetOut(&out)

	if err := cmd.Execute(); err != nil {
		t.Fatalf("Execute() error = %v", err)
	}
	if !strings.Contains(out.String(), "No connector created.") {
		t.Fatalf("output = %q, want it to mention an empty catalog", out.String())
	}
}

func TestConnectorInspectCommand_UnknownConnector(t *testing.T) {
	cmd := newConnectorInspectCommand()
	cmd.SetArgs([]string{"does-not-exist", "--data-dir", t.TempDir()})
	cmd.SetContext(context.Background())

	err := cmd.Execute()
	if err == nil {
		t.Fatal("expected an error for an unknown connector id, got nil")
	}
	if !strings.Contains(err.Error(), "not found") {
		t.Fatalf("error = %q, want it to mention the connector was not found", err.Error())
	}
}

func TestConnectorRemoveCommand_UnknownConnector(t *testing.T) {
	cmd := newConnectorRemoveCommand()
	cmd.SetArgs([]string{"does-not-exist", "--data-dir", t.TempDir()})
	cmd.SetContext(context.Background())

	err := cmd.Execute()
	if err == nil {
		t.Fatal("expected an error for an unknown connector id, got nil")
	}
	if !strings.Contains(err.Error(), "not found") {
		t.Fatalf("error = %q, want it to mention the connector was not found", err.Error())
	}
}

// TestConnectorCommands_FullLifecycle exercises create, list, inspect and
// remove in sequence, exactly as a user would type them on the command
// line — including a secret reference to an environment variable that
// isn't set, to prove inspect reports it as unresolved without failing.
func TestConnectorCommands_FullLifecycle(t *testing.T) {
	dataDir := t.TempDir()

	create := newConnectorCreateCommand()
	create.SetArgs([]string{
		"my_api", "--type", "http.request@1",
		"--config", "base_url=https://api.example.com",
		"--secret", "api_key=env:PATCHCORD_CLI_TEST_SECRET",
		"--data-dir", dataDir,
	})
	create.SetContext(context.Background())
	var createOut bytes.Buffer
	create.SetOut(&createOut)
	if err := create.Execute(); err != nil {
		t.Fatalf("connector create error = %v", err)
	}
	if !strings.Contains(createOut.String(), "my_api") {
		t.Fatalf("create output = %q, want it to mention the created connector id", createOut.String())
	}

	list := newConnectorListCommand()
	list.SetArgs([]string{"--data-dir", dataDir})
	list.SetContext(context.Background())
	var listOut bytes.Buffer
	list.SetOut(&listOut)
	if err := list.Execute(); err != nil {
		t.Fatalf("connector list error = %v", err)
	}
	if !strings.Contains(listOut.String(), "my_api") {
		t.Fatalf("list output = %q, want it to mention the connector", listOut.String())
	}

	inspect := newConnectorInspectCommand()
	inspect.SetArgs([]string{"my_api", "--data-dir", dataDir})
	inspect.SetContext(context.Background())
	var inspectOut bytes.Buffer
	inspect.SetOut(&inspectOut)
	if err := inspect.Execute(); err != nil {
		t.Fatalf("connector inspect error = %v", err)
	}
	if !strings.Contains(inspectOut.String(), "base_url: https://api.example.com") {
		t.Fatalf("inspect output = %q, want it to show the config", inspectOut.String())
	}
	if !strings.Contains(inspectOut.String(), "NOT resolved") {
		t.Fatalf("inspect output = %q, want it to report the unset secret as not resolved", inspectOut.String())
	}

	// Re-running inspect with the environment variable set must report it
	// resolved, and must never print the actual secret value.
	t.Setenv("PATCHCORD_CLI_TEST_SECRET", "s3cr3t-value")
	inspectResolved := newConnectorInspectCommand()
	inspectResolved.SetArgs([]string{"my_api", "--data-dir", dataDir})
	inspectResolved.SetContext(context.Background())
	var inspectResolvedOut bytes.Buffer
	inspectResolved.SetOut(&inspectResolvedOut)
	if err := inspectResolved.Execute(); err != nil {
		t.Fatalf("connector inspect (resolved) error = %v", err)
	}
	if !strings.Contains(inspectResolvedOut.String(), "api_key: env:PATCHCORD_CLI_TEST_SECRET (resolved)") {
		t.Fatalf("inspect output = %q, want it to report the secret as resolved", inspectResolvedOut.String())
	}
	if strings.Contains(inspectResolvedOut.String(), "s3cr3t-value") {
		t.Fatal("inspect output must never contain the resolved secret value")
	}

	createDuplicate := newConnectorCreateCommand()
	createDuplicate.SetArgs([]string{"my_api", "--type", "http.request@1", "--data-dir", dataDir})
	createDuplicate.SetContext(context.Background())
	if err := createDuplicate.Execute(); err == nil {
		t.Fatal("expected an error creating a connector with a duplicate id, got nil")
	}

	remove := newConnectorRemoveCommand()
	remove.SetArgs([]string{"my_api", "--data-dir", dataDir})
	remove.SetContext(context.Background())
	if err := remove.Execute(); err != nil {
		t.Fatalf("connector remove error = %v", err)
	}

	inspectAgain := newConnectorInspectCommand()
	inspectAgain.SetArgs([]string{"my_api", "--data-dir", dataDir})
	inspectAgain.SetContext(context.Background())
	if err := inspectAgain.Execute(); err == nil {
		t.Fatal("expected connector inspect to fail after remove, got nil error")
	}
}
