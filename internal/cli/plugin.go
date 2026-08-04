package cli

import (
	"context"
	"crypto/ed25519"
	"database/sql"
	"errors"
	"fmt"
	"io"
	"log/slog"
	"os"
	"strings"
	"time"

	"github.com/spf13/cobra"

	"github.com/lucasglmt/patchcord/internal/persistence"
	"github.com/lucasglmt/patchcord/internal/plugins"
	"github.com/lucasglmt/patchcord/internal/signing"
	"github.com/lucasglmt/patchcord/internal/trust"
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

	cmd.AddCommand(newPluginNewCommand())
	cmd.AddCommand(newPluginInstallCommand())
	cmd.AddCommand(newPluginPackCommand())
	cmd.AddCommand(newPluginListCommand())
	cmd.AddCommand(newPluginInspectCommand())
	cmd.AddCommand(newPluginUninstallCommand())

	return cmd
}

func newPluginNewCommand() *cobra.Command {
	var output string
	var version string

	cmd := &cobra.Command{
		Use:   "new <id>",
		Short: "Scaffold a new plugin",
		Long: "Writes a minimal Go plugin (main.go with one example action) and a\n" +
			"manifest.json declaring an executable for the current platform, into\n" +
			"--output — enough to `go build` then `plugin pack` without\n" +
			"hand-editing the manifest. Fails if the target directory already\n" +
			"exists and is not empty.",
		Args: cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			id := args[0]
			dir := output
			if dir == "" {
				dir = scaffoldDirName(id)
			}

			if err := plugins.Scaffold(dir, id, version); err != nil {
				return fmt.Errorf("plugin new: %w", err)
			}

			fmt.Fprintf(cmd.OutOrStdout(), "Scaffolded %s (%s) into %s\nSee %s/README.md for next steps.\n", id, version, dir, dir)

			return nil
		},
	}

	cmd.Flags().StringVarP(&output, "output", "o", "", "output directory (default: the id's last \".\"-separated segment)")
	cmd.Flags().StringVar(&version, "version", "0.1.0", "version to scaffold")

	return cmd
}

// gzipMagic is the two-byte prefix of every gzip stream (RFC 1952), which a
// raw plugin executable never starts with — used to tell a .patchcord-plugin
// archive apart from a plain executable path without relying on a file
// extension (ADR-0027: "the format of a content is never guessed from its
// extension").
var gzipMagic = [2]byte{0x1f, 0x8b}

// isPackageArchive reports whether the file at path starts with the gzip
// magic bytes.
func isPackageArchive(path string) (bool, error) {
	f, err := os.Open(path)
	if err != nil {
		return false, err
	}
	defer f.Close()

	var prefix [2]byte
	if _, err := io.ReadFull(f, prefix[:]); err != nil {
		if errors.Is(err, io.EOF) || errors.Is(err, io.ErrUnexpectedEOF) {
			return false, nil
		}
		return false, err
	}

	return prefix == gzipMagic, nil
}

func newPluginInstallCommand() *cobra.Command {
	var dataDir string
	var requireSignature bool

	cmd := &cobra.Command{
		Use:   "install <path>",
		Short: "Install a plugin from a local executable or a .patchcord-plugin package",
		Long: "Installs a plugin from either:\n\n" +
			"  - a raw executable path, launched directly for the current platform;\n" +
			"  - a .patchcord-plugin package produced by `plugin pack` — the\n" +
			"    executable matching the current platform is extracted under the\n" +
			"    agent's data directory, so the package file itself does not need\n" +
			"    to stick around afterwards.\n\n" +
			"The two are told apart by sniffing the file's content (gzip magic\n" +
			"bytes), not its extension. --require-signature only applies to a\n" +
			"package: it rejects one that is unsigned or signed by a key not yet\n" +
			"`patchcord trust add`ed for its id, and errors immediately (nothing\n" +
			"to verify) if path turns out to be a raw executable.",
		Args: cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			db, err := openDataStore(dataDir)
			if err != nil {
				return err
			}
			defer db.Close()

			isPackage, err := isPackageArchive(args[0])
			if err != nil {
				return fmt.Errorf("install plugin: %w", err)
			}
			if requireSignature && !isPackage {
				return fmt.Errorf("install plugin: --require-signature was given but %q is a raw executable, not a package — nothing to verify", args[0])
			}

			out := cmd.OutOrStdout()

			var entry *plugins.CatalogEntry
			if isPackage {
				var policy trust.PolicyResult
				entry, policy, err = plugins.InstallPackage(cmd.Context(), db, dataDir, args[0], requireSignature)
				if err == nil {
					defer printVerificationStatus(out, entry.PluginID, policy)
				}
			} else {
				entry, err = plugins.Install(cmd.Context(), db, args[0])
			}
			if err != nil {
				return fmt.Errorf("install plugin: %w", err)
			}

			fmt.Fprintf(out, "Installed %s@%s\n", entry.PluginID, entry.Version)
			fmt.Fprintf(out, "  actions:     %s\n", joinOrNone(entry.Actions))
			fmt.Fprintf(out, "  connectors:  %s\n", joinOrNone(entry.Connectors))
			fmt.Fprintf(out, "  permissions: %s\n", joinOrNone(entry.Permissions))
			fmt.Fprintln(out, "It will start with the agent the next time `patchcord serve` runs.")

			return nil
		},
	}

	cmd.Flags().StringVar(&dataDir, "data-dir", defaultDataDir, "directory holding the agent's SQLite database")
	cmd.Flags().BoolVar(&requireSignature, "require-signature", false, "reject a package that is unsigned or signed by an untrusted key")

	return cmd
}

func newPluginPackCommand() *cobra.Command {
	var output string
	var signKeyPath string

	cmd := &cobra.Command{
		Use:   "pack <dir>",
		Short: "Package a plugin directory into a .patchcord-plugin archive",
		Long: "Packs dir (which must contain a manifest.json declaring one\n" +
			"executable per supported platform, vision document section 9.1) into\n" +
			"a .patchcord-plugin archive that `plugin install` can install\n" +
			"directly. Building the per-platform executables under dir (e.g. via\n" +
			"GOOS/GOARCH cross-compilation) is left to the plugin's own build\n" +
			"tooling — pack only archives what is already there. The result\n" +
			"always carries a checksums.json; --sign-key (a private key from\n" +
			"`patchcord key generate`) additionally signs it.",
		Args: cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			manifest, err := plugins.LoadPackageManifest(args[0])
			if err != nil {
				return fmt.Errorf("pack plugin: %w", err)
			}

			var key ed25519.PrivateKey
			if signKeyPath != "" {
				key, err = signing.LoadPrivateKey(signKeyPath)
				if err != nil {
					return fmt.Errorf("pack plugin: %w", err)
				}
			}

			out := output
			if out == "" {
				out = fmt.Sprintf("%s-%s%s", manifest.ID, manifest.Version, plugins.PackageExtension)
			}

			f, err := os.Create(out)
			if err != nil {
				return fmt.Errorf("pack plugin: %w", err)
			}
			defer f.Close()

			if err := plugins.Pack(args[0], key, f); err != nil {
				return fmt.Errorf("pack plugin: %w", err)
			}

			fmt.Fprintf(cmd.OutOrStdout(), "Packed %s (%s) into %s\n", manifest.ID, manifest.Version, out)

			return nil
		},
	}

	cmd.Flags().StringVarP(&output, "output", "o", "", "output file path (default: <id>-<version>.patchcord-plugin in the current directory)")
	cmd.Flags().StringVar(&signKeyPath, "sign-key", "", "path to a private key (from `patchcord key generate`) to sign the package with")

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
