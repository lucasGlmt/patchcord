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
