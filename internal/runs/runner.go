package runs

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"time"

	"github.com/lucasglmt/patchcord/internal/connectors"
	"github.com/lucasglmt/patchcord/internal/metrics"
	"github.com/lucasglmt/patchcord/internal/secrets"
	"github.com/lucasglmt/patchcord/internal/workflow"
)

// ActionExecutor runs one action and returns its output. It is the only
// thing the runner needs to actually execute a step, which keeps this
// package free of any dependency on how plugins are launched or
// supervised — internal/plugins.Supervisor satisfies this interface.
// connector is nil unless the step bound one (see workflow.Step.Connector).
type ActionExecutor interface {
	ExecuteAction(ctx context.Context, actionID string, input map[string]any, connector *connectors.ResolvedConnector) (map[string]any, error)
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
	// (it is not treated as a user-requested cancellation). For a foreach
	// step, this budget covers the whole step — every item's action call
	// combined, not one budget per item — so a long list needs a StepTimeout
	// sized for all of its iterations together.
	StepTimeout time.Duration
	// Secrets resolves a step's bound connector's secret references.
	// Defaults to secrets.EnvStore{} when nil, so existing callers that
	// build ExecuteOptions{} directly (or only set StepTimeout) keep
	// resolving "env" references exactly as before secrets.MultiStore
	// existed.
	Secrets secrets.Store
	// Metrics records run/step transitions as they happen (run and step
	// counts by status, duration histograms, the active-runs gauge).
	// Defaults to a private, unscraped metrics.Registry when nil, so
	// existing callers keep working exactly as before metrics existed —
	// see ADR-0070.
	Metrics *metrics.Registry
}

func (o ExecuteOptions) withDefaults() ExecuteOptions {
	if o.StepTimeout <= 0 {
		o.StepTimeout = DefaultStepTimeout
	}
	if o.Secrets == nil {
		o.Secrets = secrets.EnvStore{}
	}
	o.Metrics = metrics.OrNoop(o.Metrics)
	return o
}

// resolveStepConnector resolves connector against exprCtx and, if it names
// one, looks it up — shared by the regular step path and the foreach path,
// since a step's bound connector is resolved once regardless of how many
// times (zero, for an empty connector) its action ends up being called.
func resolveStepConnector(ctx context.Context, db *sql.DB, connector string, exprCtx workflow.ExprContext, store secrets.Store) (*connectors.ResolvedConnector, error) {
	connectorID, err := workflow.ResolveConnector(connector, exprCtx)
	if err != nil || connectorID == "" {
		return nil, err
	}
	return connectors.Resolve(ctx, db, connectorID, store)
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

// Start creates a new run of workflowID's latest installed version and
// transitions it straight to Running, then returns immediately — it does
// not execute any step. Call Continue next to actually run them.
//
// inputs is resolved against def.Inputs (workflow.PrepareInputs) before
// anything is persisted: defaults are filled in, values coming from a
// string-only source (the CLI's --input flags) are coerced to their
// declared type, and a missing required input or an undeclared key fails
// fast, before a run row even exists. The returned map is this resolved
// result, not the caller's original inputs — callers must pass it, not
// their own inputs, to Continue, so step expression resolution
// (${{ workflow.inputs.<key> }}) sees the same coerced/defaulted values
// that were persisted as the run's inputs.
//
// Split from the step-running loop (Continue) so a caller that must not
// block for the run's entire duration — the HTTP API's
// POST /v1/workflows/{id}/run, which needs to answer with the new run's id
// right away so a client can start watching /v1/runs/{id}/events — can call
// Start synchronously and Continue in a background goroutine (see
// internal/api's handleRunWorkflow). Execute composes both for callers (the
// CLI, most tests) that do want to block until completion.
//
// Like Continue's own bookkeeping writes, Start's persistence is bounded by
// persistTimeout but deliberately not derived from ctx: a caller whose ctx
// is cancelled the instant it calls Start (e.g. an HTTP client that
// disconnects immediately) still gets a consistently created, Running run
// row rather than an ambiguous half-created one — the step loop in Continue
// is what turns a cancelled ctx into a properly recorded RunCancelled.
// m (nil-safe via metrics.OrNoop) records the run's transition into the
// running state, including incrementing the active-runs gauge — callers
// that also call Continue directly (rather than through Execute) must pass
// the same m to both, or the gauge will drift.
func Start(ctx context.Context, db *sql.DB, workflowID string, inputs map[string]any, m *metrics.Registry) (*workflow.Definition, *Run, map[string]any, error) {
	pctx, cancel := context.WithTimeout(context.Background(), persistTimeout)
	defer cancel()

	def, err := LatestWorkflow(pctx, db, workflowID)
	if err != nil {
		return nil, nil, nil, err
	}

	preparedInputs, err := workflow.PrepareInputs(def.Inputs, inputs)
	if err != nil {
		return nil, nil, nil, err
	}

	run, err := createRun(pctx, db, def, preparedInputs)
	if err != nil {
		return nil, nil, nil, err
	}

	if err := updateRunStatus(pctx, db, run, workflow.RunRunning, nil, nil, m); err != nil {
		return nil, nil, nil, err
	}

	return def, run, preparedInputs, nil
}

// Continue runs def's steps for run — already created and transitioned to
// Running by Start — against executor, persisting progress and the final
// status as it happens. inputs and bindings are the same values passed to
// Start (bindings maps a logical binding name, as referenced by a step's
// ${{ bindings.<name> }} connector expression, to the id of the connector
// to use — see workflow.ResolveConnector).
//
// Steps run sequentially; the first one to fail — including timing out, or
// ctx being cancelled — stops the run. Every step that never got a chance
// to run is recorded as skipped, so no step is left dangling in "pending".
// A step whose If resolves to false is also recorded as skipped, but does
// not stop the run — the loop moves on to the next step exactly as if this
// one had succeeded, only without an entry in stepOutputs (see
// workflow.ResolveIf) — unless that step also sets StopIfFalse, in which
// case every following step is recorded skipped too and the run ends
// Succeeded, not Failed: a guard clause's early return, not an error. A
// step whose ElseOf names an earlier step that actually ran is skipped
// before its own If is even evaluated (see ranSteps below) — chaining
// ElseOf onto consecutive steps builds an if/elseif/else without nesting.
// A foreach step calls its action once per resolved item, sequentially,
// sharing one StepTimeout budget across every iteration; the first item to
// fail stops the run exactly like a regular step failure would, and the
// step's recorded output is the per-item outputs collected into lists
// under the action's own output keys (see workflow.ResolveForeach).
// Continue only returns a non-nil error for a genuine persistence failure —
// a step's own failure (or the run's ctx being cancelled) is captured in
// the run's final status instead, never returned as an error here, exactly
// as Execute's callers already expect.
func Continue(ctx context.Context, db *sql.DB, executor ActionExecutor, def *workflow.Definition, run *Run, inputs map[string]any, bindings map[string]string, opts ExecuteOptions) error {
	opts = opts.withDefaults()

	pctx, cancelPersist := context.WithTimeout(context.Background(), persistTimeout)
	defer cancelPersist()

	stepOutputs := make(map[string]map[string]any, len(def.Steps))
	// ranSteps tracks, per step, whether its branch of an if/elseif/else
	// chain was "taken" — true when the step actually attempted its
	// action (its own ElseOf, if any, let it through, and If, if any, was
	// true), but *also* true when it was itself skipped via ElseOf,
	// propagating the earlier link's "taken" status forward. That
	// propagation is what makes chaining ElseOf across 3+ steps behave
	// like a real elseif/else: a later step's ElseOf must see "was
	// anything before me in this chain taken", not merely "did my direct
	// predecessor literally run" — otherwise a final catch-all step would
	// incorrectly run whenever its immediate predecessor was skipped, even
	// if an earlier link further up the chain was the one actually taken.
	// It is set independent of success/failure: a foreach or normal step
	// failing still counts as having "run" (a failure stops the run
	// outright anyway, so no later ElseOf ever gets to observe it).
	ranSteps := make(map[string]bool, len(def.Steps))
	var runOutputs map[string]any
	var runErr error
	stopped := false
	nextPending := 0

	for i, step := range def.Steps {
		if ctx.Err() != nil {
			runErr = ctx.Err()
			break
		}

		// One context covers both resolving this step's connector (a
		// secrets.Store may one day reach a real external system, e.g.
		// Vault — it deserves the same cancellation/timeout treatment as
		// the action call, not the bookkeeping pctx) and the action call
		// itself.
		stepCtx, cancel := context.WithTimeout(ctx, opts.StepTimeout)

		exprCtx := workflow.ExprContext{Inputs: inputs, StepOutputs: stepOutputs, Bindings: bindings}

		if step.ElseOf != "" && ranSteps[step.ElseOf] {
			cancel()
			if err := updateStepStatus(pctx, db, run.ID, step.ID, workflow.StepPending, workflow.StepSkipped, nil, nil, nil, opts.Metrics); err != nil {
				return err
			}
			// Propagate "taken" through the chain: this step didn't run,
			// but its branch is spoken for by the same earlier step that
			// caused it to be skipped — so a later step chaining else_of
			// onto *this* one must see it as taken too, not just "did not
			// run". Without this, a 3+ link chain (elseif/elseif/else)
			// would let its final "else" run even though an earlier link,
			// not its immediate predecessor, was the one actually taken.
			ranSteps[step.ID] = true
			nextPending = i + 1
			continue
		}

		runStep, ifErr := workflow.ResolveIf(step.If, exprCtx)
		if ifErr != nil {
			cancel()
			runErr = fmt.Errorf("step %q: if: %w", step.ID, ifErr)
			if err := updateStepStatus(pctx, db, run.ID, step.ID, workflow.StepPending, workflow.StepRunning, nil, nil, nil, opts.Metrics); err != nil {
				return err
			}
			if err := updateStepStatus(pctx, db, run.ID, step.ID, workflow.StepRunning, stepFailureStatus(runErr), nil, nil, runErr, opts.Metrics); err != nil {
				return err
			}
			nextPending = i + 1
			break
		}

		if !runStep {
			cancel()
			if err := updateStepStatus(pctx, db, run.ID, step.ID, workflow.StepPending, workflow.StepSkipped, nil, nil, nil, opts.Metrics); err != nil {
				return err
			}
			nextPending = i + 1
			if step.StopIfFalse {
				stopped = true
				break
			}
			continue
		}

		ranSteps[step.ID] = true

		if step.Foreach != nil {
			items, foreachErr := workflow.ResolveForeach(step.Foreach, exprCtx)

			// The connector, unlike With, does not vary per item — resolve
			// it once, the same way a non-foreach step does.
			var resolvedConnector *connectors.ResolvedConnector
			if foreachErr == nil {
				resolvedConnector, foreachErr = resolveStepConnector(stepCtx, db, step.Connector, exprCtx, opts.Secrets)
			}

			if foreachErr != nil {
				cancel()
				runErr = fmt.Errorf("step %q: %w", step.ID, foreachErr)
				if err := updateStepStatus(pctx, db, run.ID, step.ID, workflow.StepPending, workflow.StepRunning, nil, nil, nil, opts.Metrics); err != nil {
					return err
				}
				if err := updateStepStatus(pctx, db, run.ID, step.ID, workflow.StepRunning, stepFailureStatus(runErr), nil, nil, runErr, opts.Metrics); err != nil {
					return err
				}
				nextPending = i + 1
				break
			}

			// The raw item list, not each iteration's resolved With, is
			// what gets persisted as this step's input: With differs per
			// item, but the list being iterated is the one stable thing
			// worth recording for replay/debugging.
			foreachInput := map[string]any{"items": items}
			if err := updateStepStatus(pctx, db, run.ID, step.ID, workflow.StepPending, workflow.StepRunning, foreachInput, nil, nil, opts.Metrics); err != nil {
				cancel()
				return err
			}

			aggregated := make(map[string]any)
			var iterErr error
			for idx, item := range items {
				itemCtx := exprCtx
				itemCtx.Each = item
				itemCtx.HasEach = true

				itemInput, err := workflow.ResolveInputs(step.With, itemCtx)
				var itemOutput map[string]any
				if err == nil {
					itemOutput, err = executor.ExecuteAction(stepCtx, step.Uses, itemInput, resolvedConnector)
				}
				if err != nil {
					iterErr = fmt.Errorf("item %d: %w", idx, err)
					break
				}
				for key, value := range itemOutput {
					list, _ := aggregated[key].([]any)
					aggregated[key] = append(list, value)
				}
			}
			cancel()

			if iterErr != nil {
				runErr = fmt.Errorf("step %q: %w", step.ID, iterErr)
				if err := updateStepStatus(pctx, db, run.ID, step.ID, workflow.StepRunning, stepFailureStatus(runErr), foreachInput, nil, runErr, opts.Metrics); err != nil {
					return err
				}
				nextPending = i + 1
				break
			}

			if err := updateStepStatus(pctx, db, run.ID, step.ID, workflow.StepRunning, workflow.StepSucceeded, foreachInput, aggregated, nil, opts.Metrics); err != nil {
				return err
			}

			stepOutputs[step.ID] = aggregated
			runOutputs = aggregated
			nextPending = i + 1
			continue
		}

		resolvedInput, resolveErr := workflow.ResolveInputs(step.With, exprCtx)

		var resolvedConnector *connectors.ResolvedConnector
		if resolveErr == nil {
			resolvedConnector, resolveErr = resolveStepConnector(stepCtx, db, step.Connector, exprCtx, opts.Secrets)
		}

		if resolveErr != nil {
			cancel()
			// A step only reaches a terminal state by way of "running" —
			// even one that never got to call its action, since "pending"
			// means it was never attempted at all. resolvedInput may still
			// be non-nil here (input resolution succeeded, only the
			// connector's did not) — persist it either way rather than
			// discarding data that was actually computed.
			runErr = fmt.Errorf("step %q: %w", step.ID, resolveErr)
			if err := updateStepStatus(pctx, db, run.ID, step.ID, workflow.StepPending, workflow.StepRunning, resolvedInput, nil, nil, opts.Metrics); err != nil {
				return err
			}
			if err := updateStepStatus(pctx, db, run.ID, step.ID, workflow.StepRunning, stepFailureStatus(runErr), resolvedInput, nil, runErr, opts.Metrics); err != nil {
				return err
			}
			nextPending = i + 1
			break
		}

		if err := updateStepStatus(pctx, db, run.ID, step.ID, workflow.StepPending, workflow.StepRunning, resolvedInput, nil, nil, opts.Metrics); err != nil {
			cancel()
			return err
		}

		output, execErr := executor.ExecuteAction(stepCtx, step.Uses, resolvedInput, resolvedConnector)
		cancel()

		if execErr != nil {
			runErr = fmt.Errorf("step %q: %w", step.ID, execErr)
			if err := updateStepStatus(pctx, db, run.ID, step.ID, workflow.StepRunning, stepFailureStatus(runErr), resolvedInput, nil, execErr, opts.Metrics); err != nil {
				return err
			}
			nextPending = i + 1
			break
		}

		if err := updateStepStatus(pctx, db, run.ID, step.ID, workflow.StepRunning, workflow.StepSucceeded, resolvedInput, output, nil, opts.Metrics); err != nil {
			return err
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
			if err := updateStepStatus(pctx, db, run.ID, step.ID, workflow.StepPending, skippedStatus, nil, nil, nil, opts.Metrics); err != nil {
				return err
			}
		}
	} else if stopped {
		// A step's own If was false and StopIfFalse asked to end the run
		// here — a guard clause's early return, not a failure: every step
		// after it is recorded skipped, same as the failure path above, but
		// the run itself finishes Succeeded.
		for _, step := range def.Steps[nextPending:] {
			if err := updateStepStatus(pctx, db, run.ID, step.ID, workflow.StepPending, workflow.StepSkipped, nil, nil, nil, opts.Metrics); err != nil {
				return err
			}
		}
	}

	if err := updateRunStatus(pctx, db, run, finalStatus, runOutputs, runErr, opts.Metrics); err != nil {
		return err
	}

	return nil
}

// Execute runs the latest installed version of workflowID to completion —
// Start followed by Continue — persisting the run and every step's progress
// as it happens, and returns the completed run. See Start and Continue for
// what each half does; most callers (the CLI's `workflow run`, most tests)
// want this blocking, all-in-one behavior.
func Execute(ctx context.Context, db *sql.DB, executor ActionExecutor, workflowID string, inputs map[string]any, bindings map[string]string, opts ExecuteOptions) (*Run, error) {
	opts = opts.withDefaults()

	def, run, preparedInputs, err := Start(ctx, db, workflowID, inputs, opts.Metrics)
	if err != nil {
		return nil, err
	}

	if err := Continue(ctx, db, executor, def, run, preparedInputs, bindings, opts); err != nil {
		return nil, err
	}

	return run, nil
}
