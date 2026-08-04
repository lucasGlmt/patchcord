package api

import (
	"crypto/subtle"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"

	"github.com/lucasglmt/patchcord/internal/runs"
)

// webhookTokenHeader is the header an inbound webhook request must present
// its shared secret in, compared in constant time against the resolved
// value of the triggered workflow's trigger.secret_ref (ADR-0037).
const webhookTokenHeader = "X-Patchcord-Webhook-Token"

// handleWebhookTrigger starts a new run of the latest installed version of
// the named workflow when it declares a "webhook" trigger — the same
// runs.Start/runs.Continue path handleRunWorkflow uses for a manual run,
// just triggered by an inbound HTTP request instead of an authenticated
// client. Unlike every other route, it is never gated by withAdminAuth: an
// external sender (GitHub, Stripe, a custom script...) will never hold an
// admin token. Instead, the workflow's own trigger.secret_ref protects it,
// checked against the X-Patchcord-Webhook-Token header — see ADR-0037.
//
// The request's raw JSON body becomes the run's inputs directly, unlike
// POST /workflows/{id}/run's {"inputs": ..., "bindings": ...} envelope: no
// real webhook sender wraps its payload that way, and there is no bindings
// map here at all — workflow.Validate already rejects a connector-bound
// step on a "webhook" trigger for exactly that reason.
// @Summary      Trigger a workflow via its webhook
// @Description  Starts a new run of the latest installed version of the named workflow, if it declares a "webhook" trigger — the request body's top-level JSON object becomes the run's inputs directly, not wrapped in {"inputs": ...}. Requires "X-Patchcord-Webhook-Token: <the trigger's resolved secret_ref>" — never an admin token.
// @Tags         webhooks
// @Accept       json
// @Produce      json
// @Param        id    path  string  true  "Workflow id"
// @Success      202  {object}  runSummary
// @Failure      400  {string}  string  "the body isn't a JSON object, or its fields don't satisfy the workflow's declared input schema"
// @Failure      401  {string}  string  "missing or incorrect X-Patchcord-Webhook-Token"
// @Failure      404  {string}  string  "workflow not found, or its trigger isn't \"webhook\""
// @Failure      500  {string}  string  "no action executor configured, the secret_ref doesn't resolve, or a persistence failure"
// @Router       /webhooks/{id} [post]
func handleWebhookTrigger(deps Deps) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		workflowID := r.PathValue("id")

		def, err := runs.LatestWorkflow(r.Context(), deps.DB, workflowID)
		if errors.Is(err, runs.ErrWorkflowNotFound) {
			http.Error(w, fmt.Sprintf("no webhook trigger found for workflow %q", workflowID), http.StatusNotFound)
			return
		}
		if err != nil {
			http.Error(w, "webhook trigger: "+err.Error(), http.StatusInternalServerError)
			return
		}
		if def.Trigger.Type != "webhook" {
			http.Error(w, fmt.Sprintf("no webhook trigger found for workflow %q", workflowID), http.StatusNotFound)
			return
		}

		expected, err := deps.secrets().Resolve(r.Context(), def.Trigger.SecretRef)
		if err != nil {
			http.Error(w, "webhook trigger: resolve secret_ref: "+err.Error(), http.StatusInternalServerError)
			return
		}

		got := r.Header.Get(webhookTokenHeader)
		if got == "" || subtle.ConstantTimeCompare([]byte(got), []byte(expected)) != 1 {
			http.Error(w, fmt.Sprintf("webhook trigger: missing or incorrect %s", webhookTokenHeader), http.StatusUnauthorized)
			return
		}

		var inputs map[string]any
		if err := json.NewDecoder(r.Body).Decode(&inputs); err != nil && !errors.Is(err, io.EOF) {
			http.Error(w, "webhook trigger: request body must be a JSON object: "+err.Error(), http.StatusBadRequest)
			return
		}

		startRunAndRespond(w, r, deps, workflowID, inputs, map[string]string{})
	}
}
