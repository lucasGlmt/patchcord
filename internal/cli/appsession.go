package cli

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"os"
	"path/filepath"
	"strings"

	"github.com/spf13/cobra"

	"github.com/lucasglmt/patchcord/internal/apps"
)

// sessionFileName is what `app session create` writes next to an
// application's static files, and the name its own browser code is
// expected to fetch (same origin, no CORS involved) instead of calling
// client.apps.createSession itself.
const sessionFileName = "patchcord-session.json"

func newAppSessionCommand() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "session",
		Short: "Manage sessions for installed applications",
	}

	cmd.AddCommand(newAppSessionCreateCommand())

	return cmd
}

func newAppSessionCreateCommand() *cobra.Command {
	var dataDir string
	var baseURL string
	var adminToken string
	var output string

	cmd := &cobra.Command{
		Use:   "create <id>",
		Short: "Mint a session for an installed application, out of band from its own browser code",
		Long: "An application's own browser code can never safely mint its own\n" +
			"session once the agent has at least one admin token: ADR-0036 then\n" +
			"requires one to call POST /v1/apps/{id}/sessions, and that token must\n" +
			"never be shipped to a browser. This command is the out-of-band\n" +
			"alternative — run it yourself, holding an admin token, instead of\n" +
			"having the application request its own session.\n\n" +
			"Unlike every other one-shot `app`/`bundle` command, this one needs a\n" +
			"live agent to talk to: a session lives only in the running agent's\n" +
			"memory, never in the database (internal/auth/session.go, ADR-0026),\n" +
			"so no other process can hand one out on its behalf.\n\n" +
			"The result is written as JSON to <static-dir>/" + sessionFileName + " —\n" +
			"same origin as the application's own files, so its code can fetch it\n" +
			"directly at startup instead of calling client.apps.createSession.\n" +
			"Re-run after every rebuild that replaces static-dir's contents (a\n" +
			"fresh `vite build`, another `app install`): the previous run's\n" +
			"session file does not survive one.",
		Args: cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			dataDir = resolveDataDir(cmd, dataDir)
			db, err := openDataStore(dataDir)
			if err != nil {
				return err
			}
			defer db.Close()

			app, err := apps.Get(cmd.Context(), db, args[0])
			if err != nil {
				if errors.Is(err, apps.ErrNotFound) {
					return fmt.Errorf("create app session: %q is not installed", args[0])
				}
				return fmt.Errorf("create app session: %w", err)
			}

			if !cmd.Flags().Changed("admin-token") {
				if env := os.Getenv("PATCHCORD_ADMIN_TOKEN"); env != "" {
					adminToken = env
				}
			}

			token, err := mintAppSession(cmd.Context(), baseURL, app.ID, adminToken)
			if err != nil {
				return fmt.Errorf("create app session: %w", err)
			}

			out := output
			if out == "" {
				out = filepath.Join(app.StaticDir, sessionFileName)
			}

			payload, err := json.MarshalIndent(struct {
				Token string `json:"token"`
			}{Token: token}, "", "  ")
			if err != nil {
				return fmt.Errorf("create app session: encode session: %w", err)
			}
			// 0o600: this file carries a live credential, scoped to this one
			// app's declared permissions but still a bearer credential.
			if err := os.WriteFile(out, payload, 0o600); err != nil {
				return fmt.Errorf("create app session: write %s: %w", out, err)
			}

			fmt.Fprintf(cmd.OutOrStdout(), "Session written to %s\n", out)

			return nil
		},
	}

	cmd.Flags().StringVar(&dataDir, "data-dir", defaultDataDir, "directory holding the agent's SQLite database (env PATCHCORD_DATA_DIR)")
	cmd.Flags().StringVar(&baseURL, "base-url", "http://"+defaultListenAddr, "the running agent's base URL")
	cmd.Flags().StringVar(&adminToken, "admin-token", "", "an admin token (patchcord auth token create) — required once the agent has any admin token (env PATCHCORD_ADMIN_TOKEN)")
	cmd.Flags().StringVarP(&output, "output", "o", "", "where to write the session file (default: <static-dir>/"+sessionFileName+")")

	return cmd
}

// mintAppSession calls the running agent's POST /v1/apps/{id}/sessions
// itself — the one HTTP round trip any `app`/`bundle` CLI command makes,
// forced by a session's in-memory-only lifetime (see newAppSessionCreateCommand's
// long description). adminToken is sent as a bearer credential when
// non-empty; the agent itself decides whether one was actually required
// (ADR-0036's opt-in rule), so an empty adminToken is a valid call when no
// admin token exists yet.
func mintAppSession(ctx context.Context, baseURL, appID, adminToken string) (string, error) {
	endpoint := strings.TrimRight(baseURL, "/") + "/v1/apps/" + url.PathEscape(appID) + "/sessions"

	req, err := http.NewRequestWithContext(ctx, http.MethodPost, endpoint, nil)
	if err != nil {
		return "", fmt.Errorf("build request: %w", err)
	}
	if adminToken != "" {
		req.Header.Set("Authorization", "Bearer "+adminToken)
	}

	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return "", fmt.Errorf("reach agent at %s: %w", baseURL, err)
	}
	defer resp.Body.Close()

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return "", fmt.Errorf("read agent response: %w", err)
	}

	if resp.StatusCode != http.StatusCreated {
		return "", fmt.Errorf("agent returned %d: %s", resp.StatusCode, strings.TrimSpace(string(body)))
	}

	var parsed struct {
		Token string `json:"token"`
	}
	if err := json.Unmarshal(body, &parsed); err != nil {
		return "", fmt.Errorf("decode agent response: %w", err)
	}
	if parsed.Token == "" {
		return "", errors.New("agent response carried no token")
	}

	return parsed.Token, nil
}
