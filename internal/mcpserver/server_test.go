package mcpserver

import (
	"context"
	"database/sql"
	"slices"
	"testing"

	"github.com/modelcontextprotocol/go-sdk/mcp"

	"github.com/lucasglmt/patchcord/internal/plugins"
)

// connectInMemory builds a server via New(db), connects it to a fresh
// client over mcp.NewInMemoryTransports() — no subprocess, no real stdio
// pipe — and returns the connected client session. This exercises the
// real MCP protocol (JSON-RPC framing, argument validation against each
// tool's inferred schema, error mapping) end to end, the same principle
// CLAUDE.md §5 already requires of the plugin protocol ("mock the
// transport rather than depend on external process") applied to this one.
func connectInMemory(t *testing.T, db *sql.DB) *mcp.ClientSession {
	t.Helper()

	server := New(db)
	client := mcp.NewClient(&mcp.Implementation{Name: "test-client", Version: "0.0.0"}, nil)

	serverTransport, clientTransport := mcp.NewInMemoryTransports()

	ctx := context.Background()
	if _, err := server.Connect(ctx, serverTransport, nil); err != nil {
		t.Fatalf("server.Connect() error = %v", err)
	}

	session, err := client.Connect(ctx, clientTransport, nil)
	if err != nil {
		t.Fatalf("client.Connect() error = %v", err)
	}
	t.Cleanup(func() { _ = session.Close() })

	return session
}

func TestServer_ListsAllTenTools(t *testing.T) {
	db := openTestDB(t)
	session := connectInMemory(t, db)

	result, err := session.ListTools(context.Background(), nil)
	if err != nil {
		t.Fatalf("ListTools() error = %v", err)
	}

	var names []string
	for _, tool := range result.Tools {
		names = append(names, tool.Name)
	}

	want := []string{
		"list_plugins", "list_actions", "describe_action",
		"list_connectors", "describe_connector",
		"validate_workflow", "list_workflows", "get_workflow_source",
		"scaffold_app", "scaffold_bundle",
	}
	for _, name := range want {
		if !slices.Contains(names, name) {
			t.Fatalf("ListTools() = %v, want it to contain %q", names, name)
		}
	}
	if len(names) != len(want) {
		t.Fatalf("len(ListTools()) = %d, want %d (got %v)", len(names), len(want), names)
	}
}

func TestServer_CallTool_DescribeAction_RealRoundTrip(t *testing.T) {
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
	session := connectInMemory(t, db)

	res, err := session.CallTool(context.Background(), &mcp.CallToolParams{
		Name:      "describe_action",
		Arguments: map[string]any{"action_id": "text.uppercase@1"},
	})
	if err != nil {
		t.Fatalf("CallTool() error = %v", err)
	}
	if res.IsError {
		t.Fatalf("CallTool() IsError = true, content = %+v", res.Content)
	}

	out, ok := res.StructuredContent.(map[string]any)
	if !ok {
		t.Fatalf("StructuredContent = %#v (%T), want a map", res.StructuredContent, res.StructuredContent)
	}
	if out["description"] != "Converts a string to upper case." {
		t.Fatalf("StructuredContent[description] = %v, want the seeded description", out["description"])
	}
}

func TestServer_CallTool_UnknownAction_IsAToolError(t *testing.T) {
	db := openTestDB(t)
	session := connectInMemory(t, db)

	res, err := session.CallTool(context.Background(), &mcp.CallToolParams{
		Name:      "describe_action",
		Arguments: map[string]any{"action_id": "text.reverse@1"},
	})
	// A tool-level failure is reported in-band (IsError), never as a
	// protocol-level error the SDK surfaces as err — see plugins.go's
	// describeAction, which returns a plain wrapped error for exactly
	// this reason.
	if err != nil {
		t.Fatalf("CallTool() error = %v, want nil (a not-found action is a tool error, not a protocol error)", err)
	}
	if !res.IsError {
		t.Fatal("IsError = false, want true for an unknown action id")
	}
}

func TestServer_CallTool_ValidateWorkflow_InvalidDraftIsNotAToolError(t *testing.T) {
	db := openTestDB(t)
	session := connectInMemory(t, db)

	res, err := session.CallTool(context.Background(), &mcp.CallToolParams{
		Name:      "validate_workflow",
		Arguments: map[string]any{"source": "not: [valid: yaml"},
	})
	if err != nil {
		t.Fatalf("CallTool() error = %v", err)
	}
	// Unlike describe_action's not-found case above, an invalid workflow
	// draft is this tool's normal, successful result — IsError must stay
	// false, with the failure reported inside the structured "valid"/
	// "error" fields instead (see workflows.go's registerWorkflowTools doc
	// comment for why).
	if res.IsError {
		t.Fatalf("IsError = true, want false: an invalid draft is validate_workflow's normal result, content = %+v", res.Content)
	}

	out, ok := res.StructuredContent.(map[string]any)
	if !ok {
		t.Fatalf("StructuredContent = %#v (%T), want a map", res.StructuredContent, res.StructuredContent)
	}
	if valid, _ := out["valid"].(bool); valid {
		t.Fatal("StructuredContent[valid] = true, want false for malformed YAML")
	}
}
