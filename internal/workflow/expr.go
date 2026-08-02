package workflow

import (
	"fmt"
	"regexp"
	"strings"
)

// exprPattern matches a value that is entirely one ${{ ... }} expression —
// the only form of interpolation this engine supports (see ADR for the
// "simple path resolution" scope decision). A value that only partially
// contains ${{ ... }} is left untouched, not partially substituted.
var exprPattern = regexp.MustCompile(`^\$\{\{\s*(.+?)\s*\}\}$`)

// asExpression reports whether value is a string entirely made of one
// ${{ path }} expression, returning its inner path if so.
func asExpression(value any) (path string, ok bool) {
	s, isString := value.(string)
	if !isString {
		return "", false
	}
	match := exprPattern.FindStringSubmatch(s)
	if match == nil {
		return "", false
	}
	return match[1], true
}

// ExprContext supplies the values ${{ ... }} expressions may resolve
// against: the workflow's own inputs, the recorded outputs of the steps
// that already ran, and the connector ids bound to this run's bindings.
type ExprContext struct {
	Inputs      map[string]any
	StepOutputs map[string]map[string]any // by step id
	Bindings    map[string]string         // connector id by logical binding name
}

// validateExpression checks that path has one of the three supported
// shapes — "workflow.inputs.<key>", "steps.<id>.outputs.<key>" or
// "bindings.<name>" — and, for the steps form, that <id> refers to a step
// already seen earlier in the workflow. It does not check that the
// referenced key actually exists: workflow inputs, step outputs and
// bindings are only known at run time.
func validateExpression(path string, seenSteps map[string]struct{}) error {
	segments := strings.Split(path, ".")

	switch {
	case len(segments) == 3 && segments[0] == "workflow" && segments[1] == "inputs":
		return nil

	case len(segments) == 4 && segments[0] == "steps" && segments[2] == "outputs":
		stepID := segments[1]
		if _, ok := seenSteps[stepID]; !ok {
			return fmt.Errorf("expression %q references step %q, which is not defined before this step", path, stepID)
		}
		return nil

	case len(segments) == 2 && segments[0] == "bindings":
		return nil

	default:
		return fmt.Errorf("expression %q has an unsupported shape (expected workflow.inputs.<key>, steps.<id>.outputs.<key> or bindings.<name>)", path)
	}
}

// resolveExpression evaluates path against ctx.
func resolveExpression(path string, ctx ExprContext) (any, error) {
	segments := strings.Split(path, ".")

	switch {
	case len(segments) == 3 && segments[0] == "workflow" && segments[1] == "inputs":
		key := segments[2]
		value, ok := ctx.Inputs[key]
		if !ok {
			return nil, fmt.Errorf("workflow input %q was not provided", key)
		}
		return value, nil

	case len(segments) == 4 && segments[0] == "steps" && segments[2] == "outputs":
		stepID, key := segments[1], segments[3]
		outputs, ok := ctx.StepOutputs[stepID]
		if !ok {
			return nil, fmt.Errorf("step %q has not produced any output yet", stepID)
		}
		value, ok := outputs[key]
		if !ok {
			return nil, fmt.Errorf("step %q output %q was not produced", stepID, key)
		}
		return value, nil

	case len(segments) == 2 && segments[0] == "bindings":
		name := segments[1]
		id, ok := ctx.Bindings[name]
		if !ok {
			return nil, fmt.Errorf("binding %q was not provided", name)
		}
		return id, nil

	default:
		return nil, fmt.Errorf("expression %q has an unsupported shape", path)
	}
}

// ResolveConnector resolves a step's Connector reference against ctx. An
// empty connector means the step uses none. A non-empty connector must be
// entirely one ${{ ... }} expression — Validate rejects anything else — so
// a published, immutable workflow version (ADR-0008) never bakes in one
// deployment's specific connector identity; the indirection is what keeps
// a workflow portable across environments (typically via ${{
// bindings.<name> }}, though ${{ workflow.inputs.<key> }} and ${{
// steps.<id>.outputs.<key> }} are equally legitimate indirections).
func ResolveConnector(connector string, ctx ExprContext) (string, error) {
	if connector == "" {
		return "", nil
	}

	expr, ok := asExpression(connector)
	if !ok {
		return "", fmt.Errorf("connector %q must be a \"${{ ... }}\" expression", connector)
	}

	value, err := resolveExpression(expr, ctx)
	if err != nil {
		return "", err
	}

	id, ok := value.(string)
	if !ok {
		return "", fmt.Errorf("connector expression %q must resolve to a string, got %T", expr, value)
	}

	return id, nil
}

// ResolveInputs evaluates every ${{ ... }} expression in with against ctx,
// returning a new map with expressions replaced by their resolved values.
// Non-expression values are copied through unchanged.
func ResolveInputs(with map[string]any, ctx ExprContext) (map[string]any, error) {
	resolved := make(map[string]any, len(with))
	for key, value := range with {
		expr, ok := asExpression(value)
		if !ok {
			resolved[key] = value
			continue
		}
		v, err := resolveExpression(expr, ctx)
		if err != nil {
			return nil, fmt.Errorf("input %q: %w", key, err)
		}
		resolved[key] = v
	}
	return resolved, nil
}
