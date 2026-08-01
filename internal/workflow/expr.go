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
// against: the workflow's own inputs, and the recorded outputs of the
// steps that already ran.
type ExprContext struct {
	Inputs      map[string]any
	StepOutputs map[string]map[string]any // by step id
}

// validateExpression checks that path has one of the two supported shapes
// — "workflow.inputs.<key>" or "steps.<id>.outputs.<key>" — and, for the
// latter, that <id> refers to a step already seen earlier in the workflow.
// It does not check that the referenced key actually exists: workflow
// inputs and step outputs are only known at run time.
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

	default:
		return fmt.Errorf("expression %q has an unsupported shape (expected workflow.inputs.<key> or steps.<id>.outputs.<key>)", path)
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

	default:
		return nil, fmt.Errorf("expression %q has an unsupported shape", path)
	}
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
