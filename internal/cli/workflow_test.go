package cli

import (
	"bytes"
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/lucasglmt/patchcord/internal/workflow"
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
		{"workflow", "new"},
		{"workflow", "install"},
		{"workflow", "list"},
		{"workflow", "validate"},
		{"workflow", "export"},
		{"workflow", "run"},
		{"run", "list"},
		{"run", "inspect"},
		{"run", "logs"},
		{"run", "cancel"},
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

func TestWorkflowValidateCommand_Success(t *testing.T) {
	dataDir := t.TempDir()

	pluginInstall := newPluginInstallCommand()
	pluginInstall.SetArgs([]string{examplePluginPath, "--data-dir", dataDir})
	pluginInstall.SetContext(context.Background())
	if err := pluginInstall.Execute(); err != nil {
		t.Fatalf("plugin install error = %v", err)
	}

	path := writeWorkflowFile(t, helloPatchcordYAML)
	cmd := newWorkflowValidateCommand()
	cmd.SetArgs([]string{path, "--data-dir", dataDir})
	cmd.SetContext(context.Background())
	var out bytes.Buffer
	cmd.SetOut(&out)

	if err := cmd.Execute(); err != nil {
		t.Fatalf("workflow validate error = %v", err)
	}
	if !strings.Contains(out.String(), "is valid") {
		t.Fatalf("output = %q, want it to say the workflow is valid", out.String())
	}

	// validate must not install anything.
	list, err := newWorkflowListOutput(t, dataDir)
	if err != nil {
		t.Fatalf("workflow list error = %v", err)
	}
	if strings.Contains(list, "hello_patchcord") {
		t.Fatalf("workflow list = %q, want validate not to have installed the workflow", list)
	}
}

func TestWorkflowValidateCommand_FailsForAnUnknownAction(t *testing.T) {
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

	cmd := newWorkflowValidateCommand()
	cmd.SetArgs([]string{path, "--data-dir", t.TempDir()})
	cmd.SetContext(context.Background())

	if err := cmd.Execute(); err == nil {
		t.Fatal("expected an error for an unknown action, got nil")
	}
}

func TestWorkflowExportCommand_UnknownWorkflow(t *testing.T) {
	cmd := newWorkflowExportCommand()
	cmd.SetArgs([]string{"unknown", "--data-dir", t.TempDir()})
	cmd.SetContext(context.Background())

	err := cmd.Execute()
	if err == nil {
		t.Fatal("expected an error for an unknown workflow, got nil")
	}
	if !strings.Contains(err.Error(), "not found") {
		t.Fatalf("error = %q, want it to mention the workflow was not found", err.Error())
	}
}

func newWorkflowListOutput(t *testing.T, dataDir string) (string, error) {
	t.Helper()
	cmd := newWorkflowListCommand()
	cmd.SetArgs([]string{"--data-dir", dataDir})
	cmd.SetContext(context.Background())
	var out bytes.Buffer
	cmd.SetOut(&out)
	err := cmd.Execute()
	return out.String(), err
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

	listOutput, err := newWorkflowListOutput(t, dataDir)
	if err != nil {
		t.Fatalf("workflow list error = %v", err)
	}
	if !strings.Contains(listOutput, "hello_patchcord") {
		t.Fatalf("workflow list output = %q, want it to mention hello_patchcord", listOutput)
	}

	exportCmd := newWorkflowExportCommand()
	exportCmd.SetArgs([]string{"hello_patchcord", "--data-dir", dataDir})
	exportCmd.SetContext(context.Background())
	var exportOut bytes.Buffer
	exportCmd.SetOut(&exportOut)
	if err := exportCmd.Execute(); err != nil {
		t.Fatalf("workflow export error = %v", err)
	}
	if exportOut.String() != helloPatchcordYAML {
		t.Fatalf("exported source = %q, want the original source back verbatim", exportOut.String())
	}

	exportPath := filepath.Join(t.TempDir(), "hello_patchcord"+workflow.FileExtension)
	exportToFileCmd := newWorkflowExportCommand()
	exportToFileCmd.SetArgs([]string{"hello_patchcord", "--data-dir", dataDir, "--output", exportPath})
	exportToFileCmd.SetContext(context.Background())
	var exportToFileOut bytes.Buffer
	exportToFileCmd.SetOut(&exportToFileOut)
	if err := exportToFileCmd.Execute(); err != nil {
		t.Fatalf("workflow export --output error = %v", err)
	}
	if !strings.Contains(exportToFileOut.String(), exportPath) {
		t.Fatalf("export --output output = %q, want it to mention %q", exportToFileOut.String(), exportPath)
	}
	exportedBody, err := os.ReadFile(exportPath)
	if err != nil {
		t.Fatalf("read exported file: %v", err)
	}
	if string(exportedBody) != helloPatchcordYAML {
		t.Fatalf("exported file content = %q, want the original source back verbatim", exportedBody)
	}

	runList := newRunListCommand()
	runList.SetArgs([]string{"--data-dir", dataDir})
	runList.SetContext(context.Background())
	var runListOut bytes.Buffer
	runList.SetOut(&runListOut)
	if err := runList.Execute(); err != nil {
		t.Fatalf("run list error = %v", err)
	}
	if !strings.Contains(runListOut.String(), runID) {
		t.Fatalf("run list output = %q, want it to contain %q", runListOut.String(), runID)
	}

	runLogs := newRunLogsCommand()
	runLogs.SetArgs([]string{runID, "--data-dir", dataDir})
	runLogs.SetContext(context.Background())
	var runLogsOut bytes.Buffer
	runLogs.SetOut(&runLogsOut)
	if err := runLogs.Execute(); err != nil {
		t.Fatalf("run logs error = %v", err)
	}
	if !strings.Contains(runLogsOut.String(), "transform") || !strings.Contains(runLogsOut.String(), "succeeded") {
		t.Fatalf("run logs output = %q, want it to mention the transform step succeeding", runLogsOut.String())
	}

	// The run already finished, so cancelling it now must fail.
	runCancel := newRunCancelCommand()
	runCancel.SetArgs([]string{runID, "--data-dir", dataDir})
	runCancel.SetContext(context.Background())
	if err := runCancel.Execute(); err == nil {
		t.Fatal("expected an error cancelling an already-finished run, got nil")
	}
}

// TestWorkflowNewCommand_ThenValidate exercises `workflow new` through to
// `workflow validate`: the scaffold references text.uppercase@1 (the
// reference plugin), so validation must fail before that plugin is
// installed and succeed right after — proving the scaffold isn't just
// syntactically valid YAML but an actually runnable example once its one
// dependency is met.
func TestWorkflowNewCommand_ThenValidate(t *testing.T) {
	dataDir := t.TempDir()
	path := filepath.Join(t.TempDir(), "scaffold-test.yaml")

	newCmd := newWorkflowNewCommand()
	newCmd.SetArgs([]string{"scaffold_test", "--output", path})
	newCmd.SetContext(context.Background())
	var newOut bytes.Buffer
	newCmd.SetOut(&newOut)
	if err := newCmd.Execute(); err != nil {
		t.Fatalf("workflow new error = %v", err)
	}
	if !strings.Contains(newOut.String(), path) {
		t.Fatalf("new output = %q, want it to mention %q", newOut.String(), path)
	}

	validateBefore := newWorkflowValidateCommand()
	validateBefore.SetArgs([]string{path, "--data-dir", dataDir})
	validateBefore.SetContext(context.Background())
	if err := validateBefore.Execute(); err == nil {
		t.Fatal("expected workflow validate to fail before the reference plugin is installed, got nil error")
	}

	pluginInstall := newPluginInstallCommand()
	pluginInstall.SetArgs([]string{examplePluginPath, "--data-dir", dataDir})
	pluginInstall.SetContext(context.Background())
	if err := pluginInstall.Execute(); err != nil {
		t.Fatalf("plugin install error = %v", err)
	}

	validateAfter := newWorkflowValidateCommand()
	validateAfter.SetArgs([]string{path, "--data-dir", dataDir})
	validateAfter.SetContext(context.Background())
	if err := validateAfter.Execute(); err != nil {
		t.Fatalf("workflow validate error = %v, want the scaffold to validate once its reference action is installed", err)
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
