package runs

import (
	"context"
	"database/sql"
	"errors"
	"testing"
	"time"

	"github.com/lucasglmt/patchcord/internal/workflow"
)

// withFastPolling shrinks watchPollInterval for the duration of a test, so
// tests observe polled events without waiting on the real-world interval.
func withFastPolling(t *testing.T) {
	t.Helper()
	original := watchPollInterval
	watchPollInterval = 5 * time.Millisecond
	t.Cleanup(func() { watchPollInterval = original })
}

// collectEvents drains events until it closes or the deadline elapses,
// failing the test in the latter case.
func collectEvents(t *testing.T, events <-chan Event, deadline time.Duration) []Event {
	t.Helper()

	var got []Event
	timeout := time.After(deadline)
	for {
		select {
		case e, ok := <-events:
			if !ok {
				return got
			}
			got = append(got, e)
		case <-timeout:
			t.Fatalf("WatchRun did not close its channel within %s; events so far: %+v", deadline, got)
			return nil
		}
	}
}

func TestWatchRun_UnknownRunReturnsErrRunNotFound(t *testing.T) {
	db := openTestDB(t)

	_, err := WatchRun(context.Background(), db, "does-not-exist")
	if !errors.Is(err, ErrRunNotFound) {
		t.Fatalf("WatchRun() error = %v, want ErrRunNotFound", err)
	}
}

func TestWatchRun_ObservesEachStatusChangeInOrder(t *testing.T) {
	withFastPolling(t)
	db := openTestDB(t)
	def := installTestWorkflow(t, db, helloWorkflow)
	stepID := def.Steps[0].ID

	run, err := createRun(context.Background(), db, def, map[string]any{"value": "hello"})
	if err != nil {
		t.Fatalf("createRun() error = %v", err)
	}

	events, err := WatchRun(context.Background(), db, run.ID)
	if err != nil {
		t.Fatalf("WatchRun() error = %v", err)
	}

	// Drive the run/step lifecycle by hand, one transition at a time, with
	// gaps well above watchPollInterval so each one lands on its own poll
	// — this is what exercises WatchRun's ability to observe a run that is
	// still in flight, as opposed to one already finished by the time a
	// client connects (see the package doc comment on WatchRun).
	const gap = 20 * time.Millisecond
	go func() {
		time.Sleep(gap)
		if err := updateRunStatus(context.Background(), db, run, workflow.RunRunning, nil, nil, nil); err != nil {
			t.Errorf("updateRunStatus(running) error = %v", err)
		}

		time.Sleep(gap)
		if err := updateStepStatus(context.Background(), db, run.ID, stepID, workflow.StepPending, workflow.StepRunning, nil, nil, nil, nil); err != nil {
			t.Errorf("updateStepStatus(running) error = %v", err)
		}

		time.Sleep(gap)
		output := map[string]any{"value": "HELLO"}
		if err := updateStepStatus(context.Background(), db, run.ID, stepID, workflow.StepRunning, workflow.StepSucceeded, nil, output, nil, nil); err != nil {
			t.Errorf("updateStepStatus(succeeded) error = %v", err)
		}

		time.Sleep(gap)
		if err := updateRunStatus(context.Background(), db, run, workflow.RunSucceeded, output, nil, nil); err != nil {
			t.Errorf("updateRunStatus(succeeded) error = %v", err)
		}
	}()

	got := collectEvents(t, events, 2*time.Second)

	wantNames := []string{"run.queued", "step.pending", "run.running", "step.running", "step.succeeded", "run.succeeded"}
	if len(got) != len(wantNames) {
		t.Fatalf("got %d events, want %d: %+v", len(got), len(wantNames), got)
	}
	for i, name := range wantNames {
		if got[i].Name() != name {
			t.Fatalf("event %d name = %q, want %q (all events: %+v)", i, got[i].Name(), name, got)
		}
		if got[i].RunID != run.ID {
			t.Fatalf("event %d RunID = %q, want %q", i, got[i].RunID, run.ID)
		}
	}
}

func TestWatchRun_ClosesWhenContextIsCancelled(t *testing.T) {
	withFastPolling(t)
	db := openTestDB(t)
	installTestWorkflow(t, db, helloWorkflow)

	// A run stuck in "queued" never reaches a terminal status on its own,
	// so the only way WatchRun's channel closes here is ctx cancellation.
	run, _, err := GetRun(context.Background(), db, mustCreateQueuedRun(t, db))
	if err != nil {
		t.Fatalf("GetRun() error = %v", err)
	}

	ctx, cancel := context.WithCancel(context.Background())
	events, err := WatchRun(ctx, db, run.ID)
	if err != nil {
		t.Fatalf("WatchRun() error = %v", err)
	}

	// Drain the replay of the queued run/step events before cancelling.
	select {
	case <-events:
	case <-time.After(time.Second):
		t.Fatal("did not receive the initial replay event")
	}

	cancel()

	select {
	case _, ok := <-events:
		if ok {
			// Draining further queued events is fine; eventually the
			// channel must close.
			for range events {
			}
		}
	case <-time.After(time.Second):
		t.Fatal("WatchRun did not close its channel after ctx was cancelled")
	}
}

// mustCreateQueuedRun installs and creates a run of helloWorkflow that is
// left in the "queued" state (createRun's initial status), without running
// it — used to exercise WatchRun against a run that never reaches a
// terminal status.
func mustCreateQueuedRun(t *testing.T, db *sql.DB) string {
	t.Helper()

	def, err := LatestWorkflow(context.Background(), db, "hello_patchcord")
	if err != nil {
		t.Fatalf("LatestWorkflow() error = %v", err)
	}

	run, err := createRun(context.Background(), db, def, map[string]any{"value": "hello"})
	if err != nil {
		t.Fatalf("createRun() error = %v", err)
	}
	if run.Status != workflow.RunQueued {
		t.Fatalf("run status = %s, want %s", run.Status, workflow.RunQueued)
	}

	return run.ID
}
