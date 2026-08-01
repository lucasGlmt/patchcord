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
//     before a run ever starts.
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

		for key, value := range step.With {
			expr, ok := asExpression(value)
			if !ok {
				continue
			}
			if err := validateExpression(expr, seenSteps); err != nil {
				return fmt.Errorf("step %q: input %q: %w", step.ID, key, err)
			}
		}

		seenSteps[step.ID] = struct{}{}
	}

	return nil
}
