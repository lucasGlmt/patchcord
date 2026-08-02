package api

import (
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"log/slog"
	"net/http"
	"time"

	"github.com/lucasglmt/patchcord/internal/runs"
)

// runWorkflowRequest is the optional JSON body of POST
// /v1/workflows/{id}/run — the HTTP equivalent of `workflow run`'s
// --input/--binding flags.
type runWorkflowRequest struct {
	Inputs   map[string]any    `json:"inputs"`
	Bindings map[string]string `json:"bindings"`
}

// workflowSummary is the JSON shape of one installed workflow version, as
// returned by GET /v1/workflows.
type workflowSummary struct {
	ID          string    `json:"id"`
	Version     int       `json:"version"`
	InstalledAt time.Time `json:"installed_at"`
}

// @Summary      List installed workflows
// @Description  Returns every installed workflow version, most recently installed first. The same workflow id can appear more than once if multiple versions are installed (workflows are immutable once published — installing a new version never replaces an old one).
// @Tags         workflows
// @Produce      json
// @Success      200  {array}  workflowSummary
// @Router       /workflows [get]
func handleListWorkflows(deps Deps) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		versions, err := runs.ListWorkflows(r.Context(), deps.DB)
		if err != nil {
			http.Error(w, "list workflows: "+err.Error(), http.StatusInternalServerError)
			return
		}

		summaries := make([]workflowSummary, 0, len(versions))
		for _, v := range versions {
			summaries = append(summaries, workflowSummary{ID: v.WorkflowID, Version: v.Version, InstalledAt: v.InstalledAt})
		}

		writeJSON(w, http.StatusOK, summaries)
	}
}

// handleRunWorkflow starts a new run of the latest installed version of the
// named workflow and returns immediately with the run's id and initial
// status, without waiting for any step to execute (vision document, section
// 10.2's `client.workflows.run(...)`) — a client watches
// GET /v1/runs/{id}/events or polls GET /v1/runs/{id} for progress and the
// final result.
//
// It calls runs.Start synchronously (fast: only the run's creation is
// persisted), then runs.Continue in a background goroutine bound to
// deps.RunCtx, not this request's context — the run must keep going after
// this handler has already responded and the request is gone. A failure
// inside that background Continue call has no HTTP request left to report
// to, so it is only logged.
// @Summary      Trigger a workflow run
// @Description  Starts a new run of the latest installed version of the named workflow and returns immediately with its id and initial status — it does not wait for any step to execute. Watch GET /runs/{id}/events or poll GET /runs/{id} for progress and the final result.
// @Tags         workflows
// @Accept       json
// @Produce      json
// @Param        id    path  string               true  "Workflow id"
// @Param        body  body  runWorkflowRequest  false  "Inputs and connector bindings (both optional, default to none)"
// @Success      202  {object}  runSummary
// @Failure      400  {string}  string  "malformed request body"
// @Failure      404  {string}  string  "workflow not found"
// @Failure      500  {string}  string  "no action executor configured, or a persistence failure"
// @Router       /workflows/{id}/run [post]
func handleRunWorkflow(deps Deps) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		workflowID := r.PathValue("id")

		var body runWorkflowRequest
		if err := json.NewDecoder(r.Body).Decode(&body); err != nil && !errors.Is(err, io.EOF) {
			http.Error(w, "decode request body: "+err.Error(), http.StatusBadRequest)
			return
		}

		if deps.Executor == nil {
			http.Error(w, "run workflow: no action executor configured", http.StatusInternalServerError)
			return
		}

		def, run, err := runs.Start(r.Context(), deps.DB, workflowID, body.Inputs)
		if errors.Is(err, runs.ErrWorkflowNotFound) {
			http.Error(w, fmt.Sprintf("workflow %q was not found", workflowID), http.StatusNotFound)
			return
		}
		if err != nil {
			http.Error(w, "run workflow: "+err.Error(), http.StatusInternalServerError)
			return
		}

		go func() {
			if err := runs.Continue(deps.runCtx(), deps.DB, deps.Executor, def, run, body.Inputs, body.Bindings, runs.ExecuteOptions{}); err != nil {
				deps.logger().Error("continue run",
					slog.String("run_id", run.ID),
					slog.String("workflow_id", workflowID),
					slog.String("error", err.Error()))
			}
		}()

		writeJSON(w, http.StatusAccepted, toRunSummary(run, nil))
	}
}
