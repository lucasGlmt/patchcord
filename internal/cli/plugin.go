package cli

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"io"
	"log/slog"
	"strings"
	"time"

	"github.com/spf13/cobra"

	"github.com/lucasglmt/patchcord/internal/persistence"
	"github.com/lucasglmt/patchcord/internal/plugins"
	"github.com/lucasglmt/patchcord/migrations"
)

// openDataStore opens and migrates the agent's database for a one-shot CLI
// command. Migration logs are discarded: they belong in `patchcord serve`'s
// structured logs, not in plugin command output.
//
// Plugin commands read and write this catalog directly, independent of
// whether `patchcord serve` is currently running against the same
// --data-dir. A change only takes effect for a running agent the next time
// it starts: there is no live reload yet (see internal/runtime.Agent).
func openDataStore(dataDir string) (*sql.DB, error) {
	db, err := persistence.Open(dataDir)
	if err != nil {
		return nil, fmt.Errorf("open database: %w", err)
	}

	discardLogger := slog.New(slog.NewTextHandler(io.Discard, nil))
	if err := persistence.Migrate(context.Background(), db, migrations.FS, discardLogger); err != nil {
		_ = db.Close()
		return nil, fmt.Errorf("migrate database: %w", err)
	}

	return db, nil
}

func newPluginCommand() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "plugin",
		Short: "Manage installed plugins",
	}

	cmd.AddCommand(newPluginInstallCommand())
	cmd.AddCommand(newPluginListCommand())
	cmd.AddCommand(newPluginInspectCommand())
	cmd.AddCommand(newPluginUninstallCommand())

	return cmd
}

func newPluginInstallCommand() *cobra.Command {
	var dataDir string

	cmd := &cobra.Command{
		Use:   "install <path>",
		Short: "Install a plugin from a local executable path",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			db, err := openDataStore(dataDir)
			if err != nil {
				return err
			}
			defer db.Close()

			entry, err := plugins.Install(cmd.Context(), db, args[0])
			if err != nil {
				return fmt.Errorf("install plugin: %w", err)
			}

			out := cmd.OutOrStdout()
			fmt.Fprintf(out, "Installed %s@%s\n", entry.PluginID, entry.Version)
			fmt.Fprintf(out, "  actions:     %s\n", joinOrNone(entry.Actions))
			fmt.Fprintf(out, "  connectors:  %s\n", joinOrNone(entry.Connectors))
			fmt.Fprintf(out, "  permissions: %s\n", joinOrNone(entry.Permissions))
			fmt.Fprintln(out, "It will start with the agent the next time `patchcord serve` runs.")

			return nil
		},
	}

	cmd.Flags().StringVar(&dataDir, "data-dir", defaultDataDir, "directory holding the agent's SQLite database")

	return cmd
}

func newPluginListCommand() *cobra.Command {
	var dataDir string

	cmd := &cobra.Command{
		Use:   "list",
		Short: "List installed plugins",
		Args:  cobra.NoArgs,
		RunE: func(cmd *cobra.Command, _ []string) error {
			db, err := openDataStore(dataDir)
			if err != nil {
				return err
			}
			defer db.Close()

			entries, err := plugins.List(cmd.Context(), db)
			if err != nil {
				return fmt.Errorf("list plugins: %w", err)
			}

			out := cmd.OutOrStdout()
			if len(entries) == 0 {
				fmt.Fprintln(out, "No plugin installed.")
				return nil
			}

			for _, entry := range entries {
				fmt.Fprintf(out, "%s\t%s\t%s\n", entry.PluginID, entry.Version, entry.ExecutablePath)
			}

			return nil
		},
	}

	cmd.Flags().StringVar(&dataDir, "data-dir", defaultDataDir, "directory holding the agent's SQLite database")

	return cmd
}

func newPluginInspectCommand() *cobra.Command {
	var dataDir string

	cmd := &cobra.Command{
		Use:   "inspect <plugin-id>",
		Short: "Show details about one installed plugin",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			db, err := openDataStore(dataDir)
			if err != nil {
				return err
			}
			defer db.Close()

			entry, err := plugins.Get(cmd.Context(), db, args[0])
			if errors.Is(err, plugins.ErrNotInstalled) {
				return fmt.Errorf("inspect plugin: %q is not installed", args[0])
			}
			if err != nil {
				return fmt.Errorf("inspect plugin: %w", err)
			}

			out := cmd.OutOrStdout()
			fmt.Fprintf(out, "id:               %s\n", entry.PluginID)
			fmt.Fprintf(out, "version:          %s\n", entry.Version)
			fmt.Fprintf(out, "executable:       %s\n", entry.ExecutablePath)
			fmt.Fprintf(out, "protocol version: %d\n", entry.ProtocolVersion)
			fmt.Fprintf(out, "actions:          %s\n", joinOrNone(entry.Actions))
			fmt.Fprintf(out, "connectors:       %s\n", joinOrNone(entry.Connectors))
			fmt.Fprintf(out, "permissions:      %s\n", joinOrNone(entry.Permissions))
			fmt.Fprintf(out, "installed at:     %s\n", entry.InstalledAt.Format(time.RFC3339))

			return nil
		},
	}

	cmd.Flags().StringVar(&dataDir, "data-dir", defaultDataDir, "directory holding the agent's SQLite database")

	return cmd
}

func newPluginUninstallCommand() *cobra.Command {
	var dataDir string

	cmd := &cobra.Command{
		Use:   "uninstall <plugin-id>",
		Short: "Remove an installed plugin from the catalog",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			db, err := openDataStore(dataDir)
			if err != nil {
				return err
			}
			defer db.Close()

			if err := plugins.Uninstall(cmd.Context(), db, args[0]); err != nil {
				if errors.Is(err, plugins.ErrNotInstalled) {
					return fmt.Errorf("uninstall plugin: %q is not installed", args[0])
				}
				return fmt.Errorf("uninstall plugin: %w", err)
			}

			fmt.Fprintf(cmd.OutOrStdout(), "Uninstalled %s\n", args[0])

			return nil
		},
	}

	cmd.Flags().StringVar(&dataDir, "data-dir", defaultDataDir, "directory holding the agent's SQLite database")

	return cmd
}

func joinOrNone(values []string) string {
	if len(values) == 0 {
		return "(none)"
	}
	return strings.Join(values, ", ")
}
