package mcpserver

import (
	"context"
	"database/sql"
	"fmt"
	"time"

	"github.com/modelcontextprotocol/go-sdk/mcp"

	"github.com/lucasglmt/patchcord/internal/plugins"
	"github.com/lucasglmt/patchcord/internal/runs"
	"github.com/lucasglmt/patchcord/internal/workflow"
)

// registerWorkflowTools adds validate_workflow and the two installed-
// workflow read tools to server, closing each handler over db.
func registerWorkflowTools(server *mcp.Server, db *sql.DB) {
	mcp.AddTool(server, &mcp.Tool{
		Name: "validate_workflow",
		Description: "Parses and validates a workflow YAML draft against the live catalog of installed plugins: every step's `uses` must be a known action id, and every step's `with` must satisfy that action's declared input schema (ADR-0063). " +
			"Reports validity as data, not as a tool failure — a rejected draft is this tool's normal, successful result, meant to be read and acted on.",
	}, validateWorkflow(db))

	mcp.AddTool(server, &mcp.Tool{
		Name:        "list_workflows",
		Description: "Lists every installed workflow version.",
	}, listWorkflows(db))

	mcp.AddTool(server, &mcp.Tool{
		Name:        "get_workflow_source",
		Description: "Returns one installed workflow version's raw YAML source.",
	}, getWorkflowSource(db))
}

type validateWorkflowIn struct {
	Source string `json:"source" jsonschema:"The workflow's full YAML source text, exactly as it would be installed."`
}

type validateWorkflowOut struct {
	Valid      bool   `json:"valid"`
	Error      string `json:"error,omitempty"`
	WorkflowID string `json:"workflow_id,omitempty"`
	Version    int    `json:"version,omitempty"`
	StepCount  int    `json:"step_count,omitempty"`
}

func validateWorkflow(db *sql.DB) mcp.ToolHandlerFor[validateWorkflowIn, validateWorkflowOut] {
	return func(ctx context.Context, _ *mcp.CallToolRequest, in validateWorkflowIn) (*mcp.CallToolResult, validateWorkflowOut, error) {
		def, err := workflow.Parse([]byte(in.Source))
		if err != nil {
			// A syntax error is exactly what this tool exists to report —
			// never a Go error here, see registerWorkflowTools' doc comment.
			return nil, validateWorkflowOut{Valid: false, Error: err.Error()}, nil
		}

		knownActions, err := plugins.KnownActions(ctx, db)
		if err != nil {
			return nil, validateWorkflowOut{}, fmt.Errorf("validate_workflow: %w", err)
		}

		if err := workflow.Validate(def, knownActions); err != nil {
			return nil, validateWorkflowOut{Valid: false, Error: err.Error()}, nil
		}

		return nil, validateWorkflowOut{
			Valid:      true,
			WorkflowID: def.ID,
			Version:    def.Version,
			StepCount:  len(def.Steps),
		}, nil
	}
}

// listWorkflowsIn takes no arguments.
type listWorkflowsIn struct{}

type workflowVersionSummary struct {
	WorkflowID  string `json:"workflow_id"`
	Version     int    `json:"version"`
	InstalledAt string `json:"installed_at"`
}

type listWorkflowsOut struct {
	Workflows []workflowVersionSummary `json:"workflows"`
}

func listWorkflows(db *sql.DB) mcp.ToolHandlerFor[listWorkflowsIn, listWorkflowsOut] {
	return func(ctx context.Context, _ *mcp.CallToolRequest, _ listWorkflowsIn) (*mcp.CallToolResult, listWorkflowsOut, error) {
		summaries, err := runs.ListWorkflows(ctx, db)
		if err != nil {
			return nil, listWorkflowsOut{}, fmt.Errorf("list_workflows: %w", err)
		}

		out := listWorkflowsOut{Workflows: make([]workflowVersionSummary, len(summaries))}
		for i, s := range summaries {
			out.Workflows[i] = workflowVersionSummary{
				WorkflowID:  s.WorkflowID,
				Version:     s.Version,
				InstalledAt: s.InstalledAt.Format(time.RFC3339),
			}
		}
		return nil, out, nil
	}
}

type getWorkflowSourceIn struct {
	WorkflowID string `json:"workflow_id" jsonschema:"The workflow's stable id."`
	Version    int    `json:"version,omitempty" jsonschema:"The version to fetch. Omit or 0 for the latest installed version."`
}

type getWorkflowSourceOut struct {
	Source string `json:"source"`
}

func getWorkflowSource(db *sql.DB) mcp.ToolHandlerFor[getWorkflowSourceIn, getWorkflowSourceOut] {
	return func(ctx context.Context, _ *mcp.CallToolRequest, in getWorkflowSourceIn) (*mcp.CallToolResult, getWorkflowSourceOut, error) {
		source, err := runs.WorkflowSource(ctx, db, in.WorkflowID, in.Version)
		if err != nil {
			return nil, getWorkflowSourceOut{}, fmt.Errorf("get_workflow_source: %w", err)
		}
		return nil, getWorkflowSourceOut{Source: source}, nil
	}
}
