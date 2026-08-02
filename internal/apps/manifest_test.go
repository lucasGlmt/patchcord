package apps

import (
	"errors"
	"os"
	"path/filepath"
	"testing"
)

func TestParseManifest(t *testing.T) {
	t.Run("parses a valid manifest", func(t *testing.T) {
		source := []byte(`
id: dashboard
version: "0.1.0"
permissions:
  workflows:
    run:
      - hello_patchcord
      - greet_twice
`)
		m, err := ParseManifest(source)
		if err != nil {
			t.Fatalf("ParseManifest() error = %v", err)
		}
		if m.ID != "dashboard" {
			t.Fatalf("ID = %q, want %q", m.ID, "dashboard")
		}
		if m.Version != "0.1.0" {
			t.Fatalf("Version = %q, want %q", m.Version, "0.1.0")
		}
		want := []string{"hello_patchcord", "greet_twice"}
		if len(m.Permissions.WorkflowsRun) != len(want) {
			t.Fatalf("Permissions.WorkflowsRun = %v, want %v", m.Permissions.WorkflowsRun, want)
		}
		for i := range want {
			if m.Permissions.WorkflowsRun[i] != want[i] {
				t.Fatalf("Permissions.WorkflowsRun = %v, want %v", m.Permissions.WorkflowsRun, want)
			}
		}
	})

	t.Run("accepts an empty workflows.run list", func(t *testing.T) {
		source := []byte(`
id: dashboard
version: "0.1.0"
permissions:
  workflows:
    run: []
`)
		m, err := ParseManifest(source)
		if err != nil {
			t.Fatalf("ParseManifest() error = %v", err)
		}
		if len(m.Permissions.WorkflowsRun) != 0 {
			t.Fatalf("Permissions.WorkflowsRun = %v, want empty", m.Permissions.WorkflowsRun)
		}
	})

	t.Run("rejects malformed YAML", func(t *testing.T) {
		_, err := ParseManifest([]byte("id: [this is not"))
		if !errors.Is(err, ErrInvalidManifest) {
			t.Fatalf("ParseManifest() error = %v, want ErrInvalidManifest", err)
		}
	})

	t.Run("rejects an empty id", func(t *testing.T) {
		source := []byte(`
version: "0.1.0"
permissions:
  workflows:
    run: []
`)
		if _, err := ParseManifest(source); !errors.Is(err, ErrInvalidManifest) {
			t.Fatalf("ParseManifest() error = %v, want ErrInvalidManifest", err)
		}
	})

	t.Run("rejects an empty version", func(t *testing.T) {
		source := []byte(`
id: dashboard
permissions:
  workflows:
    run: []
`)
		if _, err := ParseManifest(source); !errors.Is(err, ErrInvalidManifest) {
			t.Fatalf("ParseManifest() error = %v, want ErrInvalidManifest", err)
		}
	})

	t.Run("rejects an empty entry in workflows.run", func(t *testing.T) {
		source := []byte(`
id: dashboard
version: "0.1.0"
permissions:
  workflows:
    run:
      - ""
`)
		if _, err := ParseManifest(source); !errors.Is(err, ErrInvalidManifest) {
			t.Fatalf("ParseManifest() error = %v, want ErrInvalidManifest", err)
		}
	})
}

func TestLoadManifest(t *testing.T) {
	t.Run("reads and parses patchcord-app.yaml from a directory", func(t *testing.T) {
		dir := t.TempDir()
		writeManifest(t, dir, "dashboard", "0.1.0", "hello_patchcord")

		m, err := LoadManifest(dir)
		if err != nil {
			t.Fatalf("LoadManifest() error = %v", err)
		}
		if m.ID != "dashboard" {
			t.Fatalf("ID = %q, want %q", m.ID, "dashboard")
		}
	})

	t.Run("fails when the manifest file is missing", func(t *testing.T) {
		if _, err := LoadManifest(t.TempDir()); err == nil {
			t.Fatal("expected an error for a missing manifest, got nil")
		}
	})
}

// writeManifest writes a minimal, valid patchcord-app.yaml into dir.
func writeManifest(t *testing.T, dir, id, version, workflowID string) {
	t.Helper()

	content := "id: " + id + "\nversion: \"" + version + "\"\npermissions:\n  workflows:\n    run:\n      - " + workflowID + "\n"
	if err := os.WriteFile(filepath.Join(dir, ManifestFileName), []byte(content), 0o644); err != nil {
		t.Fatalf("write manifest: %v", err)
	}
}
