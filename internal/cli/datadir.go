package cli

import (
	"github.com/spf13/cobra"

	"github.com/lucasglmt/patchcord/internal/config"
)

// resolveDataDir widens ADR-0038's --data-dir precedence — flag beats env
// PATCHCORD_DATA_DIR beats the built-in default — from `serve` alone to
// every one-shot command that takes --data-dir (ADR-0049). dataDir is the
// value cobra already parsed for the flag (either what the user passed,
// or its default); it is only replaced by the environment variable when
// the user did not pass --data-dir explicitly.
//
// Callers assign the result back onto their own dataDir variable as the
// first statement of RunE, before any use of it:
//
//	dataDir = resolveDataDir(cmd, dataDir)
func resolveDataDir(cmd *cobra.Command, dataDir string) string {
	if cmd.Flags().Changed("data-dir") {
		return dataDir
	}
	if env := config.FromEnv().DataDir; env != "" {
		return env
	}
	return dataDir
}
