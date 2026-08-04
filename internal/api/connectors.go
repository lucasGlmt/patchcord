package api

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"time"

	"github.com/lucasglmt/patchcord/internal/connectors"
	"github.com/lucasglmt/patchcord/internal/plugins"
	"github.com/lucasglmt/patchcord/internal/secrets"
)

// ConnectorTester attempts a live connection through the installed plugin
// that declares a resolved connector's type, returning whether it succeeded
// and a human-readable message — the HTTP counterpart to `patchcord
// connector test`. *plugins.Supervisor satisfies this by duck typing, the
// same pattern Deps.Executor already uses (ADR-0021), so this package never
// imports a concrete plugin process type.
type ConnectorTester interface {
	TestConnector(ctx context.Context, c *connectors.ResolvedConnector) (ok bool, message string, err error)
}

// connectorSecretRef is the JSON shape of one connector's secret reference —
// its logical type/key, never a resolved value (ADR-0009, ADR-0020, ADR-0021).
type connectorSecretRef struct {
	Type string `json:"type"`
	Key  string `json:"key"`
}

// connectorSummary is the JSON shape of one connector, as returned by
// GET /v1/connectors and GET /v1/connectors/{id}.
type connectorSummary struct {
	ID         string                        `json:"id"`
	Type       string                        `json:"type"`
	Config     map[string]any                `json:"config,omitempty"`
	SecretRefs map[string]connectorSecretRef `json:"secret_refs,omitempty"`
	CreatedAt  time.Time                     `json:"created_at"`
}

func toConnectorSummary(c connectors.Connector) connectorSummary {
	var refs map[string]connectorSecretRef
	if len(c.SecretRefs) > 0 {
		refs = make(map[string]connectorSecretRef, len(c.SecretRefs))
		for name, ref := range c.SecretRefs {
			refs[name] = connectorSecretRef{Type: ref.Type, Key: ref.Key}
		}
	}
	return connectorSummary{ID: c.ID, Type: c.Type, Config: c.Config, SecretRefs: refs, CreatedAt: c.CreatedAt}
}

// createConnectorRequest is the JSON body of POST /v1/connectors.
type createConnectorRequest struct {
	ID         string                        `json:"id"`
	Type       string                        `json:"type"`
	Config     map[string]any                `json:"config"`
	SecretRefs map[string]connectorSecretRef `json:"secret_refs"`
}

// connectorTestResponse is the JSON shape of POST /v1/connectors/{id}/test.
type connectorTestResponse struct {
	OK      bool   `json:"ok"`
	Message string `json:"message,omitempty"`
}

// @Summary      List connectors
// @Description  Returns every recorded connector, ordered by id. Never includes a resolved secret value — only each secret reference's logical type/key.
// @Tags         connectors
// @Produce      json
// @Success      200  {array}  connectorSummary
// @Security     BearerAuth
// @Router       /connectors [get]
func handleListConnectors(deps Deps) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		list, err := connectors.List(r.Context(), deps.DB)
		if err != nil {
			http.Error(w, "list connectors: "+err.Error(), http.StatusInternalServerError)
			return
		}

		summaries := make([]connectorSummary, 0, len(list))
		for _, c := range list {
			summaries = append(summaries, toConnectorSummary(c))
		}

		writeJSON(w, http.StatusOK, summaries)
	}
}

// @Summary      Get a connector
// @Description  Returns one connector's configuration and secret references (never a resolved secret value).
// @Tags         connectors
// @Produce      json
// @Param        id   path  string  true  "Connector id"
// @Success      200  {object}  connectorSummary
// @Failure      404  {string}  string  "connector not found"
// @Security     BearerAuth
// @Router       /connectors/{id} [get]
func handleGetConnector(deps Deps) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		id := r.PathValue("id")

		c, err := connectors.Get(r.Context(), deps.DB, id)
		if errors.Is(err, connectors.ErrNotFound) {
			http.Error(w, fmt.Sprintf("connector %q was not found", id), http.StatusNotFound)
			return
		}
		if err != nil {
			http.Error(w, "get connector: "+err.Error(), http.StatusInternalServerError)
			return
		}

		writeJSON(w, http.StatusOK, toConnectorSummary(*c))
	}
}

// @Summary      Create a connector
// @Description  Creates a new connector: a persistent, named configuration for accessing an external system. type must match a connector type declared by an installed plugin's manifest.
// @Tags         connectors
// @Accept       json
// @Produce      json
// @Param        body  body  createConnectorRequest  true  "Connector to create"
// @Success      201  {object}  connectorSummary
// @Failure      400  {string}  string  "malformed request body, or the connector is invalid (empty id/type, unknown type, unsupported secret reference type)"
// @Failure      409  {string}  string  "a connector with this id already exists"
// @Security     BearerAuth
// @Router       /connectors [post]
func handleCreateConnector(deps Deps) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		var body createConnectorRequest
		if err := json.NewDecoder(r.Body).Decode(&body); err != nil && !errors.Is(err, io.EOF) {
			http.Error(w, "decode request body: "+err.Error(), http.StatusBadRequest)
			return
		}

		secretRefs := make(map[string]secrets.Reference, len(body.SecretRefs))
		for name, ref := range body.SecretRefs {
			secretRefs[name] = secrets.Reference{Type: ref.Type, Key: ref.Key}
		}

		knownTypes, err := plugins.KnownConnectorTypes(r.Context(), deps.DB)
		if err != nil {
			http.Error(w, "create connector: "+err.Error(), http.StatusInternalServerError)
			return
		}

		c, err := connectors.Create(r.Context(), deps.DB, body.ID, body.Type, body.Config, secretRefs, knownTypes)
		if errors.Is(err, connectors.ErrAlreadyExists) {
			http.Error(w, fmt.Sprintf("connector %q already exists", body.ID), http.StatusConflict)
			return
		}
		if errors.Is(err, connectors.ErrInvalidConnector) {
			http.Error(w, "create connector: "+err.Error(), http.StatusBadRequest)
			return
		}
		if err != nil {
			http.Error(w, "create connector: "+err.Error(), http.StatusInternalServerError)
			return
		}

		writeJSON(w, http.StatusCreated, toConnectorSummary(*c))
	}
}

// @Summary      Delete a connector
// @Description  Removes a connector. There is no update endpoint (ADR-0020) — a caller that wants to change one recreates it, which is exactly what the dashboard's "Modifier" does under the hood.
// @Tags         connectors
// @Param        id   path  string  true  "Connector id"
// @Success      204  "deleted"
// @Failure      404  {string}  string  "connector not found"
// @Security     BearerAuth
// @Router       /connectors/{id} [delete]
func handleDeleteConnector(deps Deps) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		id := r.PathValue("id")

		err := connectors.Delete(r.Context(), deps.DB, id)
		if errors.Is(err, connectors.ErrNotFound) {
			http.Error(w, fmt.Sprintf("connector %q was not found", id), http.StatusNotFound)
			return
		}
		if err != nil {
			http.Error(w, "delete connector: "+err.Error(), http.StatusInternalServerError)
			return
		}

		w.WriteHeader(http.StatusNoContent)
	}
}

// @Summary      Test a connector
// @Description  Resolves the connector's configuration and secrets, then asks the installed plugin that declares its type to actually attempt a connection. Unlike GET /connectors/{id}, this calls into a live plugin process. A connection attempt that runs but fails (e.g. wrong password) is reported as ok=false, not an HTTP error.
// @Tags         connectors
// @Produce      json
// @Param        id   path  string  true  "Connector id"
// @Success      200  {object}  connectorTestResponse
// @Failure      404  {string}  string  "connector not found"
// @Failure      500  {string}  string  "no connector tester configured, a secret failed to resolve, or the plugin call itself failed"
// @Security     BearerAuth
// @Router       /connectors/{id}/test [post]
func handleTestConnector(deps Deps) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		id := r.PathValue("id")

		if deps.ConnectorTester == nil {
			http.Error(w, "test connector: no connector tester configured", http.StatusInternalServerError)
			return
		}

		resolved, err := connectors.Resolve(r.Context(), deps.DB, id, deps.secrets())
		if errors.Is(err, connectors.ErrNotFound) {
			http.Error(w, fmt.Sprintf("connector %q was not found", id), http.StatusNotFound)
			return
		}
		if err != nil {
			http.Error(w, "test connector: "+err.Error(), http.StatusInternalServerError)
			return
		}

		ok, message, err := deps.ConnectorTester.TestConnector(r.Context(), resolved)
		if err != nil {
			http.Error(w, "test connector: "+err.Error(), http.StatusInternalServerError)
			return
		}

		writeJSON(w, http.StatusOK, connectorTestResponse{OK: ok, Message: message})
	}
}
