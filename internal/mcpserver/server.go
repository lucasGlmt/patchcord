// Package mcpserver exposes Patchcord's installed-plugin catalog and
// workflow validation as an MCP (Model Context Protocol) server, so a
// coding agent (Claude Code, Codex) building a bundle/app can query real
// action/connector schemas and validate a workflow draft against the live
// catalog instead of guessing action ids or field names (ADR-0062,
// ADR-0063, ADR-0064). It is a third consumer of internal/plugins,
// internal/workflow, internal/runs, internal/apps and internal/bundles,
// alongside the CLI and the HTTP API (ADR-0005) — never a place business
// logic gets reimplemented.
//
// internal/cli/mcp.go is the only caller: it resolves --data-dir, opens
// the store, builds the server via New, and runs it over stdio.
package mcpserver

import (
	"database/sql"

	"github.com/modelcontextprotocol/go-sdk/mcp"

	"github.com/lucasglmt/patchcord/internal/version"
)

// New builds an MCP server exposing every tool this package defines,
// unstarted — the caller runs it over whatever transport it chooses
// (stdio, for patchcord mcp serve).
func New(db *sql.DB) *mcp.Server {
	server := mcp.NewServer(&mcp.Implementation{
		Name:    "patchcord",
		Version: version.Version,
	}, nil)

	registerPluginTools(server, db)
	registerWorkflowTools(server, db)
	registerScaffoldTools(server)

	return server
}
