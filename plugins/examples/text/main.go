// Command text is the reference example plugin from the vision document
// (section 20): a small library of text actions — "text.uppercase@1",
// "text.lowercase@1", "text.join@1", "text.split@1" — all served by the
// same process. It
// exists to validate the plugin protocol end to end (develop, compile,
// launch, execute, all without recompiling the agent) and to show that one
// plugin can contribute more than one action.
//
// It depends only on the SDK (sdk/go-plugin), never on any internal/
// package of the agent.
package main

import (
	"context"
	"fmt"
	"os"
	"strings"

	patchcord "github.com/lucasglmt/patchcord/sdk/go-plugin"
)

type uppercaseAction struct{}

func (uppercaseAction) ID() string { return "text.uppercase@1" }

func (uppercaseAction) Run(_ context.Context, input patchcord.ActionInput) (patchcord.ActionOutput, error) {
	value, ok := input["value"].(string)
	if !ok {
		return nil, fmt.Errorf("input %q must be a string", "value")
	}
	return patchcord.ActionOutput{"value": strings.ToUpper(value)}, nil
}

type splitAction struct{}

func (splitAction) ID() string { return "text.split@1" }

func (splitAction) Run(_ context.Context, input patchcord.ActionInput) (patchcord.ActionOutput, error) {
	value, ok := input["value"].(string)
	if !ok {
		return nil, fmt.Errorf("input %q must be a string", "value")
	}

	separator, ok := input["separator"].(string)
	if !ok {
		return nil, fmt.Errorf("input %q must be a string", "separator")
	}

	// An action output crosses the plugin protocol as a Protobuf Struct
	// (structpb), which only knows how to encode a list as []any — []string
	// fails to encode at all, so the split parts must be boxed one by one.
	parts := strings.Split(value, separator)
	values := make([]any, len(parts))
	for i, part := range parts {
		values[i] = part
	}

	return patchcord.ActionOutput{"values": values}, nil
}

type lowercaseAction struct{}

func (lowercaseAction) ID() string { return "text.lowercase@1" }

func (lowercaseAction) Run(_ context.Context, input patchcord.ActionInput) (patchcord.ActionOutput, error) {
	value, ok := input["value"].(string)
	if !ok {
		return nil, fmt.Errorf("input %q must be a string", "value")
	}
	return patchcord.ActionOutput{"value": strings.ToLower(value)}, nil
}

type joinAction struct{}

func (joinAction) ID() string { return "text.join@1" }

func (joinAction) Run(_ context.Context, input patchcord.ActionInput) (patchcord.ActionOutput, error) {
	raw, ok := input["values"].([]any)
	if !ok {
		return nil, fmt.Errorf("input %q must be a list of strings", "values")
	}
	separator, _ := input["separator"].(string)

	values := make([]string, len(raw))
	for i, v := range raw {
		s, ok := v.(string)
		if !ok {
			return nil, fmt.Errorf("input %q[%d] must be a string, got %T", "values", i, v)
		}
		values[i] = s
	}

	return patchcord.ActionOutput{"value": strings.Join(values, separator)}, nil
}

func main() {
	plugin := patchcord.Plugin{
		Manifest: patchcord.Manifest{
			ID:      "io.patchcord.example-text",
			Version: "1.0.0",
		},
		Actions: []patchcord.Action{
			uppercaseAction{},
			lowercaseAction{},
			joinAction{},
			splitAction{},
		},
	}

	if err := patchcord.Serve(plugin); err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
}
