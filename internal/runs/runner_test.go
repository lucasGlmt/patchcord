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
