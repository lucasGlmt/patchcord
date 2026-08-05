package mcpserver

import (
	"context"
	"strings"
	"testing"

	"github.com/lucasglmt/patchcord/internal/plugins"
	"github.com/lucasglmt/patchcord/internal/runs"
)

func TestValidateWorkflow(t *testing.T) {
	db := openTestDB(t)

	seedPlugin(t, db, plugins.CatalogEntry{
		PluginID: "io.patchcord.example-text",
		Version:  "1.0.0",
		Actions: []plugins.ActionDescriptor{
			{
				ID:          "text.uppercase@1",
				Description: "Converts a string to upper case.",
				InputSchema: map[string]any{
					"type":       "object",
					"properties": map[string]any{"value": map[string]any{"type": "string"}},
					"required":   []any{"value"},
				},
			},
		},
	})

	t.Run("valid workflow", func(t *testing.T) {
		source := `
schema_version: 1
id: greet
version: 1
trigger:
  type: manual
steps:
  - id: shout
    uses: text.uppercase@1
    with:
      value: hello
`
		_, out, err := validateWorkflow(db)(context.Background(), nil, validateWorkflowIn{Source: source})
		if err != nil {
			t.Fatalf("validateWorkflow() error = %v", err)
		}
		if !out.Valid {
			t.Fatalf("Valid = false, Error = %q, want a valid workflow", out.Error)
		}
		if out.WorkflowID != "greet" || out.Version != 1 || out.StepCount != 1 {
			t.Fatalf("out = %+v, want workflow_id=greet version=1 step_count=1", out)
		}
	})

	t.Run("malformed YAML is reported as data, not a tool error", func(t *testing.T) {
		_, out, err := validateWorkflow(db)(context.Background(), nil, validateWorkflowIn{Source: "not: [valid: yaml"})
		if err != nil {
			t.Fatalf("validateWorkflow() error = %v, want nil (a parse failure is a normal result)", err)
		}
		if out.Valid {
			t.Fatal("Valid = true, want false for malformed YAML")
		}
		if out.Error == "" {
			t.Fatal("Error is empty, want a parse error message")
		}
	})

	t.Run("unknown action is reported as data, not a tool error", func(t *testing.T) {
		source := `
schema_version: 1
id: greet
version: 1
trigger:
  type: manual
steps:
  - id: shout
    uses: text.reverse@1
    with:
      value: hello
`
		_, out, err := validateWorkflow(db)(context.Background(), nil, validateWorkflowIn{Source: source})
		if err != nil {
			t.Fatalf("validateWorkflow() error = %v, want nil", err)
		}
		if out.Valid {
			t.Fatal("Valid = true, want false for an unknown action")
		}
	})

	t.Run("with not matching the action's schema is reported as data, not a tool error", func(t *testing.T) {
		source := `
schema_version: 1
id: greet
version: 1
trigger:
  type: manual
steps:
  - id: shout
    uses: text.uppercase@1
    with:
      value: 42
`
		_, out, err := validateWorkflow(db)(context.Background(), nil, validateWorkflowIn{Source: source})
		if err != nil {
			t.Fatalf("validateWorkflow() error = %v, want nil", err)
		}
		if out.Valid {
			t.Fatal("Valid = true, want false: value=42 doesn't match the declared string type")
		}
		if !strings.Contains(out.Error, "value") {
			t.Fatalf("Error = %q, want it to mention the offending field", out.Error)
		}
	})
}

func TestListWorkflows(t *testing.T) {
	db := openTestDB(t)

	t.Run("empty catalog", func(t *testing.T) {
		_, out, err := listWorkflows(db)(context.Background(), nil, listWorkflowsIn{})
		if err != nil {
			t.Fatalf("listWorkflows() error = %v", err)
		}
		if len(out.Workflows) != 0 {
			t.Fatalf("Workflows = %v, want empty", out.Workflows)
		}
	})

	seedPlugin(t, db, plugins.CatalogEntry{
		PluginID: "io.patchcord.example-text",
		Version:  "1.0.0",
		Actions:  []plugins.ActionDescriptor{{ID: "text.uppercase@1"}},
	})
	knownActions, err := plugins.KnownActions(context.Background(), db)
	if err != nil {
		t.Fatalf("plugins.KnownActions() error = %v", err)
	}
	source := []byte(`
schema_version: 1
id: greet
version: 1
trigger:
  type: manual
steps:
  - id: shout
    uses: text.uppercase@1
    with:
      value: hello
`)
	if _, err := runs.InstallWorkflow(context.Background(), db, source, knownActions); err != nil {
		t.Fatalf("runs.InstallWorkflow() error = %v", err)
	}

	_, out, err := listWorkflows(db)(context.Background(), nil, listWorkflowsIn{})
	if err != nil {
		t.Fatalf("listWorkflows() error = %v", err)
	}
	if len(out.Workflows) != 1 || out.Workflows[0].WorkflowID != "greet" || out.Workflows[0].Version != 1 {
		t.Fatalf("Workflows = %+v, want one greet/1", out.Workflows)
	}
}

func TestGetWorkflowSource(t *testing.T) {
	db := openTestDB(t)
	seedPlugin(t, db, plugins.CatalogEntry{
		PluginID: "io.patchcord.example-text",
		Version:  "1.0.0",
		Actions:  []plugins.ActionDescriptor{{ID: "text.uppercase@1"}},
	})
	knownActions, err := plugins.KnownActions(context.Background(), db)
	if err != nil {
		t.Fatalf("plugins.KnownActions() error = %v", err)
	}
	source := []byte(`
schema_version: 1
id: greet
version: 1
trigger:
  type: manual
steps:
  - id: shout
    uses: text.uppercase@1
    with:
      value: hello
`)
	if _, err := runs.InstallWorkflow(context.Background(), db, source, knownActions); err != nil {
		t.Fatalf("runs.InstallWorkflow() error = %v", err)
	}

	t.Run("latest version (0)", func(t *testing.T) {
		_, out, err := getWorkflowSource(db)(context.Background(), nil, getWorkflowSourceIn{WorkflowID: "greet"})
		if err != nil {
			t.Fatalf("getWorkflowSource() error = %v", err)
		}
		if !strings.Contains(out.Source, "id: greet") {
			t.Fatalf("Source = %q, want it to contain the workflow's YAML", out.Source)
		}
	})

	t.Run("unknown workflow returns an error", func(t *testing.T) {
		_, _, err := getWorkflowSource(db)(context.Background(), nil, getWorkflowSourceIn{WorkflowID: "does-not-exist"})
		if err == nil {
			t.Fatal("expected an error, got nil")
		}
	})
}
