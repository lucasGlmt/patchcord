package runs

import (
	"context"
	"errors"
	"strings"
	"testing"

	"github.com/lucasglmt/patchcord/internal/metrics"
	"github.com/lucasglmt/patchcord/internal/workflow"
)

const helloWorkflow = `
schema_version: 1
id: hello_patchcord
version: 1
trigger:
  type: manual
steps:
  - id: transform
    uses: text.uppercase@1
    with:
      value: "Welcome Patchcord"
`

func TestInstallWorkflow(t *testing.T) {
	t.Run("installs a well-formed workflow", func(t *testing.T) {
		db := openTestDB(t)
		def := installTestWorkflow(t, db, helloWorkflow)
		if def.ID != "hello_patchcord" {
			t.Fatalf("ID = %q, want %q", def.ID, "hello_patchcord")
		}
	})

	t.Run("rejects a workflow using an unknown action", func(t *testing.T) {
		db := openTestDB(t)
		source := `
schema_version: 1
id: broken
version: 1
trigger:
  type: manual
steps:
  - id: step
    uses: does.not.exist@1
`
		if _, err := InstallWorkflow(context.Background(), db, []byte(source), knownActions); err == nil {
			t.Fatal("expected an error, got nil")
		}
	})

	t.Run("rejects reinstalling the exact same version", func(t *testing.T) {
		db := openTestDB(t)
		installTestWorkflow(t, db, helloWorkflow)

		if _, err := InstallWorkflow(context.Background(), db, []byte(helloWorkflow), knownActions); err == nil {
			t.Fatal("expected an error re-installing the same (id, version), got nil")
		}
	})

	t.Run("accepts a new, higher version of an existing workflow", func(t *testing.T) {
		db := openTestDB(t)
		installTestWorkflow(t, db, helloWorkflow)

		v2 := `
schema_version: 1
id: hello_patchcord
version: 2
trigger:
  type: manual
steps:
  - id: transform
    uses: text.uppercase@1
    with:
      value: "Welcome Patchcord v2"
`
		def := installTestWorkflow(t, db, v2)
		if def.Version != 2 {
			t.Fatalf("Version = %d, want 2", def.Version)
		}
	})
}

func TestInstallWorkflowAtVersion(t *testing.T) {
	t.Run("records source under version, normalizing its own declared version field to match", func(t *testing.T) {
		db := openTestDB(t)

		def, err := InstallWorkflowAtVersion(context.Background(), db, []byte(helloWorkflow), 7, knownActions)
		if err != nil {
			t.Fatalf("InstallWorkflowAtVersion() error = %v", err)
		}
		if def.Version != 7 {
			t.Fatalf("Version = %d, want 7 (the requested version, not the declared version: 1)", def.Version)
		}

		// The stored copy's own `version:` field is normalized to 7 too
		// (workflow.RewriteVersion) — otherwise re-parsing it later (as
		// this call itself does) would disagree with the row it came
		// from.
		source, err := WorkflowSource(context.Background(), db, "hello_patchcord", 7)
		if err != nil {
			t.Fatalf("WorkflowSource() error = %v", err)
		}
		want := strings.Replace(helloWorkflow, "\nversion: 1\n", "\nversion: 7\n", 1)
		if source != want {
			t.Fatalf("WorkflowSource() = %q, want %q", source, want)
		}

		// Re-parsing the stored copy must agree with the DB row.
		reparsed, err := workflow.Parse([]byte(source))
		if err != nil {
			t.Fatalf("workflow.Parse(stored source) error = %v", err)
		}
		if reparsed.Version != 7 {
			t.Fatalf("re-parsed stored source's Version = %d, want 7", reparsed.Version)
		}
	})

	t.Run("rejects a workflow using an unknown action, same as InstallWorkflow", func(t *testing.T) {
		db := openTestDB(t)
		source := `
schema_version: 1
id: broken
version: 1
trigger:
  type: manual
steps:
  - id: step
    uses: does.not.exist@1
`
		if _, err := InstallWorkflowAtVersion(context.Background(), db, []byte(source), 1, knownActions); err == nil {
			t.Fatal("expected an error, got nil")
		}
	})

	t.Run("rejects reinstalling an already-recorded version", func(t *testing.T) {
		db := openTestDB(t)
		if _, err := InstallWorkflowAtVersion(context.Background(), db, []byte(helloWorkflow), 1, knownActions); err != nil {
			t.Fatalf("first InstallWorkflowAtVersion() error = %v", err)
		}

		if _, err := InstallWorkflowAtVersion(context.Background(), db, []byte(helloWorkflow), 1, knownActions); err == nil {
			t.Fatal("expected an error re-installing version 1, got nil")
		}
	})
}

func TestNextWorkflowVersion(t *testing.T) {
	t.Run("returns 1 when no version is installed yet", func(t *testing.T) {
		db := openTestDB(t)

		next, err := NextWorkflowVersion(context.Background(), db, "hello_patchcord")
		if err != nil {
			t.Fatalf("NextWorkflowVersion() error = %v", err)
		}
		if next != 1 {
			t.Fatalf("NextWorkflowVersion() = %d, want 1", next)
		}
	})

	t.Run("returns one past the highest installed version", func(t *testing.T) {
		db := openTestDB(t)
		installTestWorkflow(t, db, helloWorkflow) // version 1

		v2 := `
schema_version: 1
id: hello_patchcord
version: 2
trigger:
  type: manual
steps:
  - id: transform
    uses: text.uppercase@1
    with:
      value: "v2"
`
		installTestWorkflow(t, db, v2)

		next, err := NextWorkflowVersion(context.Background(), db, "hello_patchcord")
		if err != nil {
			t.Fatalf("NextWorkflowVersion() error = %v", err)
		}
		if next != 3 {
			t.Fatalf("NextWorkflowVersion() = %d, want 3", next)
		}
	})
}

func TestLatestWorkflow(t *testing.T) {
	t.Run("returns the highest installed version", func(t *testing.T) {
		db := openTestDB(t)
		installTestWorkflow(t, db, helloWorkflow)

		v2 := `
schema_version: 1
id: hello_patchcord
version: 2
trigger:
  type: manual
steps:
  - id: transform
    uses: text.uppercase@1
    with:
      value: "v2"
`
		installTestWorkflow(t, db, v2)

		def, err := LatestWorkflow(context.Background(), db, "hello_patchcord")
		if err != nil {
			t.Fatalf("LatestWorkflow() error = %v", err)
		}
		if def.Version != 2 {
			t.Fatalf("Version = %d, want 2", def.Version)
		}
	})

	t.Run("returns ErrWorkflowNotFound for an unknown id", func(t *testing.T) {
		db := openTestDB(t)

		_, err := LatestWorkflow(context.Background(), db, "unknown")
		if !errors.Is(err, ErrWorkflowNotFound) {
			t.Fatalf("LatestWorkflow() error = %v, want ErrWorkflowNotFound", err)
		}
	})
}

func TestGetRun_ReturnsErrRunNotFoundForAnUnknownID(t *testing.T) {
	db := openTestDB(t)

	_, _, err := GetRun(context.Background(), db, "unknown")
	if !errors.Is(err, ErrRunNotFound) {
		t.Fatalf("GetRun() error = %v, want ErrRunNotFound", err)
	}
}

func TestListWorkflows(t *testing.T) {
	db := openTestDB(t)
	installTestWorkflow(t, db, helloWorkflow)
	installTestWorkflow(t, db, `
schema_version: 1
id: other
version: 1
trigger:
  type: manual
steps:
  - id: only
    uses: text.uppercase@1
    with:
      value: "x"
`)

	versions, err := ListWorkflows(context.Background(), db)
	if err != nil {
		t.Fatalf("ListWorkflows() error = %v", err)
	}
	if len(versions) != 2 {
		t.Fatalf("len(versions) = %d, want 2", len(versions))
	}
}

func TestWorkflowSource(t *testing.T) {
	db := openTestDB(t)
	installTestWorkflow(t, db, helloWorkflow)

	t.Run("returns the source of the latest version when version is 0", func(t *testing.T) {
		source, err := WorkflowSource(context.Background(), db, "hello_patchcord", 0)
		if err != nil {
			t.Fatalf("WorkflowSource() error = %v", err)
		}
		if source != helloWorkflow {
			t.Fatalf("source = %q, want %q", source, helloWorkflow)
		}
	})

	t.Run("returns ErrWorkflowNotFound for an unknown id", func(t *testing.T) {
		_, err := WorkflowSource(context.Background(), db, "unknown", 0)
		if !errors.Is(err, ErrWorkflowNotFound) {
			t.Fatalf("WorkflowSource() error = %v, want ErrWorkflowNotFound", err)
		}
	})

	t.Run("returns ErrWorkflowNotFound for an unknown version", func(t *testing.T) {
		_, err := WorkflowSource(context.Background(), db, "hello_patchcord", 99)
		if !errors.Is(err, ErrWorkflowNotFound) {
			t.Fatalf("WorkflowSource() error = %v, want ErrWorkflowNotFound", err)
		}
	})
}

func TestListRuns(t *testing.T) {
	db := openTestDB(t)
	installTestWorkflow(t, db, helloWorkflow)

	executor := &fakeExecutor{responses: map[string]map[string]any{"text.uppercase@1": {"value": "X"}}}
	if _, err := Execute(context.Background(), db, executor, "hello_patchcord", nil, nil, ExecuteOptions{}); err != nil {
		t.Fatalf("Execute() error = %v", err)
	}
	if _, err := Execute(context.Background(), db, executor, "hello_patchcord", nil, nil, ExecuteOptions{}); err != nil {
		t.Fatalf("Execute() error = %v", err)
	}

	t.Run("lists every run when workflowID is empty", func(t *testing.T) {
		runList, err := ListRuns(context.Background(), db, "")
		if err != nil {
			t.Fatalf("ListRuns() error = %v", err)
		}
		if len(runList) != 2 {
			t.Fatalf("len(runList) = %d, want 2", len(runList))
		}
	})

	t.Run("filters by workflow id", func(t *testing.T) {
		runList, err := ListRuns(context.Background(), db, "does-not-exist")
		if err != nil {
			t.Fatalf("ListRuns() error = %v", err)
		}
		if len(runList) != 0 {
			t.Fatalf("len(runList) = %d, want 0", len(runList))
		}
	})
}

func TestCancelRun(t *testing.T) {
	db := openTestDB(t)
	installTestWorkflow(t, db, helloWorkflow)

	t.Run("returns ErrRunNotFound for an unknown id", func(t *testing.T) {
		err := CancelRun(context.Background(), db, "unknown", nil)
		if !errors.Is(err, ErrRunNotFound) {
			t.Fatalf("CancelRun() error = %v, want ErrRunNotFound", err)
		}
	})

	t.Run("returns ErrRunNotCancellable for a finished run", func(t *testing.T) {
		executor := &fakeExecutor{responses: map[string]map[string]any{"text.uppercase@1": {"value": "X"}}}
		run, err := Execute(context.Background(), db, executor, "hello_patchcord", nil, nil, ExecuteOptions{})
		if err != nil {
			t.Fatalf("Execute() error = %v", err)
		}

		err = CancelRun(context.Background(), db, run.ID, nil)
		if !errors.Is(err, ErrRunNotCancellable) {
			t.Fatalf("CancelRun() error = %v, want ErrRunNotCancellable", err)
		}
	})

	t.Run("cancels a run stuck in a non-terminal state", func(t *testing.T) {
		def, err := LatestWorkflow(context.Background(), db, "hello_patchcord")
		if err != nil {
			t.Fatalf("LatestWorkflow() error = %v", err)
		}
		run, err := createRun(context.Background(), db, def, nil)
		if err != nil {
			t.Fatalf("createRun() error = %v", err)
		}

		m := metrics.New()
		if err := CancelRun(context.Background(), db, run.ID, m); err != nil {
			t.Fatalf("CancelRun() error = %v", err)
		}
		if got := m.Snapshot().Runs.Transitions["cancelled"]; got != 1 {
			t.Fatalf("run_transitions_total{status=cancelled} = %d, want 1", got)
		}

		got, steps, err := GetRun(context.Background(), db, run.ID)
		if err != nil {
			t.Fatalf("GetRun() error = %v", err)
		}
		if got.Status != workflow.RunCancelled {
			t.Fatalf("run status = %s, want %s", got.Status, workflow.RunCancelled)
		}
		for _, step := range steps {
			if step.Status != workflow.StepCancelled {
				t.Fatalf("step %q status = %s, want %s", step.StepID, step.Status, workflow.StepCancelled)
			}
		}
	})
}
