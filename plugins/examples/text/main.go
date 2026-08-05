// Command text is the reference example plugin from the vision document
// (section 20): a small library of text actions — "text.uppercase@1",
// "text.lowercase@1", "text.join@1", "text.split@1", "text.echo_connector@1"
// — all served by the same process. It exists to validate the plugin
// protocol end to end (develop, compile, launch, execute, all without
// recompiling the agent), to show that one plugin can contribute more than
// one action, and — via text.echo_connector@1 — that a connector bound to a
// workflow step reaches the plugin process intact (ADR-0021). It is also
// the reference for how an action declares its schema (ADR-0062).
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

// stringProp/schema are tiny local helpers to keep the JSON Schema literals
// below readable — every example plugin in this directory repeats this
// pattern rather than sharing it, since a plugin depends only on the SDK,
// never on another plugin or an internal/ package (CLAUDE.md §2).
func schema(properties map[string]any, required ...string) patchcord.Schema {
	s := patchcord.Schema{
		"type":       "object",
		"properties": properties,
	}
	if len(required) > 0 {
		req := make([]any, len(required))
		for i, r := range required {
			req[i] = r
		}
		s["required"] = req
	}
	return s
}

func stringProp(description string) map[string]any {
	return map[string]any{"type": "string", "description": description}
}

type uppercaseAction struct{}

func (uppercaseAction) ID() string          { return "text.uppercase@1" }
func (uppercaseAction) Description() string { return "Converts a string to upper case." }

func (uppercaseAction) InputSchema() patchcord.Schema {
	return schema(map[string]any{"value": stringProp("The string to convert.")}, "value")
}

func (uppercaseAction) OutputSchema() patchcord.Schema {
	return schema(map[string]any{"value": stringProp("value, converted to upper case.")}, "value")
}

func (uppercaseAction) Run(_ context.Context, input patchcord.ActionInput, _ *patchcord.ConnectorConfig) (patchcord.ActionOutput, error) {
	value, ok := input["value"].(string)
	if !ok {
		return nil, fmt.Errorf("input %q must be a string", "value")
	}
	return patchcord.ActionOutput{"value": strings.ToUpper(value)}, nil
}

type splitAction struct{}

func (splitAction) ID() string          { return "text.split@1" }
func (splitAction) Description() string { return "Splits a string into a list on a separator." }

func (splitAction) InputSchema() patchcord.Schema {
	return schema(map[string]any{
		"value":     stringProp("The string to split."),
		"separator": stringProp("The separator to split on."),
	}, "value", "separator")
}

func (splitAction) OutputSchema() patchcord.Schema {
	return schema(map[string]any{
		"values": map[string]any{
			"type":        "array",
			"items":       map[string]any{"type": "string"},
			"description": "The parts of value, in order.",
		},
	}, "values")
}

func (splitAction) Run(_ context.Context, input patchcord.ActionInput, _ *patchcord.ConnectorConfig) (patchcord.ActionOutput, error) {
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

type replaceAction struct{}

func (replaceAction) ID() string          { return "text.replace@1" }
func (replaceAction) Description() string { return "Replaces every occurrence of a substring." }

func (replaceAction) InputSchema() patchcord.Schema {
	return schema(map[string]any{
		"value": stringProp("The string to search in."),
		"old":   stringProp("The substring to replace."),
		"new":   stringProp("The replacement string."),
	}, "value", "old", "new")
}

func (replaceAction) OutputSchema() patchcord.Schema {
	return schema(map[string]any{"value": stringProp("value, with every occurrence of old replaced by new.")}, "value")
}

func (replaceAction) Run(_ context.Context, input patchcord.ActionInput, _ *patchcord.ConnectorConfig) (patchcord.ActionOutput, error) {
	value, ok := input["value"].(string)
	if !ok {
		return nil, fmt.Errorf("input %q must be a string", "value")
	}

	old, ok := input["old"].(string)
	if !ok {
		return nil, fmt.Errorf("input %q must be a string", "old")
	}

	new, ok := input["new"].(string)
	if !ok {
		return nil, fmt.Errorf("input %q must be a string", "new")
	}

	result := strings.ReplaceAll(value, old, new)

	return patchcord.ActionOutput{"value": result}, nil
}

type lowercaseAction struct{}

func (lowercaseAction) ID() string          { return "text.lowercase@1" }
func (lowercaseAction) Description() string { return "Converts a string to lower case." }

func (lowercaseAction) InputSchema() patchcord.Schema {
	return schema(map[string]any{"value": stringProp("The string to convert.")}, "value")
}

func (lowercaseAction) OutputSchema() patchcord.Schema {
	return schema(map[string]any{"value": stringProp("value, converted to lower case.")}, "value")
}

func (lowercaseAction) Run(_ context.Context, input patchcord.ActionInput, _ *patchcord.ConnectorConfig) (patchcord.ActionOutput, error) {
	value, ok := input["value"].(string)
	if !ok {
		return nil, fmt.Errorf("input %q must be a string", "value")
	}
	return patchcord.ActionOutput{"value": strings.ToLower(value)}, nil
}

type joinAction struct{}

func (joinAction) ID() string          { return "text.join@1" }
func (joinAction) Description() string { return "Joins a list of strings with a separator." }

func (joinAction) InputSchema() patchcord.Schema {
	return schema(map[string]any{
		"values": map[string]any{
			"type":        "array",
			"items":       map[string]any{"type": "string"},
			"description": "The strings to join.",
		},
		"separator": stringProp("The separator to insert between values. Defaults to empty."),
	}, "values")
}

func (joinAction) OutputSchema() patchcord.Schema {
	return schema(map[string]any{"value": stringProp("values, joined by separator.")}, "value")
}

func (joinAction) Run(_ context.Context, input patchcord.ActionInput, _ *patchcord.ConnectorConfig) (patchcord.ActionOutput, error) {
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

type echoConnectorAction struct{}

func (echoConnectorAction) ID() string { return "text.echo_connector@1" }
func (echoConnectorAction) Description() string {
	return "Reports whether a connector was bound to this call, and its non-secret config if so."
}

func (echoConnectorAction) InputSchema() patchcord.Schema {
	return schema(map[string]any{})
}

func (echoConnectorAction) OutputSchema() patchcord.Schema {
	return schema(map[string]any{
		"bound":  map[string]any{"type": "boolean", "description": "Whether a connector was bound to this call."},
		"type":   stringProp("The bound connector's type, if bound."),
		"config": map[string]any{"type": "object", "description": "The bound connector's non-secret config, if bound."},
	}, "bound")
}

// Run reports whether a connector was bound to this call and, if so, its
// type and non-secret config — proof that the protocol's connector field
// reaches a real plugin process. It deliberately never includes
// connector.Secrets in its output: an action's output is recorded in run
// history in the clear, so echoing a resolved secret back would defeat the
// whole point of never persisting one (ADR-0009, ADR-0020, ADR-0021). Any
// real connector-consuming action must follow the same rule.
func (echoConnectorAction) Run(_ context.Context, _ patchcord.ActionInput, connector *patchcord.ConnectorConfig) (patchcord.ActionOutput, error) {
	if connector == nil {
		return patchcord.ActionOutput{"bound": false}, nil
	}
	return patchcord.ActionOutput{
		"bound":  true,
		"type":   connector.Type,
		"config": connector.Config,
	}, nil
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
			echoConnectorAction{},
			replaceAction{},
		},
		Connectors: []patchcord.Connector{
			{
				// "demo.connection@1" only exists to give
				// connector_binding_demo.yaml a real, installed plugin to
				// validate against (ADR-0022) — it isn't tied to any actual
				// external system.
				Type:         "demo.connection@1",
				Description:  "A connector with no real backing system, for exercising connector binding in demo workflows.",
				ConfigSchema: schema(map[string]any{"label": stringProp("Any string; never interpreted.")}),
			},
		},
	}

	if err := patchcord.Serve(plugin); err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
}
