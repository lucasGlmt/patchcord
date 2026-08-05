// Package cli defines the Patchcord command-line interface. Commands call
// the same internal services as the public API — never duplicated logic.
package cli

import (
	"github.com/lucasglmt/patchcord/internal/version"
	"github.com/spf13/cobra"
)

// NewRootCommand builds the root "patchcord" command with all subcommands
// attached.
func NewRootCommand() *cobra.Command {
	root := &cobra.Command{
		Use:           "patchcord",
		Short:         "Patchcord Agent — connect anything, automate everything, build on top.",
		Version:       version.Version,
		SilenceUsage:  true,
		SilenceErrors: true,
	}
	// Setting Version above makes cobra register the -v/--version flag
	// for the short form ("patchcord version 0.1.0"); `patchcord version`
	// (below) is the long form with commit and build date.
	root.SetVersionTemplate("{{.Name}} version {{.Version}}\n")

	root.AddCommand(newVersionCommand())
	root.AddCommand(newServeCommand())
	root.AddCommand(newDevCommand())
	root.AddCommand(newMCPCommand())
	root.AddCommand(newPluginCommand())
	root.AddCommand(newConnectorCommand())
	root.AddCommand(newWorkflowCommand())
	root.AddCommand(newRunCommand())
	root.AddCommand(newAppCommand())
	root.AddCommand(newBundleCommand())
	root.AddCommand(newRegistryCommand())
	root.AddCommand(newKeyCommand())
	root.AddCommand(newTrustCommand())
	root.AddCommand(newAuthCommand())
	root.AddCommand(newSecretCommand())

	return root
}

// Execute runs the root command using the process's own arguments and
// returns any error it produced.
func Execute() error {
	return NewRootCommand().Execute()
}
