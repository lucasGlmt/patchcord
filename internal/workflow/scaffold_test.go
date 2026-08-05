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

func TestScaffoldTemplate_Foreach(t *testing.T) {
	path := filepath.Join(t.TempDir(), "my_workflow.yaml")

	if err := ScaffoldTemplate(path, "my_workflow", 1, ScaffoldTemplateForeach); err != nil {
		t.Fatalf("ScaffoldTemplate() error = %v", err)
	}

	source, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("read scaffolded file: %v", err)
	}

	def, err := Parse(source)
	if err != nil {
		t.Fatalf("Parse() error = %v", err)
	}
	if len(def.Steps) != 1 {
		t.Fatalf("len(Steps) = %d, want 1", len(def.Steps))
	}
	if def.Steps[0].Foreach == nil {
		t.Fatal("Steps[0].Foreach is nil, want the scaffolded literal list")
	}

	knownActions := map[string]struct{}{"text.uppercase@1": {}}
	if err := Validate(def, knownActions); err != nil {
		t.Fatalf("Validate() error = %v, want the foreach scaffold to validate once its reference action is known", err)
	}
}

func TestScaffoldTemplate_Minimal_MatchesScaffold(t *testing.T) {
	viaTemplate := filepath.Join(t.TempDir(), "a.yaml")
	viaScaffold := filepath.Join(t.TempDir(), "b.yaml")

	if err := ScaffoldTemplate(viaTemplate, "my_workflow", 1, ScaffoldTemplateMinimal); err != nil {
		t.Fatalf("ScaffoldTemplate() error = %v", err)
	}
	if err := Scaffold(viaScaffold, "my_workflow", 1); err != nil {
		t.Fatalf("Scaffold() error = %v", err)
	}

	a, err := os.ReadFile(viaTemplate)
	if err != nil {
		t.Fatalf("read %q: %v", viaTemplate, err)
	}
	b, err := os.ReadFile(viaScaffold)
	if err != nil {
		t.Fatalf("read %q: %v", viaScaffold, err)
	}
	if string(a) != string(b) {
		t.Fatalf("ScaffoldTemplate(..., ScaffoldTemplateMinimal) output differs from Scaffold()'s")
	}
}

func TestScaffoldTemplate_RejectsUnknownTemplate(t *testing.T) {
	path := filepath.Join(t.TempDir(), "my_workflow.yaml")

	if err := ScaffoldTemplate(path, "my_workflow", 1, "bogus"); err == nil {
		t.Fatal("expected an error for an unknown template, got nil")
	}
	if _, err := os.Stat(path); !os.IsNotExist(err) {
		t.Fatal("ScaffoldTemplate() must not write a file when the template is invalid")
	}
}
