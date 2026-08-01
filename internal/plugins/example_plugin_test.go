package plugins

import (
	"context"
	"slices"
	"testing"
	"time"
)

// TestExamplePlugin_UppercasesEndToEnd exercises the whole chain the
// vision document's section 20 vertical slice describes: launch a real
// plugin binary (built on the actual SDK), complete the handshake, and
// execute its action — all without the agent knowing anything about the
// plugin's implementation.
func TestExamplePlugin_UppercasesEndToEnd(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	proc, err := Launch(ctx, examplePluginPath, time.Second)
	if err != nil {
		t.Fatalf("Launch() error = %v", err)
	}
	defer func() {
		closeCtx, closeCancel := context.WithTimeout(context.Background(), 2*time.Second)
		defer closeCancel()
		if err := proc.Close(closeCtx); err != nil {
			t.Errorf("Close() error = %v", err)
		}
	}()

	manifest, err := Handshake(ctx, proc.Client)
	if err != nil {
		t.Fatalf("Handshake() error = %v", err)
	}
	if manifest.PluginID != "io.patchcord.example-text" {
		t.Fatalf("PluginID = %q, want %q", manifest.PluginID, "io.patchcord.example-text")
	}
	if !slices.Contains(manifest.Actions, "text.uppercase@1") {
		t.Fatalf("Actions = %v, want it to contain %q", manifest.Actions, "text.uppercase@1")
	}

	output, err := ExecuteAction(ctx, proc.Client, "text.uppercase@1", map[string]any{
		"value": "Welcome Patchcord",
	})
	if err != nil {
		t.Fatalf("ExecuteAction() error = %v", err)
	}
	if output["value"] != "WELCOME PATCHCORD" {
		t.Fatalf(`output["value"] = %v, want %q`, output["value"], "WELCOME PATCHCORD")
	}
}
