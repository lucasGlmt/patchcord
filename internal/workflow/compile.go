package workflow

import (
	"fmt"

	"github.com/robfig/cron/v3"

	"github.com/lucasglmt/patchcord/internal/secrets"
)

// SupportedSchemaVersion is the only workflow schema version this engine
// understands.
const SupportedSchemaVersion = 1

// Validate checks def against the rules a workflow must satisfy before it
// can be installed or run (vision document, section 12.5):
//
//   - a supported schema version;
//   - a non-empty id, a positive version, a "manual", "schedule" or
//     "webhook" trigger;
//   - for a "schedule" trigger: a valid 5-field cron expression, a
//     recognized on_missed policy, no required input lacking a default and
//     no connector-bound step — a schedule fires unattended, with nobody to
//     supply inputs or bindings at run time (see ADR-0035);
//   - for a "webhook" trigger: a valid secret reference and no
//     connector-bound step — a webhook has an inbound caller to supply
//     inputs (its request body), but never a bindings map, so a required
//     input without a default is fine but a connector binding still isn't
//     (see ADR-0037);
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
	if err := validateTrigger(def); err != nil {
		return err
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

// validateTrigger checks def.Trigger against the rules described in
// Validate's doc comment. A "schedule" trigger rejects any required input
// lacking a default and any connector-bound step: it fires unattended, so
// there is no caller left to supply either at run time (unlike
// POST /workflows/{id}/run's body.Inputs and body.Bindings) — see
// ADR-0035. A "webhook" trigger only rejects a connector-bound step: its
// inbound HTTP request is a real caller and supplies inputs (its body), but
// never a bindings map — see ADR-0037.
func validateTrigger(def *Definition) error {
	hasSecretRef := def.Trigger.SecretRef.Type != "" || def.Trigger.SecretRef.Key != ""

	switch def.Trigger.Type {
	case "manual":
		if def.Trigger.Cron != "" || def.Trigger.OnMissed != "" {
			return fmt.Errorf("trigger: cron and on_missed only apply to a \"schedule\" trigger, not \"manual\"")
		}
		if hasSecretRef {
			return fmt.Errorf("trigger: secret_ref only applies to a \"webhook\" trigger, not \"manual\"")
		}
		return nil

	case "schedule":
		if _, err := cron.ParseStandard(def.Trigger.Cron); err != nil {
			return fmt.Errorf("trigger: invalid cron expression %q: %w", def.Trigger.Cron, err)
		}

		switch def.Trigger.OnMissed {
		case "", "skip", "fire_once":
		default:
			return fmt.Errorf("trigger: on_missed must be \"skip\" or \"fire_once\", got %q", def.Trigger.OnMissed)
		}

		if hasSecretRef {
			return fmt.Errorf("trigger: secret_ref only applies to a \"webhook\" trigger, not \"schedule\"")
		}

		for _, input := range def.Inputs {
			if input.Required && input.Default == nil {
				return fmt.Errorf("trigger: schedule requires input %q to declare a default — a scheduled run has no caller to supply it", input.Name)
			}
		}
		return validateNoConnectorBoundStep(def, "schedule")

	case "webhook":
		if def.Trigger.Cron != "" || def.Trigger.OnMissed != "" {
			return fmt.Errorf("trigger: cron and on_missed only apply to a \"schedule\" trigger, not \"webhook\"")
		}
		if def.Trigger.SecretRef.Key == "" {
			return fmt.Errorf("trigger: webhook requires secret_ref (type and key) to verify inbound requests")
		}
		if err := secrets.ValidateType(def.Trigger.SecretRef.Type); err != nil {
			return fmt.Errorf("trigger: secret_ref: %w", err)
		}
		return validateNoConnectorBoundStep(def, "webhook")

	default:
		return fmt.Errorf("unsupported trigger type %q, only \"manual\", \"schedule\" and \"webhook\" are supported", def.Trigger.Type)
	}
}

// validateNoConnectorBoundStep rejects any step with a non-empty Connector
// — shared by the "schedule" and "webhook" trigger cases, neither of which
// has a bindings map for a connector expression to resolve against.
func validateNoConnectorBoundStep(def *Definition, triggerType string) error {
	for _, step := range def.Steps {
		if step.Connector != "" {
			return fmt.Errorf("trigger: %s does not support step %q's connector binding — there is no caller to supply a bindings map", triggerType, step.ID)
		}
	}
	return nil
}
