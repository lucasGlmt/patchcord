package runs

import (
	"context"
	"errors"
	"testing"

	"github.com/lucasglmt/patchcord/internal/workflow"
)

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

	run, err := Execute(context.Background(), db, executor, "chained", map[string]any{"value": "hello"})
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

	run, err := Execute(context.Background(), db, executor, "chained", nil)
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

func TestExecute_UnknownWorkflowFailsFast(t *testing.T) {
	db := openTestDB(t)
	executor := &fakeExecutor{}

	_, err := Execute(context.Background(), db, executor, "unknown", nil)
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
	run, err := Execute(context.Background(), db, executor, "needs_input", nil)
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
