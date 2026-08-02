// Package workflow implements the workflow engine: parsing and validating
// declarative workflow definitions, resolving their expressions, and the
// explicit state machines Run and Step transitions follow. It has no
// knowledge of persistence or of how actions actually execute — that is
// internal/runs's job, orchestrating on top of this package.
package workflow

import "gopkg.in/yaml.v3"

// Definition is a parsed, declarative workflow, as described in the vision
// document (section 7.5). It is serialized as YAML.
type Definition struct {
	SchemaVersion int     `yaml:"schema_version"`
	ID            string  `yaml:"id"`
	Version       int     `yaml:"version"`
	Trigger       Trigger `yaml:"trigger"`
	Steps         []Step  `yaml:"steps"`
}

// Trigger declares how a workflow starts. Only the "manual" type is
// supported so far; scheduled and webhook triggers belong to the
// scheduler (a later phase).
type Trigger struct {
	Type string `yaml:"type"`
}

// Step is one action invocation within a workflow.
type Step struct {
	// ID identifies this step within its workflow; other steps reference
	// its outputs as steps.<ID>.outputs.<key>.
	ID string `yaml:"id"`
	// Uses is the action identifier this step invokes, e.g. "text.uppercase@1".
	Uses string `yaml:"uses"`
	// With holds the action's input values. A string value that is
	// entirely one ${{ ... }} expression is resolved at run time against
	// the workflow's inputs or a prior step's outputs; every other value
	// is passed through unchanged.
	With map[string]any `yaml:"with"`
	// Connector, if non-empty, binds a connector to this step's action
	// call. It must be entirely one ${{ ... }} expression — never a
	// literal connector id — so a published, immutable workflow version
	// (ADR-0008) never bakes in one deployment's specific connector
	// identity; see ResolveConnector and ADR-0021.
	Connector string `yaml:"connector,omitempty"`
}

// Parse parses a workflow definition from its YAML source. It only parses:
// call Validate to check it against the rules described in the vision
// document (section 12.5) before treating it as runnable.
func Parse(source []byte) (*Definition, error) {
	var def Definition
	if err := yaml.Unmarshal(source, &def); err != nil {
		return nil, &ParseError{Err: err}
	}
	return &def, nil
}

// ParseError wraps a YAML syntax error encountered while parsing a
// workflow definition.
type ParseError struct {
	Err error
}

func (e *ParseError) Error() string { return "parse workflow definition: " + e.Err.Error() }
func (e *ParseError) Unwrap() error { return e.Err }
