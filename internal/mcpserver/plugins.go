package mcpserver

import (
	"context"
	"database/sql"
	"fmt"

	"github.com/modelcontextprotocol/go-sdk/mcp"

	"github.com/lucasglmt/patchcord/internal/plugins"
)

// registerPluginTools adds every catalog-discovery tool to server, closing
// each handler over db.
func registerPluginTools(server *mcp.Server, db *sql.DB) {
	mcp.AddTool(server, &mcp.Tool{
		Name:        "list_plugins",
		Description: "Lists every plugin currently installed in this Patchcord agent's catalog.",
	}, listPlugins(db))

	mcp.AddTool(server, &mcp.Tool{
		Name:        "list_actions",
		Description: "Lists every action contributed by installed plugins, optionally filtered to one plugin. Descriptions only — use describe_action for one action's full input/output schema.",
	}, listActions(db))

	mcp.AddTool(server, &mcp.Tool{
		Name:        "describe_action",
		Description: "Returns one action's full description, input/output JSON Schema, and default timeout.",
	}, describeAction(db))

	mcp.AddTool(server, &mcp.Tool{
		Name:        "list_connectors",
		Description: "Lists every connector type contributed by installed plugins. Descriptions only — use describe_connector for one connector's full configuration schema.",
	}, listConnectors(db))

	mcp.AddTool(server, &mcp.Tool{
		Name:        "describe_connector",
		Description: "Returns one connector type's full description and non-secret configuration JSON Schema.",
	}, describeConnector(db))
}

// listPluginsIn takes no arguments.
type listPluginsIn struct{}

type pluginSummary struct {
	ID              string   `json:"id"`
	Version         string   `json:"version"`
	ProtocolVersion uint32   `json:"protocol_version"`
	Permissions     []string `json:"permissions,omitempty"`
	ActionCount     int      `json:"action_count"`
	ConnectorCount  int      `json:"connector_count"`
}

type listPluginsOut struct {
	Plugins []pluginSummary `json:"plugins"`
}

func listPlugins(db *sql.DB) mcp.ToolHandlerFor[listPluginsIn, listPluginsOut] {
	return func(ctx context.Context, _ *mcp.CallToolRequest, _ listPluginsIn) (*mcp.CallToolResult, listPluginsOut, error) {
		entries, err := plugins.List(ctx, db)
		if err != nil {
			return nil, listPluginsOut{}, fmt.Errorf("list_plugins: %w", err)
		}

		out := listPluginsOut{Plugins: make([]pluginSummary, len(entries))}
		for i, entry := range entries {
			out.Plugins[i] = pluginSummary{
				ID:              entry.PluginID,
				Version:         entry.Version,
				ProtocolVersion: entry.ProtocolVersion,
				Permissions:     entry.Permissions,
				ActionCount:     len(entry.Actions),
				ConnectorCount:  len(entry.Connectors),
			}
		}
		return nil, out, nil
	}
}

// listActionsIn optionally filters to one plugin's actions.
type listActionsIn struct {
	PluginID string `json:"plugin_id,omitempty" jsonschema:"Only list actions contributed by this plugin id. Omit to list every action from every installed plugin."`
}

type actionSummary struct {
	PluginID    string `json:"plugin_id"`
	ActionID    string `json:"action_id"`
	Description string `json:"description"`
}

type listActionsOut struct {
	Actions []actionSummary `json:"actions"`
}

func listActions(db *sql.DB) mcp.ToolHandlerFor[listActionsIn, listActionsOut] {
	return func(ctx context.Context, _ *mcp.CallToolRequest, in listActionsIn) (*mcp.CallToolResult, listActionsOut, error) {
		entries, err := plugins.List(ctx, db)
		if err != nil {
			return nil, listActionsOut{}, fmt.Errorf("list_actions: %w", err)
		}

		var out listActionsOut
		for _, entry := range entries {
			if in.PluginID != "" && entry.PluginID != in.PluginID {
				continue
			}
			for _, action := range entry.Actions {
				out.Actions = append(out.Actions, actionSummary{
					PluginID:    entry.PluginID,
					ActionID:    action.ID,
					Description: action.Description,
				})
			}
		}
		return nil, out, nil
	}
}

// describeActionIn names the action to describe.
type describeActionIn struct {
	ActionID string `json:"action_id" jsonschema:"The action's stable, versioned identifier, e.g. \"text.uppercase@1\"."`
}

type describeActionOut struct {
	PluginID              string         `json:"plugin_id"`
	ActionID              string         `json:"action_id"`
	Description           string         `json:"description"`
	InputSchema           map[string]any `json:"input_schema,omitempty"`
	OutputSchema          map[string]any `json:"output_schema,omitempty"`
	DefaultTimeoutSeconds uint32         `json:"default_timeout_seconds,omitempty"`
}

func describeAction(db *sql.DB) mcp.ToolHandlerFor[describeActionIn, describeActionOut] {
	return func(ctx context.Context, _ *mcp.CallToolRequest, in describeActionIn) (*mcp.CallToolResult, describeActionOut, error) {
		action, pluginID, err := plugins.FindAction(ctx, db, in.ActionID)
		if err != nil {
			return nil, describeActionOut{}, fmt.Errorf("describe_action: %w", err)
		}
		return nil, describeActionOut{
			PluginID:              pluginID,
			ActionID:              action.ID,
			Description:           action.Description,
			InputSchema:           action.InputSchema,
			OutputSchema:          action.OutputSchema,
			DefaultTimeoutSeconds: action.DefaultTimeoutSeconds,
		}, nil
	}
}

// listConnectorsIn takes no arguments.
type listConnectorsIn struct{}

type connectorSummary struct {
	PluginID    string `json:"plugin_id"`
	Type        string `json:"type"`
	Description string `json:"description"`
}

type listConnectorsOut struct {
	Connectors []connectorSummary `json:"connectors"`
}

func listConnectors(db *sql.DB) mcp.ToolHandlerFor[listConnectorsIn, listConnectorsOut] {
	return func(ctx context.Context, _ *mcp.CallToolRequest, _ listConnectorsIn) (*mcp.CallToolResult, listConnectorsOut, error) {
		entries, err := plugins.List(ctx, db)
		if err != nil {
			return nil, listConnectorsOut{}, fmt.Errorf("list_connectors: %w", err)
		}

		var out listConnectorsOut
		for _, entry := range entries {
			for _, connector := range entry.Connectors {
				out.Connectors = append(out.Connectors, connectorSummary{
					PluginID:    entry.PluginID,
					Type:        connector.Type,
					Description: connector.Description,
				})
			}
		}
		return nil, out, nil
	}
}

// describeConnectorIn names the connector type to describe.
type describeConnectorIn struct {
	ConnectorType string `json:"connector_type" jsonschema:"The connector type's stable, versioned identifier, e.g. \"postgresql.connection@1\"."`
}

type describeConnectorOut struct {
	PluginID     string         `json:"plugin_id"`
	Type         string         `json:"type"`
	Description  string         `json:"description"`
	ConfigSchema map[string]any `json:"config_schema,omitempty"`
}

func describeConnector(db *sql.DB) mcp.ToolHandlerFor[describeConnectorIn, describeConnectorOut] {
	return func(ctx context.Context, _ *mcp.CallToolRequest, in describeConnectorIn) (*mcp.CallToolResult, describeConnectorOut, error) {
		connector, pluginID, err := plugins.FindConnector(ctx, db, in.ConnectorType)
		if err != nil {
			return nil, describeConnectorOut{}, fmt.Errorf("describe_connector: %w", err)
		}
		return nil, describeConnectorOut{
			PluginID:     pluginID,
			Type:         connector.Type,
			Description:  connector.Description,
			ConfigSchema: connector.ConfigSchema,
		}, nil
	}
}
