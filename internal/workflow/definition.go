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
	SchemaVersion int        `yaml:"schema_version"`
	ID            string     `yaml:"id"`
	Version       int        `yaml:"version"`
	Trigger       Trigger    `yaml:"trigger"`
	Inputs        []InputDef `yaml:"inputs,omitempty"`
	Steps         []Step     `yaml:"steps"`
}

// InputDef declares one input a workflow expects, so a client (the CLI, the
// HTTP API, a dashboard) can validate and collect it before a run starts,
// instead of only discovering a missing or misspelled ${{
// workflow.inputs.<key> }} reference deep inside a step at run time. A
// workflow with no declared Inputs keeps today's behavior: any input key
// may be passed, unvalidated (see PrepareInputs).
type InputDef struct {
	// Name is the key a step references as ${{ workflow.inputs.<Name> }}.
	Name string `yaml:"name"`
	// Type is one of "string" (the default when empty), "number",
	// "boolean" or "enum". See validateInputDefs and PrepareInputs.
	Type string `yaml:"type,omitempty"`
	// Required means a run must supply this input unless Default is set —
	// validateInputDefs rejects declaring both, since Default would
	// silently satisfy Required, making the flag meaningless.
	Required bool `yaml:"required,omitempty"`
	// Description is a human-readable hint for whoever fills this input in
	// (e.g. a generated form field's label or help text).
	Description string `yaml:"description,omitempty"`
	// Default is used when a run does not supply this input. Its Go type
	// (as YAML unmarshals it) must match Type.
	Default any `yaml:"default,omitempty"`
	// Enum lists the values a "enum"-typed input may take. Required for
	// type "enum", rejected for every other type.
	Enum []string `yaml:"enum,omitempty"`
}

// Trigger declares how a workflow starts: "manual" (the default so far) or
// "schedule", fired unattended by internal/scheduler on a cron cadence.
// Webhook and event triggers remain a later phase.
type Trigger struct {
	Type string `yaml:"type"`
	// Cron is a standard 5-field cron expression ("minute hour dom month
	// dow"). Required when Type is "schedule", rejected otherwise. See
	// ADR-0035.
	Cron string `yaml:"cron,omitempty"`
	// OnMissed controls what happens to occurrences the scheduler could not
	// fire because the agent was offline past them: "skip" (the default,
	// used when empty) drops the backlog and resumes at the next future
	// occurrence; "fire_once" runs once for the most recently missed
	// occurrence, then resumes normal cadence. Only meaningful when Type is
	// "schedule". See ADR-0035.
	OnMissed string `yaml:"on_missed,omitempty"`
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
	// If, when set, gates whether this step runs at all: a literal bool,
	// or a ${{ ... }} expression that must resolve to one. A step whose
	// If resolves to false is recorded as skipped — never invoked, and its
	// outputs are unavailable to later steps — without failing the run.
	// Nil (the field omitted) means the step always runs. See ResolveIf
	// and ADR-0031.
	If any `yaml:"if,omitempty"`
	// Foreach, when set, runs this step's action once per item of a
	// literal list or a ${{ ... }} expression that must resolve to one,
	// with the current item available to With as ${{ each }}. Outputs are
	// collected into lists under the action's own output keys — a
	// foreach step's steps.<ID>.outputs.<key> is an array, not a scalar.
	// Nil (the field omitted) means the step runs exactly once, as if it
	// had no Foreach at all. See ResolveForeach and ADR-0032.
	Foreach any `yaml:"foreach,omitempty"`
	// StopIfFalse changes what a false If does: instead of skipping only
	// this step (the default), it skips this step and every step after it,
	// ending the run as succeeded — no error, exactly a guard clause's
	// early return. Ignored when If is nil (nothing to be false) or when
	// ElseOf causes this step to be skipped before If is even evaluated.
	// See ADR-0033.
	StopIfFalse bool `yaml:"stop_if_false,omitempty"`
	// ElseOf, when set, names an earlier step (by ID): this step is
	// skipped whenever that step actually ran — regardless of this step's
	// own If, which (if present) still applies on top, as an additional
	// condition. Chaining ElseOf onto the previous link in a sequence of
	// steps — not onto the first one — builds an if/elseif/else chain
	// without nesting: a step with ElseOf but no If is that chain's
	// unconditional "else". See ADR-0033.
	ElseOf string `yaml:"else_of,omitempty"`
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
