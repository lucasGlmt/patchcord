package workflow

import (
	"encoding/json"
	"fmt"
	"maps"

	"github.com/santhosh-tekuri/jsonschema/v5"
)

// KnownAction is what Validate needs to know about one action currently
// installed plugins contribute: that it exists, and the JSON Schema its
// With must satisfy (plugins.ActionDescriptor.InputSchema, ADR-0062). The
// caller (internal/plugins.KnownActions, via internal/runs/internal/cli)
// builds this from the plugin catalog; internal/workflow stays free of any
// dependency on internal/plugins, the same separation ADR-0017 and
// ADR-0022 already established for a plain knownActions/knownTypes set.
type KnownAction struct {
	InputSchema map[string]any
}

// validateInputSchema checks step's With against action's declared
// input_schema (ADR-0062), closed by ADR-0063. It runs two independent
// passes rather than one whole-document validation, because a With value
// may be a "${{ ... }}" expression whose real type is only known at run
// time (internal/runs resolves it, not this package):
//
//   - every name in inputSchema's "required" must be a key of with,
//     whether its value is a literal or contains an expression anywhere in
//     its subtree — presence is knowable at validate time even when
//     content isn't;
//   - for every key of with that also appears in inputSchema's
//     "properties", if that value's subtree contains no expression
//     anywhere (checked recursively, mirroring validateValueExpressions'
//     traversal), the whole value is type-checked against that property's
//     schema. A value whose subtree contains an expression anywhere is left
//     entirely unchecked — a documented v1 simplification (ADR-0063), not
//     a partial check.
//
// A missing inputSchema, or one with no "required"/"properties", or a
// property with no recognized "type" (including ADR-0062's "any" shape,
// e.g. json.parse@1's decoded value), constrains nothing — never an error
// by itself. Policing a plugin's own schema authoring isn't this
// function's job.
func validateInputSchema(with map[string]any, inputSchema map[string]any) error {
	for _, name := range stringList(inputSchema["required"]) {
		if _, ok := with[name]; !ok {
			return fmt.Errorf("input %q is required", name)
		}
	}

	properties, _ := inputSchema["properties"].(map[string]any)
	if len(properties) == 0 {
		return nil
	}

	checkable := make(map[string]any)
	for key, value := range with {
		if _, declared := properties[key]; !declared {
			continue
		}
		if containsExpression(value) {
			continue
		}
		checkable[key] = normalizeNumbers(value)
	}
	if len(checkable) == 0 {
		return nil
	}

	schema, err := compileInputSchema(inputSchema)
	if err != nil {
		return fmt.Errorf("compile input schema: %w", err)
	}
	if err := schema.Validate(checkable); err != nil {
		return fmt.Errorf("%w", err)
	}
	return nil
}

// containsExpression reports whether value or anything nested inside it
// (through []any or map[string]any) is a "${{ ... }}" expression, per
// asExpression. Mirrors validateValueExpressions' traversal shape (expr.go).
func containsExpression(value any) bool {
	if _, ok := asExpression(value); ok {
		return true
	}
	switch v := value.(type) {
	case []any:
		for _, item := range v {
			if containsExpression(item) {
				return true
			}
		}
	case map[string]any:
		for _, item := range v {
			if containsExpression(item) {
				return true
			}
		}
	}
	return false
}

// normalizeNumbers returns a deep copy of value with every Go int/int64
// replaced by float64. santhosh-tekuri/jsonschema expects the raw values
// encoding/json would produce (float64 for every number), but every With
// value in this engine is decoded by yaml.v3, which decodes a whole-number
// scalar as native int — the same duality internal/workflow/inputs.go's
// coerceInputValue already special-cases for workflow-level inputs.
func normalizeNumbers(value any) any {
	switch v := value.(type) {
	case int:
		return float64(v)
	case int64:
		return float64(v)
	case []any:
		out := make([]any, len(v))
		for i, item := range v {
			out[i] = normalizeNumbers(item)
		}
		return out
	case map[string]any:
		out := make(map[string]any, len(v))
		for k, item := range v {
			out[k] = normalizeNumbers(item)
		}
		return out
	default:
		return v
	}
}

// compileInputSchema compiles inputSchema, after stripping its top-level
// "required" — required-field presence is checked separately by
// validateInputSchema itself, tolerant of expression-valued fields in a way
// the library has no concept of. Validating the checked-only subset of
// with against inputSchema's own "required" would otherwise reject a
// required field that is genuinely present but expression-valued, since
// that field is deliberately absent from the subset being validated.
func compileInputSchema(inputSchema map[string]any) (*jsonschema.Schema, error) {
	stripped := maps.Clone(inputSchema)
	delete(stripped, "required")

	encoded, err := json.Marshal(stripped)
	if err != nil {
		return nil, fmt.Errorf("encode: %w", err)
	}
	return jsonschema.CompileString("input_schema.json", string(encoded))
}

// stringList converts a JSON Schema "required"-shaped value ([]any of
// strings, as decoded from a plugin's declared schema) into a []string,
// tolerating a missing or malformed value as "no required names" — the
// same defensive stance validateInputSchema takes toward the rest of a
// declared schema.
func stringList(value any) []string {
	raw, ok := value.([]any)
	if !ok {
		return nil
	}
	names := make([]string, 0, len(raw))
	for _, item := range raw {
		if s, ok := item.(string); ok {
			names = append(names, s)
		}
	}
	return names
}
