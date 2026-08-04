package cli

import (
	"fmt"

	"github.com/spf13/cobra"

	"github.com/lucasglmt/patchcord/internal/signing"
)

// defaultSigningKeyPath is where `key generate` writes a key pair when
// --output is not given.
const defaultSigningKeyPath = "patchcord-signing-key"

// newKeyCommand groups package-signing key management. Unlike `secret` and
// `trust`, it never touches the agent's database — a signing key is a
// packaging-time developer tool, not something the running agent needs to
// know about (only the trust store, populated by `trust add`, does).
func newKeyCommand() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "key",
		Short: "Manage package signing keys",
	}

	cmd.AddCommand(newKeyGenerateCommand())

	return cmd
}

func newKeyGenerateCommand() *cobra.Command {
	var output string

	cmd := &cobra.Command{
		Use:   "generate",
		Short: "Generate a new Ed25519 package signing key pair",
		Long: "Writes a new Ed25519 key pair to --output (private key) and\n" +
			"--output.pub (public key), e.g.:\n\n" +
			"  patchcord key generate -o my-signing-key\n\n" +
			"The private key stays on your machine and is passed to `plugin pack\n" +
			"--sign-key`/`app pack --sign-key`/`bundle pack --sign-key`. Distribute\n" +
			"the public key (.pub) to whoever should be able to `patchcord trust\n" +
			"add` it. There is no recovery for a lost private key, only generating\n" +
			"another one and re-signing.",
		Args: cobra.NoArgs,
		RunE: func(cmd *cobra.Command, _ []string) error {
			pub, priv, err := signing.GenerateKeyPair()
			if err != nil {
				return fmt.Errorf("generate key: %w", err)
			}
			if err := signing.WriteKeyPair(output, pub, priv); err != nil {
				return fmt.Errorf("generate key: %w", err)
			}

			out := cmd.OutOrStdout()
			fmt.Fprintf(out, "Wrote private key to %s and public key to %s%s\n", output, output, signing.PublicKeyExtension)
			fmt.Fprintf(out, "Public key fingerprint: %s\n", signing.Fingerprint(pub))

			return nil
		},
	}

	cmd.Flags().StringVarP(&output, "output", "o", defaultSigningKeyPath, "path to write the private key to (the public key is written alongside, with a .pub suffix)")

	return cmd
}
