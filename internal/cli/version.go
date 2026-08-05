package cli

import (
	"fmt"

	"github.com/lucasglmt/patchcord/internal/version"
	"github.com/spf13/cobra"
)

// newVersionCommand builds "patchcord version", printing the full build
// metadata (version, commit, build date). The root command also exposes a
// bare --version/-v flag (set up in NewRootCommand) that prints just the
// version number — this subcommand is the long form.
func newVersionCommand() *cobra.Command {
	return &cobra.Command{
		Use:   "version",
		Short: "Print the agent's version and build metadata",
		Args:  cobra.NoArgs,
		RunE: func(cmd *cobra.Command, args []string) error {
			fmt.Fprintln(cmd.OutOrStdout(), version.String())
			return nil
		},
	}
}
