package cli

import (
	"fmt"
	"log/slog"
	"os"
	"os/signal"
	"syscall"

	"github.com/spf13/cobra"

	"github.com/lucasglmt/patchcord/internal/runtime"
)

const (
	defaultListenAddr = "127.0.0.1:7331"
	defaultDataDir    = "./data"
)

func newServeCommand() *cobra.Command {
	var listenAddr string
	var dataDir string

	cmd := &cobra.Command{
		Use:   "serve",
		Short: "Start the Patchcord agent and its local HTTP API",
		RunE: func(cmd *cobra.Command, _ []string) error {
			logger := slog.New(slog.NewTextHandler(os.Stdout, nil))

			cfg := runtime.Config{ListenAddr: listenAddr, DataDir: dataDir}
			agent, err := runtime.NewAgent(cfg, logger)
			if err != nil {
				return fmt.Errorf("create agent: %w", err)
			}

			ctx, stop := signal.NotifyContext(cmd.Context(), os.Interrupt, syscall.SIGTERM)
			defer stop()

			return agent.Run(ctx)
		},
	}

	cmd.Flags().StringVar(&listenAddr, "listen", defaultListenAddr, "address the agent's HTTP API listens on")
	cmd.Flags().StringVar(&dataDir, "data-dir", defaultDataDir, "directory holding the agent's SQLite database")

	return cmd
}
