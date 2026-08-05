package cli

import (
	"context"
	"crypto/ed25519"
	"database/sql"
	"errors"
	"fmt"
	"os"
	"os/signal"
	"syscall"
	"time"

	"github.com/spf13/cobra"

	"github.com/lucasglmt/patchcord/internal/bundles"
	"github.com/lucasglmt/patchcord/internal/plugins"
	"github.com/lucasglmt/patchcord/internal/registry"
	"github.com/lucasglmt/patchcord/internal/signing"
)

func newBundleCommand() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "bundle",
		Short: "Manage bundles (app + workflows + plugin dependencies)",
	}

	cmd.AddCommand(newBundleNewCommand())
	cmd.AddCommand(newBundleInstallCommand())
	cmd.AddCommand(newBundleUpdateCommand())
	cmd.AddCommand(newBundleDevCommand())
	cmd.AddCommand(newBundlePackCommand())
	cmd.AddCommand(newBundleListCommand())
	cmd.AddCommand(newBundleInspectCommand())

	return cmd
}

// resolveBundleInstallSource returns a local file path bundles.InstallPackage
// can consume for arg: arg itself, if it names an existing local file (the
// obvious interpretation, tried first); otherwise arg is treated as a
// registry reference ("id" or "id@version") and resolved/downloaded via
// internal/registry. The returned cleanup always removes any temporary
// download directory it created — a no-op when arg was already a local
// file.
func resolveBundleInstallSource(ctx context.Context, db *sql.DB, arg string) (path string, cleanup func(), err error) {
	noop := func() {}

	if _, statErr := os.Stat(arg); statErr == nil {
		return arg, noop, nil
	}

	id, version := registry.ParseRef(arg)
	resolved, err := registry.Resolve(ctx, db, id, version)
	if err != nil {
		return "", noop, err
	}
	if resolved.Kind != "bundle" {
		return "", noop, fmt.Errorf("%q is a %s package in registry %q, not a bundle", id, resolved.Kind, resolved.RegistryName)
	}

	tmpDir, err := os.MkdirTemp("", "patchcord-bundle-registry-*")
	if err != nil {
		return "", noop, fmt.Errorf("create download directory: %w", err)
	}
	cleanup = func() { _ = os.RemoveAll(tmpDir) }

	path, err = registry.Fetch(ctx, resolved, tmpDir)
	if err != nil {
		cleanup()
		return "", noop, err
	}

	return path, cleanup, nil
}

func newBundleNewCommand() *cobra.Command {
	var output string
	var version string
	var template string

	cmd := &cobra.Command{
		Use:   "new <id>",
		Short: "Scaffold a new bundle",
		Long: "Writes a minimal bundle.yaml into --output, plus an embedded app\n" +
			"and workflow (workflows/main.yaml) — with an empty requires_plugins\n" +
			"(there is no way to know what plugin you'll depend on ahead of\n" +
			"time; add entries to bundle.yaml yourself). --template static\n" +
			"(default) writes a plain HTML app (app/), ready for `bundle\n" +
			"pack`/`bundle install` as-is. --template vite writes a Vite +\n" +
			"TypeScript app (app/) instead — no UI framework opinion, npm\n" +
			"packages welcome — whose bundle.yaml already points app at\n" +
			"app/dist, but that only exists after building it once:\n" +
			"\n" +
			"  cd <dir>/app && npm install && npm run build\n" +
			"  patchcord bundle dev <dir> --watch\n" +
			"\n" +
			"Fails if the target directory already exists and is not empty.",
		Args: cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			if err := validateScaffoldTemplate(template); err != nil {
				return fmt.Errorf("bundle new: %w", err)
			}

			id := args[0]
			dir := output
			if dir == "" {
				dir = scaffoldDirName(id)
			}

			if template == scaffoldTemplateVite {
				if err := bundles.ScaffoldVite(dir, id, version); err != nil {
					return fmt.Errorf("bundle new: %w", err)
				}
				fmt.Fprintf(cmd.OutOrStdout(), "Scaffolded %s (%s) into %s\nBuild the embedded app, then start developing:\n  cd %s/app && npm install && npm run build\n  patchcord bundle dev %s --watch\n", id, version, dir, dir, dir)
				return nil
			}

			if err := bundles.Scaffold(dir, id, version); err != nil {
				return fmt.Errorf("bundle new: %w", err)
			}

			fmt.Fprintf(cmd.OutOrStdout(), "Scaffolded %s (%s) into %s\n", id, version, dir)

			return nil
		},
	}

	cmd.Flags().StringVarP(&output, "output", "o", "", "output directory (default: the id's last \".\"-separated segment)")
	cmd.Flags().StringVar(&version, "version", "0.1.0", "version to scaffold")
	cmd.Flags().StringVar(&template, "template", scaffoldTemplateStatic, "embedded app template to scaffold: static (plain HTML) or vite (Vite + TypeScript, build before bundle pack/install/dev)")

	return cmd
}

func newBundleInstallCommand() *cobra.Command {
	var dataDir string
	var requireSignature bool

	cmd := &cobra.Command{
		Use:   "install <path-or-ref>",
		Short: "Install a bundle from a .patchcord-bundle package or a registry reference",
		Long: "Installs a .patchcord-bundle package produced by `bundle pack`\n" +
			"(vision document, section 9.3). Every plugin the bundle declares in\n" +
			"requires_plugins must already be installed at the exact version\n" +
			"named — install does not fetch missing dependencies automatically.\n" +
			"The embedded app and workflows are installed exactly as `app install`\n" +
			"and `workflow install` would, covered by the bundle's own signature\n" +
			"(--require-signature rejects an unsigned or untrusted bundle; it is\n" +
			"not re-checked separately for the embedded app or workflows).\n\n" +
			"If path-or-ref does not name an existing local file, it is treated\n" +
			"as a registry reference instead — \"id\" (resolves to the latest\n" +
			"version) or \"id@version\" — and resolved against every configured\n" +
			"registry (see `patchcord registry add`). Re-installing an\n" +
			"already-installed bundle id, from either a local path or a\n" +
			"registry reference, updates it in place; `bundle update` is the\n" +
			"more convenient way to do that by id alone.",
		Args: cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			dataDir = resolveDataDir(cmd, dataDir)
			db, err := openDataStore(dataDir)
			if err != nil {
				return err
			}
			defer db.Close()

			packagePath, cleanup, err := resolveBundleInstallSource(cmd.Context(), db, args[0])
			if err != nil {
				return fmt.Errorf("install bundle: %w", err)
			}
			defer cleanup()

			knownActions, err := plugins.KnownActions(cmd.Context(), db)
			if err != nil {
				return fmt.Errorf("install bundle: list known actions: %w", err)
			}

			b, policy, err := bundles.InstallPackage(cmd.Context(), db, dataDir, packagePath, knownActions, requireSignature)
			if err != nil {
				return fmt.Errorf("install bundle: %w", err)
			}

			out := cmd.OutOrStdout()
			fmt.Fprintf(out, "Installed %s (%s)\n", b.ID, b.Version)
			printVerificationStatus(out, b.ID, policy)

			return nil
		},
	}

	cmd.Flags().StringVar(&dataDir, "data-dir", defaultDataDir, "directory holding the agent's SQLite database (env PATCHCORD_DATA_DIR)")
	cmd.Flags().BoolVar(&requireSignature, "require-signature", false, "reject a package that is unsigned or signed by an untrusted key")

	return cmd
}

func newBundleUpdateCommand() *cobra.Command {
	var dataDir string
	var requireSignature bool

	cmd := &cobra.Command{
		Use:   "update <id>[@version]",
		Short: "Update an installed bundle from a configured registry",
		Long: "Resolves id's latest version (or the pinned @version, if given)\n" +
			"against every configured registry (see `patchcord registry add`),\n" +
			"and installs it exactly as `bundle install` would if the resolved\n" +
			"version differs from what is currently installed. id must already\n" +
			"be installed — run `bundle install` first. If the resolved version\n" +
			"is already installed, this is a no-op.",
		Args: cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			dataDir = resolveDataDir(cmd, dataDir)
			db, err := openDataStore(dataDir)
			if err != nil {
				return err
			}
			defer db.Close()

			id, pinnedVersion := registry.ParseRef(args[0])

			current, err := bundles.Get(cmd.Context(), db, id)
			if errors.Is(err, bundles.ErrNotFound) {
				return fmt.Errorf("update bundle: %q is not installed, run `patchcord bundle install` first", id)
			}
			if err != nil {
				return fmt.Errorf("update bundle: %w", err)
			}

			resolved, err := registry.Resolve(cmd.Context(), db, id, pinnedVersion)
			if err != nil {
				return fmt.Errorf("update bundle: %w", err)
			}
			if resolved.Kind != "bundle" {
				return fmt.Errorf("update bundle: %q is a %s package in registry %q, not a bundle", id, resolved.Kind, resolved.RegistryName)
			}

			out := cmd.OutOrStdout()
			if resolved.Version == current.Version {
				fmt.Fprintf(out, "%s is already up to date (%s)\n", id, current.Version)
				return nil
			}

			tmpDir, err := os.MkdirTemp("", "patchcord-bundle-registry-*")
			if err != nil {
				return fmt.Errorf("update bundle: create download directory: %w", err)
			}
			defer os.RemoveAll(tmpDir)

			packagePath, err := registry.Fetch(cmd.Context(), resolved, tmpDir)
			if err != nil {
				return fmt.Errorf("update bundle: %w", err)
			}

			knownActions, err := plugins.KnownActions(cmd.Context(), db)
			if err != nil {
				return fmt.Errorf("update bundle: list known actions: %w", err)
			}

			b, policy, err := bundles.InstallPackage(cmd.Context(), db, dataDir, packagePath, knownActions, requireSignature)
			if err != nil {
				return fmt.Errorf("update bundle: %w", err)
			}

			fmt.Fprintf(out, "Updated %s: %s -> %s\n", id, current.Version, b.Version)
			printVerificationStatus(out, b.ID, policy)

			return nil
		},
	}

	cmd.Flags().StringVar(&dataDir, "data-dir", defaultDataDir, "directory holding the agent's SQLite database (env PATCHCORD_DATA_DIR)")
	cmd.Flags().BoolVar(&requireSignature, "require-signature", false, "reject a package that is unsigned or signed by an untrusted key")

	return cmd
}

func newBundleDevCommand() *cobra.Command {
	var dataDir string
	var watch bool

	cmd := &cobra.Command{
		Use:   "dev <dir>",
		Short: "Install or update a bundle from a directory for local development",
		Long: "Like `bundle install`, but installs straight from dir instead of a\n" +
			".patchcord-bundle package (bundles.InstallDir): no pack/verify round\n" +
			"trip, no signature — dir is local, unsigned-by-design source under\n" +
			"active development.\n\n" +
			"The embedded app (if any) is installed in place, exactly as `app\n" +
			"dev` would: served live from dir's app subdirectory, never copied or\n" +
			"moved, so rebuilding it (e.g. `vite build --watch`) needs no further\n" +
			"agent involvement. Each embedded workflow is installed under a\n" +
			"dev-only rule (ADR-0055): identical content at its declared version\n" +
			"is a no-op, and different content at an already-used version is\n" +
			"auto-installed under the next unused version instead of being\n" +
			"rejected — editing a workflow's body and saving again just works,\n" +
			"no manual version bump needed, and the file on disk is never\n" +
			"rewritten. `bundle install`/`bundle update` stay strict under\n" +
			"ADR-0008: an unbumped version there still fails, since that path\n" +
			"installs a package a developer chose to publish. requires_plugins\n" +
			"is still enforced — a missing dependency is not installed\n" +
			"automatically.\n\n" +
			"--watch keeps this command running, reinstalling on every change\n" +
			"under dir (debounced) until interrupted (Ctrl+C). An install\n" +
			"failure while watching is printed but does not stop the watch —\n" +
			"fix it (e.g. a missing required plugin, or invalid YAML) and save\n" +
			"again.",
		Args: cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			dataDir = resolveDataDir(cmd, dataDir)
			db, err := openDataStore(dataDir)
			if err != nil {
				return err
			}
			defer db.Close()

			dir := args[0]
			out := cmd.OutOrStdout()

			install := func() error {
				knownActions, err := plugins.KnownActions(cmd.Context(), db)
				if err != nil {
					return fmt.Errorf("list known actions: %w", err)
				}
				b, err := bundles.InstallDir(cmd.Context(), db, dir, knownActions)
				if err != nil {
					return err
				}
				fmt.Fprintf(out, "Installed %s (%s) from %s\n", b.ID, b.Version, dir)
				return nil
			}

			if !watch {
				if err := install(); err != nil {
					return fmt.Errorf("bundle dev: %w", err)
				}
				return nil
			}

			if err := install(); err != nil {
				fmt.Fprintf(cmd.ErrOrStderr(), "bundle dev: %v\n", err)
			}

			ctx, stop := signal.NotifyContext(cmd.Context(), os.Interrupt, syscall.SIGTERM)
			defer stop()

			fmt.Fprintf(out, "Watching %s for changes (Ctrl+C to stop)...\n", dir)
			errOut := cmd.ErrOrStderr()
			return watchDir(ctx, dir, install, func(err error) {
				fmt.Fprintf(errOut, "bundle dev: %v\n", err)
			})
		},
	}

	cmd.Flags().StringVar(&dataDir, "data-dir", defaultDataDir, "directory holding the agent's SQLite database (env PATCHCORD_DATA_DIR)")
	cmd.Flags().BoolVar(&watch, "watch", false, "keep running, reinstalling on every change under dir until interrupted (Ctrl+C)")

	return cmd
}

func newBundlePackCommand() *cobra.Command {
	var output string
	var signKeyPath string

	cmd := &cobra.Command{
		Use:   "pack <dir>",
		Short: "Package a bundle directory into a .patchcord-bundle archive",
		Long: "Packs dir (which must contain a bundle.yaml manifest, plus the app\n" +
			"and workflow files it references) into a .patchcord-bundle archive\n" +
			"that `bundle install` can install directly. The result always\n" +
			"carries a checksums.json; --sign-key (a private key from `patchcord\n" +
			"key generate`) additionally signs it — covering the embedded app and\n" +
			"workflows too.",
		Args: cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			manifest, err := bundles.LoadManifest(args[0])
			if err != nil {
				return fmt.Errorf("pack bundle: %w", err)
			}

			var key ed25519.PrivateKey
			if signKeyPath != "" {
				key, err = signing.LoadPrivateKey(signKeyPath)
				if err != nil {
					return fmt.Errorf("pack bundle: %w", err)
				}
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

			if err := bundles.Pack(args[0], key, f); err != nil {
				return fmt.Errorf("pack bundle: %w", err)
			}

			fmt.Fprintf(cmd.OutOrStdout(), "Packed %s (%s) into %s\n", manifest.ID, manifest.Version, out)

			return nil
		},
	}

	cmd.Flags().StringVarP(&output, "output", "o", "", "output file path (default: <id>-<version>.patchcord-bundle in the current directory)")
	cmd.Flags().StringVar(&signKeyPath, "sign-key", "", "path to a private key (from `patchcord key generate`) to sign the package with")

	return cmd
}

func newBundleListCommand() *cobra.Command {
	var dataDir string

	cmd := &cobra.Command{
		Use:   "list",
		Short: "List installed bundles",
		Args:  cobra.NoArgs,
		RunE: func(cmd *cobra.Command, _ []string) error {
			dataDir = resolveDataDir(cmd, dataDir)
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

	cmd.Flags().StringVar(&dataDir, "data-dir", defaultDataDir, "directory holding the agent's SQLite database (env PATCHCORD_DATA_DIR)")

	return cmd
}

func newBundleInspectCommand() *cobra.Command {
	var dataDir string

	cmd := &cobra.Command{
		Use:   "inspect <id>",
		Short: "Show a bundle's declared manifest",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			dataDir = resolveDataDir(cmd, dataDir)
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

	cmd.Flags().StringVar(&dataDir, "data-dir", defaultDataDir, "directory holding the agent's SQLite database (env PATCHCORD_DATA_DIR)")

	return cmd
}
