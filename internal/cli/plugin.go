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

	"github.com/lucasglmt/patchcord/internal/ghrelease"
	"github.com/lucasglmt/patchcord/internal/persistence"
	"github.com/lucasglmt/patchcord/internal/plugins"
	"github.com/lucasglmt/patchcord/internal/signing"
	"github.com/lucasglmt/patchcord/internal/trust"
	"github.com/lucasglmt/patchcord/migrations"
)

// openDataStore opens and migrates the agent's database for a one-shot CLI
// command, then seeds Patchcord's bundled reference plugins into its
// catalog the first time this data directory is used (plugins.SeedEmbedded,
// ADR-0059) — a no-op on every call after that. Migration and seeding logs
// are discarded: they belong in `patchcord serve`'s structured logs, not in
// one-shot command output.
//
// Every CLI command that touches the database goes through this one
// function, which makes it the single place that needs to seed embedded
// plugins for the whole CLI surface to see them consistently — e.g.
// `workflow install`/`workflow validate` checking a step's action against
// plugins.KnownActions must see the same catalog `workflow run` will later
// execute against.
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

	if err := plugins.SeedEmbedded(context.Background(), db, dataDir, discardLogger); err != nil {
		_ = db.Close()
		return nil, fmt.Errorf("seed embedded plugins: %w", err)
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
	var modulePath string

	cmd := &cobra.Command{
		Use:   "new <id>",
		Short: "Scaffold a new plugin",
		Long: "Writes a standalone Go plugin module (go.mod, main.go with one\n" +
			"example action, manifest.json, README.md, Makefile, .gitignore) into\n" +
			"--output. The manifest declares an executable for the current\n" +
			"platform — enough to `go build` then `plugin pack` without\n" +
			"hand-editing it. Fails if the target directory already exists and is\n" +
			"not empty.",
		Args: cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			id := args[0]
			dir := output
			if dir == "" {
				dir = scaffoldDirName(id)
			}
			if modulePath == "" {
				modulePath = id
			}

			if err := plugins.ScaffoldWithModule(dir, id, version, modulePath); err != nil {
				return fmt.Errorf("plugin new: %w", err)
			}

			fmt.Fprintf(cmd.OutOrStdout(), "Scaffolded %s (%s) into %s\nSee %s/README.md for next steps.\n", id, version, dir, dir)

			return nil
		},
	}

	cmd.Flags().StringVarP(&output, "output", "o", "", "output directory (default: the id's last \".\"-separated segment)")
	cmd.Flags().StringVar(&version, "version", "0.1.0", "version to scaffold")
	cmd.Flags().StringVar(&modulePath, "module", "", "Go module path to write in go.mod (default: the plugin id)")

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

// pluginInstallSource is what resolvePluginInstallSource found for one
// `plugin install <path-or-ref>` argument.
type pluginInstallSource struct {
	// path is a local .patchcord-plugin/executable path ready for the
	// existing install logic — either arg itself (a local file) or a
	// freshly downloaded temp file (a GitHub release asset).
	path string

	// isGitHubPackage is true when path was downloaded from a GitHub
	// Release: always a .patchcord-plugin package (ghrelease.Resolve only
	// ever selects such an asset), so the caller must skip
	// isPackageArchive's gzip sniff and go straight to InstallPackage.
	isGitHubPackage bool

	// source is a human-readable "github.com/owner/repo@tag" description
	// for the success message, empty when path was a local file (nothing
	// to report — the user already knows what they passed).
	source string

	// cleanup is always non-nil; removes any temp download directory.
	cleanup func()
}

// ghreleaseAPIBaseURL overrides ghrelease's GitHub API base URL. Empty in
// production, meaning the real api.github.com; set by tests only.
var ghreleaseAPIBaseURL string

// resolvePluginInstallSource returns a local path plugin install's
// existing package/executable logic can consume for arg: arg itself, if
// it names an existing local file; otherwise, if arg matches ghrelease's
// github.com/<owner>/<repo>[@<tag>] syntax, the repository's resolved
// release's single .patchcord-plugin asset, downloaded into a fresh temp
// directory. Neither local file nor GitHub ref: a plain error naming both
// possibilities.
func resolvePluginInstallSource(ctx context.Context, arg, githubToken string) (pluginInstallSource, error) {
	if _, err := os.Stat(arg); err == nil {
		return pluginInstallSource{path: arg, cleanup: func() {}}, nil
	}

	ref, ok := ghrelease.ParseRef(arg)
	if !ok {
		return pluginInstallSource{}, fmt.Errorf("%q is not a local file and not a github.com/<owner>/<repo>[@<tag>] reference", arg)
	}

	tmpDir, err := os.MkdirTemp("", "patchcord-plugin-github-*")
	if err != nil {
		return pluginInstallSource{}, fmt.Errorf("create download directory: %w", err)
	}
	cleanup := func() { _ = os.RemoveAll(tmpDir) }

	opts := ghrelease.Options{APIBaseURL: ghreleaseAPIBaseURL, Token: githubToken}

	resolved, err := ghrelease.Resolve(ctx, ref, plugins.PackageExtension, opts)
	if err != nil {
		cleanup()
		return pluginInstallSource{}, err
	}

	path, err := ghrelease.Download(ctx, resolved, tmpDir, opts)
	if err != nil {
		cleanup()
		return pluginInstallSource{}, err
	}

	return pluginInstallSource{
		path:            path,
		isGitHubPackage: true,
		source:          fmt.Sprintf("github.com/%s/%s@%s", ref.Owner, ref.Repo, resolved.Tag),
		cleanup:         cleanup,
	}, nil
}

func newPluginInstallCommand() *cobra.Command {
	var dataDir string
	var requireSignature bool
	var githubToken string

	cmd := &cobra.Command{
		Use:   "install <path-or-ref>",
		Short: "Install a plugin from a local executable, a .patchcord-plugin package, or a GitHub Releases reference",
		Long: "Installs a plugin from any of:\n\n" +
			"  - a raw executable path, launched directly for the current platform;\n" +
			"  - a .patchcord-plugin package produced by `plugin pack` — the\n" +
			"    executable matching the current platform is extracted under the\n" +
			"    agent's data directory, so the package file itself does not need\n" +
			"    to stick around afterwards;\n" +
			"  - a github.com/<owner>/<repo>[@<tag>] reference: the repository's\n" +
			"    latest release (or the release tagged <tag>, taken verbatim, no\n" +
			"    \"v\"-prefix guessing) must have exactly one .patchcord-plugin\n" +
			"    asset attached — produced by the plugin author's own `plugin\n" +
			"    pack` and uploaded to the GitHub Release. patchcord never clones\n" +
			"    or builds the repository; the downloaded asset goes through the\n" +
			"    exact same verification as a local package. Public repositories\n" +
			"    only. --github-token (or GITHUB_TOKEN) is optional and only\n" +
			"    raises GitHub's unauthenticated API rate limit — it does not\n" +
			"    enable installing from a private repository. See ADR-0067.\n\n" +
			"A local file and a package archive are told apart by sniffing the\n" +
			"file's content (gzip magic bytes), not its extension.\n" +
			"--require-signature rejects a package that is unsigned or signed by a\n" +
			"key not yet `patchcord trust add`ed for its id, whether the package\n" +
			"came from a local path or a GitHub release; it errors immediately\n" +
			"(nothing to verify) if path-or-ref resolves to a raw executable.",
		Args: cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			dataDir = resolveDataDir(cmd, dataDir)
			db, err := openDataStore(dataDir)
			if err != nil {
				return err
			}
			defer db.Close()

			token := githubToken
			if token == "" {
				token = os.Getenv("GITHUB_TOKEN")
			}

			out := cmd.OutOrStdout()
			sp := newSpinner(out)

			sp.start("Resolving plugin source…")
			src, err := resolvePluginInstallSource(cmd.Context(), args[0], token)
			sp.stop()
			if err != nil {
				return fmt.Errorf("install plugin: %w", err)
			}
			defer src.cleanup()

			isPackage := src.isGitHubPackage
			if !isPackage {
				isPackage, err = isPackageArchive(src.path)
				if err != nil {
					return fmt.Errorf("install plugin: %w", err)
				}
			}
			if requireSignature && !isPackage {
				return fmt.Errorf("install plugin: --require-signature was given but %q is a raw executable, not a package — nothing to verify", args[0])
			}

			sp.start("Installing plugin…")
			var entry *plugins.CatalogEntry
			if isPackage {
				var policy trust.PolicyResult
				entry, policy, err = plugins.InstallPackage(cmd.Context(), db, dataDir, src.path, requireSignature)
				if err == nil {
					defer printVerificationStatus(out, entry.PluginID, policy)
				}
			} else {
				entry, err = plugins.Install(cmd.Context(), db, src.path)
			}
			sp.stop()
			if err != nil {
				return fmt.Errorf("install plugin: %w", err)
			}

			fmt.Fprintf(out, "Installed %s@%s\n", entry.PluginID, entry.Version)
			if src.source != "" {
				fmt.Fprintf(out, "  source:      %s\n", src.source)
			}
			fmt.Fprintf(out, "  actions:     %s\n", joinOrNone(plugins.ActionIDs(entry.Actions)))
			fmt.Fprintf(out, "  connectors:  %s\n", joinOrNone(plugins.ConnectorTypes(entry.Connectors)))
			fmt.Fprintf(out, "  permissions: %s\n", joinOrNone(entry.Permissions))
			fmt.Fprintln(out, "It will start with the agent the next time `patchcord serve` runs.")

			return nil
		},
	}

	cmd.Flags().StringVar(&dataDir, "data-dir", defaultDataDir, "directory holding the agent's SQLite database (env PATCHCORD_DATA_DIR)")
	cmd.Flags().BoolVar(&requireSignature, "require-signature", false, "reject a package that is unsigned or signed by an untrusted key")
	cmd.Flags().StringVar(&githubToken, "github-token", "", "GitHub token used only to raise the unauthenticated API rate limit for a github.com/<owner>/<repo> install (also read from GITHUB_TOKEN); public repositories only")

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

			if err := packToFile(out, func(w io.Writer) error {
				return plugins.Pack(args[0], key, w)
			}); err != nil {
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
			dataDir = resolveDataDir(cmd, dataDir)
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

	cmd.Flags().StringVar(&dataDir, "data-dir", defaultDataDir, "directory holding the agent's SQLite database (env PATCHCORD_DATA_DIR)")

	return cmd
}

func newPluginInspectCommand() *cobra.Command {
	var dataDir string

	cmd := &cobra.Command{
		Use:   "inspect <plugin-id>",
		Short: "Show details about one installed plugin",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			dataDir = resolveDataDir(cmd, dataDir)
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
			fmt.Fprintf(out, "actions:          %s\n", joinOrNone(plugins.ActionIDs(entry.Actions)))
			fmt.Fprintf(out, "connectors:       %s\n", joinOrNone(plugins.ConnectorTypes(entry.Connectors)))
			fmt.Fprintf(out, "permissions:      %s\n", joinOrNone(entry.Permissions))
			fmt.Fprintf(out, "installed at:     %s\n", entry.InstalledAt.Format(time.RFC3339))

			return nil
		},
	}

	cmd.Flags().StringVar(&dataDir, "data-dir", defaultDataDir, "directory holding the agent's SQLite database (env PATCHCORD_DATA_DIR)")

	return cmd
}

func newPluginUninstallCommand() *cobra.Command {
	var dataDir string

	cmd := &cobra.Command{
		Use:   "uninstall <plugin-id>",
		Short: "Remove an installed plugin from the catalog",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			dataDir = resolveDataDir(cmd, dataDir)
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

	cmd.Flags().StringVar(&dataDir, "data-dir", defaultDataDir, "directory holding the agent's SQLite database (env PATCHCORD_DATA_DIR)")

	return cmd
}

func joinOrNone(values []string) string {
	if len(values) == 0 {
		return "(none)"
	}
	return strings.Join(values, ", ")
}
