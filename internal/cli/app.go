package cli

import (
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/spf13/cobra"

	"github.com/lucasglmt/patchcord/internal/apps"
)

func newAppCommand() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "app",
		Short: "Manage applications",
	}

	cmd.AddCommand(newAppInstallCommand())
	cmd.AddCommand(newAppListCommand())
	cmd.AddCommand(newAppRemoveCommand())

	return cmd
}

func newAppInstallCommand() *cobra.Command {
	var dataDir string

	cmd := &cobra.Command{
		Use:   "install <dir>",
		Short: "Install an application from a directory",
		Long: "Installs an application: dir must contain a patchcord-app.yaml manifest\n" +
			"(vision document, section 7.6) alongside the application's built static\n" +
			"files. The agent serves the application straight from dir — there is no\n" +
			"packaging or copy step yet.",
		Args: cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			db, err := openDataStore(dataDir)
			if err != nil {
				return err
			}
			defer db.Close()

			app, err := apps.Install(cmd.Context(), db, args[0])
			if errors.Is(err, apps.ErrAlreadyExists) {
				return fmt.Errorf("install app: %w", err)
			}
			if err != nil {
				return fmt.Errorf("install app: %w", err)
			}

			fmt.Fprintf(cmd.OutOrStdout(), "Installed %s (%s)\n", app.ID, app.Version)

			return nil
		},
	}

	cmd.Flags().StringVar(&dataDir, "data-dir", defaultDataDir, "directory holding the agent's SQLite database")

	return cmd
}

func newAppListCommand() *cobra.Command {
	var dataDir string

	cmd := &cobra.Command{
		Use:   "list",
		Short: "List installed applications",
		Args:  cobra.NoArgs,
		RunE: func(cmd *cobra.Command, _ []string) error {
			db, err := openDataStore(dataDir)
			if err != nil {
				return err
			}
			defer db.Close()

			list, err := apps.List(cmd.Context(), db)
			if err != nil {
				return fmt.Errorf("list apps: %w", err)
			}

			out := cmd.OutOrStdout()
			if len(list) == 0 {
				fmt.Fprintln(out, "No app installed.")
				return nil
			}

			for _, app := range list {
				fmt.Fprintf(out, "%s\t%s\t%s\t%s\n",
					app.ID, app.Version, strings.Join(app.Permissions.WorkflowsRun, ","), app.CreatedAt.Format(time.RFC3339))
			}

			return nil
		},
	}

	cmd.Flags().StringVar(&dataDir, "data-dir", defaultDataDir, "directory holding the agent's SQLite database")

	return cmd
}

func newAppRemoveCommand() *cobra.Command {
	var dataDir string

	cmd := &cobra.Command{
		Use:   "remove <id>",
		Short: "Remove an installed application",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			db, err := openDataStore(dataDir)
			if err != nil {
				return err
			}
			defer db.Close()

			if err := apps.Uninstall(cmd.Context(), db, args[0]); err != nil {
				if errors.Is(err, apps.ErrNotFound) {
					return fmt.Errorf("remove app: %q was not found", args[0])
				}
				return fmt.Errorf("remove app: %w", err)
			}

			fmt.Fprintf(cmd.OutOrStdout(), "Removed %s\n", args[0])

			return nil
		},
	}

	cmd.Flags().StringVar(&dataDir, "data-dir", defaultDataDir, "directory holding the agent's SQLite database")

	return cmd
}
