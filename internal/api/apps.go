package api

import (
	"errors"
	"fmt"
	"net/http"
	"strings"
	"time"

	"github.com/lucasglmt/patchcord/internal/apps"
)

// appSummary is the JSON shape of one installed application, as returned
// by GET /v1/apps.
type appSummary struct {
	ID           string   `json:"id"`
	Version      string   `json:"version"`
	WorkflowsRun []string `json:"workflows_run"`
}

// appSessionResponse is the JSON shape of a newly issued application
// session, as returned by POST /v1/apps/{id}/sessions.
type appSessionResponse struct {
	Token        string    `json:"token"`
	AppID        string    `json:"app_id"`
	WorkflowsRun []string  `json:"workflows_run"`
	IssuedAt     time.Time `json:"issued_at"`
}

func toAppSummary(app apps.App) appSummary {
	return appSummary{ID: app.ID, Version: app.Version, WorkflowsRun: app.Permissions.WorkflowsRun}
}

// @Summary      List installed applications
// @Description  Returns every installed application and the permissions its sessions are limited to.
// @Tags         apps
// @Produce      json
// @Success      200  {array}  appSummary
// @Router       /apps [get]
func handleListApps(deps Deps) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		list, err := apps.List(r.Context(), deps.DB)
		if err != nil {
			http.Error(w, "list apps: "+err.Error(), http.StatusInternalServerError)
			return
		}

		summaries := make([]appSummary, 0, len(list))
		for _, app := range list {
			summaries = append(summaries, toAppSummary(app))
		}

		writeJSON(w, http.StatusOK, summaries)
	}
}

// handleCreateAppSession issues a new session for the named application,
// limited to its manifest's declared permissions (vision document, section
// 15.4). It requires no credential of its own — the agent has no
// admin-level authentication anywhere else yet either (see withCORS's doc
// comment), so this endpoint inherits that same, already-documented gap
// rather than inventing a partial one just for applications (ADR-0026).
// @Summary      Issue an application session
// @Description  Issues a new session for the named application, limited to its manifest's declared permissions. The token is a bearer credential: pass it as "Authorization: Bearer <token>" to POST /workflows/{id}/run.
// @Tags         apps
// @Produce      json
// @Param        id   path  string  true  "App id"
// @Success      201  {object}  appSessionResponse
// @Failure      404  {string}  string  "app not found"
// @Router       /apps/{id}/sessions [post]
func handleCreateAppSession(deps Deps) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		id := r.PathValue("id")

		if deps.Sessions == nil {
			http.Error(w, "issue app session: no session store configured", http.StatusInternalServerError)
			return
		}

		app, err := apps.Get(r.Context(), deps.DB, id)
		if errors.Is(err, apps.ErrNotFound) {
			http.Error(w, fmt.Sprintf("app %q was not found", id), http.StatusNotFound)
			return
		}
		if err != nil {
			http.Error(w, "get app: "+err.Error(), http.StatusInternalServerError)
			return
		}

		session := deps.Sessions.Issue(*app)

		writeJSON(w, http.StatusCreated, appSessionResponse{
			Token:        session.Token,
			AppID:        session.AppID,
			WorkflowsRun: session.Permissions.WorkflowsRun,
			IssuedAt:     session.IssuedAt,
		})
	}
}

// handleServeApp serves one installed application's static files
// (vision document, section 10.3: "http://127.0.0.1:7331/apps/{id}/").
// Not part of the JSON API, so it carries no swag annotation.
func handleServeApp(deps Deps) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		id := r.PathValue("id")

		app, err := apps.Get(r.Context(), deps.DB, id)
		if errors.Is(err, apps.ErrNotFound) {
			http.Error(w, fmt.Sprintf("app %q was not found", id), http.StatusNotFound)
			return
		}
		if err != nil {
			http.Error(w, "get app: "+err.Error(), http.StatusInternalServerError)
			return
		}

		prefix := "/apps/" + id + "/"
		http.StripPrefix(prefix, http.FileServer(http.Dir(app.StaticDir))).ServeHTTP(w, r)
	}
}

// bearerToken extracts the token from an "Authorization: Bearer <token>"
// request header, if present.
func bearerToken(r *http.Request) (string, bool) {
	header := r.Header.Get("Authorization")
	token, ok := strings.CutPrefix(header, "Bearer ")
	if !ok || token == "" {
		return "", false
	}
	return token, true
}

// withOptionalAppSession only checks an application session's permissions
// when the request actually presents one — a request with no Authorization
// header reaches next exactly as before this package existed. This keeps
// the rest of the public API's current, already-documented lack of
// authentication unchanged (ADR-0026) while giving an installed
// application's session something real to be limited by.
func withOptionalAppSession(deps Deps, next http.HandlerFunc) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		token, ok := bearerToken(r)
		if !ok {
			next(w, r)
			return
		}
		if deps.Sessions == nil {
			http.Error(w, "app session: no session store configured", http.StatusInternalServerError)
			return
		}

		session, err := deps.Sessions.Validate(token)
		if err != nil {
			http.Error(w, "app session: "+err.Error(), http.StatusUnauthorized)
			return
		}

		workflowID := r.PathValue("id")
		if !session.CanRunWorkflow(workflowID) {
			http.Error(w, fmt.Sprintf("app %q is not permitted to run workflow %q", session.AppID, workflowID), http.StatusForbidden)
			return
		}

		next(w, r)
	}
}
