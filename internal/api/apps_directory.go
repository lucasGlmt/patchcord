package api

import (
	"embed"
	"html/template"
	"net/http"

	"github.com/lucasglmt/patchcord/internal/apps"
)

//go:embed templates/apps_directory.html.tmpl
var appsDirectoryTemplateFS embed.FS

// appsDirectoryTemplate renders GET /apps/. Parsed once at package init —
// template.Must panics on a malformed template, which is exactly the
// "caught before it ships" behavior wanted for a template that never
// varies with runtime input.
var appsDirectoryTemplate = template.Must(template.ParseFS(appsDirectoryTemplateFS, "templates/apps_directory.html.tmpl"))

// appsDirectoryView is the data appsDirectoryTemplate renders.
type appsDirectoryView struct {
	Apps []apps.App
}

// handleAppsDirectory serves GET /apps/, an Apache-style index page listing
// every installed application with a link to its /apps/{id}/ (ADR-0061).
// Off by default (deps.AppsDirectoryListingEnabled false): a plain 404,
// unchanged from before this route existed — an operator opts in via the
// apps.directory_listing.enabled config key or the
// PATCHCORD_APPS_DIRECTORY_LISTING_ENABLED environment variable
// (internal/config). Never behind withAdminAuth, for the same reason
// GET /apps/{id}/ isn't (see NewRouter's doc comment): it exposes nothing an
// end user's browser couldn't already reach one /apps/{id}/ URL at a time.
// Not part of the JSON API, so it carries no swag annotation.
func handleAppsDirectory(deps Deps) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		if !deps.AppsDirectoryListingEnabled {
			http.NotFound(w, r)
			return
		}

		list, err := apps.List(r.Context(), deps.DB)
		if err != nil {
			http.Error(w, "list apps: "+err.Error(), http.StatusInternalServerError)
			return
		}

		w.Header().Set("Content-Type", "text/html; charset=utf-8")
		if err := appsDirectoryTemplate.Execute(w, appsDirectoryView{Apps: list}); err != nil {
			// The response may already be partially written at this point
			// (Execute streams as it renders), so the best this can do is
			// log — a second header/status write would be a no-op or panic.
			deps.logger().Error("render apps directory page", "error", err)
		}
	}
}
