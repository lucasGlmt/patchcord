package workflow

import (
	"fmt"
)

// SupportedSchemaVersion is the only workflow schema version this engine
// understands.
const SupportedSchemaVersion = 1

// Validate checks def against the rules a workflow must satisfy before it
// can be installed or run (vision document, section 12.5):
//
//   - a supported schema version;
//   - a non-empty id, a positive version, exactly one "manual" trigger;
//   - at least one step, with unique, non-empty step ids;
//   - every step's action exists among knownActions;
//   - every ${{ steps.<id>.outputs...}} expression refers to an earlier
//     step in the same workflow, catching typos and forward references
//     before a run ever starts;
//   - a step's if, when set, is a literal bool or a ${{ ... }} expression
//     of a supported shape — never any other literal type;
//   - a step's foreach, when set, is a literal list or a ${{ ... }}
//     expression of a supported shape;
//   - "${{ each }}" only appears inside a foreach step's own with — every
//     other spot (if, connector, foreach itself, another step's with) is
//     rejected, since no iteration is in progress there;
//   - a comparison expression's ("<path> <op> <literal>") left-hand side is
//     a supported shape and its literal right-hand side is well-formed;
//   - stop_if_false requires if to be set;
//   - else_of, when set, names a step defined earlier in the same workflow.
//
// knownActions is the set of action identifiers currently installed
// plugins contribute; the caller (internal/runs) is responsible for
// fetching it from the plugin catalog, keeping this package free of any
// persistence or process dependency.
func Validate(def *Definition, knownActions map[string]struct{}) error {
	if def.SchemaVersion != SupportedSchemaVersion {
		return fmt.Errorf("unsupported schema_version %d, expected %d", def.SchemaVersion, SupportedSchemaVersion)
	}
	if def.ID == "" {
		return fmt.Errorf("workflow id is required")
	}
	if def.Version < 1 {
		return fmt.Errorf("workflow version must be a positive integer, got %d", def.Version)
	}
	if def.Trigger.Type != "manual" {
		return fmt.Errorf("unsupported trigger type %q, only \"manual\" is supported", def.Trigger.Type)
	}
	if len(def.Steps) == 0 {
		return fmt.Errorf("workflow must declare at least one step")
	}
	if err := validateInputDefs(def.Inputs); err != nil {
		return err
	}

	seenSteps := make(map[string]struct{}, len(def.Steps))
	for i, step := range def.Steps {
		if step.ID == "" {
			return fmt.Errorf("step %d: id is required", i)
		}
		if _, exists := seenSteps[step.ID]; exists {
			return fmt.Errorf("step %q: duplicate step id", step.ID)
		}
		if step.Uses == "" {
			return fmt.Errorf("step %q: uses is required", step.ID)
		}
		if _, ok := knownActions[step.Uses]; !ok {
			return fmt.Errorf("step %q: unknown action %q (no installed plugin contributes it)", step.ID, step.Uses)
		}

		if step.Foreach != nil {
			if expr, ok := asExpression(step.Foreach); ok {
				if err := validateExpression(expr, seenSteps, false); err != nil {
					return fmt.Errorf("step %q: foreach: %w", step.ID, err)
				}
			} else if _, ok := step.Foreach.([]any); !ok {
				return fmt.Errorf("step %q: foreach must be a list or a \"${{ ... }}\" expression, got %T", step.ID, step.Foreach)
			}
		}

		for key, value := range step.With {
			if err := validateValueExpressions(value, seenSteps, step.Foreach != nil); err != nil {
				return fmt.Errorf("step %q: input %q: %w", step.ID, key, err)
			}
		}

		if step.Connector != "" {
			expr, ok := asExpression(step.Connector)
			if !ok {
				return fmt.Errorf("step %q: connector must be a \"${{ ... }}\" expression, not a literal value", step.ID)
			}
			if err := validateExpression(expr, seenSteps, false); err != nil {
				return fmt.Errorf("step %q: connector: %w", step.ID, err)
			}
		}

		if step.If != nil {
			if expr, ok := asExpression(step.If); ok {
				if err := validateExpression(expr, seenSteps, false); err != nil {
					return fmt.Errorf("step %q: if: %w", step.ID, err)
				}
			} else if _, ok := step.If.(bool); !ok {
				return fmt.Errorf("step %q: if must be a boolean or a \"${{ ... }}\" expression, got %T", step.ID, step.If)
			}
		}

		if step.StopIfFalse && step.If == nil {
			return fmt.Errorf("step %q: stop_if_false requires if to be set — there is nothing for it to be false", step.ID)
		}

		if step.ElseOf != "" {
			if _, ok := seenSteps[step.ElseOf]; !ok {
				return fmt.Errorf("step %q: else_of references step %q, which is not defined before this step", step.ID, step.ElseOf)
			}
		}

		seenSteps[step.ID] = struct{}{}
	}

	return nil
}
