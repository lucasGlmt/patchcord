package cli

import (
	"bytes"
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

const helloPatchcordYAML = `
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

func writeWorkflowFile(t *testing.T, source string) string {
	t.Helper()
	path := filepath.Join(t.TempDir(), "workflow.yaml")
	if err := os.WriteFile(path, []byte(source), 0o644); err != nil {
		t.Fatalf("write workflow file: %v", err)
	}
	return path
}

func TestNewRootCommand_HasWorkflowAndRunSubcommands(t *testing.T) {
	root := NewRootCommand()

	for _, path := range [][]string{
		{"workflow", "install"},
		{"workflow", "run"},
		{"run", "inspect"},
	} {
		t.Run(strings.Join(path, " "), func(t *testing.T) {
			cmd, _, err := root.Find(path)
			if err != nil {
				t.Fatalf("Find(%v) error = %v", path, err)
			}
			if cmd.Name() != path[len(path)-1] {
				t.Fatalf("found command %q, want %q", cmd.Name(), path[len(path)-1])
			}
		})
	}
}

func TestWorkflowInstallCommand_FailsForAMissingFile(t *testing.T) {
	cmd := newWorkflowInstallCommand()
	cmd.SetArgs([]string{filepath.Join(t.TempDir(), "does-not-exist.yaml"), "--data-dir", t.TempDir()})
	cmd.SetContext(context.Background())

	if err := cmd.Execute(); err == nil {
		t.Fatal("expected an error for a missing workflow file, got nil")
	}
}

func TestWorkflowInstallCommand_FailsForAnUnknownAction(t *testing.T) {
	path := writeWorkflowFile(t, `
schema_version: 1
id: broken
version: 1
trigger:
  type: manual
steps:
  - id: step
    uses: does.not.exist@1
`)

	cmd := newWorkflowInstallCommand()
	cmd.SetArgs([]string{path, "--data-dir", t.TempDir()})
	cmd.SetContext(context.Background())

	if err := cmd.Execute(); err == nil {
		t.Fatal("expected an error for an unknown action, got nil")
	}
}

// TestWorkflowLifecycle_HelloPatchcordEndToEnd reproduces the vision
// document's section 20 reference example through the actual CLI commands
// a user types: install the plugin, install the workflow, run it, then
// inspect the resulting run.
func TestWorkflowLifecycle_HelloPatchcordEndToEnd(t *testing.T) {
	dataDir := t.TempDir()

	pluginInstall := newPluginInstallCommand()
	pluginInstall.SetArgs([]string{examplePluginPath, "--data-dir", dataDir})
	pluginInstall.SetContext(context.Background())
	if err := pluginInstall.Execute(); err != nil {
		t.Fatalf("plugin install error = %v", err)
	}

	workflowPath := writeWorkflowFile(t, helloPatchcordYAML)
	workflowInstall := newWorkflowInstallCommand()
	workflowInstall.SetArgs([]string{workflowPath, "--data-dir", dataDir})
	workflowInstall.SetContext(context.Background())
	var installOut bytes.Buffer
	workflowInstall.SetOut(&installOut)
	if err := workflowInstall.Execute(); err != nil {
		t.Fatalf("workflow install error = %v", err)
	}
	if !strings.Contains(installOut.String(), "hello_patchcord") {
		t.Fatalf("install output = %q, want it to mention hello_patchcord", installOut.String())
	}

	workflowRun := newWorkflowRunCommand()
	workflowRun.SetArgs([]string{"hello_patchcord", "--data-dir", dataDir})
	workflowRun.SetContext(context.Background())
	var runOut bytes.Buffer
	workflowRun.SetOut(&runOut)
	if err := workflowRun.Execute(); err != nil {
		t.Fatalf("workflow run error = %v", err)
	}

	runOutput := runOut.String()
	if !strings.Contains(runOutput, "status: succeeded") {
		t.Fatalf("run output = %q, want status: succeeded", runOutput)
	}
	if !strings.Contains(runOutput, "WELCOME PATCHCORD") {
		t.Fatalf("run output = %q, want it to contain WELCOME PATCHCORD", runOutput)
	}

	runID := extractRunID(t, runOutput)

	runInspect := newRunInspectCommand()
	runInspect.SetArgs([]string{runID, "--data-dir", dataDir})
	runInspect.SetContext(context.Background())
	var inspectOut bytes.Buffer
	runInspect.SetOut(&inspectOut)
	if err := runInspect.Execute(); err != nil {
		t.Fatalf("run inspect error = %v", err)
	}

	inspectOutput := inspectOut.String()
	if !strings.Contains(inspectOutput, "transform: succeeded") {
		t.Fatalf("inspect output = %q, want it to show the transform step as succeeded", inspectOutput)
	}
}

func extractRunID(t *testing.T, runOutput string) string {
	t.Helper()
	for _, line := range strings.Split(runOutput, "\n") {
		if strings.HasPrefix(line, "run:") {
			return strings.TrimSpace(strings.TrimPrefix(line, "run:"))
		}
	}
	t.Fatalf("could not find a \"run:\" line in output %q", runOutput)
	return ""
}
