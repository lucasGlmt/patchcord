package cli

import (
	"errors"
	"fmt"
	"time"

	"github.com/spf13/cobra"

	"github.com/lucasglmt/patchcord/internal/runs"
)

func newRunCommand() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "run",
		Short: "Inspect and manage workflow runs",
	}

	cmd.AddCommand(newRunListCommand())
	cmd.AddCommand(newRunInspectCommand())
	cmd.AddCommand(newRunLogsCommand())
	cmd.AddCommand(newRunCancelCommand())

	return cmd
}

func newRunListCommand() *cobra.Command {
	var dataDir string
	var workflowID string

	cmd := &cobra.Command{
		Use:   "list",
		Short: "List workflow runs",
		Args:  cobra.NoArgs,
		RunE: func(cmd *cobra.Command, _ []string) error {
			dataDir = resolveDataDir(cmd, dataDir)
			db, err := openDataStore(dataDir)
			if err != nil {
				return err
			}
			defer db.Close()

			runList, err := runs.ListRuns(cmd.Context(), db, workflowID)
			if err != nil {
				return fmt.Errorf("list runs: %w", err)
			}

			out := cmd.OutOrStdout()
			if len(runList) == 0 {
				fmt.Fprintln(out, "No run recorded.")
				return nil
			}

			for _, run := range runList {
				fmt.Fprintf(out, "%s\t%s v%d\t%s\t%s\n", run.ID, run.WorkflowID, run.WorkflowVersion, run.Status, run.CreatedAt.Format(time.RFC3339))
			}

			return nil
		},
	}

	cmd.Flags().StringVar(&dataDir, "data-dir", defaultDataDir, "directory holding the agent's SQLite database (env PATCHCORD_DATA_DIR)")
	cmd.Flags().StringVar(&workflowID, "workflow", "", "only list runs of this workflow")

	return cmd
}

func newRunInspectCommand() *cobra.Command {
	var dataDir string

	cmd := &cobra.Command{
		Use:   "inspect <run-id>",
		Short: "Show details about one workflow run",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			dataDir = resolveDataDir(cmd, dataDir)
			db, err := openDataStore(dataDir)
			if err != nil {
				return err
			}
			defer db.Close()

			run, steps, err := runs.GetRun(cmd.Context(), db, args[0])
			if errors.Is(err, runs.ErrRunNotFound) {
				return fmt.Errorf("inspect run: %q was not found", args[0])
			}
			if err != nil {
				return fmt.Errorf("inspect run: %w", err)
			}

			out := cmd.OutOrStdout()
			fmt.Fprintf(out, "id:       %s\n", run.ID)
			fmt.Fprintf(out, "workflow: %s v%d\n", run.WorkflowID, run.WorkflowVersion)
			fmt.Fprintf(out, "status:   %s\n", run.Status)
			if run.Error != "" {
				fmt.Fprintf(out, "error:    %s\n", run.Error)
			}
			fmt.Fprintf(out, "inputs:   %v\n", run.Inputs)
			fmt.Fprintf(out, "outputs:  %v\n", run.Outputs)
			fmt.Fprintln(out, "steps:")
			for _, step := range steps {
				fmt.Fprintf(out, "  - %s: %s\n", step.StepID, step.Status)
				if step.Error != "" {
					fmt.Fprintf(out, "      error:  %s\n", step.Error)
				}
			}

			return nil
		},
	}

	cmd.Flags().StringVar(&dataDir, "data-dir", defaultDataDir, "directory holding the agent's SQLite database (env PATCHCORD_DATA_DIR)")

	return cmd
}

func newRunLogsCommand() *cobra.Command {
	var dataDir string

	cmd := &cobra.Command{
		Use:   "logs <run-id>",
		Short: "Show a timestamped transcript of one workflow run",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			dataDir = resolveDataDir(cmd, dataDir)
			db, err := openDataStore(dataDir)
			if err != nil {
				return err
			}
			defer db.Close()

			run, steps, err := runs.GetRun(cmd.Context(), db, args[0])
			if errors.Is(err, runs.ErrRunNotFound) {
				return fmt.Errorf("run logs: %q was not found", args[0])
			}
			if err != nil {
				return fmt.Errorf("run logs: %w", err)
			}

			out := cmd.OutOrStdout()
			fmt.Fprintf(out, "[%s] run %s created (%s v%d)\n",
				run.CreatedAt.Format(time.RFC3339), run.ID, run.WorkflowID, run.WorkflowVersion)

			for _, step := range steps {
				if step.StartedAt != nil {
					fmt.Fprintf(out, "[%s] step %q started\n", step.StartedAt.Format(time.RFC3339), step.StepID)
				}
				if step.FinishedAt != nil {
					fmt.Fprintf(out, "[%s] step %q %s\n", step.FinishedAt.Format(time.RFC3339), step.StepID, step.Status)
					if step.Error != "" {
						fmt.Fprintf(out, "    error: %s\n", step.Error)
					}
				}
			}

			if run.FinishedAt != nil {
				fmt.Fprintf(out, "[%s] run %s\n", run.FinishedAt.Format(time.RFC3339), run.Status)
			}

			return nil
		},
	}

	cmd.Flags().StringVar(&dataDir, "data-dir", defaultDataDir, "directory holding the agent's SQLite database (env PATCHCORD_DATA_DIR)")

	return cmd
}

func newRunCancelCommand() *cobra.Command {
	var dataDir string

	cmd := &cobra.Command{
		Use:   "cancel <run-id>",
		Short: "Mark a run stuck in queued or running as cancelled",
		Long: "Marks a run still in \"queued\" or \"running\" state as cancelled.\n\n" +
			"patchcord workflow run executes synchronously within its own process,\n" +
			"so this cannot interrupt a run actively in progress elsewhere — it is\n" +
			"meant to clean up a run left behind by a crashed process.",
		Args: cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			dataDir = resolveDataDir(cmd, dataDir)
			db, err := openDataStore(dataDir)
			if err != nil {
				return err
			}
			defer db.Close()

			err = runs.CancelRun(cmd.Context(), db, args[0])
			if errors.Is(err, runs.ErrRunNotFound) {
				return fmt.Errorf("cancel run: %q was not found", args[0])
			}
			if errors.Is(err, runs.ErrRunNotCancellable) {
				return fmt.Errorf("cancel run: %q has already finished", args[0])
			}
			if err != nil {
				return fmt.Errorf("cancel run: %w", err)
			}

			fmt.Fprintf(cmd.OutOrStdout(), "Cancelled %s\n", args[0])

			return nil
		},
	}

	cmd.Flags().StringVar(&dataDir, "data-dir", defaultDataDir, "directory holding the agent's SQLite database (env PATCHCORD_DATA_DIR)")

	return cmd
}
