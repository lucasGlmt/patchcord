package api

import (
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"time"

	"github.com/lucasglmt/patchcord/internal/runs"
)

// runEventPayload is the JSON body of one SSE "data:" field.
type runEventPayload struct {
	RunID  string    `json:"run_id"`
	StepID string    `json:"step_id,omitempty"`
	Status string    `json:"status"`
	Error  string    `json:"error,omitempty"`
	Time   time.Time `json:"time"`
}

// handleRunEvents streams a run's status changes, and its steps', as
// Server-Sent Events until the run reaches a terminal status or the client
// disconnects (vision document, section 10.1, "/v1/events", and section
// 14, "diffusion en temps réel"; see ADR-0019 for why this is polling-based
// rather than a push from the runner).
// @Summary      Stream a run's events
// @Description  Streams a run's status changes, and its steps', as Server-Sent Events until the run reaches a terminal status or the client disconnects. The "event:" field is "run.<status>" or "step.<status>"; "data:" is a JSON-encoded event payload.
// @Tags         runs
// @Produce      text/event-stream
// @Param        id   path  string  true  "Run id"
// @Success      200  {string}  string  "text/event-stream"
// @Failure      404  {string}  string  "run not found"
// @Security     BearerAuth
// @Router       /runs/{id}/events [get]
func handleRunEvents(deps Deps) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		runID := r.PathValue("id")

		events, err := runs.WatchRun(r.Context(), deps.DB, runID)
		if errors.Is(err, runs.ErrRunNotFound) {
			http.Error(w, fmt.Sprintf("run %q was not found", runID), http.StatusNotFound)
			return
		}
		if err != nil {
			http.Error(w, "watch run: "+err.Error(), http.StatusInternalServerError)
			return
		}

		flusher, ok := w.(http.Flusher)
		if !ok {
			http.Error(w, "streaming not supported", http.StatusInternalServerError)
			return
		}

		w.Header().Set("Content-Type", "text/event-stream")
		w.Header().Set("Cache-Control", "no-cache")
		w.Header().Set("Connection", "keep-alive")
		w.WriteHeader(http.StatusOK)
		flusher.Flush()

		for event := range events {
			payload, err := json.Marshal(runEventPayload{
				RunID:  event.RunID,
				StepID: event.StepID,
				Status: event.Status,
				Error:  event.Error,
				Time:   event.Time,
			})
			if err != nil {
				continue
			}

			if _, err := fmt.Fprintf(w, "event: %s\ndata: %s\n\n", event.Name(), payload); err != nil {
				return
			}
			flusher.Flush()
		}
	}
}
