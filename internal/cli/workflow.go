package cli

import (
	"fmt"
	"log/slog"
	"os"
	"os/signal"
	"syscall"

	"github.com/spf13/cobra"

	"github.com/lucasglmt/patchcord/internal/plugins"
	"github.com/lucasglmt/patchcord/internal/runs"
)

func newWorkflowCommand() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "workflow",
		Short: "Manage and run workflows",
	}

	cmd.AddCommand(newWorkflowInstallCommand())
	cmd.AddCommand(newWorkflowRunCommand())

	return cmd
}

func newWorkflowInstallCommand() *cobra.Command {
	var dataDir string

	cmd := &cobra.Command{
		Use:   "install <path.yaml>",
		Short: "Validate and publish a new workflow version",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			source, err := os.ReadFile(args[0])
			if err != nil {
				return fmt.Errorf("read workflow file: %w", err)
			}

			db, err := openDataStore(dataDir)
			if err != nil {
				return err
			}
			defer db.Close()

			knownActions, err := plugins.KnownActions(cmd.Context(), db)
			if err != nil {
				return fmt.Errorf("list known actions: %w", err)
			}

			def, err := runs.InstallWorkflow(cmd.Context(), db, source, knownActions)
			if err != nil {
				return fmt.Errorf("install workflow: %w", err)
			}

			fmt.Fprintf(cmd.OutOrStdout(), "Installed %s version %d (%d step(s))\n", def.ID, def.Version, len(def.Steps))

			return nil
		},
	}

	cmd.Flags().StringVar(&dataDir, "data-dir", defaultDataDir, "directory holding the agent's SQLite database")

	return cmd
}

func newWorkflowRunCommand() *cobra.Command {
	var dataDir string
	var inputFlags map[string]string

	cmd := &cobra.Command{
		Use:   "run <workflow-id>",
		Short: "Run the latest installed version of a workflow",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			logger := slog.New(slog.NewTextHandler(os.Stdout, nil))

			db, err := openDataStore(dataDir)
			if err != nil {
				return err
			}
			defer db.Close()

			// Running a workflow means calling into live plugin processes,
			// so this command launches and supervises the installed
			// plugins for the duration of this one run — mirroring what
			// `patchcord serve` does at startup (see ADR for why workflow
			// execution can't stay a pure catalog read, unlike the other
			// one-shot plugin/workflow commands).
			supervisor := plugins.NewSupervisor(plugins.SupervisorConfig{}, logger)
			if err := supervisor.Start(cmd.Context(), db); err != nil {
				return fmt.Errorf("start plugin supervisor: %w", err)
			}
			defer func() {
				stopCtx, cancel := signal.NotifyContext(cmd.Context(), os.Interrupt, syscall.SIGTERM)
				defer cancel()
				supervisor.Stop(stopCtx)
			}()

			ctx, stop := signal.NotifyContext(cmd.Context(), os.Interrupt, syscall.SIGTERM)
			defer stop()

			inputs := make(map[string]any, len(inputFlags))
			for k, v := range inputFlags {
				inputs[k] = v
			}

			run, err := runs.Execute(ctx, db, supervisor, args[0], inputs)
			if err != nil {
				return fmt.Errorf("run workflow: %w", err)
			}

			out := cmd.OutOrStdout()
			fmt.Fprintf(out, "run:    %s\n", run.ID)
			fmt.Fprintf(out, "status: %s\n", run.Status)
			if run.Error != "" {
				fmt.Fprintf(out, "error:  %s\n", run.Error)
			}
			fmt.Fprintf(out, "output: %v\n", run.Outputs)

			return nil
		},
	}

	cmd.Flags().StringVar(&dataDir, "data-dir", defaultDataDir, "directory holding the agent's SQLite database")
	cmd.Flags().StringToStringVar(&inputFlags, "input", nil, "workflow input as key=value, repeatable")

	return cmd
}
