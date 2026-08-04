package cli

import (
	"fmt"
	"io"
	"strings"

	"github.com/spf13/cobra"

	"github.com/lucasglmt/patchcord/internal/secrets"
)

const secretsMasterKeyFileFlagUsage = "path to the file holding the base64 AES-256 master key for the \"file\" secret store (env PATCHCORD_SECRETS_MASTER_KEY_FILE, required when --type file)"

// newSecretCommand groups direct read/write access to the "keychain" and
// "file" secret stores. A secret set here can be referenced by any number
// of connectors' --secret <name>=<type>:<key> — it is not owned by any one
// connector, unlike a "env" reference, which is provisioned however the
// agent's own environment gets set and never written through this
// command.
func newSecretCommand() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "secret",
		Short: "Manage secret values in the OS keychain or the file vault",
	}

	cmd.AddCommand(newSecretKeygenCommand())
	cmd.AddCommand(newSecretSetCommand())
	cmd.AddCommand(newSecretRemoveCommand())

	return cmd
}

func newSecretKeygenCommand() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "keygen",
		Short: "Generate a new master key for the file secret store",
		Long: "Prints a new random base64 AES-256 key on stdout, once, with no side effect —\n" +
			"redirect it yourself to the file --secrets-master-key-file (or\n" +
			"PATCHCORD_SECRETS_MASTER_KEY_FILE) is set to, e.g.:\n\n" +
			"  patchcord secret keygen > /path/to/key\n\n" +
			"There is no recovery for a lost key, only generating another one — every\n" +
			"secret already encrypted under the old key becomes unreadable.",
		Args: cobra.NoArgs,
		RunE: func(cmd *cobra.Command, _ []string) error {
			key, err := secrets.GenerateMasterKey()
			if err != nil {
				return fmt.Errorf("generate master key: %w", err)
			}
			fmt.Fprintln(cmd.OutOrStdout(), key)
			return nil
		},
	}
	return cmd
}

func newSecretSetCommand() *cobra.Command {
	var dataDir string
	var secretType string
	var secretsMasterKeyFile string

	cmd := &cobra.Command{
		Use:   "set <key>",
		Short: "Store a secret value under key",
		Long: "Reads the secret's value from stdin — never a flag, which would leak it into\n" +
			"the shell history and the process list — and stores it under key in the OS\n" +
			"keychain or the file vault, depending on --type. Example:\n\n" +
			"  printf '%s' \"$API_KEY\" | patchcord secret set --type file PG_PASSWORD\n\n" +
			"A connector's --secret <name>=<type>:<key> then resolves against this same key.",
		Args: cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			store, err := openWritableSecretStore(secretType, dataDir, secretsMasterKeyFile)
			if err != nil {
				return fmt.Errorf("set secret: %w", err)
			}

			value, err := readSecretValue(cmd.InOrStdin())
			if err != nil {
				return fmt.Errorf("set secret: %w", err)
			}

			if err := store.Set(cmd.Context(), args[0], value); err != nil {
				return fmt.Errorf("set secret: %w", err)
			}

			fmt.Fprintf(cmd.OutOrStdout(), "Set %s:%s\n", secretType, args[0])
			return nil
		},
	}

	cmd.Flags().StringVar(&dataDir, "data-dir", defaultDataDir, "directory holding the agent's SQLite database")
	cmd.Flags().StringVar(&secretType, "type", "", "secret store to write to: \"keychain\" or \"file\"")
	cmd.Flags().StringVar(&secretsMasterKeyFile, "secrets-master-key-file", "", secretsMasterKeyFileFlagUsage)
	_ = cmd.MarkFlagRequired("type")

	return cmd
}

func newSecretRemoveCommand() *cobra.Command {
	var dataDir string
	var secretType string
	var secretsMasterKeyFile string

	cmd := &cobra.Command{
		Use:   "remove <key>",
		Short: "Delete a stored secret value",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			store, err := openWritableSecretStore(secretType, dataDir, secretsMasterKeyFile)
			if err != nil {
				return fmt.Errorf("remove secret: %w", err)
			}

			if err := store.Remove(cmd.Context(), args[0]); err != nil {
				return fmt.Errorf("remove secret: %w", err)
			}

			fmt.Fprintf(cmd.OutOrStdout(), "Removed %s:%s\n", secretType, args[0])
			return nil
		},
	}

	cmd.Flags().StringVar(&dataDir, "data-dir", defaultDataDir, "directory holding the agent's SQLite database")
	cmd.Flags().StringVar(&secretType, "type", "", "secret store to remove from: \"keychain\" or \"file\"")
	cmd.Flags().StringVar(&secretsMasterKeyFile, "secrets-master-key-file", "", secretsMasterKeyFileFlagUsage)
	_ = cmd.MarkFlagRequired("type")

	return cmd
}

// openWritableSecretStore builds the one store `secret set`/`secret
// remove` write to — deliberately not secrets.BuildStore, which returns
// every configured adapter dispatched by Reference.Type: writing a secret
// always targets exactly one adapter, named directly by --type, with no
// "env" case (see secrets.WritableStore's doc comment).
func openWritableSecretStore(secretType, dataDir, masterKeyFile string) (secrets.WritableStore, error) {
	switch secretType {
	case "keychain":
		return secrets.NewKeychainStore(), nil
	case "file":
		if masterKeyFile == "" {
			return nil, fmt.Errorf("--secrets-master-key-file is required for --type file")
		}
		store, err := secrets.NewFileStore(dataDir, masterKeyFile)
		if err != nil {
			return nil, err
		}
		return store, nil
	default:
		return nil, fmt.Errorf("unsupported --type %q (must be \"keychain\" or \"file\")", secretType)
	}
}

// readSecretValue reads all of r and trims exactly one trailing newline
// (LF or CRLF), the same convention `printf '%s' value | ...` and `echo
// value | ...` both produce — so either works without the caller having
// to remember which.
func readSecretValue(r io.Reader) (string, error) {
	data, err := io.ReadAll(r)
	if err != nil {
		return "", fmt.Errorf("read secret value from stdin: %w", err)
	}
	return strings.TrimSuffix(strings.TrimSuffix(string(data), "\n"), "\r"), nil
}
