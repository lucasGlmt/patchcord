package mcpserver

import (
	"context"
	"errors"
	"testing"

	"github.com/lucasglmt/patchcord/internal/plugins"
)

func TestListPlugins(t *testing.T) {
	db := openTestDB(t)

	t.Run("empty catalog", func(t *testing.T) {
		_, out, err := listPlugins(db)(context.Background(), nil, listPluginsIn{})
		if err != nil {
			t.Fatalf("listPlugins() error = %v", err)
		}
		if len(out.Plugins) != 0 {
			t.Fatalf("Plugins = %v, want empty", out.Plugins)
		}
	})

	seedPlugin(t, db, plugins.CatalogEntry{
		PluginID: "io.patchcord.example-text",
		Version:  "1.0.0",
		Actions: []plugins.ActionDescriptor{
			{ID: "text.uppercase@1", Description: "Converts a string to upper case."},
		},
		Connectors: []plugins.ConnectorDescriptor{
			{Type: "demo.connection@1", Description: "A demo connector."},
		},
		Permissions: []string{"network.outbound"},
	})

	_, out, err := listPlugins(db)(context.Background(), nil, listPluginsIn{})
	if err != nil {
		t.Fatalf("listPlugins() error = %v", err)
	}
	if len(out.Plugins) != 1 {
		t.Fatalf("len(Plugins) = %d, want 1", len(out.Plugins))
	}
	got := out.Plugins[0]
	if got.ID != "io.patchcord.example-text" || got.Version != "1.0.0" {
		t.Fatalf("Plugins[0] = %+v, want id/version io.patchcord.example-text/1.0.0", got)
	}
	if got.ActionCount != 1 || got.ConnectorCount != 1 {
		t.Fatalf("Plugins[0] counts = %+v, want 1/1", got)
	}
}

func TestListActions(t *testing.T) {
	db := openTestDB(t)
	seedPlugin(t, db, plugins.CatalogEntry{
		PluginID: "io.patchcord.example-text",
		Version:  "1.0.0",
		Actions: []plugins.ActionDescriptor{
			{ID: "text.uppercase@1", Description: "Converts a string to upper case."},
			{ID: "text.lowercase@1", Description: "Converts a string to lower case."},
		},
	})
	seedPlugin(t, db, plugins.CatalogEntry{
		PluginID: "io.patchcord.example-json",
		Version:  "1.0.0",
		Actions: []plugins.ActionDescriptor{
			{ID: "json.parse@1", Description: "Parses a JSON-encoded string."},
		},
	})

	t.Run("no filter lists every action", func(t *testing.T) {
		_, out, err := listActions(db)(context.Background(), nil, listActionsIn{})
		if err != nil {
			t.Fatalf("listActions() error = %v", err)
		}
		if len(out.Actions) != 3 {
			t.Fatalf("len(Actions) = %d, want 3", len(out.Actions))
		}
	})

	t.Run("filtered by plugin_id", func(t *testing.T) {
		_, out, err := listActions(db)(context.Background(), nil, listActionsIn{PluginID: "io.patchcord.example-text"})
		if err != nil {
			t.Fatalf("listActions() error = %v", err)
		}
		if len(out.Actions) != 2 {
			t.Fatalf("len(Actions) = %d, want 2", len(out.Actions))
		}
		for _, a := range out.Actions {
			if a.PluginID != "io.patchcord.example-text" {
				t.Fatalf("Actions contains %+v, want only io.patchcord.example-text", a)
			}
		}
	})

	t.Run("unknown plugin_id returns no actions, not an error", func(t *testing.T) {
		_, out, err := listActions(db)(context.Background(), nil, listActionsIn{PluginID: "io.patchcord.unknown"})
		if err != nil {
			t.Fatalf("listActions() error = %v", err)
		}
		if len(out.Actions) != 0 {
			t.Fatalf("Actions = %v, want empty", out.Actions)
		}
	})
}

func TestDescribeAction(t *testing.T) {
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
				DefaultTimeoutSeconds: 30,
			},
		},
	})

	t.Run("known action", func(t *testing.T) {
		_, out, err := describeAction(db)(context.Background(), nil, describeActionIn{ActionID: "text.uppercase@1"})
		if err != nil {
			t.Fatalf("describeAction() error = %v", err)
		}
		if out.PluginID != "io.patchcord.example-text" {
			t.Fatalf("PluginID = %q, want io.patchcord.example-text", out.PluginID)
		}
		if out.Description != "Converts a string to upper case." {
			t.Fatalf("Description = %q, want the seeded description", out.Description)
		}
		if out.InputSchema["type"] != "object" {
			t.Fatalf("InputSchema = %v, want the seeded schema to round-trip", out.InputSchema)
		}
		if out.DefaultTimeoutSeconds != 30 {
			t.Fatalf("DefaultTimeoutSeconds = %d, want 30", out.DefaultTimeoutSeconds)
		}
	})

	t.Run("unknown action returns an error", func(t *testing.T) {
		_, _, err := describeAction(db)(context.Background(), nil, describeActionIn{ActionID: "text.reverse@1"})
		if !errors.Is(err, plugins.ErrActionNotFound) {
			t.Fatalf("describeAction() error = %v, want ErrActionNotFound", err)
		}
	})
}

func TestListConnectors(t *testing.T) {
	db := openTestDB(t)
	seedPlugin(t, db, plugins.CatalogEntry{
		PluginID: "io.patchcord.example-postgresql",
		Version:  "1.0.0",
		Connectors: []plugins.ConnectorDescriptor{
			{Type: "postgresql.connection@1", Description: "A PostgreSQL server."},
		},
	})

	_, out, err := listConnectors(db)(context.Background(), nil, listConnectorsIn{})
	if err != nil {
		t.Fatalf("listConnectors() error = %v", err)
	}
	if len(out.Connectors) != 1 || out.Connectors[0].Type != "postgresql.connection@1" {
		t.Fatalf("Connectors = %+v, want one postgresql.connection@1", out.Connectors)
	}
}

func TestDescribeConnector(t *testing.T) {
	db := openTestDB(t)
	seedPlugin(t, db, plugins.CatalogEntry{
		PluginID: "io.patchcord.example-postgresql",
		Version:  "1.0.0",
		Connectors: []plugins.ConnectorDescriptor{
			{
				Type:        "postgresql.connection@1",
				Description: "A PostgreSQL server, with an optional password secret.",
				ConfigSchema: map[string]any{
					"type":       "object",
					"properties": map[string]any{"host": map[string]any{"type": "string"}},
					"required":   []any{"host"},
				},
			},
		},
	})

	t.Run("known connector", func(t *testing.T) {
		_, out, err := describeConnector(db)(context.Background(), nil, describeConnectorIn{ConnectorType: "postgresql.connection@1"})
		if err != nil {
			t.Fatalf("describeConnector() error = %v", err)
		}
		if out.PluginID != "io.patchcord.example-postgresql" {
			t.Fatalf("PluginID = %q, want io.patchcord.example-postgresql", out.PluginID)
		}
		if out.ConfigSchema["type"] != "object" {
			t.Fatalf("ConfigSchema = %v, want the seeded schema to round-trip", out.ConfigSchema)
		}
	})

	t.Run("unknown connector returns an error", func(t *testing.T) {
		_, _, err := describeConnector(db)(context.Background(), nil, describeConnectorIn{ConnectorType: "mysql.connection@1"})
		if !errors.Is(err, plugins.ErrConnectorNotFound) {
			t.Fatalf("describeConnector() error = %v, want ErrConnectorNotFound", err)
		}
	})
}
