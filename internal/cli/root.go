// Package cli defines the Patchcord command-line interface. Commands call
// the same internal services as the public API — never duplicated logic.
package cli

import "github.com/spf13/cobra"

// NewRootCommand builds the root "patchcord" command with all subcommands
// attached.
func NewRootCommand() *cobra.Command {
	root := &cobra.Command{
		Use:           "patchcord",
		Short:         "Patchcord Agent — connect anything, automate everything, build on top.",
		SilenceUsage:  true,
		SilenceErrors: true,
	}

	root.AddCommand(newServeCommand())
	root.AddCommand(newPluginCommand())

	return root
}

// Execute runs the root command using the process's own arguments and
// returns any error it produced.
func Execute() error {
	return NewRootCommand().Execute()
}
