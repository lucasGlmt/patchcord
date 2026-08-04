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

const (
	defaultListenAddr = "127.0.0.1:7331"
	defaultDataDir    = "./data"
)

func newServeCommand() *cobra.Command {
	var listenAddr string
	var dataDir string
	var configPath string

	cmd := &cobra.Command{
		Use:   "serve",
		Short: "Start the Patchcord agent and its local HTTP API",
		Long: "Starts the agent. Settings layer from four sources, in increasing order of\n" +
			"precedence — a built-in default, --config's YAML file, a PATCHCORD_* environment\n" +
			"variable, then an explicitly passed flag (ADR-0038; see docs/book/src/cli/configuration.md).",
		RunE: func(cmd *cobra.Command, _ []string) error {
			logger := slog.New(slog.NewTextHandler(os.Stdout, nil))

			resolved := config.Config{Listen: defaultListenAddr, DataDir: defaultDataDir}

			if configPath != "" {
				fileCfg, err := config.Load(configPath)
				if err != nil {
					return fmt.Errorf("load config file: %w", err)
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
			resolved = config.Merge(resolved, flagCfg)

			cfg := runtime.Config{ListenAddr: resolved.Listen, DataDir: resolved.DataDir}
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

	return cmd
}
