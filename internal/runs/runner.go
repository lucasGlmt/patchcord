package runs

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"time"

	"github.com/lucasglmt/patchcord/internal/workflow"
)

// ActionExecutor runs one action and returns its output. It is the only
// thing the runner needs to actually execute a step, which keeps this
// package free of any dependency on how plugins are launched or
// supervised — internal/plugins.Supervisor satisfies this interface.
type ActionExecutor interface {
	ExecuteAction(ctx context.Context, actionID string, input map[string]any) (map[string]any, error)
}

// DefaultStepTimeout bounds how long a single step's action call may run
// when ExecuteOptions.StepTimeout is left zero.
const DefaultStepTimeout = 30 * time.Second

// persistTimeout bounds every bookkeeping write Execute makes (creating the
// run, recording each step's progress, the final status). It is
// deliberately not derived from the caller's ctx: once a run needs to
// record that it failed or was cancelled, that write must still go
// through even though ctx is exactly what triggered the cancellation —
// otherwise the run would be left stuck as "running" in the database
// forever. Only the actual action call is bound to the caller's ctx.
const persistTimeout = 10 * time.Second

// ExecuteOptions controls how Execute runs a workflow.
type ExecuteOptions struct {
	// StepTimeout bounds each individual step's action call. Defaults to
	// DefaultStepTimeout when zero. A step that times out fails the run
	// (it is not treated as a user-requested cancellation).
	StepTimeout time.Duration
}

func (o ExecuteOptions) withDefaults() ExecuteOptions {
	if o.StepTimeout <= 0 {
		o.StepTimeout = DefaultStepTimeout
	}
	return o
}

// stepFailureStatus reports the terminal status a step should record for
// err: cancelled if err stems from the run's own context being cancelled,
// failed for anything else (including a step's own timeout, which is a
// failure of that step, not a cancellation of the run).
func stepFailureStatus(err error) workflow.StepStatus {
	if errors.Is(err, context.Canceled) {
		return workflow.StepCancelled
	}
	return workflow.StepFailed
}

// Execute runs the latest installed version of workflowID with the given
// inputs against executor, persisting the run and every step's progress as
// it happens, and returns the completed run.
//
// Steps run sequentially; the first one to fail — including timing out, or
// ctx being cancelled — stops the run. Every step that never got a chance
// to run is recorded as skipped, so no step is left dangling in "pending".
func Execute(ctx context.Context, db *sql.DB, executor ActionExecutor, workflowID string, inputs map[string]any, opts ExecuteOptions) (*Run, error) {
	opts = opts.withDefaults()

	pctx, cancelPersist := context.WithTimeout(context.Background(), persistTimeout)
	defer cancelPersist()

	def, err := LatestWorkflow(pctx, db, workflowID)
	if err != nil {
		return nil, err
	}

	run, err := createRun(pctx, db, def, inputs)
	if err != nil {
		return nil, err
	}

	if err := updateRunStatus(pctx, db, run, workflow.RunRunning, nil, nil); err != nil {
		return nil, err
	}

	stepOutputs := make(map[string]map[string]any, len(def.Steps))
	var runOutputs map[string]any
	var runErr error
	nextPending := 0

	for i, step := range def.Steps {
		if ctx.Err() != nil {
			runErr = ctx.Err()
			break
		}

		resolvedInput, resolveErr := workflow.ResolveInputs(step.With, workflow.ExprContext{
			Inputs:      inputs,
			StepOutputs: stepOutputs,
		})
		if resolveErr != nil {
			// A step only reaches a terminal state by way of "running" —
			// even one that never got to call its action, since "pending"
			// means it was never attempted at all.
			runErr = fmt.Errorf("step %q: %w", step.ID, resolveErr)
			if err := updateStepStatus(pctx, db, run.ID, step.ID, workflow.StepPending, workflow.StepRunning, nil, nil, nil); err != nil {
				return nil, err
			}
			if err := updateStepStatus(pctx, db, run.ID, step.ID, workflow.StepRunning, stepFailureStatus(runErr), nil, nil, runErr); err != nil {
				return nil, err
			}
			nextPending = i + 1
			break
		}

		if err := updateStepStatus(pctx, db, run.ID, step.ID, workflow.StepPending, workflow.StepRunning, resolvedInput, nil, nil); err != nil {
			return nil, err
		}

		stepCtx, cancel := context.WithTimeout(ctx, opts.StepTimeout)
		output, execErr := executor.ExecuteAction(stepCtx, step.Uses, resolvedInput)
		cancel()

		if execErr != nil {
			runErr = fmt.Errorf("step %q: %w", step.ID, execErr)
			if err := updateStepStatus(pctx, db, run.ID, step.ID, workflow.StepRunning, stepFailureStatus(runErr), resolvedInput, nil, execErr); err != nil {
				return nil, err
			}
			nextPending = i + 1
			break
		}

		if err := updateStepStatus(pctx, db, run.ID, step.ID, workflow.StepRunning, workflow.StepSucceeded, resolvedInput, output, nil); err != nil {
			return nil, err
		}

		stepOutputs[step.ID] = output
		runOutputs = output
		nextPending = i + 1
	}

	finalStatus := workflow.RunSucceeded
	if runErr != nil {
		finalStatus = workflow.RunFailed
		// Only the run's own context being cancelled — typically a signal
		// caught by the CLI — counts as a user-requested cancellation. A
		// step timing out on its own derived context is a failure of that
		// step, not a cancellation of the run.
		skippedStatus := workflow.StepSkipped
		if errors.Is(runErr, context.Canceled) {
			finalStatus = workflow.RunCancelled
			skippedStatus = workflow.StepCancelled
		}

		for _, step := range def.Steps[nextPending:] {
			if err := updateStepStatus(pctx, db, run.ID, step.ID, workflow.StepPending, skippedStatus, nil, nil, nil); err != nil {
				return nil, err
			}
		}
	}

	if err := updateRunStatus(pctx, db, run, finalStatus, runOutputs, runErr); err != nil {
		return nil, err
	}

	return run, nil
}
