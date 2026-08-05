package cli

import (
	"fmt"
	"log/slog"
	"os"
	"os/signal"
	"syscall"

	"github.com/spf13/cobra"

	"github.com/lucasglmt/patchcord/internal/config"
	"github.com/lucasglmt/patchcord/internal/runtime"
)

const defaultListenAddr = "127.0.0.1:7331"

// defaultDataDir is the built-in, lowest-precedence default for every
// command's --data-dir/PATCHCORD_DATA_DIR (ADR-0038, ADR-0049, ADR-0052):
// a per-user, per-machine directory (see config.DefaultDataDir), resolved
// once here since it is the same regardless of which command runs.
var defaultDataDir = config.DefaultDataDir()

// resolveRuntimeConfig resolves the settings needed to run the agent from
// four sources, in increasing order of precedence — a built-in default,
// configPath's YAML file, a PATCHCORD_* environment variable, then an
// explicitly passed flag (ADR-0038; see
// docs/book/src/cli/configuration.md). Shared by `serve` and `dev`
// (internal/cli/dev.go) so this resolution lives in exactly one place —
// cmd must have registered "listen", "data-dir" and
// "secrets-master-key-file" flags under those exact names for
// cmd.Flags().Changed to find them.
func resolveRuntimeConfig(cmd *cobra.Command, listenAddr, dataDir, configPath, secretsMasterKeyFile string) (runtime.Config, error) {
	resolved := config.Config{Listen: defaultListenAddr, DataDir: defaultDataDir}

	if configPath != "" {
		fileCfg, err := config.Load(configPath)
		if err != nil {
			return runtime.Config{}, fmt.Errorf("load config file: %w", err)
		}
		resolved = config.Merge(resolved, fileCfg)
	}

	resolved = config.Merge(resolved, config.FromEnv())

	var flagCfg config.Config
	if cmd.Flags().Changed("listen") {
		flagCfg.Listen = listenAddr
	}
	if cmd.Flags().Changed("data-dir") {
		flagCfg.DataDir = dataDir
	}
	if cmd.Flags().Changed("secrets-master-key-file") {
		flagCfg.SecretsMasterKeyFile = secretsMasterKeyFile
	}
	resolved = config.Merge(resolved, flagCfg)

	return runtime.Config{
		ListenAddr:           resolved.Listen,
		DataDir:              resolved.DataDir,
		SecretsMasterKeyFile: resolved.SecretsMasterKeyFile,
	}, nil
}

func newServeCommand() *cobra.Command {
	var listenAddr string
	var dataDir string
	var configPath string
	var secretsMasterKeyFile string

	cmd := &cobra.Command{
		Use:   "serve",
		Short: "Start the Patchcord agent and its local HTTP API",
		Long: "Starts the agent. Settings layer from four sources, in increasing order of\n" +
			"precedence — a built-in default, --config's YAML file, a PATCHCORD_* environment\n" +
			"variable, then an explicitly passed flag (ADR-0038; see docs/book/src/cli/configuration.md).",
		RunE: func(cmd *cobra.Command, _ []string) error {
			logger := slog.New(slog.NewTextHandler(os.Stdout, nil))

			cfg, err := resolveRuntimeConfig(cmd, listenAddr, dataDir, configPath, secretsMasterKeyFile)
			if err != nil {
				return err
			}

			agent, err := runtime.NewAgent(cfg, logger)
			if err != nil {
				return fmt.Errorf("create agent: %w", err)
			}

			ctx, stop := signal.NotifyContext(cmd.Context(), os.Interrupt, syscall.SIGTERM)
			defer stop()

			return agent.Run(ctx)
		},
	}

	cmd.Flags().StringVar(&listenAddr, "listen", defaultListenAddr, "address the agent's HTTP API listens on (env PATCHCORD_LISTEN)")
	cmd.Flags().StringVar(&dataDir, "data-dir", defaultDataDir, "directory holding the agent's SQLite database (env PATCHCORD_DATA_DIR)")
	cmd.Flags().StringVar(&configPath, "config", "", "path to a YAML config file — lowest-precedence source, see docs/book/src/cli/configuration.md")
	cmd.Flags().StringVar(&secretsMasterKeyFile, "secrets-master-key-file", "", "path to the file holding the base64 AES-256 master key for the \"file\" secret store (env PATCHCORD_SECRETS_MASTER_KEY_FILE) — see `patchcord secret keygen`")

	return cmd
}
