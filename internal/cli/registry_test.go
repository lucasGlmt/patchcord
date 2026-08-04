package cli

import (
	"bytes"
	"context"
	"strings"
	"testing"
)

func TestNewRootCommand_HasRegistrySubcommands(t *testing.T) {
	root := NewRootCommand()

	for _, name := range []string{"add", "list", "remove"} {
		t.Run(name, func(t *testing.T) {
			cmd, _, err := root.Find([]string{"registry", name})
			if err != nil {
				t.Fatalf("Find(registry %s) error = %v", name, err)
			}
			if cmd.Name() != name {
				t.Fatalf("found command %q, want %q", cmd.Name(), name)
			}
		})
	}
}

func TestRegistryCommands_AddListRemove(t *testing.T) {
	dataDir := t.TempDir()

	list := newRegistryListCommand()
	list.SetArgs([]string{"--data-dir", dataDir})
	list.SetContext(context.Background())
	var emptyOut bytes.Buffer
	list.SetOut(&emptyOut)
	if err := list.Execute(); err != nil {
		t.Fatalf("registry list error = %v", err)
	}
	if !strings.Contains(emptyOut.String(), "No registry configured.") {
		t.Fatalf("list output = %q, want the empty-state message", emptyOut.String())
	}

	add := newRegistryAddCommand()
	add.SetArgs([]string{"local", "/srv/registry", "--data-dir", dataDir})
	add.SetContext(context.Background())
	var addOut bytes.Buffer
	add.SetOut(&addOut)
	if err := add.Execute(); err != nil {
		t.Fatalf("registry add error = %v", err)
	}
	if !strings.Contains(addOut.String(), "local") || !strings.Contains(addOut.String(), "/srv/registry") {
		t.Fatalf("add output = %q, want it to mention the name and location", addOut.String())
	}

	list2 := newRegistryListCommand()
	list2.SetArgs([]string{"--data-dir", dataDir})
	list2.SetContext(context.Background())
	var listOut bytes.Buffer
	list2.SetOut(&listOut)
	if err := list2.Execute(); err != nil {
		t.Fatalf("registry list error = %v", err)
	}
	if !strings.Contains(listOut.String(), "local") || !strings.Contains(listOut.String(), "/srv/registry") {
		t.Fatalf("list output = %q, want it to mention the configured registry", listOut.String())
	}

	remove := newRegistryRemoveCommand()
	remove.SetArgs([]string{"local", "--data-dir", dataDir})
	remove.SetContext(context.Background())
	var removeOut bytes.Buffer
	remove.SetOut(&removeOut)
	if err := remove.Execute(); err != nil {
		t.Fatalf("registry remove error = %v", err)
	}
	if !strings.Contains(removeOut.String(), "local") {
		t.Fatalf("remove output = %q, want it to mention the removed name", removeOut.String())
	}

	removeAgain := newRegistryRemoveCommand()
	removeAgain.SetArgs([]string{"local", "--data-dir", dataDir})
	removeAgain.SetContext(context.Background())
	if err := removeAgain.Execute(); err == nil {
		t.Fatal("expected an error removing an already-removed registry, got nil")
	}
}
