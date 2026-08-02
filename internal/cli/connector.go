package cli

import (
	"errors"
	"fmt"
	"sort"
	"strings"
	"time"

	"github.com/spf13/cobra"

	"github.com/lucasglmt/patchcord/internal/connectors"
	"github.com/lucasglmt/patchcord/internal/secrets"
)

func newConnectorCommand() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "connector",
		Short: "Manage connectors",
	}

	cmd.AddCommand(newConnectorCreateCommand())
	cmd.AddCommand(newConnectorListCommand())
	cmd.AddCommand(newConnectorInspectCommand())
	cmd.AddCommand(newConnectorRemoveCommand())

	return cmd
}

// parseSecretRefs turns "name=type:key" flag values, as collected by
// --secret, into secret references keyed by their logical name. Splitting
// stops at the first ":" — a future reference type's key (e.g. a Vault
// path) may itself contain colons.
func parseSecretRefs(raw map[string]string) (map[string]secrets.Reference, error) {
	refs := make(map[string]secrets.Reference, len(raw))
	for name, v := range raw {
		parts := strings.SplitN(v, ":", 2)
		if len(parts) != 2 {
			return nil, fmt.Errorf("invalid secret reference %q for %q, expected type:key (e.g. env:MY_VAR)", v, name)
		}
		refs[name] = secrets.Reference{Type: parts[0], Key: parts[1]}
	}
	return refs, nil
}

func newConnectorCreateCommand() *cobra.Command {
	var dataDir string
	var connectorType string
	var configFlags map[string]string
	var secretFlags map[string]string

	cmd := &cobra.Command{
		Use:   "create <id>",
		Short: "Create a new connector",
		Long: "Creates a new connector: a persistent, named configuration for accessing an\n" +
			"external system. --type should follow the same convention as action ids\n" +
			"(<name>.<subtype>@<version>, e.g. \"postgresql.connection@1\") so it can later be\n" +
			"matched against the connector types a plugin declares — nothing enforces this\n" +
			"yet, but it avoids having to rename connectors once that validation exists.",
		Args: cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			db, err := openDataStore(dataDir)
			if err != nil {
				return err
			}
			defer db.Close()

			secretRefs, err := parseSecretRefs(secretFlags)
			if err != nil {
				return fmt.Errorf("create connector: %w", err)
			}

			config := make(map[string]any, len(configFlags))
			for k, v := range configFlags {
				config[k] = v
			}

			conn, err := connectors.Create(cmd.Context(), db, args[0], connectorType, config, secretRefs)
			if errors.Is(err, connectors.ErrAlreadyExists) {
				return fmt.Errorf("create connector: %q already exists", args[0])
			}
			if err != nil {
				return fmt.Errorf("create connector: %w", err)
			}

			fmt.Fprintf(cmd.OutOrStdout(), "Created %s (%s)\n", conn.ID, conn.Type)

			return nil
		},
	}

	cmd.Flags().StringVar(&dataDir, "data-dir", defaultDataDir, "directory holding the agent's SQLite database")
	cmd.Flags().StringVar(&connectorType, "type", "", "connector type, e.g. \"postgresql.connection@1\"")
	cmd.Flags().StringToStringVar(&configFlags, "config", nil, "non-secret configuration value as key=value, repeatable")
	cmd.Flags().StringToStringVar(&secretFlags, "secret", nil, "secret reference as name=type:key (e.g. api_key=env:MY_API_KEY), repeatable")

	return cmd
}

func newConnectorListCommand() *cobra.Command {
	var dataDir string

	cmd := &cobra.Command{
		Use:   "list",
		Short: "List connectors",
		Args:  cobra.NoArgs,
		RunE: func(cmd *cobra.Command, _ []string) error {
			db, err := openDataStore(dataDir)
			if err != nil {
				return err
			}
			defer db.Close()

			conns, err := connectors.List(cmd.Context(), db)
			if err != nil {
				return fmt.Errorf("list connectors: %w", err)
			}

			out := cmd.OutOrStdout()
			if len(conns) == 0 {
				fmt.Fprintln(out, "No connector created.")
				return nil
			}

			for _, conn := range conns {
				fmt.Fprintf(out, "%s\t%s\t%s\n", conn.ID, conn.Type, conn.CreatedAt.Format(time.RFC3339))
			}

			return nil
		},
	}

	cmd.Flags().StringVar(&dataDir, "data-dir", defaultDataDir, "directory holding the agent's SQLite database")

	return cmd
}

func newConnectorInspectCommand() *cobra.Command {
	var dataDir string

	cmd := &cobra.Command{
		Use:   "inspect <id>",
		Short: "Show details about one connector",
		Long: "Shows a connector's configuration and, for each of its secret references,\n" +
			"whether it currently resolves — never the resolved value itself. This is not a\n" +
			"real connectivity test (that requires a plugin able to attempt the connection,\n" +
			"see ADR-0020); it only checks that each reference can be resolved.",
		Args: cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			db, err := openDataStore(dataDir)
			if err != nil {
				return err
			}
			defer db.Close()

			conn, err := connectors.Get(cmd.Context(), db, args[0])
			if errors.Is(err, connectors.ErrNotFound) {
				return fmt.Errorf("inspect connector: %q was not found", args[0])
			}
			if err != nil {
				return fmt.Errorf("inspect connector: %w", err)
			}

			out := cmd.OutOrStdout()
			fmt.Fprintf(out, "id:      %s\n", conn.ID)
			fmt.Fprintf(out, "type:    %s\n", conn.Type)
			fmt.Fprintf(out, "created: %s\n", conn.CreatedAt.Format(time.RFC3339))

			fmt.Fprintln(out, "config:")
			if len(conn.Config) == 0 {
				fmt.Fprintln(out, "  (none)")
			}
			for _, k := range sortedKeys(conn.Config) {
				fmt.Fprintf(out, "  %s: %v\n", k, conn.Config[k])
			}

			fmt.Fprintln(out, "secrets:")
			if len(conn.SecretRefs) == 0 {
				fmt.Fprintln(out, "  (none)")
			}
			store := secrets.EnvStore{}
			for _, name := range sortedSecretRefKeys(conn.SecretRefs) {
				ref := conn.SecretRefs[name]
				if _, err := store.Resolve(cmd.Context(), ref); err != nil {
					fmt.Fprintf(out, "  %s: %s:%s (NOT resolved: %s)\n", name, ref.Type, ref.Key, err)
					continue
				}
				fmt.Fprintf(out, "  %s: %s:%s (resolved)\n", name, ref.Type, ref.Key)
			}

			return nil
		},
	}

	cmd.Flags().StringVar(&dataDir, "data-dir", defaultDataDir, "directory holding the agent's SQLite database")

	return cmd
}

func sortedKeys(m map[string]any) []string {
	keys := make([]string, 0, len(m))
	for k := range m {
		keys = append(keys, k)
	}
	sort.Strings(keys)
	return keys
}

func sortedSecretRefKeys(m map[string]secrets.Reference) []string {
	keys := make([]string, 0, len(m))
	for k := range m {
		keys = append(keys, k)
	}
	sort.Strings(keys)
	return keys
}

func newConnectorRemoveCommand() *cobra.Command {
	var dataDir string

	cmd := &cobra.Command{
		Use:   "remove <id>",
		Short: "Remove a connector",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			db, err := openDataStore(dataDir)
			if err != nil {
				return err
			}
			defer db.Close()

			if err := connectors.Delete(cmd.Context(), db, args[0]); err != nil {
				if errors.Is(err, connectors.ErrNotFound) {
					return fmt.Errorf("remove connector: %q was not found", args[0])
				}
				return fmt.Errorf("remove connector: %w", err)
			}

			fmt.Fprintf(cmd.OutOrStdout(), "Removed %s\n", args[0])

			return nil
		},
	}

	cmd.Flags().StringVar(&dataDir, "data-dir", defaultDataDir, "directory holding the agent's SQLite database")

	return cmd
}
