package workflow

import (
	"fmt"
	"regexp"
	"strconv"
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

// comparisonPattern splits a comparison expression's inner content into its
// path, operator and literal parts — e.g. "steps.x.outputs.value >= 2"
// becomes ("steps.x.outputs.value", ">=", "2"). Longer operators are listed
// before their prefixes (">=" before ">") so Go's leftmost-first
// alternation prefers them. A plain path (no comparison) never matches,
// since a dot-path never contains "=", "<" or ">" — comparisonPattern and
// the plain path grammar are mutually exclusive by construction, not by
// precedence.
var comparisonPattern = regexp.MustCompile(`^(.+?)\s*(==|!=|>=|<=|>|<)\s*(.+)$`)

// parseComparison reports whether path is a comparison ("<path> <op>
// <literal>") and, if so, splits it into its three parts.
func parseComparison(path string) (lhs, op, rhs string, ok bool) {
	match := comparisonPattern.FindStringSubmatch(path)
	if match == nil {
		return "", "", "", false
	}
	return match[1], match[2], match[3], true
}

// parseComparisonLiteral parses a comparison's right-hand side: a
// single- or double-quoted string, "true"/"false", or a number. Comparing
// against another expression (path vs. path, rather than path vs. literal)
// is deliberately not supported — kept out of scope to keep the comparison
// grammar a closed, fixed set of operators over one resolved value and one
// literal, not a growing expression language embedded in YAML.
func parseComparisonLiteral(raw string) (any, error) {
	raw = strings.TrimSpace(raw)
	if len(raw) >= 2 {
		if (raw[0] == '\'' && raw[len(raw)-1] == '\'') || (raw[0] == '"' && raw[len(raw)-1] == '"') {
			return raw[1 : len(raw)-1], nil
		}
	}
	switch raw {
	case "true":
		return true, nil
	case "false":
		return false, nil
	}
	if n, err := strconv.ParseFloat(raw, 64); err == nil {
		return n, nil
	}
	return nil, fmt.Errorf("%q is not a valid comparison literal (expected a quoted string, a number, or true/false)", raw)
}

// compareValues applies op to left (a resolved expression value) and right
// (a literal parsed by parseComparisonLiteral). "==" and "!=" compare any
// pair of the three literal-compatible types (string, float64, bool),
// always false across mismatched types. The ordering operators only accept
// numbers — comparing, say, strings lexicographically would be a silent
// surprise (locale/case rules) rather than the arithmetic comparison the
// syntax suggests, so it is a clear error instead.
func compareValues(op string, left, right any) (bool, error) {
	if op == "==" || op == "!=" {
		equal := valuesEqual(left, right)
		if op == "!=" {
			return !equal, nil
		}
		return equal, nil
	}

	leftNum, leftOK := left.(float64)
	rightNum, rightOK := right.(float64)
	if !leftOK || !rightOK {
		return false, fmt.Errorf("comparison operator %q requires numeric operands, got %T and %T", op, left, right)
	}

	switch op {
	case ">":
		return leftNum > rightNum, nil
	case ">=":
		return leftNum >= rightNum, nil
	case "<":
		return leftNum < rightNum, nil
	case "<=":
		return leftNum <= rightNum, nil
	default:
		return false, fmt.Errorf("unsupported comparison operator %q", op)
	}
}

// valuesEqual compares two comparison operands for "==": equal only when
// both are the same Go type (float64, string or bool) produced by
// resolveExpression/parseComparisonLiteral, and equal by value.
func valuesEqual(a, b any) bool {
	switch av := a.(type) {
	case float64:
		bv, ok := b.(float64)
		return ok && av == bv
	case string:
		bv, ok := b.(string)
		return ok && av == bv
	case bool:
		bv, ok := b.(bool)
		return ok && av == bv
	default:
		return false
	}
}

// ExprContext supplies the values ${{ ... }} expressions may resolve
// against: the workflow's own inputs, the recorded outputs of the steps
// that already ran, the connector ids bound to this run's bindings, and —
// only while a foreach step is iterating — the current item.
type ExprContext struct {
	Inputs      map[string]any
	StepOutputs map[string]map[string]any // by step id
	Bindings    map[string]string         // connector id by logical binding name
	// Each is the current item, set only while resolving a foreach step's
	// With for one iteration (see ResolveForeach and internal/runs's
	// runner). HasEach distinguishes "no iteration in progress" from an
	// item whose value happens to be nil — ${{ each }} must fail in the
	// former case, not resolve to nil.
	Each    any
	HasEach bool
}

// validateExpression checks that path has one of the four supported shapes
// — "workflow.inputs.<key>", "steps.<id>.outputs.<key>", "bindings.<name>"
// or "each" — and, for the steps form, that <id> refers to a step already
// seen earlier in the workflow. It does not check that the referenced key
// actually exists: workflow inputs, step outputs and bindings are only
// known at run time. allowEach gates the "each" shape: it is only a valid
// reference inside a foreach step's own With, where an iteration actually
// supplies an item — everywhere else (a step's If, Connector, Foreach
// itself, or another step's With) it is rejected at compile time rather
// than failing every run with "used outside a foreach iteration".
//
// path may also be a comparison ("<path> <op> <literal>", see
// parseComparison) — its left-hand side is validated recursively against
// these same four shapes, and its literal right-hand side is checked for
// well-formedness (parseComparisonLiteral), but not resolved: only
// resolveExpression does that, once the left-hand side's actual value is
// known.
func validateExpression(path string, seenSteps map[string]struct{}, allowEach bool) error {
	if lhs, _, rhs, ok := parseComparison(path); ok {
		if err := validateExpression(lhs, seenSteps, allowEach); err != nil {
			return fmt.Errorf("comparison %q: %w", path, err)
		}
		if _, err := parseComparisonLiteral(rhs); err != nil {
			return fmt.Errorf("comparison %q: %w", path, err)
		}
		return nil
	}

	segments := strings.Split(path, ".")

	switch {
	case len(segments) == 1 && segments[0] == "each":
		if !allowEach {
			return fmt.Errorf("expression %q is only valid inside a foreach step's own with", path)
		}
		return nil

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
		return fmt.Errorf("expression %q has an unsupported shape (expected workflow.inputs.<key>, steps.<id>.outputs.<key>, bindings.<name> or each)", path)
	}
}

// validateValueExpressions recursively validates every ${{ ... }} expression
// found in value, including those nested inside []any or map[string]any
// input values (e.g. text.join@1's "values" array). Non-expression values,
// at any nesting depth, are not validated — they are literals. allowEach is
// forwarded to validateExpression as-is.
func validateValueExpressions(value any, seenSteps map[string]struct{}, allowEach bool) error {
	if expr, ok := asExpression(value); ok {
		return validateExpression(expr, seenSteps, allowEach)
	}
	switch v := value.(type) {
	case []any:
		for _, item := range v {
			if err := validateValueExpressions(item, seenSteps, allowEach); err != nil {
				return err
			}
		}
	case map[string]any:
		for _, item := range v {
			if err := validateValueExpressions(item, seenSteps, allowEach); err != nil {
				return err
			}
		}
	}
	return nil
}

// resolveExpression evaluates path against ctx. A comparison ("<path> <op>
// <literal>") resolves its left-hand side against ctx like any other
// expression, then compares it to the literal right-hand side, yielding a
// bool.
func resolveExpression(path string, ctx ExprContext) (any, error) {
	if lhs, op, rhs, ok := parseComparison(path); ok {
		leftValue, err := resolveExpression(lhs, ctx)
		if err != nil {
			return nil, err
		}
		rightValue, err := parseComparisonLiteral(rhs)
		if err != nil {
			return nil, err
		}
		return compareValues(op, leftValue, rightValue)
	}

	segments := strings.Split(path, ".")

	switch {
	case len(segments) == 1 && segments[0] == "each":
		if !ctx.HasEach {
			return nil, fmt.Errorf("expression %q used outside a foreach iteration", path)
		}
		return ctx.Each, nil

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

// ResolveIf resolves a step's If condition against ctx, defaulting to true
// when the step declares none (ifValue is nil). A non-empty If is either a
// literal bool or a ${{ ... }} expression — Validate rejects anything else
// — and the expression form must itself resolve to a bool; anything else
// is a run-time error rather than a silently always-true or always-false
// step.
func ResolveIf(ifValue any, ctx ExprContext) (bool, error) {
	if ifValue == nil {
		return true, nil
	}

	resolved := ifValue
	if expr, ok := asExpression(ifValue); ok {
		var err error
		resolved, err = resolveExpression(expr, ctx)
		if err != nil {
			return false, err
		}
	}

	b, ok := resolved.(bool)
	if !ok {
		return false, fmt.Errorf("if must resolve to a boolean, got %T", resolved)
	}
	return b, nil
}

// ResolveForeach resolves a step's Foreach declaration against ctx into the
// list of items to iterate over. Foreach nil means the step does not
// iterate at all — callers use this to tell a foreach step from a regular
// one. A non-nil Foreach is either a literal list or a ${{ ... }}
// expression — Validate rejects anything else — and the expression form
// must itself resolve to a list; anything else is a run-time error. ctx
// need not (and should not) have HasEach set: the list being iterated is
// resolved once, before any item is bound.
func ResolveForeach(foreachValue any, ctx ExprContext) ([]any, error) {
	if foreachValue == nil {
		return nil, nil
	}

	resolved := foreachValue
	if expr, ok := asExpression(foreachValue); ok {
		var err error
		resolved, err = resolveExpression(expr, ctx)
		if err != nil {
			return nil, err
		}
	}

	items, ok := resolved.([]any)
	if !ok {
		return nil, fmt.Errorf("foreach must resolve to a list, got %T", resolved)
	}
	return items, nil
}

// resolveValue resolves value against ctx, recursing into []any and
// map[string]any so a ${{ ... }} expression nested inside a list or object
// input — e.g. one of text.join@1's "values" entries — is substituted the
// same way a top-level string input is. Each individual string is still
// either entirely one expression or left untouched; this only changes where
// asExpression is applied, not the "no partial interpolation" rule.
func resolveValue(value any, ctx ExprContext) (any, error) {
	if expr, ok := asExpression(value); ok {
		return resolveExpression(expr, ctx)
	}
	switch v := value.(type) {
	case []any:
		resolved := make([]any, len(v))
		for i, item := range v {
			r, err := resolveValue(item, ctx)
			if err != nil {
				return nil, err
			}
			resolved[i] = r
		}
		return resolved, nil
	case map[string]any:
		resolved := make(map[string]any, len(v))
		for k, item := range v {
			r, err := resolveValue(item, ctx)
			if err != nil {
				return nil, err
			}
			resolved[k] = r
		}
		return resolved, nil
	default:
		return value, nil
	}
}

// ResolveInputs evaluates every ${{ ... }} expression in with against ctx,
// including expressions nested inside list or object values, returning a
// new map with expressions replaced by their resolved values. Non-expression
// values are copied through unchanged.
func ResolveInputs(with map[string]any, ctx ExprContext) (map[string]any, error) {
	resolved := make(map[string]any, len(with))
	for key, value := range with {
		v, err := resolveValue(value, ctx)
		if err != nil {
			return nil, fmt.Errorf("input %q: %w", key, err)
		}
		resolved[key] = v
	}
	return resolved, nil
}
