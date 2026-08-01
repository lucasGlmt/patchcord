package runs

import (
	"context"
	"database/sql"
	"errors"
	"fmt"

	"github.com/lucasglmt/patchcord/internal/workflow"
)

// ActionExecutor runs one action and returns its output. It is the only
// thing the runner needs to actually execute a step, which keeps this
// package free of any dependency on how plugins are launched or
// supervised — internal/plugins.Supervisor satisfies this interface.
type ActionExecutor interface {
	ExecuteAction(ctx context.Context, actionID string, input map[string]any) (map[string]any, error)
}

// Execute runs the latest installed version of workflowID with the given
// inputs against executor, persisting the run and every step's progress as
// it happens, and returns the completed run.
//
// Steps run sequentially; the first one to fail (or ctx being cancelled)
// stops the run. Every step that never got a chance to run is recorded as
// skipped, so no step is left dangling in "pending".
func Execute(ctx context.Context, db *sql.DB, executor ActionExecutor, workflowID string, inputs map[string]any) (*Run, error) {
	def, err := LatestWorkflow(ctx, db, workflowID)
	if err != nil {
		return nil, err
	}

	run, err := createRun(ctx, db, def, inputs)
	if err != nil {
		return nil, err
	}

	if err := updateRunStatus(ctx, db, run, workflow.RunRunning, nil, nil); err != nil {
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
			// A step only reaches "failed" by way of "running" — even one
			// that never got to call its action, since "pending" means it
			// was never attempted at all.
			runErr = fmt.Errorf("step %q: %w", step.ID, resolveErr)
			if err := updateStepStatus(ctx, db, run.ID, step.ID, workflow.StepPending, workflow.StepRunning, nil, nil, nil); err != nil {
				return nil, err
			}
			if err := updateStepStatus(ctx, db, run.ID, step.ID, workflow.StepRunning, workflow.StepFailed, nil, nil, runErr); err != nil {
				return nil, err
			}
			nextPending = i + 1
			break
		}

		if err := updateStepStatus(ctx, db, run.ID, step.ID, workflow.StepPending, workflow.StepRunning, resolvedInput, nil, nil); err != nil {
			return nil, err
		}

		output, execErr := executor.ExecuteAction(ctx, step.Uses, resolvedInput)
		if execErr != nil {
			runErr = fmt.Errorf("step %q: %w", step.ID, execErr)
			if err := updateStepStatus(ctx, db, run.ID, step.ID, workflow.StepRunning, workflow.StepFailed, resolvedInput, nil, execErr); err != nil {
				return nil, err
			}
			nextPending = i + 1
			break
		}

		if err := updateStepStatus(ctx, db, run.ID, step.ID, workflow.StepRunning, workflow.StepSucceeded, resolvedInput, output, nil); err != nil {
			return nil, err
		}

		stepOutputs[step.ID] = output
		runOutputs = output
		nextPending = i + 1
	}

	finalStatus := workflow.RunSucceeded
	if runErr != nil {
		finalStatus = workflow.RunFailed
		if errors.Is(runErr, context.Canceled) || errors.Is(runErr, context.DeadlineExceeded) {
			finalStatus = workflow.RunCancelled
		}

		for _, step := range def.Steps[nextPending:] {
			if err := updateStepStatus(ctx, db, run.ID, step.ID, workflow.StepPending, workflow.StepSkipped, nil, nil, nil); err != nil {
				return nil, err
			}
		}
	}

	if err := updateRunStatus(ctx, db, run, finalStatus, runOutputs, runErr); err != nil {
		return nil, err
	}

	return run, nil
}
