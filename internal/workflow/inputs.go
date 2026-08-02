package workflow

import (
	"errors"
	"fmt"
	"slices"
	"strconv"
)

// ErrInvalidInputs is wrapped by every error PrepareInputs and
// validateInputDefs return, so a caller (internal/api's handleRunWorkflow)
// can tell "the run's inputs don't satisfy the workflow's declared schema"
// apart from an internal failure and respond 400 instead of 500.
var ErrInvalidInputs = errors.New("invalid workflow inputs")

// inputTypes is the set of types an InputDef.Type may declare. Empty means
// "string".
var inputTypes = map[string]struct{}{
	"string":  {},
	"number":  {},
	"boolean": {},
	"enum":    {},
}

// validateInputDefs checks defs against the rules a workflow's declared
// inputs must satisfy (called from Validate): unique, non-empty names; a
// known type; "enum" requires a non-empty, unique Enum list and every
// other type rejects one; Required and Default together is rejected (a
// default would silently satisfy Required); Default, if set, must match
// the declared Type.
func validateInputDefs(defs []InputDef) error {
	seen := make(map[string]struct{}, len(defs))
	for _, def := range defs {
		if def.Name == "" {
			return fmt.Errorf("input: name is required")
		}
		if _, exists := seen[def.Name]; exists {
			return fmt.Errorf("input %q: duplicate name", def.Name)
		}
		seen[def.Name] = struct{}{}

		typ := inputType(def)
		if _, ok := inputTypes[typ]; !ok {
			return fmt.Errorf("input %q: unsupported type %q (expected string, number, boolean or enum)", def.Name, def.Type)
		}

		if typ == "enum" {
			if len(def.Enum) == 0 {
				return fmt.Errorf("input %q: type \"enum\" requires a non-empty enum list", def.Name)
			}
			seenValues := make(map[string]struct{}, len(def.Enum))
			for _, v := range def.Enum {
				if _, exists := seenValues[v]; exists {
					return fmt.Errorf("input %q: duplicate enum value %q", def.Name, v)
				}
				seenValues[v] = struct{}{}
			}
		} else if len(def.Enum) > 0 {
			return fmt.Errorf("input %q: enum is only valid for type \"enum\", got %q", def.Name, typ)
		}

		if def.Required && def.Default != nil {
			return fmt.Errorf("input %q: required and default are mutually exclusive", def.Name)
		}

		if def.Default != nil {
			if _, err := coerceInputValue(def, def.Default); err != nil {
				return fmt.Errorf("input %q: default: %w", def.Name, err)
			}
		}
	}

	return nil
}

// inputType returns def's effective type, defaulting an empty Type to
// "string".
func inputType(def InputDef) string {
	if def.Type == "" {
		return "string"
	}
	return def.Type
}

// coerceInputValue checks value against def's declared type, converting a
// string into a number or boolean where needed — the CLI's --input flag
// (internal/cli/workflow.go's StringToStringVar) only ever supplies
// strings, while an HTTP JSON body already carries typed values, so both
// paths go through the same coercion here.
func coerceInputValue(def InputDef, value any) (any, error) {
	switch inputType(def) {
	case "string":
		s, ok := value.(string)
		if !ok {
			return nil, fmt.Errorf("must be a string, got %T", value)
		}
		return s, nil

	case "number":
		switch v := value.(type) {
		case float64:
			return v, nil
		case int:
			return float64(v), nil
		case string:
			n, err := strconv.ParseFloat(v, 64)
			if err != nil {
				return nil, fmt.Errorf("must be a number, got %q", v)
			}
			return n, nil
		default:
			return nil, fmt.Errorf("must be a number, got %T", value)
		}

	case "boolean":
		switch v := value.(type) {
		case bool:
			return v, nil
		case string:
			b, err := strconv.ParseBool(v)
			if err != nil {
				return nil, fmt.Errorf("must be a boolean, got %q", v)
			}
			return b, nil
		default:
			return nil, fmt.Errorf("must be a boolean, got %T", value)
		}

	case "enum":
		s, ok := value.(string)
		if !ok {
			return nil, fmt.Errorf("must be a string, got %T", value)
		}
		if !slices.Contains(def.Enum, s) {
			return nil, fmt.Errorf("must be one of %v, got %q", def.Enum, s)
		}
		return s, nil

	default:
		return nil, fmt.Errorf("unsupported type %q", def.Type)
	}
}

// PrepareInputs resolves provided against defs — the workflow's declared
// input schema — filling in defaults, rejecting a missing required input
// or an undeclared key, and coercing every value to its declared type. It
// returns a new map; provided is never mutated.
//
// When defs is empty (the workflow declares no input schema), provided is
// returned unchanged: every workflow installed before this schema existed,
// and every workflow that doesn't need one, keeps working exactly as
// before.
func PrepareInputs(defs []InputDef, provided map[string]any) (map[string]any, error) {
	if len(defs) == 0 {
		return provided, nil
	}

	declared := make(map[string]struct{}, len(defs))
	for _, def := range defs {
		declared[def.Name] = struct{}{}
	}
	for key := range provided {
		if _, ok := declared[key]; !ok {
			return nil, fmt.Errorf("%w: %q is not a declared input", ErrInvalidInputs, key)
		}
	}

	prepared := make(map[string]any, len(defs))
	for _, def := range defs {
		value, ok := provided[def.Name]
		if !ok {
			if def.Default != nil {
				prepared[def.Name] = def.Default
				continue
			}
			if def.Required {
				return nil, fmt.Errorf("%w: required input %q was not provided", ErrInvalidInputs, def.Name)
			}
			continue
		}

		coerced, err := coerceInputValue(def, value)
		if err != nil {
			return nil, fmt.Errorf("%w: input %q: %s", ErrInvalidInputs, def.Name, err)
		}
		prepared[def.Name] = coerced
	}

	return prepared, nil
}
