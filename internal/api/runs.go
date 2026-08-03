package api

import (
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"time"

	"github.com/lucasglmt/patchcord/internal/runs"
)

// runSummary is the JSON shape of one run, shared by handleRunWorkflow's
// 202 response and handleGetRun's 200 response.
type runSummary struct {
	ID              string         `json:"id"`
	WorkflowID      string         `json:"workflow_id"`
	WorkflowVersion int            `json:"workflow_version"`
	Status          string         `json:"status"`
	Inputs          map[string]any `json:"inputs,omitempty"`
	Outputs         map[string]any `json:"outputs,omitempty"`
	Error           string         `json:"error,omitempty"`
	CreatedAt       time.Time      `json:"created_at"`
	StartedAt       *time.Time     `json:"started_at,omitempty"`
	FinishedAt      *time.Time     `json:"finished_at,omitempty"`
	Steps           []runStep      `json:"steps,omitempty"`
}

// runStep is the JSON shape of one step within a runSummary.
type runStep struct {
	ID         string         `json:"id"`
	Status     string         `json:"status"`
	Input      map[string]any `json:"input,omitempty"`
	Output     map[string]any `json:"output,omitempty"`
	Error      string         `json:"error,omitempty"`
	StartedAt  *time.Time     `json:"started_at,omitempty"`
	FinishedAt *time.Time     `json:"finished_at,omitempty"`
}

func toRunSummary(run *runs.Run, steps []runs.Step) runSummary {
	summary := runSummary{
		ID:              run.ID,
		WorkflowID:      run.WorkflowID,
		WorkflowVersion: run.WorkflowVersion,
		Status:          string(run.Status),
		Inputs:          run.Inputs,
		Outputs:         run.Outputs,
		Error:           run.Error,
		CreatedAt:       run.CreatedAt,
		StartedAt:       run.StartedAt,
		FinishedAt:      run.FinishedAt,
	}
	for _, step := range steps {
		summary.Steps = append(summary.Steps, runStep{
			ID:         step.StepID,
			Status:     string(step.Status),
			Input:      step.Input,
			Output:     step.Output,
			Error:      step.Error,
			StartedAt:  step.StartedAt,
			FinishedAt: step.FinishedAt,
		})
	}
	return summary
}

func writeJSON(w http.ResponseWriter, statusCode int, body any) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(statusCode)
	_ = json.NewEncoder(w).Encode(body)
}

// handleGetRun returns one run's current status, outputs and steps
// (vision document, section 10.1, "/v1/runs") — the counterpart to
// handleRunWorkflow's immediate response, letting a client poll a run or
// fetch its final result once handleRunEvents' stream has closed.
// @Summary      Get a run
// @Description  Returns one run's current status, outputs and steps — the counterpart to POST /workflows/{id}/run's immediate response, letting a client poll a run or fetch its final result once GET /runs/{id}/events has closed.
// @Tags         runs
// @Produce      json
// @Param        id   path  string  true  "Run id"
// @Success      200  {object}  runSummary
// @Failure      404  {string}  string  "run not found"
// @Security     BearerAuth
// @Router       /runs/{id} [get]
func handleGetRun(deps Deps) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		id := r.PathValue("id")

		run, steps, err := runs.GetRun(r.Context(), deps.DB, id)
		if errors.Is(err, runs.ErrRunNotFound) {
			http.Error(w, fmt.Sprintf("run %q was not found", id), http.StatusNotFound)
			return
		}
		if err != nil {
			http.Error(w, "get run: "+err.Error(), http.StatusInternalServerError)
			return
		}

		writeJSON(w, http.StatusOK, toRunSummary(run, steps))
	}
}

// @Summary      List runs
// @Description  Returns every recorded run, most recently created first.
// @Tags         runs
// @Produce      json
// @Param        workflow_id  query  string  false  "Restrict to runs of this workflow id"
// @Success      200  {array}  runSummary
// @Security     BearerAuth
// @Router       /runs [get]
func handleListRuns(deps Deps) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		workflowID := r.URL.Query().Get("workflow_id")

		runList, err := runs.ListRuns(r.Context(), deps.DB, workflowID)
		if err != nil {
			http.Error(w, "list runs: "+err.Error(), http.StatusInternalServerError)
			return
		}

		summaries := make([]runSummary, 0, len(runList))
		for _, run := range runList {
			summaries = append(summaries, toRunSummary(&run, nil))
		}

		writeJSON(w, http.StatusOK, summaries)
	}
}

// handleCancelRun marks a run still queued or running as cancelled. It does
// not interrupt a run actively executing within this same process — a
// background run started by handleRunWorkflow keeps going until its next
// persistence checkpoint, where the state machine then rejects its next
// transition attempt and it stops without corrupting anything (see
// ADR-0024) — only its recorded status changes immediately.
// @Summary      Cancel a run
// @Description  Marks a run still queued or running as cancelled. Does not interrupt a run actively executing within this same process — it only flips its recorded status; an in-flight step keeps running until its next persistence checkpoint, where it then stops cleanly.
// @Tags         runs
// @Produce      json
// @Param        id   path  string  true  "Run id"
// @Success      200  {object}  runSummary
// @Failure      404  {string}  string  "run not found"
// @Failure      409  {string}  string  "run has already finished"
// @Security     BearerAuth
// @Router       /runs/{id}/cancel [post]
func handleCancelRun(deps Deps) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		id := r.PathValue("id")

		err := runs.CancelRun(r.Context(), deps.DB, id)
		if errors.Is(err, runs.ErrRunNotFound) {
			http.Error(w, fmt.Sprintf("run %q was not found", id), http.StatusNotFound)
			return
		}
		if errors.Is(err, runs.ErrRunNotCancellable) {
			http.Error(w, fmt.Sprintf("run %q has already finished", id), http.StatusConflict)
			return
		}
		if err != nil {
			http.Error(w, "cancel run: "+err.Error(), http.StatusInternalServerError)
			return
		}

		run, steps, err := runs.GetRun(r.Context(), deps.DB, id)
		if err != nil {
			http.Error(w, "get run: "+err.Error(), http.StatusInternalServerError)
			return
		}

		writeJSON(w, http.StatusOK, toRunSummary(run, steps))
	}
}
