package cli

import (
	"errors"
	"fmt"

	"github.com/spf13/cobra"

	"github.com/lucasglmt/patchcord/internal/runs"
)

func newRunCommand() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "run",
		Short: "Inspect workflow runs",
	}

	cmd.AddCommand(newRunInspectCommand())

	return cmd
}

func newRunInspectCommand() *cobra.Command {
	var dataDir string

	cmd := &cobra.Command{
		Use:   "inspect <run-id>",
		Short: "Show details about one workflow run",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
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

	cmd.Flags().StringVar(&dataDir, "data-dir", defaultDataDir, "directory holding the agent's SQLite database")

	return cmd
}
