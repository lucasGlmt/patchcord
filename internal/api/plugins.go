package api

import (
	"net/http"

	"github.com/lucasglmt/patchcord/internal/plugins"
)

// pluginSummary is the JSON shape of one installed plugin, as returned by
// GET /v1/plugins — the connector types, action ids and permissions its
// manifest declares, not its executable path or protocol version, which
// are operational details no API client needs.
type pluginSummary struct {
	ID          string   `json:"id"`
	Version     string   `json:"version"`
	Connectors  []string `json:"connectors,omitempty"`
	Actions     []string `json:"actions,omitempty"`
	Permissions []string `json:"permissions,omitempty"`
}

func toPluginSummary(entry plugins.CatalogEntry) pluginSummary {
	return pluginSummary{
		ID:          entry.PluginID,
		Version:     entry.Version,
		Connectors:  plugins.ConnectorTypes(entry.Connectors),
		Actions:     plugins.ActionIDs(entry.Actions),
		Permissions: entry.Permissions,
	}
}

// @Summary      List installed plugins
// @Description  Returns every installed plugin and the connector types/action ids/permissions its manifest declares — enough for a client to build a connector-type picker without a copy of the CLI's own catalog logic.
// @Tags         plugins
// @Produce      json
// @Success      200  {array}  pluginSummary
// @Security     BearerAuth
// @Router       /plugins [get]
func handleListPlugins(deps Deps) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		list, err := plugins.List(r.Context(), deps.DB)
		if err != nil {
			http.Error(w, "list plugins: "+err.Error(), http.StatusInternalServerError)
			return
		}

		summaries := make([]pluginSummary, 0, len(list))
		for _, entry := range list {
			summaries = append(summaries, toPluginSummary(entry))
		}

		writeJSON(w, http.StatusOK, summaries)
	}
}
