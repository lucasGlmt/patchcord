package cli

import (
	"log/slog"
	"os"
	"os/signal"
	"syscall"

	"github.com/modelcontextprotocol/go-sdk/mcp"
	"github.com/spf13/cobra"

	"github.com/lucasglmt/patchcord/internal/mcpserver"
)

func newMCPCommand() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "mcp",
		Short: "Model Context Protocol integration for coding agents",
	}
	cmd.AddCommand(newMCPServeCommand())
	return cmd
}

func newMCPServeCommand() *cobra.Command {
	var dataDir string

	cmd := &cobra.Command{
		Use:   "serve",
		Short: "Start a local MCP server over stdio, for a coding agent to register",
		Long: "Starts an MCP (Model Context Protocol) server over stdio — the model a coding\n" +
			"agent (Claude Code, Codex) uses to register a local tool server as a subprocess.\n" +
			"It exposes this agent's installed-plugin catalog (actions, connectors, their\n" +
			"declared JSON Schema — ADR-0062) and workflow validation (ADR-0063) as tools, so\n" +
			"an agent building a bundle/app can ground itself in the real catalog instead of\n" +
			"guessing action ids or field names, plus two tools to scaffold a new app/bundle\n" +
			"project (ADR-0064). See docs/book/src/cli/commands/mcp.md for the full tool list\n" +
			"and how to register this command with an agent.",
		Args: cobra.NoArgs,
		RunE: func(cmd *cobra.Command, _ []string) error {
			dataDir = resolveDataDir(cmd, dataDir)
			db, err := openDataStore(dataDir)
			if err != nil {
				return err
			}
			defer db.Close()

			// stdout is reserved for the MCP JSON-RPC stream a stdio client
			// parses — unlike `serve`'s logger, this one MUST write to
			// stderr, or every log line corrupts the protocol stream.
			logger := slog.New(slog.NewTextHandler(os.Stderr, nil))
			logger.Info("starting MCP server", slog.String("data_dir", dataDir))

			server := mcpserver.New(db)

			ctx, stop := signal.NotifyContext(cmd.Context(), os.Interrupt, syscall.SIGTERM)
			defer stop()

			return server.Run(ctx, &mcp.StdioTransport{})
		},
	}

	cmd.Flags().StringVar(&dataDir, "data-dir", defaultDataDir, "directory holding the agent's SQLite database (env PATCHCORD_DATA_DIR)")

	return cmd
}
