package cli

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"

	"github.com/spf13/cobra"

	"github.com/lucasglmt/patchcord/internal/codegen"
	"github.com/lucasglmt/patchcord/internal/plugins"
)

func newDevCodegenCommand() *cobra.Command {
	var tsFlag bool
	var outDir string
	var dataDir string

	cmd := &cobra.Command{
		Use:   "codegen <plugin-id>",
		Short: "Generate typed source code from an installed plugin's action and connector schemas",
		Long: "Reads the installed plugin's catalog entry (action input/output schemas,\n" +
			"connector config schemas) and generates typed source code in the target\n" +
			"language. The agent does not need to be running: the catalog is read\n" +
			"directly from the SQLite database.\n\n" +
			"Currently supported: --ts (TypeScript). The generated file is written to\n" +
			"--out (default: current directory) as <plugin-id>.ts.",
		Args: cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			if !tsFlag {
				return errors.New("specify a language flag: --ts")
			}

			pluginID := args[0]

			dataDir = resolveDataDir(cmd, dataDir)
			db, err := openDataStore(dataDir)
			if err != nil {
				return err
			}
			defer db.Close()

			entry, err := plugins.Get(cmd.Context(), db, pluginID)
			if errors.Is(err, plugins.ErrNotInstalled) {
				return fmt.Errorf("plugin %q is not installed — run `patchcord plugin install` first", pluginID)
			}
			if err != nil {
				return fmt.Errorf("read plugin catalog: %w", err)
			}

			content, err := codegen.GenerateTypeScript(entry)
			if err != nil {
				return fmt.Errorf("generate typescript: %w", err)
			}

			outPath := filepath.Join(outDir, pluginID+".ts")
			if err := os.WriteFile(outPath, content, 0o644); err != nil {
				return fmt.Errorf("write %s: %w", outPath, err)
			}

			fmt.Fprintf(cmd.OutOrStdout(), "Generated %s\n", outPath)
			return nil
		},
	}

	cmd.Flags().BoolVar(&tsFlag, "ts", false, "generate TypeScript interfaces")
	cmd.Flags().StringVar(&outDir, "out", ".", "output directory")
	cmd.Flags().StringVar(&dataDir, "data-dir", defaultDataDir, "directory holding the agent's SQLite database (env PATCHCORD_DATA_DIR)")

	return cmd
}
