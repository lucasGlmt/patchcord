package workflow

import (
	"os"
	"path/filepath"
	"testing"
)

func TestScaffold_WritesAValidDefinition(t *testing.T) {
	path := filepath.Join(t.TempDir(), "my_workflow.yaml")

	if err := Scaffold(path, "my_workflow", 1); err != nil {
		t.Fatalf("Scaffold() error = %v", err)
	}

	source, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("read scaffolded file: %v", err)
	}

	def, err := Parse(source)
	if err != nil {
		t.Fatalf("Parse() error = %v", err)
	}
	if def.ID != "my_workflow" {
		t.Fatalf("ID = %q, want %q", def.ID, "my_workflow")
	}
	if def.Version != 1 {
		t.Fatalf("Version = %d, want 1", def.Version)
	}
	if len(def.Steps) != 1 {
		t.Fatalf("len(Steps) = %d, want 1", len(def.Steps))
	}

	knownActions := map[string]struct{}{"text.uppercase@1": {}}
	if err := Validate(def, knownActions); err != nil {
		t.Fatalf("Validate() error = %v, want the scaffold to validate once its reference action is known", err)
	}
}

func TestScaffold_RefusesToOverwrite(t *testing.T) {
	path := filepath.Join(t.TempDir(), "my_workflow.yaml")
	if err := Scaffold(path, "my_workflow", 1); err != nil {
		t.Fatalf("first Scaffold() error = %v", err)
	}

	if err := Scaffold(path, "my_workflow", 1); err == nil {
		t.Fatal("expected an error for an existing path, got nil")
	}
}
