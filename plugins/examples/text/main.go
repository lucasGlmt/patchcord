// Command text-uppercase is the reference example plugin from the vision
// document (section 20): a single action, "text.uppercase@1", that
// uppercases its "value" input. It exists to validate the plugin protocol
// end to end — develop, compile, launch, execute, all without recompiling
// the agent.
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

func main() {
	plugin := patchcord.Plugin{
		Manifest: patchcord.Manifest{
			ID:      "io.patchcord.example-text",
			Version: "1.0.0",
		},
		Actions: []patchcord.Action{uppercaseAction{}},
	}

	if err := patchcord.Serve(plugin); err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
}
