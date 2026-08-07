package api

import (
	"errors"
	"fmt"
	"net/http"
	"strings"
	"time"

	"github.com/lucasglmt/patchcord/internal/apps"
	"github.com/lucasglmt/patchcord/internal/auth"
)

// appSummary is the JSON shape of one installed application, as returned
// by GET /v1/apps.
type appSummary struct {
	ID            string   `json:"id"`
	Version       string   `json:"version"`
	WorkflowsRun  []string `json:"workflows_run"`
	ConnectorsUse []string `json:"connectors_use"`
}

// appSessionResponse is the JSON shape of a newly issued application
// session, as returned by POST /v1/apps/{id}/sessions.
type appSessionResponse struct {
	Token         string    `json:"token"`
	AppID         string    `json:"app_id"`
	WorkflowsRun  []string  `json:"workflows_run"`
	ConnectorsUse []string  `json:"connectors_use"`
	IssuedAt      time.Time `json:"issued_at"`
}

func toAppSummary(app apps.App) appSummary {
	return appSummary{
		ID:            app.ID,
		Version:       app.Version,
		WorkflowsRun:  app.Permissions.WorkflowsRun,
		ConnectorsUse: app.Permissions.ConnectorsUse,
	}
}

// @Summary      List installed applications
// @Description  Returns every installed application and the permissions its sessions are limited to.
// @Tags         apps
// @Produce      json
// @Success      200  {array}  appSummary
// @Security     BearerAuth
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
// 15.4). Wrapped in withAdminAuth by the router: anyone able to mint a
// session for an arbitrary installed app is exactly the gap ADR-0026 flagged
// as needing to be closed once admin authentication existed (ADR-0036) — so
// once at least one admin token has been created, only an admin may issue
// one. Before then, this endpoint keeps ADR-0026's original, still-valid
// default-open behavior.
// @Summary      Issue an application session
// @Description  Issues a new session for the named application, limited to its manifest's declared permissions. The token is a bearer credential: pass it as "Authorization: Bearer <token>" to POST /workflows/{id}/run.
// @Tags         apps
// @Produce      json
// @Param        id   path  string  true  "App id"
// @Success      201  {object}  appSessionResponse
// @Failure      404  {string}  string  "app not found"
// @Security     BearerAuth
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
			Token:         session.Token,
			AppID:         session.AppID,
			WorkflowsRun:  session.Permissions.WorkflowsRun,
			ConnectorsUse: session.Permissions.ConnectorsUse,
			IssuedAt:      session.IssuedAt,
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

// appSessionAllowsRun validates token as an application session and reports
// whether it is permitted to run the workflow named by the request's {id}
// path value — the two ways it can say no are distinguished by status: an
// unknown/expired token is 401, a valid session for the wrong workflow is
// 403. Shared by withRunAuth (adminauth.go) between its "no admin token
// exists yet" branch (ADR-0026's original behavior) and its "does this
// bearer token happen to be a valid app session" fallback once admin
// authentication is enabled (ADR-0036).
//
// The resolved session is returned alongside ok so withRunAuth can stash it
// in the request context (ADR-0071) — startRunAndRespond reads it back from
// there to restrict which connectors the run may bind to
// (session.Permissions.ConnectorsUse). It is the zero value when ok is
// false.
func appSessionAllowsRun(deps Deps, r *http.Request, token string) (session auth.Session, ok bool, status int, msg string) {
	if deps.Sessions == nil {
		return auth.Session{}, false, http.StatusInternalServerError, "app session: no session store configured"
	}

	session, err := deps.Sessions.Validate(token)
	if err != nil {
		return auth.Session{}, false, http.StatusUnauthorized, "app session: " + err.Error()
	}

	workflowID := r.PathValue("id")
	if !session.CanRunWorkflow(workflowID) {
		return auth.Session{}, false, http.StatusForbidden, fmt.Sprintf("app %q is not permitted to run workflow %q", session.AppID, workflowID)
	}

	return session, true, 0, ""
}
