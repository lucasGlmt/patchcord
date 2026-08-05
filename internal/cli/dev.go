package cli

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"log/slog"
	"net"
	"os"
	"os/exec"
	"os/signal"
	"path/filepath"
	"strings"
	"syscall"
	"time"

	"github.com/spf13/cobra"
	"golang.org/x/sync/errgroup"

	"github.com/lucasglmt/patchcord/internal/bundles"
	"github.com/lucasglmt/patchcord/internal/plugins"
	"github.com/lucasglmt/patchcord/internal/runtime"
)

// appDevWaitDelay bounds how long the embedded app's dev subprocess
// (`npm run dev`, by default) is given to exit after being sent SIGTERM on
// Ctrl+C before it is force-killed — mirrors runtime.Agent's own shutdown
// timeout in spirit, just much shorter since a dev server has no in-flight
// work worth waiting on.
const appDevWaitDelay = 5 * time.Second

func newDevCommand() *cobra.Command {
	var listenAddr string
	var dataDir string
	var configPath string
	var secretsMasterKeyFile string
	var appDevCmd string
	var noAppDev bool

	cmd := &cobra.Command{
		Use:   "dev <dir>",
		Short: "Run the agent, watch a bundle, and start its embedded app's dev server — one command",
		Long: "Composes three things a bundle under active development otherwise needs\n" +
			"in three separate terminals, in order — `patchcord serve`, `patchcord\n" +
			"bundle dev --watch`, and the embedded app's own dev server (`npm run\n" +
			"dev`) — into one (ADR-0054). It duplicates no logic: each piece is the\n" +
			"exact same internal service `serve` and `bundle dev` already use.\n\n" +
			"1. Tries to start the agent (same settings resolution as `serve`, see\n" +
			"   below). If --listen is already bound by another `serve`/`dev`, that\n" +
			"   agent is reused instead — this command never fails just because one\n" +
			"   is already running.\n" +
			"2. Installs dir as a bundle (bundles.InstallDir, exactly what `bundle\n" +
			"   dev` does) and watches it for changes, reinstalling on every save\n" +
			"   (ADR-0055: an embedded workflow edited without bumping its version\n" +
			"   is auto-installed under the next version instead of rejected).\n" +
			"3. If the embedded app has a package.json declaring a \"dev\" script,\n" +
			"   runs it (default `npm run dev`, override with --app-dev-cmd,\n" +
			"   disable with --no-app-dev) — its output is prefixed \"[app] \".\n\n" +
			"Ctrl+C stops all three. If any of them fails outright (not counting an\n" +
			"install failure while watching, which is only printed — same as\n" +
			"`bundle dev --watch`), the other two are stopped too and the error is\n" +
			"returned.\n\n" +
			"Settings layer from four sources, in increasing order of precedence —\n" +
			"a built-in default, --config's YAML file, a PATCHCORD_* environment\n" +
			"variable, then an explicitly passed flag (ADR-0038) — identical to\n" +
			"`serve`.",
		Args: cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			dir := args[0]
			out := cmd.OutOrStdout()
			errOut := cmd.ErrOrStderr()

			cfg, err := resolveRuntimeConfig(cmd, listenAddr, dataDir, configPath, secretsMasterKeyFile)
			if err != nil {
				return err
			}

			ctx, stop := signal.NotifyContext(cmd.Context(), os.Interrupt, syscall.SIGTERM)
			defer stop()

			g, gctx := errgroup.WithContext(ctx)

			logger := slog.New(slog.NewTextHandler(out, nil))
			agent, err := runtime.NewAgent(cfg, logger)
			switch {
			case err == nil:
				fmt.Fprintf(out, "Agent listening on %s\n", agent.Addr())
				g.Go(func() error { return agent.Run(gctx) })
			case addressAlreadyInUse(err):
				fmt.Fprintf(out, "Agent already running at %s, reusing it\n", cfg.ListenAddr)
			default:
				return fmt.Errorf("patchcord dev: start agent: %w", err)
			}

			db, err := openDataStore(cfg.DataDir)
			if err != nil {
				return fmt.Errorf("patchcord dev: %w", err)
			}
			defer db.Close()

			install := func() error {
				knownActions, err := plugins.KnownActions(gctx, db)
				if err != nil {
					return fmt.Errorf("list known actions: %w", err)
				}
				b, err := bundles.InstallDir(gctx, db, dir, knownActions)
				if err != nil {
					return err
				}
				fmt.Fprintf(out, "Installed %s (%s) from %s\n", b.ID, b.Version, dir)
				return nil
			}

			if err := install(); err != nil {
				fmt.Fprintf(errOut, "patchcord dev: %v\n", err)
			}

			fmt.Fprintf(out, "Watching %s for changes (Ctrl+C to stop)...\n", dir)
			g.Go(func() error {
				return watchDir(gctx, dir, install, func(err error) {
					fmt.Fprintf(errOut, "patchcord dev: %v\n", err)
				})
			})

			if !noAppDev {
				appDir, found, err := findAppDevDir(dir)
				if err != nil {
					fmt.Fprintf(errOut, "patchcord dev: %v\n", err)
				} else if found {
					cmdParts := strings.Fields(appDevCmd)
					fmt.Fprintf(out, "Starting %q in %s\n", appDevCmd, appDir)
					g.Go(func() error { return runAppDev(gctx, appDir, cmdParts, out) })
				}
			}

			return g.Wait()
		},
	}

	cmd.Flags().StringVar(&listenAddr, "listen", defaultListenAddr, "address the agent's HTTP API listens on (env PATCHCORD_LISTEN)")
	cmd.Flags().StringVar(&dataDir, "data-dir", defaultDataDir, "directory holding the agent's SQLite database (env PATCHCORD_DATA_DIR)")
	cmd.Flags().StringVar(&configPath, "config", "", "path to a YAML config file — lowest-precedence source, see docs/book/src/cli/configuration.md")
	cmd.Flags().StringVar(&secretsMasterKeyFile, "secrets-master-key-file", "", "path to the file holding the base64 AES-256 master key for the \"file\" secret store (env PATCHCORD_SECRETS_MASTER_KEY_FILE)")
	cmd.Flags().StringVar(&appDevCmd, "app-dev-cmd", "npm run dev", "command to run the embedded app's dev server, in the directory holding its package.json — split on whitespace, no shell involved")
	cmd.Flags().BoolVar(&noAppDev, "no-app-dev", false, "never start the embedded app's dev server, even if package.json declares one")

	return cmd
}

// addressAlreadyInUse reports whether err (as returned by runtime.NewAgent)
// is the listener bind failing because something else already holds the
// address — the signal `dev` uses to reuse an already-running agent
// instead of treating a second `patchcord dev`/`patchcord serve` on the
// same --listen as an error.
func addressAlreadyInUse(err error) bool {
	var opErr *net.OpError
	return errors.As(err, &opErr) && errors.Is(opErr.Err, syscall.EADDRINUSE)
}

// findAppDevDir looks for a package.json declaring a "dev" script under
// bundleDir's embedded app — bundle.yaml's own `app` field (e.g. `app/dist`
// for the Vite template, since that's where patchcord-app.yaml ends up
// after a build) or, failing that, its parent directory (`app`, where a
// Vite project's own package.json actually lives). It returns ok == false,
// no error, whenever there is nothing to run: no embedded app at all, or
// no package.json with a "dev" script in either candidate (e.g. the
// static app template).
func findAppDevDir(bundleDir string) (dir string, ok bool, err error) {
	manifest, err := bundles.LoadManifest(bundleDir)
	if err != nil {
		return "", false, fmt.Errorf("load %s: %w", bundleDir, err)
	}
	if manifest.App == "" {
		return "", false, nil
	}

	candidates := []string{manifest.App, filepath.Dir(manifest.App)}
	tried := make(map[string]bool, len(candidates))
	for _, rel := range candidates {
		if rel == "" || rel == "." || tried[rel] {
			continue
		}
		tried[rel] = true

		candidate := filepath.Join(bundleDir, rel)
		hasDevScript, err := packageJSONHasDevScript(filepath.Join(candidate, "package.json"))
		if err != nil {
			return "", false, err
		}
		if hasDevScript {
			return candidate, true, nil
		}
	}

	return "", false, nil
}

// packageJSONHasDevScript reports whether the package.json at path exists
// and declares a "scripts.dev" entry. A missing file is not an error — it
// just means this candidate directory isn't an npm project at all.
func packageJSONHasDevScript(path string) (bool, error) {
	data, err := os.ReadFile(path)
	if errors.Is(err, os.ErrNotExist) {
		return false, nil
	}
	if err != nil {
		return false, fmt.Errorf("read %q: %w", path, err)
	}

	var pkg struct {
		Scripts map[string]string `json:"scripts"`
	}
	if err := json.Unmarshal(data, &pkg); err != nil {
		return false, fmt.Errorf("parse %q: %w", path, err)
	}

	_, ok := pkg.Scripts["dev"]
	return ok, nil
}

// runAppDev runs cmdParts (already split on whitespace) in dir until ctx is
// cancelled, prefixing its combined output "[app] " onto out. On Ctrl+C
// (ctx cancelled) it signals the whole process group — not just the direct
// child — since `npm run dev` typically execs a further child (e.g. vite)
// that would otherwise survive it; SIGTERM first, force-killed after
// appDevWaitDelay if it hasn't exited. An error is only returned for a
// failure unrelated to that shutdown (ctx.Err() == nil at the time Run
// returns) — e.g. the command not existing, or the dev server crashing on
// its own.
func runAppDev(ctx context.Context, dir string, cmdParts []string, out io.Writer) error {
	if len(cmdParts) == 0 {
		return errors.New("--app-dev-cmd must not be empty")
	}

	c := exec.CommandContext(ctx, cmdParts[0], cmdParts[1:]...)
	c.Dir = dir
	c.Env = os.Environ()
	prefixed := &linePrefixWriter{prefix: "[app] ", out: out}
	c.Stdout = prefixed
	c.Stderr = prefixed
	setProcessGroup(c)
	c.Cancel = func() error { return stopProcessGroup(c) }
	c.WaitDelay = appDevWaitDelay

	err := c.Run()
	if err != nil && ctx.Err() == nil {
		return fmt.Errorf("%s: %w", strings.Join(cmdParts, " "), err)
	}
	return nil
}

// linePrefixWriter prefixes every complete line written to it with prefix
// before forwarding to out, buffering any trailing partial line until the
// next Write completes it — so the embedded app's dev server output is
// visibly distinct from the agent's own logs and patchcord dev's own
// messages when all three interleave on one terminal.
type linePrefixWriter struct {
	prefix string
	out    io.Writer
	buf    bytes.Buffer
}

func (w *linePrefixWriter) Write(p []byte) (int, error) {
	w.buf.Write(p)
	for {
		line, err := w.buf.ReadString('\n')
		if err != nil {
			// No newline left in the buffer: err's line is the leftover
			// partial line ReadString drained on its way to EOF — put it
			// back for the next Write to complete.
			w.buf.Reset()
			w.buf.WriteString(line)
			break
		}
		fmt.Fprint(w.out, w.prefix, line)
	}
	return len(p), nil
}
