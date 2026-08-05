package plugins

import (
	"context"
	"slices"
	"testing"
	"time"

	"github.com/lucasglmt/patchcord/internal/connectors"
)

// TestExamplePlugin_EndToEnd exercises the whole chain the vision
// document's section 20 vertical slice describes: launch a real plugin
// binary (built on the actual SDK), complete the handshake, and execute
// each of the actions it contributes — all without the agent knowing
// anything about the plugin's implementation. The plugin bundles more than
// one action (a small "text" library) to prove that a single process may
// contribute several actions, not just one.
func TestExamplePlugin_EndToEnd(t *testing.T) {
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
	actionIDs := ActionIDs(manifest.Actions)
	for _, action := range []string{"text.uppercase@1", "text.lowercase@1", "text.join@1", "text.split@1", "text.echo_connector@1"} {
		if !slices.Contains(actionIDs, action) {
			t.Fatalf("Actions = %v, want it to contain %q", actionIDs, action)
		}
	}
	for _, action := range manifest.Actions {
		if action.Description == "" {
			t.Fatalf("action %q has an empty Description", action.ID)
		}
	}

	uppercased, err := ExecuteAction(ctx, proc.Client, "text.uppercase@1", map[string]any{
		"value": "Welcome Patchcord",
	}, nil)
	if err != nil {
		t.Fatalf("ExecuteAction(text.uppercase@1) error = %v", err)
	}
	if uppercased["value"] != "WELCOME PATCHCORD" {
		t.Fatalf(`ExecuteAction(text.uppercase@1)["value"] = %v, want %q`, uppercased["value"], "WELCOME PATCHCORD")
	}

	lowercased, err := ExecuteAction(ctx, proc.Client, "text.lowercase@1", map[string]any{
		"value": "Welcome Patchcord",
	}, nil)
	if err != nil {
		t.Fatalf("ExecuteAction(text.lowercase@1) error = %v", err)
	}
	if lowercased["value"] != "welcome patchcord" {
		t.Fatalf(`ExecuteAction(text.lowercase@1)["value"] = %v, want %q`, lowercased["value"], "welcome patchcord")
	}

	joined, err := ExecuteAction(ctx, proc.Client, "text.join@1", map[string]any{
		"values":    []any{"welcome", "to", "patchcord"},
		"separator": " ",
	}, nil)
	if err != nil {
		t.Fatalf("ExecuteAction(text.join@1) error = %v", err)
	}
	if joined["value"] != "welcome to patchcord" {
		t.Fatalf(`ExecuteAction(text.join@1)["value"] = %v, want %q`, joined["value"], "welcome to patchcord")
	}

	split, err := ExecuteAction(ctx, proc.Client, "text.split@1", map[string]any{
		"value":     "welcome to patchcord",
		"separator": " ",
	}, nil)
	if err != nil {
		t.Fatalf("ExecuteAction(text.split@1) error = %v", err)
	}
	wantSplit := []any{"welcome", "to", "patchcord"}
	if !slices.Equal(split["values"].([]any), wantSplit) {
		t.Fatalf("ExecuteAction(text.split@1)[\"values\"] = %v, want %v", split["values"], wantSplit)
	}

	// The whole point of the protocol's connector field: a resolved
	// connector's Type/Config make a real round trip over gRPC/Protobuf to
	// this real subprocess and back out in the action's output — not just
	// in memory, unlike every ExecuteAction call above (which pass nil).
	echoed, err := ExecuteAction(ctx, proc.Client, "text.echo_connector@1", nil, &connectors.ResolvedConnector{
		Type:    "demo.connection@1",
		Config:  map[string]any{"greeting": "hello"},
		Secrets: map[string]any{"token": "s3cr3t"},
	})
	if err != nil {
		t.Fatalf("ExecuteAction(text.echo_connector@1) error = %v", err)
	}
	if echoed["bound"] != true {
		t.Fatalf(`ExecuteAction(text.echo_connector@1)["bound"] = %v, want true`, echoed["bound"])
	}
	if echoed["type"] != "demo.connection@1" {
		t.Fatalf(`ExecuteAction(text.echo_connector@1)["type"] = %v, want %q`, echoed["type"], "demo.connection@1")
	}
	config, ok := echoed["config"].(map[string]any)
	if !ok || config["greeting"] != "hello" {
		t.Fatalf(`ExecuteAction(text.echo_connector@1)["config"] = %v, want {"greeting":"hello"}`, echoed["config"])
	}
	if _, leaked := echoed["secrets"]; leaked {
		t.Fatal(`ExecuteAction(text.echo_connector@1) output must never include "secrets"`)
	}

	unbound, err := ExecuteAction(ctx, proc.Client, "text.echo_connector@1", nil, nil)
	if err != nil {
		t.Fatalf("ExecuteAction(text.echo_connector@1) (no connector) error = %v", err)
	}
	if unbound["bound"] != false {
		t.Fatalf(`ExecuteAction(text.echo_connector@1) (no connector)["bound"] = %v, want false`, unbound["bound"])
	}
}
