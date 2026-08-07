package cli

import (
	"errors"
	"fmt"
	"log/slog"
	"os"
	"os/signal"
	"syscall"
	"time"

	"github.com/spf13/cobra"

	"github.com/lucasglmt/patchcord/internal/plugins"
	"github.com/lucasglmt/patchcord/internal/runs"
	"github.com/lucasglmt/patchcord/internal/scheduler"
	"github.com/lucasglmt/patchcord/internal/secrets"
	"github.com/lucasglmt/patchcord/internal/workflow"
)

// inputCountSuffix returns ", N input(s)" when def declares an input
// schema, or "" otherwise — so `workflow install`/`workflow validate`
// surface it without duplicating any parsing logic (def is already parsed
// and validated by the caller).
func inputCountSuffix(def *workflow.Definition) string {
	if len(def.Inputs) == 0 {
		return ""
	}
	return fmt.Sprintf(", %d input(s)", len(def.Inputs))
}

func newWorkflowCommand() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "workflow",
		Short: "Manage and run workflows",
	}

	cmd.AddCommand(newWorkflowNewCommand())
	cmd.AddCommand(newWorkflowInstallCommand())
	cmd.AddCommand(newWorkflowListCommand())
	cmd.AddCommand(newWorkflowValidateCommand())
	cmd.AddCommand(newWorkflowExportCommand())
	cmd.AddCommand(newWorkflowRunCommand())

	return cmd
}

func newWorkflowNewCommand() *cobra.Command {
	var output string
	var version int
	var template string

	cmd := &cobra.Command{
		Use:   "new <id>",
		Short: "Scaffold a new workflow",
		Long: "Writes a workflow definition to --output: a manual trigger and one\n" +
			"step using text.uppercase@1 (the reference plugin) — replace it with\n" +
			"your own action, or install `patchcord plugin install\n" +
			"bin/plugins/text` to run the scaffold as-is. Fails if the target file\n" +
			"already exists.\n\n" +
			"--template minimal (default) writes a single step run once.\n" +
			"--template foreach writes a single step run once per item of a\n" +
			"literal list, demonstrating ${{ each }} (ADR-0032) — replace the list\n" +
			"and the step's own action with your own.",
		Args: cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			id := args[0]
			path := output
			if path == "" {
				path = id + ".yaml"
			}

			if err := workflow.ScaffoldTemplate(path, id, version, template); err != nil {
				return fmt.Errorf("workflow new: %w", err)
			}

			fmt.Fprintf(cmd.OutOrStdout(), "Scaffolded %s (version %d) into %s\n", id, version, path)

			return nil
		},
	}

	cmd.Flags().StringVarP(&output, "output", "o", "", "output file path (default: <id>.yaml)")
	cmd.Flags().IntVar(&version, "version", 1, "version to scaffold")
	cmd.Flags().StringVar(&template, "template", workflow.ScaffoldTemplateMinimal, "template to scaffold: minimal (one step, runs once) or foreach (one step, runs once per item of a literal list)")

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

			dataDir = resolveDataDir(cmd, dataDir)
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

			if err := scheduler.Sync(cmd.Context(), db, def); err != nil {
				return fmt.Errorf("schedule workflow: %w", err)
			}

			fmt.Fprintf(cmd.OutOrStdout(), "Installed %s version %d (%d step(s)%s)\n", def.ID, def.Version, len(def.Steps), inputCountSuffix(def))

			return nil
		},
	}

	cmd.Flags().StringVar(&dataDir, "data-dir", defaultDataDir, "directory holding the agent's SQLite database (env PATCHCORD_DATA_DIR)")

	return cmd
}

func newWorkflowListCommand() *cobra.Command {
	var dataDir string

	cmd := &cobra.Command{
		Use:   "list",
		Short: "List installed workflow versions",
		Args:  cobra.NoArgs,
		RunE: func(cmd *cobra.Command, _ []string) error {
			dataDir = resolveDataDir(cmd, dataDir)
			db, err := openDataStore(dataDir)
			if err != nil {
				return err
			}
			defer db.Close()

			versions, err := runs.ListWorkflows(cmd.Context(), db)
			if err != nil {
				return fmt.Errorf("list workflows: %w", err)
			}

			out := cmd.OutOrStdout()
			if len(versions) == 0 {
				fmt.Fprintln(out, "No workflow installed.")
				return nil
			}

			for _, v := range versions {
				fmt.Fprintf(out, "%s\tv%d\t%s\n", v.WorkflowID, v.Version, v.InstalledAt.Format(time.RFC3339))
			}

			return nil
		},
	}

	cmd.Flags().StringVar(&dataDir, "data-dir", defaultDataDir, "directory holding the agent's SQLite database (env PATCHCORD_DATA_DIR)")

	return cmd
}

func newWorkflowValidateCommand() *cobra.Command {
	var dataDir string

	cmd := &cobra.Command{
		Use:   "validate <path.yaml>",
		Short: "Check a workflow file without installing it",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			source, err := os.ReadFile(args[0])
			if err != nil {
				return fmt.Errorf("read workflow file: %w", err)
			}

			dataDir = resolveDataDir(cmd, dataDir)
			db, err := openDataStore(dataDir)
			if err != nil {
				return err
			}
			defer db.Close()

			knownActions, err := plugins.KnownActions(cmd.Context(), db)
			if err != nil {
				return fmt.Errorf("list known actions: %w", err)
			}

			def, err := workflow.Parse(source)
			if err != nil {
				return fmt.Errorf("validate workflow: %w", err)
			}
			if err := workflow.Validate(def, knownActions); err != nil {
				return fmt.Errorf("validate workflow: %w", err)
			}

			fmt.Fprintf(cmd.OutOrStdout(), "%s version %d is valid (%d step(s)%s)\n", def.ID, def.Version, len(def.Steps), inputCountSuffix(def))

			return nil
		},
	}

	cmd.Flags().StringVar(&dataDir, "data-dir", defaultDataDir, "directory holding the agent's SQLite database (env PATCHCORD_DATA_DIR)")

	return cmd
}

func newWorkflowExportCommand() *cobra.Command {
	var dataDir string
	var version int
	var output string

	cmd := &cobra.Command{
		Use:   "export <workflow-id>",
		Short: "Print a workflow version's YAML source",
		Long: "Prints a workflow version's YAML source to stdout, or writes it to a\n" +
			"file with --output — conventionally named <id>-v<version>" + workflow.FileExtension + "\n" +
			"(vision document, section 9.3: a workflow package is exactly this\n" +
			"declarative YAML, no archive involved).",
		Args: cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			dataDir = resolveDataDir(cmd, dataDir)
			db, err := openDataStore(dataDir)
			if err != nil {
				return err
			}
			defer db.Close()

			source, err := runs.WorkflowSource(cmd.Context(), db, args[0], version)
			if errors.Is(err, runs.ErrWorkflowNotFound) {
				return fmt.Errorf("export workflow: %q was not found", args[0])
			}
			if err != nil {
				return fmt.Errorf("export workflow: %w", err)
			}

			if output == "" {
				fmt.Fprint(cmd.OutOrStdout(), source)
				return nil
			}

			if err := os.WriteFile(output, []byte(source), 0o644); err != nil {
				return fmt.Errorf("export workflow: write %q: %w", output, err)
			}
			fmt.Fprintf(cmd.OutOrStdout(), "Exported %s into %s\n", args[0], output)

			return nil
		},
	}

	cmd.Flags().StringVar(&dataDir, "data-dir", defaultDataDir, "directory holding the agent's SQLite database (env PATCHCORD_DATA_DIR)")
	cmd.Flags().IntVar(&version, "version", 0, "workflow version to export (defaults to the latest)")
	cmd.Flags().StringVarP(&output, "output", "o", "", "write to this file instead of stdout")

	return cmd
}

func newWorkflowRunCommand() *cobra.Command {
	var dataDir string
	var inputFlags map[string]string
	var bindingFlags map[string]string
	var stepTimeout time.Duration
	var secretsMasterKeyFile string

	cmd := &cobra.Command{
		Use:   "run <workflow-id>",
		Short: "Run the latest installed version of a workflow",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			logger := slog.New(slog.NewTextHandler(os.Stdout, nil))

			dataDir = resolveDataDir(cmd, dataDir)
			db, err := openDataStore(dataDir)
			if err != nil {
				return err
			}
			defer db.Close()

			// Running a workflow means calling into live plugin processes,
			// so this command launches and supervises the installed
			// plugins for the duration of this one run — mirroring what
			// `patchcord serve` does at startup (see ADR-0017 for why
			// workflow execution can't stay a pure catalog read, unlike
			// the other one-shot plugin/workflow commands).
			supervisor := plugins.NewSupervisor(plugins.SupervisorConfig{}, logger, nil)
			if err := supervisor.Start(cmd.Context(), db); err != nil {
				return fmt.Errorf("start plugin supervisor: %w", err)
			}
			defer func() {
				stopCtx, cancel := signal.NotifyContext(cmd.Context(), os.Interrupt, syscall.SIGTERM)
				defer cancel()
				supervisor.Stop(stopCtx)
			}()

			// A SIGINT/SIGTERM here cancels ctx, which runs.Execute checks
			// between steps and passes down to the in-flight action call —
			// Ctrl+C during a run marks it (and its remaining steps)
			// cancelled rather than just killing the process mid-write.
			ctx, stop := signal.NotifyContext(cmd.Context(), os.Interrupt, syscall.SIGTERM)
			defer stop()

			inputs := make(map[string]any, len(inputFlags))
			for k, v := range inputFlags {
				inputs[k] = v
			}

			secretStore, err := secrets.BuildStore(dataDir, secretsMasterKeyFile)
			if err != nil {
				return fmt.Errorf("run workflow: %w", err)
			}

			run, err := runs.Execute(ctx, db, supervisor, args[0], inputs, bindingFlags, runs.ExecuteOptions{StepTimeout: stepTimeout, Secrets: secretStore})
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

	cmd.Flags().StringVar(&dataDir, "data-dir", defaultDataDir, "directory holding the agent's SQLite database (env PATCHCORD_DATA_DIR)")
	cmd.Flags().StringToStringVar(&inputFlags, "input", nil, "workflow input as key=value, repeatable")
	cmd.Flags().StringToStringVar(&bindingFlags, "binding", nil, "connector binding as name=connector-id, repeatable (see a step's connector: field)")
	cmd.Flags().DurationVar(&stepTimeout, "step-timeout", runs.DefaultStepTimeout, "maximum duration of a single step's action call")
	cmd.Flags().StringVar(&secretsMasterKeyFile, "secrets-master-key-file", "", "path to the file holding the base64 AES-256 master key for the \"file\" secret store (env PATCHCORD_SECRETS_MASTER_KEY_FILE)")

	return cmd
}
