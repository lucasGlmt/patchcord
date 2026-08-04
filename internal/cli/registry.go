package cli

import (
	"errors"
	"fmt"
	"time"

	"github.com/spf13/cobra"

	"github.com/lucasglmt/patchcord/internal/registry"
)

// newRegistryCommand groups configured package registries (ADR-0044): a
// registry is a name pointing at a local directory or an http(s) URL
// serving a static index.json plus package files — `bundle install`/
// `bundle update` resolve an "id" or "id@version" reference against every
// configured registry, in the order `registry add` recorded them.
func newRegistryCommand() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "registry",
		Short: "Manage configured package registries",
	}

	cmd.AddCommand(newRegistryAddCommand())
	cmd.AddCommand(newRegistryListCommand())
	cmd.AddCommand(newRegistryRemoveCommand())

	return cmd
}

func newRegistryAddCommand() *cobra.Command {
	var dataDir string

	cmd := &cobra.Command{
		Use:   "add <name> <location>",
		Short: "Configure a package registry",
		Long: "Configures location under name: either a local directory or an\n" +
			"http(s):// URL, serving a static index.json plus package files — no\n" +
			"bespoke registry server, no auth. `bundle install`/`bundle update`\n" +
			"resolve an \"id\" or \"id@version\" reference against every configured\n" +
			"registry, in the order they were added; the first one whose index\n" +
			"lists the id wins. Re-adding the same name updates its location\n" +
			"instead of failing.",
		Args: cobra.ExactArgs(2),
		RunE: func(cmd *cobra.Command, args []string) error {
			name, location := args[0], args[1]

			db, err := openDataStore(dataDir)
			if err != nil {
				return err
			}
			defer db.Close()

			if err := registry.Add(cmd.Context(), db, name, location); err != nil {
				return fmt.Errorf("registry add: %w", err)
			}

			fmt.Fprintf(cmd.OutOrStdout(), "Configured registry %s -> %s\n", name, location)

			return nil
		},
	}

	cmd.Flags().StringVar(&dataDir, "data-dir", defaultDataDir, "directory holding the agent's SQLite database")

	return cmd
}

func newRegistryListCommand() *cobra.Command {
	var dataDir string

	cmd := &cobra.Command{
		Use:   "list",
		Short: "List configured package registries",
		Args:  cobra.NoArgs,
		RunE: func(cmd *cobra.Command, _ []string) error {
			db, err := openDataStore(dataDir)
			if err != nil {
				return err
			}
			defer db.Close()

			registries, err := registry.List(cmd.Context(), db)
			if err != nil {
				return fmt.Errorf("registry list: %w", err)
			}

			out := cmd.OutOrStdout()
			if len(registries) == 0 {
				fmt.Fprintln(out, "No registry configured.")
				return nil
			}

			for _, r := range registries {
				fmt.Fprintf(out, "%s\t%s\t%s\n", r.Name, r.Location, r.AddedAt.Format(time.RFC3339))
			}

			return nil
		},
	}

	cmd.Flags().StringVar(&dataDir, "data-dir", defaultDataDir, "directory holding the agent's SQLite database")

	return cmd
}

func newRegistryRemoveCommand() *cobra.Command {
	var dataDir string

	cmd := &cobra.Command{
		Use:   "remove <name>",
		Short: "Remove a configured package registry",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			name := args[0]

			db, err := openDataStore(dataDir)
			if err != nil {
				return err
			}
			defer db.Close()

			if err := registry.Remove(cmd.Context(), db, name); err != nil {
				if errors.Is(err, registry.ErrNotFound) {
					return fmt.Errorf("registry remove: %q is not configured", name)
				}
				return fmt.Errorf("registry remove: %w", err)
			}

			fmt.Fprintf(cmd.OutOrStdout(), "Removed registry %s\n", name)

			return nil
		},
	}

	cmd.Flags().StringVar(&dataDir, "data-dir", defaultDataDir, "directory holding the agent's SQLite database")

	return cmd
}
