package cli

import (
	"errors"
	"fmt"
	"time"

	"github.com/spf13/cobra"

	"github.com/lucasglmt/patchcord/internal/auth"
)

// newAuthCommand groups admin token management. Tokens can only be created,
// listed and revoked here, over the CLI (opening the agent's SQLite
// database directly, like every other `patchcord <resource> create`
// command) — never over HTTP: the very first token could never pass
// through an API that would already require one (ADR-0036).
func newAuthCommand() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "auth",
		Short: "Manage admin authentication",
	}

	cmd.AddCommand(newAuthTokenCommand())

	return cmd
}

func newAuthTokenCommand() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "token",
		Short: "Manage admin tokens",
	}

	cmd.AddCommand(newAuthTokenCreateCommand())
	cmd.AddCommand(newAuthTokenListCommand())
	cmd.AddCommand(newAuthTokenRevokeCommand())

	return cmd
}

func newAuthTokenCreateCommand() *cobra.Command {
	var dataDir string

	cmd := &cobra.Command{
		Use:   "create <name>",
		Short: "Create a new admin token",
		Long: "Creates a new admin token: a full, unscoped bearer credential for the public\n" +
			"HTTP API. Printed once, in full, right after creation — there is no way to\n" +
			"recover it afterwards, only to create another one and `auth token revoke` this\n" +
			"one. Creating the very first admin token switches the entire API from today's\n" +
			"default-open behavior to requiring a valid token on every admin-gated request\n" +
			"(see docs/book/src/cli/commands/auth.md).",
		Args: cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			db, err := openDataStore(dataDir)
			if err != nil {
				return err
			}
			defer db.Close()

			plaintext, token, err := auth.CreateToken(cmd.Context(), db, args[0])
			if err != nil {
				return fmt.Errorf("create admin token: %w", err)
			}

			out := cmd.OutOrStdout()
			fmt.Fprintf(out, "Created admin token %q (id %s)\n", token.Name, token.ID)
			fmt.Fprintf(out, "\n  %s\n\n", plaintext)
			fmt.Fprintln(out, "Save it now — it will not be shown again. Pass it as \"Authorization: Bearer <token>\".")

			return nil
		},
	}

	cmd.Flags().StringVar(&dataDir, "data-dir", defaultDataDir, "directory holding the agent's SQLite database")

	return cmd
}

func newAuthTokenListCommand() *cobra.Command {
	var dataDir string

	cmd := &cobra.Command{
		Use:   "list",
		Short: "List admin tokens",
		Long:  "Lists every admin token's id, name and creation time — never its plaintext or hash, which cannot be recovered once shown at creation.",
		Args:  cobra.NoArgs,
		RunE: func(cmd *cobra.Command, _ []string) error {
			db, err := openDataStore(dataDir)
			if err != nil {
				return err
			}
			defer db.Close()

			tokens, err := auth.ListTokens(cmd.Context(), db)
			if err != nil {
				return fmt.Errorf("list admin tokens: %w", err)
			}

			out := cmd.OutOrStdout()
			if len(tokens) == 0 {
				fmt.Fprintln(out, "No admin token created. The API is open to every request until one exists.")
				return nil
			}

			for _, token := range tokens {
				fmt.Fprintf(out, "%s\t%s\t%s\n", token.ID, token.Name, token.CreatedAt.Format(time.RFC3339))
			}

			return nil
		},
	}

	cmd.Flags().StringVar(&dataDir, "data-dir", defaultDataDir, "directory holding the agent's SQLite database")

	return cmd
}

func newAuthTokenRevokeCommand() *cobra.Command {
	var dataDir string

	cmd := &cobra.Command{
		Use:   "revoke <id>",
		Short: "Revoke an admin token",
		Long:  "Revoking the last remaining admin token switches the API back to today's default-open behavior — the same state a fresh agent starts in.",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			db, err := openDataStore(dataDir)
			if err != nil {
				return err
			}
			defer db.Close()

			if err := auth.RevokeToken(cmd.Context(), db, args[0]); err != nil {
				if errors.Is(err, auth.ErrInvalidToken) {
					return fmt.Errorf("revoke admin token: %q was not found", args[0])
				}
				return fmt.Errorf("revoke admin token: %w", err)
			}

			fmt.Fprintf(cmd.OutOrStdout(), "Revoked %s\n", args[0])

			return nil
		},
	}

	cmd.Flags().StringVar(&dataDir, "data-dir", defaultDataDir, "directory holding the agent's SQLite database")

	return cmd
}
