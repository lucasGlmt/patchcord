package cli

import (
	"crypto/ed25519"
	"errors"
	"fmt"
	"io"
	"os"
	"strings"
	"time"

	"github.com/spf13/cobra"

	"github.com/lucasglmt/patchcord/internal/apps"
	"github.com/lucasglmt/patchcord/internal/signing"
	"github.com/lucasglmt/patchcord/internal/trust"
)

func newAppCommand() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "app",
		Short: "Manage applications",
	}

	cmd.AddCommand(newAppNewCommand())
	cmd.AddCommand(newAppInstallCommand())
	cmd.AddCommand(newAppDevCommand())
	cmd.AddCommand(newAppPackCommand())
	cmd.AddCommand(newAppListCommand())
	cmd.AddCommand(newAppRemoveCommand())
	cmd.AddCommand(newAppSessionCommand())

	return cmd
}

func newAppNewCommand() *cobra.Command {
	var output string
	var version string
	var template string

	cmd := &cobra.Command{
		Use:   "new <id>",
		Short: "Scaffold a new application",
		Long: "Writes a minimal application project into --output. --template static\n" +
			"(default) writes a patchcord-app.yaml and index.html ready for `app\n" +
			"pack`/`app install` as-is. --template vite writes a Vite + TypeScript\n" +
			"project instead — no UI framework opinion, npm packages welcome —\n" +
			"whose patchcord-app.yaml only exists at <dir>/dist after building it:\n" +
			"\n" +
			"  cd <dir> && npm install && npm run build\n" +
			"  patchcord app install <dir>/dist\n" +
			"\n" +
			"Fails if the target directory already exists and is not empty.",
		Args: cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			if err := validateScaffoldTemplate(template); err != nil {
				return fmt.Errorf("app new: %w", err)
			}

			id := args[0]
			dir := output
			if dir == "" {
				dir = scaffoldDirName(id)
			}

			if template == scaffoldTemplateVite {
				if err := apps.ScaffoldVite(dir, id, version); err != nil {
					return fmt.Errorf("app new: %w", err)
				}
				fmt.Fprintf(cmd.OutOrStdout(), "Scaffolded %s (%s) into %s\nBuild it, then install %s/dist:\n  cd %s && npm install && npm run build\n  patchcord app install %s/dist\n", id, version, dir, dir, dir, dir)
				return nil
			}

			if err := apps.Scaffold(dir, id, version); err != nil {
				return fmt.Errorf("app new: %w", err)
			}

			fmt.Fprintf(cmd.OutOrStdout(), "Scaffolded %s (%s) into %s\n", id, version, dir)

			return nil
		},
	}

	cmd.Flags().StringVarP(&output, "output", "o", "", "output directory (default: the id's last \".\"-separated segment)")
	cmd.Flags().StringVar(&version, "version", "0.1.0", "version to scaffold")
	cmd.Flags().StringVar(&template, "template", scaffoldTemplateStatic, "project template to scaffold: static (plain HTML) or vite (Vite + TypeScript, build before app install/dev/pack)")

	return cmd
}

func newAppInstallCommand() *cobra.Command {
	var dataDir string
	var requireSignature bool

	cmd := &cobra.Command{
		Use:   "install <dir-or-package>",
		Short: "Install an application from a directory or a .patchcord-app package",
		Long: "Installs an application from either:\n\n" +
			"  - a directory containing a patchcord-app.yaml manifest (vision\n" +
			"    document, section 7.6) alongside the application's built static\n" +
			"    files — the agent serves it straight from that directory;\n" +
			"  - a .patchcord-app package produced by `app pack` — its contents are\n" +
			"    extracted under the agent's data directory, so the package file\n" +
			"    itself does not need to stick around afterwards.",
		Args: cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			dataDir = resolveDataDir(cmd, dataDir)
			db, err := openDataStore(dataDir)
			if err != nil {
				return err
			}
			defer db.Close()

			info, err := os.Stat(args[0])
			if err != nil {
				return fmt.Errorf("install app: %w", err)
			}
			if requireSignature && info.IsDir() {
				return fmt.Errorf("install app: --require-signature was given but %q is a directory, not a package — nothing to verify", args[0])
			}

			out := cmd.OutOrStdout()

			var app *apps.App
			if info.IsDir() {
				app, err = apps.Install(cmd.Context(), db, args[0])
			} else {
				var policy trust.PolicyResult
				app, policy, err = apps.InstallPackage(cmd.Context(), db, dataDir, args[0], requireSignature)
				if err == nil {
					defer printVerificationStatus(out, app.ID, policy)
				}
			}
			if errors.Is(err, apps.ErrAlreadyExists) {
				return fmt.Errorf("install app: %w", err)
			}
			if err != nil {
				return fmt.Errorf("install app: %w", err)
			}

			fmt.Fprintf(out, "Installed %s (%s)\n", app.ID, app.Version)

			return nil
		},
	}

	cmd.Flags().StringVar(&dataDir, "data-dir", defaultDataDir, "directory holding the agent's SQLite database (env PATCHCORD_DATA_DIR)")
	cmd.Flags().BoolVar(&requireSignature, "require-signature", false, "reject a package that is unsigned or signed by an untrusted key")

	return cmd
}

func newAppDevCommand() *cobra.Command {
	var dataDir string

	cmd := &cobra.Command{
		Use:   "dev <dir>",
		Short: "Install or update an application from a directory for local development",
		Long: "Like `app install`, but updates the application in place instead of\n" +
			"failing when its id is already installed — the friction that would\n" +
			"otherwise mean an `app remove` before every reinstall while iterating.\n\n" +
			"The agent always serves an application's files straight off disk, so\n" +
			"rebuilding dir's contents (e.g. `vite build --watch`) is reflected on\n" +
			"the next browser refresh without rerunning this command.",
		Args: cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			dataDir = resolveDataDir(cmd, dataDir)
			db, err := openDataStore(dataDir)
			if err != nil {
				return err
			}
			defer db.Close()

			app, err := apps.InstallOrUpdate(cmd.Context(), db, args[0])
			if err != nil {
				return fmt.Errorf("app dev: %w", err)
			}

			fmt.Fprintf(cmd.OutOrStdout(), "Serving %s (%s) live from %s\n", app.ID, app.Version, app.StaticDir)

			return nil
		},
	}

	cmd.Flags().StringVar(&dataDir, "data-dir", defaultDataDir, "directory holding the agent's SQLite database (env PATCHCORD_DATA_DIR)")

	return cmd
}

func newAppPackCommand() *cobra.Command {
	var output string
	var signKeyPath string

	cmd := &cobra.Command{
		Use:   "pack <dir>",
		Short: "Package an application directory into a .patchcord-app archive",
		Long: "Packs dir (which must contain a patchcord-app.yaml manifest) into a\n" +
			".patchcord-app archive (vision document, section 9.3) that `app install`\n" +
			"can install directly. The result always carries a checksums.json;\n" +
			"--sign-key (a private key from `patchcord key generate`) additionally\n" +
			"signs it.",
		Args: cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			manifest, err := apps.LoadManifest(args[0])
			if err != nil {
				return fmt.Errorf("pack app: %w", err)
			}

			var key ed25519.PrivateKey
			if signKeyPath != "" {
				key, err = signing.LoadPrivateKey(signKeyPath)
				if err != nil {
					return fmt.Errorf("pack app: %w", err)
				}
			}

			out := output
			if out == "" {
				out = fmt.Sprintf("%s-%s%s", manifest.ID, manifest.Version, apps.PackageExtension)
			}

			if err := packToFile(out, func(w io.Writer) error {
				return apps.Pack(args[0], key, w)
			}); err != nil {
				return fmt.Errorf("pack app: %w", err)
			}

			fmt.Fprintf(cmd.OutOrStdout(), "Packed %s (%s) into %s\n", manifest.ID, manifest.Version, out)

			return nil
		},
	}

	cmd.Flags().StringVarP(&output, "output", "o", "", "output file path (default: <id>-<version>.patchcord-app in the current directory)")
	cmd.Flags().StringVar(&signKeyPath, "sign-key", "", "path to a private key (from `patchcord key generate`) to sign the package with")

	return cmd
}

func newAppListCommand() *cobra.Command {
	var dataDir string

	cmd := &cobra.Command{
		Use:   "list",
		Short: "List installed applications",
		Args:  cobra.NoArgs,
		RunE: func(cmd *cobra.Command, _ []string) error {
			dataDir = resolveDataDir(cmd, dataDir)
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

	cmd.Flags().StringVar(&dataDir, "data-dir", defaultDataDir, "directory holding the agent's SQLite database (env PATCHCORD_DATA_DIR)")

	return cmd
}

func newAppRemoveCommand() *cobra.Command {
	var dataDir string

	cmd := &cobra.Command{
		Use:   "remove <id>",
		Short: "Remove an installed application",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			dataDir = resolveDataDir(cmd, dataDir)
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

	cmd.Flags().StringVar(&dataDir, "data-dir", defaultDataDir, "directory holding the agent's SQLite database (env PATCHCORD_DATA_DIR)")

	return cmd
}
