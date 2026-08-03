package runs

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/lucasglmt/patchcord/internal/connectors"
	"github.com/lucasglmt/patchcord/internal/secrets"
	"github.com/lucasglmt/patchcord/internal/workflow"
)

// slowExecutor takes delay to run any action, honoring ctx cancellation —
// used to exercise step timeouts and mid-run cancellation.
type slowExecutor struct {
	delay time.Duration
}

func (s *slowExecutor) ExecuteAction(ctx context.Context, _ string, _ map[string]any, _ *connectors.ResolvedConnector) (map[string]any, error) {
	select {
	case <-time.After(s.delay):
		return map[string]any{}, nil
	case <-ctx.Done():
		return nil, ctx.Err()
	}
}

func TestExecute_RunsStepsSequentiallyAndSucceeds(t *testing.T) {
	db := openTestDB(t)
	installTestWorkflow(t, db, `
schema_version: 1
id: chained
version: 1
trigger:
  type: manual
steps:
  - id: first
    uses: text.uppercase@1
    with:
      value: "${{ workflow.inputs.value }}"
  - id: second
    uses: text.uppercase@1
    with:
      value: "${{ steps.first.outputs.value }}"
`)

	executor := &fakeExecutor{
		responses: map[string]map[string]any{
			"text.uppercase@1": {"value": "HELLO"},
		},
	}

	run, err := Execute(context.Background(), db, executor, "chained", map[string]any{"value": "hello"}, nil, ExecuteOptions{})
	if err != nil {
		t.Fatalf("Execute() error = %v", err)
	}

	if run.Status != workflow.RunSucceeded {
		t.Fatalf("run status = %s, want %s", run.Status, workflow.RunSucceeded)
	}
	if run.Outputs["value"] != "HELLO" {
		t.Fatalf(`run outputs["value"] = %v, want "HELLO"`, run.Outputs["value"])
	}
	if len(executor.calls) != 2 {
		t.Fatalf("executor was called %d times, want 2", len(executor.calls))
	}

	_, steps, err := GetRun(context.Background(), db, run.ID)
	if err != nil {
		t.Fatalf("GetRun() error = %v", err)
	}
	if len(steps) != 2 {
		t.Fatalf("len(steps) = %d, want 2", len(steps))
	}
	for _, step := range steps {
		if step.Status != workflow.StepSucceeded {
			t.Fatalf("step %q status = %s, want %s", step.StepID, step.Status, workflow.StepSucceeded)
		}
	}
}

func TestExecute_StepFailureFailsTheRunAndSkipsRemainingSteps(t *testing.T) {
	db := openTestDB(t)
	installTestWorkflow(t, db, `
schema_version: 1
id: chained
version: 1
trigger:
  type: manual
steps:
  - id: first
    uses: text.uppercase@1
    with:
      value: "hello"
  - id: second
    uses: text.uppercase@1
    with:
      value: "${{ steps.first.outputs.value }}"
`)

	boom := errors.New("boom")
	executor := &fakeExecutor{
		errs: map[string]error{"text.uppercase@1": boom},
	}

	run, err := Execute(context.Background(), db, executor, "chained", nil, nil, ExecuteOptions{})
	if err != nil {
		t.Fatalf("Execute() error = %v", err)
	}

	if run.Status != workflow.RunFailed {
		t.Fatalf("run status = %s, want %s", run.Status, workflow.RunFailed)
	}
	if len(executor.calls) != 1 {
		t.Fatalf("executor was called %d times, want 1 (the run should stop at the first failure)", len(executor.calls))
	}

	_, steps, err := GetRun(context.Background(), db, run.ID)
	if err != nil {
		t.Fatalf("GetRun() error = %v", err)
	}

	byID := make(map[string]Step, len(steps))
	for _, step := range steps {
		byID[step.StepID] = step
	}

	if byID["first"].Status != workflow.StepFailed {
		t.Fatalf(`steps["first"].Status = %s, want %s`, byID["first"].Status, workflow.StepFailed)
	}
	if byID["second"].Status != workflow.StepSkipped {
		t.Fatalf(`steps["second"].Status = %s, want %s`, byID["second"].Status, workflow.StepSkipped)
	}
}

func TestExecute_SkipsAStepWhoseIfResolvesToFalseWithoutFailingTheRun(t *testing.T) {
	db := openTestDB(t)
	installTestWorkflow(t, db, `
schema_version: 1
id: conditional
version: 1
trigger:
  type: manual
steps:
  - id: first
    uses: text.uppercase@1
    with:
      value: "hello"
  - id: skipped
    if: false
    uses: text.uppercase@1
    with:
      value: "hello"
  - id: last
    uses: text.uppercase@1
    with:
      value: "${{ steps.first.outputs.value }}"
`)

	executor := &fakeExecutor{
		responses: map[string]map[string]any{"text.uppercase@1": {"value": "HELLO"}},
	}

	run, err := Execute(context.Background(), db, executor, "conditional", nil, nil, ExecuteOptions{})
	if err != nil {
		t.Fatalf("Execute() error = %v", err)
	}
	if run.Status != workflow.RunSucceeded {
		t.Fatalf("run status = %s, want %s", run.Status, workflow.RunSucceeded)
	}
	// Only "first" and "last" ever call the executor — "skipped" must not.
	if len(executor.calls) != 2 {
		t.Fatalf("executor was called %d times, want 2 (the skipped step must not call the action)", len(executor.calls))
	}

	_, steps, err := GetRun(context.Background(), db, run.ID)
	if err != nil {
		t.Fatalf("GetRun() error = %v", err)
	}
	byID := make(map[string]Step, len(steps))
	for _, step := range steps {
		byID[step.StepID] = step
	}
	if byID["first"].Status != workflow.StepSucceeded {
		t.Fatalf(`steps["first"].Status = %s, want %s`, byID["first"].Status, workflow.StepSucceeded)
	}
	if byID["skipped"].Status != workflow.StepSkipped {
		t.Fatalf(`steps["skipped"].Status = %s, want %s`, byID["skipped"].Status, workflow.StepSkipped)
	}
	if byID["last"].Status != workflow.StepSucceeded {
		t.Fatalf(`steps["last"].Status = %s, want %s (run must continue past a skipped step)`, byID["last"].Status, workflow.StepSucceeded)
	}
}

func TestExecute_StopIfFalseEndsTheRunSucceededWithoutRunningLaterSteps(t *testing.T) {
	db := openTestDB(t)
	installTestWorkflow(t, db, `
schema_version: 1
id: guard
version: 1
trigger:
  type: manual
steps:
  - id: first
    uses: text.uppercase@1
    with:
      value: "hello"
  - id: guard
    if: false
    stop_if_false: true
    uses: text.uppercase@1
    with:
      value: "hello"
  - id: never
    uses: text.uppercase@1
    with:
      value: "hello"
`)

	executor := &fakeExecutor{
		responses: map[string]map[string]any{"text.uppercase@1": {"value": "HELLO"}},
	}

	run, err := Execute(context.Background(), db, executor, "guard", nil, nil, ExecuteOptions{})
	if err != nil {
		t.Fatalf("Execute() error = %v", err)
	}
	if run.Status != workflow.RunSucceeded {
		t.Fatalf("run status = %s, want %s (a guard clause is not an error)", run.Status, workflow.RunSucceeded)
	}
	// Only "first" ever calls the executor — "guard" is false (never calls
	// its action) and "never" is skipped by the early stop.
	if len(executor.calls) != 1 {
		t.Fatalf("executor was called %d times, want 1", len(executor.calls))
	}

	_, steps, err := GetRun(context.Background(), db, run.ID)
	if err != nil {
		t.Fatalf("GetRun() error = %v", err)
	}
	byID := make(map[string]Step, len(steps))
	for _, step := range steps {
		byID[step.StepID] = step
	}
	if byID["first"].Status != workflow.StepSucceeded {
		t.Fatalf(`steps["first"].Status = %s, want %s`, byID["first"].Status, workflow.StepSucceeded)
	}
	if byID["guard"].Status != workflow.StepSkipped {
		t.Fatalf(`steps["guard"].Status = %s, want %s`, byID["guard"].Status, workflow.StepSkipped)
	}
	if byID["never"].Status != workflow.StepSkipped {
		t.Fatalf(`steps["never"].Status = %s, want %s (stop_if_false must skip every later step too)`, byID["never"].Status, workflow.StepSkipped)
	}
}

func TestExecute_ElseOfBuildsAnIfElseChainWithoutNesting(t *testing.T) {
	db := openTestDB(t)
	installTestWorkflow(t, db, `
schema_version: 1
id: branching
version: 1
trigger:
  type: manual
steps:
  - id: case_true
    if: "${{ workflow.inputs.flag }}"
    uses: text.uppercase@1
    with:
      value: "true-branch"
  - id: case_false
    else_of: case_true
    uses: text.uppercase@1
    with:
      value: "false-branch"
`)

	executor := &echoValueExecutor{}

	t.Run("flag true runs case_true, skips case_false", func(t *testing.T) {
		run, err := Execute(context.Background(), db, executor, "branching", map[string]any{"flag": true}, nil, ExecuteOptions{})
		if err != nil {
			t.Fatalf("Execute() error = %v", err)
		}
		if run.Status != workflow.RunSucceeded {
			t.Fatalf("run status = %s, want %s", run.Status, workflow.RunSucceeded)
		}

		_, steps, err := GetRun(context.Background(), db, run.ID)
		if err != nil {
			t.Fatalf("GetRun() error = %v", err)
		}
		byID := make(map[string]Step, len(steps))
		for _, step := range steps {
			byID[step.StepID] = step
		}
		if byID["case_true"].Status != workflow.StepSucceeded {
			t.Fatalf(`steps["case_true"].Status = %s, want %s`, byID["case_true"].Status, workflow.StepSucceeded)
		}
		if byID["case_false"].Status != workflow.StepSkipped {
			t.Fatalf(`steps["case_false"].Status = %s, want %s (else_of must skip it since case_true ran)`, byID["case_false"].Status, workflow.StepSkipped)
		}
	})

	executor2 := &echoValueExecutor{}
	t.Run("flag false skips case_true, runs case_false", func(t *testing.T) {
		run, err := Execute(context.Background(), db, executor2, "branching", map[string]any{"flag": false}, nil, ExecuteOptions{})
		if err != nil {
			t.Fatalf("Execute() error = %v", err)
		}
		if run.Status != workflow.RunSucceeded {
			t.Fatalf("run status = %s, want %s", run.Status, workflow.RunSucceeded)
		}

		_, steps, err := GetRun(context.Background(), db, run.ID)
		if err != nil {
			t.Fatalf("GetRun() error = %v", err)
		}
		byID := make(map[string]Step, len(steps))
		for _, step := range steps {
			byID[step.StepID] = step
		}
		if byID["case_true"].Status != workflow.StepSkipped {
			t.Fatalf(`steps["case_true"].Status = %s, want %s`, byID["case_true"].Status, workflow.StepSkipped)
		}
		if byID["case_false"].Status != workflow.StepSucceeded {
			t.Fatalf(`steps["case_false"].Status = %s, want %s (else_of must let it run since case_true was skipped)`, byID["case_false"].Status, workflow.StepSucceeded)
		}
		if len(executor2.calls) != 1 || executor2.calls[0]["value"] != "false-branch" {
			t.Fatalf("executor2.calls = %+v, want exactly one call with value \"false-branch\"", executor2.calls)
		}
	})
}

// TestExecute_ElseOfChainOfThreeOnlyRunsOneCase exercises a proper
// elseif/elseif/else chain (three links, not just an if/else pair) — the
// case that exposed a real bug during manual testing: a naive else_of that
// only checks "did my immediate predecessor literally run" lets the final
// catch-all run even when an *earlier* link further up the chain (not its
// direct predecessor) was the one actually taken. Each of the three cases
// must run exactly once, whichever branch a given score falls into.
func TestExecute_ElseOfChainOfThreeOnlyRunsOneCase(t *testing.T) {
	db := openTestDB(t)
	installTestWorkflow(t, db, `
schema_version: 1
id: switch_demo
version: 1
trigger:
  type: manual
inputs:
  - name: score
    type: number
    required: true
steps:
  - id: case_high
    if: "${{ workflow.inputs.score >= 8 }}"
    uses: text.uppercase@1
    with:
      value: "high"
  - id: case_mid
    else_of: case_high
    if: "${{ workflow.inputs.score >= 5 }}"
    uses: text.uppercase@1
    with:
      value: "mid"
  - id: case_low
    else_of: case_mid
    uses: text.uppercase@1
    with:
      value: "low"
`)

	tests := []struct {
		score float64
		want  string // the one case expected to succeed
	}{
		{score: 9, want: "case_high"},
		{score: 6, want: "case_mid"},
		{score: 2, want: "case_low"},
	}

	for _, tt := range tests {
		t.Run(tt.want, func(t *testing.T) {
			executor := &fakeExecutor{
				responses: map[string]map[string]any{"text.uppercase@1": {"value": "X"}},
			}

			run, err := Execute(context.Background(), db, executor, "switch_demo", map[string]any{"score": tt.score}, nil, ExecuteOptions{})
			if err != nil {
				t.Fatalf("Execute() error = %v", err)
			}
			if run.Status != workflow.RunSucceeded {
				t.Fatalf("run status = %s, want %s", run.Status, workflow.RunSucceeded)
			}
			// Exactly one of the three cases ever calls the action —
			// the bug produced 2 here for score=9 and score=6.
			if len(executor.calls) != 1 {
				t.Fatalf("executor was called %d times, want exactly 1", len(executor.calls))
			}

			_, steps, err := GetRun(context.Background(), db, run.ID)
			if err != nil {
				t.Fatalf("GetRun() error = %v", err)
			}
			for _, step := range steps {
				wantStatus := workflow.StepSkipped
				if step.StepID == tt.want {
					wantStatus = workflow.StepSucceeded
				}
				if step.Status != wantStatus {
					t.Fatalf("score=%v: steps[%q].Status = %s, want %s", tt.score, step.StepID, step.Status, wantStatus)
				}
			}
		})
	}
}

func TestExecute_IfComparisonExpressionGatesAStep(t *testing.T) {
	db := openTestDB(t)
	installTestWorkflow(t, db, `
schema_version: 1
id: threshold
version: 1
trigger:
  type: manual
steps:
  - id: count
    uses: text.uppercase@1
    with:
      value: "hello"
  - id: over_threshold
    if: "${{ steps.count.outputs.value == 'HELLO' }}"
    uses: text.uppercase@1
    with:
      value: "hello"
`)

	executor := &fakeExecutor{
		responses: map[string]map[string]any{"text.uppercase@1": {"value": "HELLO"}},
	}

	run, err := Execute(context.Background(), db, executor, "threshold", nil, nil, ExecuteOptions{})
	if err != nil {
		t.Fatalf("Execute() error = %v", err)
	}
	if run.Status != workflow.RunSucceeded {
		t.Fatalf("run status = %s, want %s", run.Status, workflow.RunSucceeded)
	}

	_, steps, err := GetRun(context.Background(), db, run.ID)
	if err != nil {
		t.Fatalf("GetRun() error = %v", err)
	}
	byID := make(map[string]Step, len(steps))
	for _, step := range steps {
		byID[step.StepID] = step
	}
	if byID["over_threshold"].Status != workflow.StepSucceeded {
		t.Fatalf(`steps["over_threshold"].Status = %s, want %s (the comparison should have resolved true)`, byID["over_threshold"].Status, workflow.StepSucceeded)
	}
}

func TestExecute_FailsTheRunWhenIfCannotBeResolved(t *testing.T) {
	db := openTestDB(t)
	installTestWorkflow(t, db, `
schema_version: 1
id: bad_if
version: 1
trigger:
  type: manual
steps:
  - id: only
    if: "${{ workflow.inputs.enabled }}"
    uses: text.uppercase@1
    with:
      value: "hello"
`)

	executor := &fakeExecutor{
		responses: map[string]map[string]any{"text.uppercase@1": {"value": "HELLO"}},
	}

	// No "enabled" input is provided, so the if expression cannot resolve.
	run, err := Execute(context.Background(), db, executor, "bad_if", nil, nil, ExecuteOptions{})
	if err != nil {
		t.Fatalf("Execute() error = %v", err)
	}
	if run.Status != workflow.RunFailed {
		t.Fatalf("run status = %s, want %s", run.Status, workflow.RunFailed)
	}
	if len(executor.calls) != 0 {
		t.Fatalf("executor was called %d times, want 0 (should fail before calling the action)", len(executor.calls))
	}
}

func TestExecute_ForeachRunsTheActionOncePerItemAndAggregatesOutputs(t *testing.T) {
	db := openTestDB(t)
	installTestWorkflow(t, db, `
schema_version: 1
id: foreach_demo
version: 1
trigger:
  type: manual
steps:
  - id: shout
    uses: text.uppercase@1
    foreach: "${{ workflow.inputs.names }}"
    with:
      value: "${{ each }}"
`)

	executor := &echoValueExecutor{}

	run, err := Execute(context.Background(), db, executor, "foreach_demo",
		map[string]any{"names": []any{"alice", "bob", "carol"}}, nil, ExecuteOptions{})
	if err != nil {
		t.Fatalf("Execute() error = %v", err)
	}
	if run.Status != workflow.RunSucceeded {
		t.Fatalf("run status = %s, want %s", run.Status, workflow.RunSucceeded)
	}
	if len(executor.calls) != 3 {
		t.Fatalf("executor was called %d times, want 3 (one per item)", len(executor.calls))
	}
	for i, want := range []string{"alice", "bob", "carol"} {
		if executor.calls[i]["value"] != want {
			t.Fatalf(`executor.calls[%d]["value"] = %v, want %q (each item resolved in order)`, i, executor.calls[i]["value"], want)
		}
	}

	want := []any{"alice", "bob", "carol"}
	got, ok := run.Outputs["value"].([]any)
	if !ok || len(got) != len(want) {
		t.Fatalf(`run.Outputs["value"] = %v, want %v`, run.Outputs["value"], want)
	}
	for i := range want {
		if got[i] != want[i] {
			t.Fatalf(`run.Outputs["value"][%d] = %v, want %v`, i, got[i], want[i])
		}
	}

	_, steps, err := GetRun(context.Background(), db, run.ID)
	if err != nil {
		t.Fatalf("GetRun() error = %v", err)
	}
	if len(steps) != 1 || steps[0].Status != workflow.StepSucceeded {
		t.Fatalf("steps = %+v, want a single succeeded step", steps)
	}
}

func TestExecute_ForeachStopsTheRunAtTheFirstFailingItem(t *testing.T) {
	db := openTestDB(t)
	installTestWorkflow(t, db, `
schema_version: 1
id: foreach_failure
version: 1
trigger:
  type: manual
steps:
  - id: shout
    uses: text.uppercase@1
    foreach: "${{ workflow.inputs.names }}"
    with:
      value: "${{ each }}"
  - id: after
    uses: text.uppercase@1
    with:
      value: "hello"
`)

	boom := errors.New("boom")
	executor := &echoValueExecutor{errs: map[string]error{"bob": boom}}

	run, err := Execute(context.Background(), db, executor, "foreach_failure",
		map[string]any{"names": []any{"alice", "bob", "carol"}}, nil, ExecuteOptions{})
	if err != nil {
		t.Fatalf("Execute() error = %v", err)
	}
	if run.Status != workflow.RunFailed {
		t.Fatalf("run status = %s, want %s", run.Status, workflow.RunFailed)
	}
	// "bob" (item 1) fails, so "carol" (item 2) must never be attempted.
	if len(executor.calls) != 2 {
		t.Fatalf("executor was called %d times, want 2 (stops at the failing item)", len(executor.calls))
	}

	_, steps, err := GetRun(context.Background(), db, run.ID)
	if err != nil {
		t.Fatalf("GetRun() error = %v", err)
	}
	byID := make(map[string]Step, len(steps))
	for _, step := range steps {
		byID[step.StepID] = step
	}
	if byID["shout"].Status != workflow.StepFailed {
		t.Fatalf(`steps["shout"].Status = %s, want %s`, byID["shout"].Status, workflow.StepFailed)
	}
	if byID["after"].Status != workflow.StepSkipped {
		t.Fatalf(`steps["after"].Status = %s, want %s`, byID["after"].Status, workflow.StepSkipped)
	}
}

func TestExecute_ForeachWithNoItemsSucceedsWithoutCallingTheAction(t *testing.T) {
	db := openTestDB(t)
	installTestWorkflow(t, db, `
schema_version: 1
id: foreach_empty
version: 1
trigger:
  type: manual
steps:
  - id: shout
    uses: text.uppercase@1
    foreach: "${{ workflow.inputs.names }}"
    with:
      value: "${{ each }}"
`)

	executor := &echoValueExecutor{}

	run, err := Execute(context.Background(), db, executor, "foreach_empty",
		map[string]any{"names": []any{}}, nil, ExecuteOptions{})
	if err != nil {
		t.Fatalf("Execute() error = %v", err)
	}
	if run.Status != workflow.RunSucceeded {
		t.Fatalf("run status = %s, want %s", run.Status, workflow.RunSucceeded)
	}
	if len(executor.calls) != 0 {
		t.Fatalf("executor was called %d times, want 0 (empty foreach list)", len(executor.calls))
	}
}

func TestExecute_UnknownWorkflowFailsFast(t *testing.T) {
	db := openTestDB(t)
	executor := &fakeExecutor{}

	_, err := Execute(context.Background(), db, executor, "unknown", nil, nil, ExecuteOptions{})
	if !errors.Is(err, ErrWorkflowNotFound) {
		t.Fatalf("Execute() error = %v, want ErrWorkflowNotFound", err)
	}
	if len(executor.calls) != 0 {
		t.Fatalf("executor was called %d times, want 0", len(executor.calls))
	}
}

func TestExecute_FailsWhenAnExpressionCannotBeResolved(t *testing.T) {
	db := openTestDB(t)
	installTestWorkflow(t, db, `
schema_version: 1
id: needs_input
version: 1
trigger:
  type: manual
steps:
  - id: only
    uses: text.uppercase@1
    with:
      value: "${{ workflow.inputs.value }}"
`)

	executor := &fakeExecutor{
		responses: map[string]map[string]any{"text.uppercase@1": {"value": "x"}},
	}

	// No "value" input is provided, so the expression cannot resolve.
	run, err := Execute(context.Background(), db, executor, "needs_input", nil, nil, ExecuteOptions{})
	if err != nil {
		t.Fatalf("Execute() error = %v", err)
	}
	if run.Status != workflow.RunFailed {
		t.Fatalf("run status = %s, want %s", run.Status, workflow.RunFailed)
	}
	if len(executor.calls) != 0 {
		t.Fatalf("executor was called %d times, want 0 (should fail before calling the action)", len(executor.calls))
	}
}

func TestExecute_StepTimeoutFailsTheRun(t *testing.T) {
	db := openTestDB(t)
	installTestWorkflow(t, db, helloWorkflow)

	executor := &slowExecutor{delay: 300 * time.Millisecond}

	run, err := Execute(context.Background(), db, executor, "hello_patchcord", nil, nil,
		ExecuteOptions{StepTimeout: 20 * time.Millisecond})
	if err != nil {
		t.Fatalf("Execute() error = %v", err)
	}

	// A step timing out on its own derived context fails the run — it is
	// not a user-requested cancellation.
	if run.Status != workflow.RunFailed {
		t.Fatalf("run status = %s, want %s", run.Status, workflow.RunFailed)
	}

	_, steps, err := GetRun(context.Background(), db, run.ID)
	if err != nil {
		t.Fatalf("GetRun() error = %v", err)
	}
	if steps[0].Status != workflow.StepFailed {
		t.Fatalf("step status = %s, want %s", steps[0].Status, workflow.StepFailed)
	}
}

func TestExecute_ContextCancellationCancelsTheRun(t *testing.T) {
	db := openTestDB(t)
	installTestWorkflow(t, db, `
schema_version: 1
id: chained
version: 1
trigger:
  type: manual
steps:
  - id: first
    uses: text.uppercase@1
    with:
      value: "hello"
  - id: second
    uses: text.uppercase@1
    with:
      value: "hello"
`)

	ctx, cancel := context.WithCancel(context.Background())
	executor := &slowExecutor{delay: time.Second}

	type result struct {
		run *Run
		err error
	}
	resultCh := make(chan result, 1)
	go func() {
		run, err := Execute(ctx, db, executor, "chained", nil, nil, ExecuteOptions{StepTimeout: 5 * time.Second})
		resultCh <- result{run, err}
	}()

	// Let Execute start its first (slow) step, then cancel mid-flight —
	// exactly what a SIGINT during `patchcord workflow run` produces.
	time.Sleep(50 * time.Millisecond)
	cancel()

	select {
	case r := <-resultCh:
		if r.err != nil {
			t.Fatalf("Execute() error = %v", r.err)
		}
		if r.run.Status != workflow.RunCancelled {
			t.Fatalf("run status = %s, want %s", r.run.Status, workflow.RunCancelled)
		}

		_, steps, err := GetRun(context.Background(), db, r.run.ID)
		if err != nil {
			t.Fatalf("GetRun() error = %v", err)
		}
		byID := make(map[string]Step, len(steps))
		for _, step := range steps {
			byID[step.StepID] = step
		}
		if byID["first"].Status != workflow.StepCancelled {
			t.Fatalf(`steps["first"].Status = %s, want %s (cancelled mid-call)`, byID["first"].Status, workflow.StepCancelled)
		}
		if byID["second"].Status != workflow.StepCancelled {
			t.Fatalf(`steps["second"].Status = %s, want %s`, byID["second"].Status, workflow.StepCancelled)
		}
	case <-time.After(3 * time.Second):
		t.Fatal("Execute() did not return after ctx was cancelled")
	}
}

func TestExecute_ThreadsTheBoundConnectorToTheExecutor(t *testing.T) {
	db := openTestDB(t)
	installTestWorkflow(t, db, `
schema_version: 1
id: connector_flow
version: 1
trigger:
  type: manual
steps:
  - id: bound
    uses: text.uppercase@1
    connector: "${{ bindings.demo }}"
    with:
      value: "hello"
  - id: unbound
    uses: text.uppercase@1
    with:
      value: "hello"
`)

	t.Setenv("PATCHCORD_RUNNER_TEST_SECRET", "s3cr3t")
	config := map[string]any{"host": "db.internal"}
	secretRefs := map[string]secrets.Reference{"password": {Type: "env", Key: "PATCHCORD_RUNNER_TEST_SECRET"}}
	knownConnectorTypes := map[string]struct{}{"postgresql.connection@1": {}}
	if _, err := connectors.Create(context.Background(), db, "my_conn", "postgresql.connection@1", config, secretRefs, knownConnectorTypes); err != nil {
		t.Fatalf("connectors.Create() error = %v", err)
	}

	executor := &fakeExecutor{
		responses: map[string]map[string]any{"text.uppercase@1": {"value": "HELLO"}},
	}

	run, err := Execute(context.Background(), db, executor, "connector_flow", nil, map[string]string{"demo": "my_conn"}, ExecuteOptions{})
	if err != nil {
		t.Fatalf("Execute() error = %v", err)
	}
	if run.Status != workflow.RunSucceeded {
		t.Fatalf("run status = %s, want %s", run.Status, workflow.RunSucceeded)
	}

	if len(executor.connectorsReceived) != 2 {
		t.Fatalf("connectorsReceived = %d entries, want 2", len(executor.connectorsReceived))
	}

	bound := executor.connectorsReceived[0]
	if bound == nil {
		t.Fatal("first step's connector = nil, want the resolved connector")
	}
	if bound.Type != "postgresql.connection@1" {
		t.Fatalf("connector.Type = %q, want %q", bound.Type, "postgresql.connection@1")
	}
	if bound.Config["host"] != "db.internal" {
		t.Fatalf("connector.Config[host] = %v, want %q", bound.Config["host"], "db.internal")
	}
	if bound.Secrets["password"] != "s3cr3t" {
		t.Fatalf("connector.Secrets[password] = %v, want %q", bound.Secrets["password"], "s3cr3t")
	}

	if executor.connectorsReceived[1] != nil {
		t.Fatalf("second step's connector = %v, want nil (step bound none)", executor.connectorsReceived[1])
	}
}

func TestExecute_FailsTheStepWhenTheBoundConnectorCannotBeResolved(t *testing.T) {
	db := openTestDB(t)
	installTestWorkflow(t, db, `
schema_version: 1
id: connector_missing
version: 1
trigger:
  type: manual
steps:
  - id: only
    uses: text.uppercase@1
    connector: "${{ bindings.demo }}"
    with:
      value: "hello"
`)

	executor := &fakeExecutor{
		responses: map[string]map[string]any{"text.uppercase@1": {"value": "HELLO"}},
	}

	// "demo" is never bound to a connector id, so resolving the connector
	// must fail — before the executor is ever called.
	run, err := Execute(context.Background(), db, executor, "connector_missing", nil, nil, ExecuteOptions{})
	if err != nil {
		t.Fatalf("Execute() error = %v", err)
	}
	if run.Status != workflow.RunFailed {
		t.Fatalf("run status = %s, want %s", run.Status, workflow.RunFailed)
	}
	if len(executor.calls) != 0 {
		t.Fatalf("executor was called %d times, want 0 (should fail before calling the action)", len(executor.calls))
	}

	_, steps, err := GetRun(context.Background(), db, run.ID)
	if err != nil {
		t.Fatalf("GetRun() error = %v", err)
	}
	if steps[0].Status != workflow.StepFailed {
		t.Fatalf("step status = %s, want %s", steps[0].Status, workflow.StepFailed)
	}
	// The input had already resolved successfully before the connector
	// failed to resolve — it must still be persisted, not discarded.
	if steps[0].Input["value"] != "hello" {
		t.Fatalf(`steps[0].Input["value"] = %v, want %q (resolved input must survive a later connector failure)`, steps[0].Input["value"], "hello")
	}
}

func TestStart_CreatesARunningRunWithoutExecutingAnyStep(t *testing.T) {
	db := openTestDB(t)
	installTestWorkflow(t, db, helloWorkflow)

	def, run, _, err := Start(context.Background(), db, "hello_patchcord", map[string]any{"value": "hi"})
	if err != nil {
		t.Fatalf("Start() error = %v", err)
	}

	if def.ID != "hello_patchcord" {
		t.Fatalf("def.ID = %q, want %q", def.ID, "hello_patchcord")
	}
	if run.Status != workflow.RunRunning {
		t.Fatalf("run.Status = %s, want %s", run.Status, workflow.RunRunning)
	}

	_, steps, err := GetRun(context.Background(), db, run.ID)
	if err != nil {
		t.Fatalf("GetRun() error = %v", err)
	}
	if len(steps) != 1 || steps[0].Status != workflow.StepPending {
		t.Fatalf("steps = %+v, want a single pending step (Start must not run any step)", steps)
	}
}

func TestStart_UnknownWorkflowFailsFast(t *testing.T) {
	db := openTestDB(t)

	if _, _, _, err := Start(context.Background(), db, "unknown", nil); !errors.Is(err, ErrWorkflowNotFound) {
		t.Fatalf("Start() error = %v, want ErrWorkflowNotFound", err)
	}
}

func TestStart_AppliesDeclaredInputDefaults(t *testing.T) {
	db := openTestDB(t)
	installTestWorkflow(t, db, `
schema_version: 1
id: greet
version: 1
trigger:
  type: manual
inputs:
  - name: name
    type: string
    default: world
steps:
  - id: shout
    uses: text.uppercase@1
    with:
      value: "${{ workflow.inputs.name }}"
`)

	_, run, preparedInputs, err := Start(context.Background(), db, "greet", map[string]any{})
	if err != nil {
		t.Fatalf("Start() error = %v", err)
	}
	if preparedInputs["name"] != "world" {
		t.Fatalf(`preparedInputs["name"] = %v, want "world"`, preparedInputs["name"])
	}
	if run.Inputs["name"] != "world" {
		t.Fatalf(`run.Inputs["name"] = %v, want "world" (persisted run must reflect the applied default)`, run.Inputs["name"])
	}
}

func TestStart_RejectsMissingRequiredInput(t *testing.T) {
	db := openTestDB(t)
	installTestWorkflow(t, db, `
schema_version: 1
id: greet
version: 1
trigger:
  type: manual
inputs:
  - name: name
    type: string
    required: true
steps:
  - id: shout
    uses: text.uppercase@1
    with:
      value: "${{ workflow.inputs.name }}"
`)

	if _, _, _, err := Start(context.Background(), db, "greet", map[string]any{}); !errors.Is(err, workflow.ErrInvalidInputs) {
		t.Fatalf("Start() error = %v, want workflow.ErrInvalidInputs", err)
	}
}

func TestExecute_UsesPreparedInputsForExpressionResolution(t *testing.T) {
	db := openTestDB(t)
	installTestWorkflow(t, db, `
schema_version: 1
id: counted
version: 1
trigger:
  type: manual
inputs:
  - name: count
    type: number
steps:
  - id: echo
    uses: text.uppercase@1
    with:
      value: "${{ workflow.inputs.count }}"
`)

	executor := &fakeExecutor{
		responses: map[string]map[string]any{"text.uppercase@1": {"value": "42"}},
	}

	// A CLI-style string input ("42") must resolve through
	// ${{ workflow.inputs.count }} the same way a typed HTTP JSON number
	// would: Start's PrepareInputs coerces it to float64(42), and Execute
	// must forward that coerced value to Continue — not the original
	// string — for step input resolution to see the declared type.
	run, err := Execute(context.Background(), db, executor, "counted", map[string]any{"count": "42"}, nil, ExecuteOptions{})
	if err != nil {
		t.Fatalf("Execute() error = %v", err)
	}
	if run.Status != workflow.RunSucceeded {
		t.Fatalf("run status = %s, want %s", run.Status, workflow.RunSucceeded)
	}

	_, steps, err := GetRun(context.Background(), db, run.ID)
	if err != nil {
		t.Fatalf("GetRun() error = %v", err)
	}
	if len(steps) != 1 || steps[0].Input["value"] != float64(42) {
		t.Fatalf(`steps[0].Input["value"] = %v, want float64(42)`, steps[0].Input["value"])
	}
}

func TestContinue_DrivesAnAlreadyStartedRunToCompletion(t *testing.T) {
	db := openTestDB(t)
	installTestWorkflow(t, db, helloWorkflow)

	def, run, _, err := Start(context.Background(), db, "hello_patchcord", map[string]any{"value": "hi"})
	if err != nil {
		t.Fatalf("Start() error = %v", err)
	}

	executor := &fakeExecutor{
		responses: map[string]map[string]any{"text.uppercase@1": {"value": "HI"}},
	}

	if err := Continue(context.Background(), db, executor, def, run, map[string]any{"value": "hi"}, nil, ExecuteOptions{}); err != nil {
		t.Fatalf("Continue() error = %v", err)
	}

	if run.Status != workflow.RunSucceeded {
		t.Fatalf("run.Status = %s, want %s", run.Status, workflow.RunSucceeded)
	}
	if run.Outputs["value"] != "HI" {
		t.Fatalf(`run.Outputs["value"] = %v, want "HI"`, run.Outputs["value"])
	}
}

// TestStartThenBackgroundContinue rehearses exactly what internal/api's
// async workflow-run handler does: call Start synchronously to get a run id
// right away, then run Continue in a background goroutine while a
// concurrent WatchRun observes its progress — proving the split is safe to
// use that way before internal/api builds on top of it.
func TestStartThenBackgroundContinue(t *testing.T) {
	db := openTestDB(t)
	installTestWorkflow(t, db, helloWorkflow)

	def, run, _, err := Start(context.Background(), db, "hello_patchcord", map[string]any{"value": "hi"})
	if err != nil {
		t.Fatalf("Start() error = %v", err)
	}

	events, err := WatchRun(context.Background(), db, run.ID)
	if err != nil {
		t.Fatalf("WatchRun() error = %v", err)
	}

	executor := &fakeExecutor{
		responses: map[string]map[string]any{"text.uppercase@1": {"value": "HI"}},
	}
	go func() {
		if err := Continue(context.Background(), db, executor, def, run, map[string]any{"value": "hi"}, nil, ExecuteOptions{}); err != nil {
			t.Errorf("Continue() error = %v", err)
		}
	}()

	var sawTerminalRunEvent bool
	timeout := time.After(3 * time.Second)
	for {
		select {
		case e, ok := <-events:
			if !ok {
				if !sawTerminalRunEvent {
					t.Fatal("events channel closed before observing a terminal run event")
				}
				return
			}
			if e.StepID == "" && e.Status == string(workflow.RunSucceeded) {
				sawTerminalRunEvent = true
			}
		case <-timeout:
			t.Fatal("did not observe the run reach a terminal status in time")
		}
	}
}
