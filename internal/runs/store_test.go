package runs

import (
	"context"
	"errors"
	"testing"
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
