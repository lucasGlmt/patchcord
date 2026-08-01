package runs

import (
	"context"
	"database/sql"
	"time"

	"github.com/lucasglmt/patchcord/internal/workflow"
)

// watchPollInterval is how often WatchRun re-reads a run's state from the
// database to detect changes. It is a package variable, not a const, so
// tests can shrink it instead of waiting on the real interval.
var watchPollInterval = 250 * time.Millisecond

// Event is one observed status change for a run or one of its steps,
// delivered by WatchRun. It mirrors the event log sketched in the vision
// document (section 14).
type Event struct {
	RunID  string
	StepID string // empty for a run-level event
	Status string
	Error  string
	Time   time.Time
}

// Name returns the event's dotted name, e.g. "run.running" or
// "step.succeeded" — suitable as an SSE "event:" field or a log line.
func (e Event) Name() string {
	if e.StepID == "" {
		return "run." + e.Status
	}
	return "step." + e.Status
}

// WatchRun returns a channel delivering a status Event each time runID or
// one of its steps moves to a new status, starting from an empty baseline
// so a client connecting mid-run still gets the status each entity
// currently holds instead of only the ones still to come. It closes the
// channel once the run reaches a terminal status or ctx is cancelled.
//
// Because the database only ever holds the current status (there is no
// event log to replay — see the vision document, section 14, "Event log"),
// a client that connects after a fast run has already finished only
// observes each entity's single final status, not the intermediate ones it
// passed through; a client watching a run already in flight observes every
// transition from the moment it connects onward.
//
// Patchcord runs a workflow synchronously within a single process from
// start to finish (ADR-0018), so there is no in-process event bus another
// process — such as the agent's HTTP server — could subscribe to. WatchRun
// polls the database instead, the only channel shared between a `workflow
// run` process and anyone watching it (see ADR-0019).
//
// It returns ErrRunNotFound immediately if no such run exists.
func WatchRun(ctx context.Context, db *sql.DB, runID string) (<-chan Event, error) {
	if _, _, err := GetRun(ctx, db, runID); err != nil {
		return nil, err
	}

	events := make(chan Event)

	go func() {
		defer close(events)

		var lastRunStatus workflow.RunStatus
		lastStepStatus := make(map[string]workflow.StepStatus)

		ticker := time.NewTicker(watchPollInterval)
		defer ticker.Stop()

		for {
			run, steps, err := GetRun(ctx, db, runID)
			if err != nil {
				return
			}

			if run.Status != lastRunStatus {
				if !sendEvent(ctx, events, Event{RunID: run.ID, Status: string(run.Status), Error: run.Error, Time: time.Now()}) {
					return
				}
				lastRunStatus = run.Status
			}

			for _, step := range steps {
				if lastStepStatus[step.StepID] == step.Status {
					continue
				}
				if !sendEvent(ctx, events, Event{RunID: run.ID, StepID: step.StepID, Status: string(step.Status), Error: step.Error, Time: time.Now()}) {
					return
				}
				lastStepStatus[step.StepID] = step.Status
			}

			if isTerminalRunStatus(run.Status) {
				return
			}

			select {
			case <-ctx.Done():
				return
			case <-ticker.C:
			}
		}
	}()

	return events, nil
}

// sendEvent delivers e to events, unless ctx is cancelled first. It reports
// whether the send happened.
func sendEvent(ctx context.Context, events chan<- Event, e Event) bool {
	select {
	case events <- e:
		return true
	case <-ctx.Done():
		return false
	}
}

func isTerminalRunStatus(s workflow.RunStatus) bool {
	return s == workflow.RunSucceeded || s == workflow.RunFailed || s == workflow.RunCancelled
}
