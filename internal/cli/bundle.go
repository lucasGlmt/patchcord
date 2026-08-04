package cli

import (
	"errors"
	"fmt"
	"os"
	"time"

	"github.com/spf13/cobra"

	"github.com/lucasglmt/patchcord/internal/bundles"
	"github.com/lucasglmt/patchcord/internal/plugins"
)

func newBundleCommand() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "bundle",
		Short: "Manage bundles (app + workflows + plugin dependencies)",
	}

	cmd.AddCommand(newBundleInstallCommand())
	cmd.AddCommand(newBundlePackCommand())
	cmd.AddCommand(newBundleListCommand())
	cmd.AddCommand(newBundleInspectCommand())

	return cmd
}

func newBundleInstallCommand() *cobra.Command {
	var dataDir string

	cmd := &cobra.Command{
		Use:   "install <path>",
		Short: "Install a bundle from a .patchcord-bundle package",
		Long: "Installs a .patchcord-bundle package produced by `bundle pack`\n" +
			"(vision document, section 9.3). Every plugin the bundle declares in\n" +
			"requires_plugins must already be installed at the exact version\n" +
			"named — install does not fetch missing dependencies automatically.\n" +
			"The embedded app and workflows are installed exactly as `app install`\n" +
			"and `workflow install` would.",
		Args: cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			db, err := openDataStore(dataDir)
			if err != nil {
				return err
			}
			defer db.Close()

			knownActions, err := plugins.KnownActions(cmd.Context(), db)
			if err != nil {
				return fmt.Errorf("install bundle: list known actions: %w", err)
			}

			b, err := bundles.InstallPackage(cmd.Context(), db, dataDir, args[0], knownActions)
			if err != nil {
				return fmt.Errorf("install bundle: %w", err)
			}

			fmt.Fprintf(cmd.OutOrStdout(), "Installed %s (%s)\n", b.ID, b.Version)

			return nil
		},
	}

	cmd.Flags().StringVar(&dataDir, "data-dir", defaultDataDir, "directory holding the agent's SQLite database")

	return cmd
}

func newBundlePackCommand() *cobra.Command {
	var output string

	cmd := &cobra.Command{
		Use:   "pack <dir>",
		Short: "Package a bundle directory into a .patchcord-bundle archive",
		Long: "Packs dir (which must contain a bundle.yaml manifest, plus the app\n" +
			"and workflow files it references) into a .patchcord-bundle archive\n" +
			"that `bundle install` can install directly.",
		Args: cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			manifest, err := bundles.LoadManifest(args[0])
			if err != nil {
				return fmt.Errorf("pack bundle: %w", err)
			}

			out := output
			if out == "" {
				out = fmt.Sprintf("%s-%s%s", manifest.ID, manifest.Version, bundles.PackageExtension)
			}

			f, err := os.Create(out)
			if err != nil {
				return fmt.Errorf("pack bundle: %w", err)
			}
			defer f.Close()

			if err := bundles.Pack(args[0], f); err != nil {
				return fmt.Errorf("pack bundle: %w", err)
			}

			fmt.Fprintf(cmd.OutOrStdout(), "Packed %s (%s) into %s\n", manifest.ID, manifest.Version, out)

			return nil
		},
	}

	cmd.Flags().StringVarP(&output, "output", "o", "", "output file path (default: <id>-<version>.patchcord-bundle in the current directory)")

	return cmd
}

func newBundleListCommand() *cobra.Command {
	var dataDir string

	cmd := &cobra.Command{
		Use:   "list",
		Short: "List installed bundles",
		Args:  cobra.NoArgs,
		RunE: func(cmd *cobra.Command, _ []string) error {
			db, err := openDataStore(dataDir)
			if err != nil {
				return err
			}
			defer db.Close()

			list, err := bundles.List(cmd.Context(), db)
			if err != nil {
				return fmt.Errorf("list bundles: %w", err)
			}

			out := cmd.OutOrStdout()
			if len(list) == 0 {
				fmt.Fprintln(out, "No bundle installed.")
				return nil
			}

			for _, b := range list {
				fmt.Fprintf(out, "%s\t%s\t%s\n", b.ID, b.Version, b.InstalledAt.Format(time.RFC3339))
			}

			return nil
		},
	}

	cmd.Flags().StringVar(&dataDir, "data-dir", defaultDataDir, "directory holding the agent's SQLite database")

	return cmd
}

func newBundleInspectCommand() *cobra.Command {
	var dataDir string

	cmd := &cobra.Command{
		Use:   "inspect <id>",
		Short: "Show a bundle's declared manifest",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			db, err := openDataStore(dataDir)
			if err != nil {
				return err
			}
			defer db.Close()

			b, err := bundles.Get(cmd.Context(), db, args[0])
			if errors.Is(err, bundles.ErrNotFound) {
				return fmt.Errorf("inspect bundle: %q was not found", args[0])
			}
			if err != nil {
				return fmt.Errorf("inspect bundle: %w", err)
			}

			out := cmd.OutOrStdout()
			fmt.Fprintf(out, "id:           %s\n", b.ID)
			fmt.Fprintf(out, "version:      %s\n", b.Version)
			fmt.Fprintf(out, "installed at: %s\n", b.InstalledAt.Format(time.RFC3339))
			fmt.Fprintln(out, "manifest:")
			fmt.Fprint(out, b.Manifest)

			return nil
		},
	}

	cmd.Flags().StringVar(&dataDir, "data-dir", defaultDataDir, "directory holding the agent's SQLite database")

	return cmd
}
