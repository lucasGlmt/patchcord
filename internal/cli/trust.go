package cli

import (
	"errors"
	"fmt"
	"time"

	"github.com/spf13/cobra"

	"github.com/lucasglmt/patchcord/internal/signing"
	"github.com/lucasglmt/patchcord/internal/trust"
)

// newTrustCommand groups the trust store for package signing keys
// (ADR-0043): a public key must be explicitly approved for a package id,
// with `trust add`, before `plugin install`/`app install`/`bundle install
// --require-signature` accepts it — and before a plain install (no
// --require-signature) stops warning about it.
func newTrustCommand() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "trust",
		Short: "Manage trusted package signing keys",
	}

	cmd.AddCommand(newTrustAddCommand())
	cmd.AddCommand(newTrustListCommand())
	cmd.AddCommand(newTrustRemoveCommand())

	return cmd
}

func newTrustAddCommand() *cobra.Command {
	var dataDir string

	cmd := &cobra.Command{
		Use:   "add <id> <pubkey-path>",
		Short: "Approve a public key to sign packages for a given id",
		Long: "Approves the public key at pubkey-path (as written by `patchcord key\n" +
			"generate`) to sign packages for id — e.g. io.patchcord.example-text.\n" +
			"Trust is bound to this exact (id, key) pair: approving a key for one\n" +
			"id never trusts it for another. Re-adding the same pair updates its\n" +
			"label instead of failing.",
		Args: cobra.ExactArgs(2),
		RunE: func(cmd *cobra.Command, args []string) error {
			id, pubKeyPath := args[0], args[1]

			pub, err := signing.LoadPublicKey(pubKeyPath)
			if err != nil {
				return fmt.Errorf("trust add: %w", err)
			}

			dataDir = resolveDataDir(cmd, dataDir)
			db, err := openDataStore(dataDir)
			if err != nil {
				return err
			}
			defer db.Close()

			if err := trust.Add(cmd.Context(), db, id, pub, pubKeyPath); err != nil {
				return fmt.Errorf("trust add: %w", err)
			}

			fmt.Fprintf(cmd.OutOrStdout(), "Trusted %s for %s (fingerprint %s)\n", pubKeyPath, id, signing.Fingerprint(pub))

			return nil
		},
	}

	cmd.Flags().StringVar(&dataDir, "data-dir", defaultDataDir, "directory holding the agent's SQLite database (env PATCHCORD_DATA_DIR)")

	return cmd
}

func newTrustListCommand() *cobra.Command {
	var dataDir string

	cmd := &cobra.Command{
		Use:   "list [id]",
		Short: "List trusted package signing keys",
		Long:  "Lists every trusted key, or only those trusted for id when given.",
		Args:  cobra.MaximumNArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			var id string
			if len(args) == 1 {
				id = args[0]
			}

			dataDir = resolveDataDir(cmd, dataDir)
			db, err := openDataStore(dataDir)
			if err != nil {
				return err
			}
			defer db.Close()

			keys, err := trust.List(cmd.Context(), db, id)
			if err != nil {
				return fmt.Errorf("trust list: %w", err)
			}

			out := cmd.OutOrStdout()
			if len(keys) == 0 {
				fmt.Fprintln(out, "No trusted key.")
				return nil
			}

			for _, key := range keys {
				fmt.Fprintf(out, "%s\t%s\t%s\t%s\n", key.ID, signing.Fingerprint(key.PublicKey), key.Label, key.TrustedAt.Format(time.RFC3339))
			}

			return nil
		},
	}

	cmd.Flags().StringVar(&dataDir, "data-dir", defaultDataDir, "directory holding the agent's SQLite database (env PATCHCORD_DATA_DIR)")

	return cmd
}

func newTrustRemoveCommand() *cobra.Command {
	var dataDir string

	cmd := &cobra.Command{
		Use:   "remove <id> <pubkey-path>",
		Short: "Revoke a trusted package signing key",
		Args:  cobra.ExactArgs(2),
		RunE: func(cmd *cobra.Command, args []string) error {
			id, pubKeyPath := args[0], args[1]

			pub, err := signing.LoadPublicKey(pubKeyPath)
			if err != nil {
				return fmt.Errorf("trust remove: %w", err)
			}

			dataDir = resolveDataDir(cmd, dataDir)
			db, err := openDataStore(dataDir)
			if err != nil {
				return err
			}
			defer db.Close()

			if err := trust.Remove(cmd.Context(), db, id, pub); err != nil {
				if errors.Is(err, trust.ErrNotFound) {
					return fmt.Errorf("trust remove: %s is not trusted for %s", pubKeyPath, id)
				}
				return fmt.Errorf("trust remove: %w", err)
			}

			fmt.Fprintf(cmd.OutOrStdout(), "Revoked trust for %s on %s\n", pubKeyPath, id)

			return nil
		},
	}

	cmd.Flags().StringVar(&dataDir, "data-dir", defaultDataDir, "directory holding the agent's SQLite database (env PATCHCORD_DATA_DIR)")

	return cmd
}
