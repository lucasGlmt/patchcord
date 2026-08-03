package workflow

import "testing"

func validDefinition() *Definition {
	return &Definition{
		SchemaVersion: 1,
		ID:            "hello_patchcord",
		Version:       1,
		Trigger:       Trigger{Type: "manual"},
		Steps: []Step{
			{ID: "first", Uses: "text.uppercase@1", With: map[string]any{"value": "hello"}},
			{ID: "second", Uses: "text.uppercase@1", With: map[string]any{
				"value": "${{ steps.first.outputs.value }}",
			}},
		},
	}
}

func TestValidate(t *testing.T) {
	knownActions := map[string]struct{}{"text.uppercase@1": {}}

	tests := []struct {
		name    string
		mutate  func(*Definition)
		wantErr bool
	}{
		{name: "accepts a well-formed workflow"},
		{
			name:    "rejects an unsupported schema version",
			mutate:  func(d *Definition) { d.SchemaVersion = 2 },
			wantErr: true,
		},
		{
			name:    "rejects a missing id",
			mutate:  func(d *Definition) { d.ID = "" },
			wantErr: true,
		},
		{
			name:    "rejects a zero version",
			mutate:  func(d *Definition) { d.Version = 0 },
			wantErr: true,
		},
		{
			name:    "rejects a non-manual trigger",
			mutate:  func(d *Definition) { d.Trigger.Type = "cron" },
			wantErr: true,
		},
		{
			name:    "rejects an empty step list",
			mutate:  func(d *Definition) { d.Steps = nil },
			wantErr: true,
		},
		{
			name:    "rejects a missing step id",
			mutate:  func(d *Definition) { d.Steps[0].ID = "" },
			wantErr: true,
		},
		{
			name:    "rejects duplicate step ids",
			mutate:  func(d *Definition) { d.Steps[1].ID = "first" },
			wantErr: true,
		},
		{
			name:    "rejects a missing action",
			mutate:  func(d *Definition) { d.Steps[0].Uses = "" },
			wantErr: true,
		},
		{
			name:    "rejects an action no installed plugin contributes",
			mutate:  func(d *Definition) { d.Steps[0].Uses = "text.reverse@1" },
			wantErr: true,
		},
		{
			name: "rejects an expression referencing a step defined later",
			mutate: func(d *Definition) {
				d.Steps[0].With["value"] = "${{ steps.second.outputs.value }}"
			},
			wantErr: true,
		},
		{
			name: "rejects an expression referencing an undefined step",
			mutate: func(d *Definition) {
				d.Steps[1].With["value"] = "${{ steps.unknown.outputs.value }}"
			},
			wantErr: true,
		},
		{
			name: "rejects a malformed expression shape",
			mutate: func(d *Definition) {
				d.Steps[1].With["value"] = "${{ nonsense }}"
			},
			wantErr: true,
		},
		{
			name: "rejects an expression nested inside a list value referencing a step defined later",
			mutate: func(d *Definition) {
				d.Steps[0].With["value"] = []any{"prefix", "${{ steps.second.outputs.value }}"}
			},
			wantErr: true,
		},
		{
			name: "accepts a well-formed expression nested inside a list value",
			mutate: func(d *Definition) {
				d.Steps[1].With["value"] = []any{"prefix", "${{ steps.first.outputs.value }}"}
			},
		},
		{
			name: "accepts a connector bound via a bindings expression",
			mutate: func(d *Definition) {
				d.Steps[0].Connector = "${{ bindings.ai_provider }}"
			},
		},
		{
			name: "rejects a literal connector id",
			mutate: func(d *Definition) {
				d.Steps[0].Connector = "my_connector"
			},
			wantErr: true,
		},
		{
			name: "rejects a malformed connector expression shape",
			mutate: func(d *Definition) {
				d.Steps[0].Connector = "${{ nonsense }}"
			},
			wantErr: true,
		},
		{
			name: "accepts a literal boolean if",
			mutate: func(d *Definition) {
				d.Steps[0].If = false
			},
		},
		{
			name: "accepts an if bound via a steps expression",
			mutate: func(d *Definition) {
				d.Steps[1].If = "${{ steps.first.outputs.value }}"
			},
		},
		{
			name: "rejects a non-boolean literal if",
			mutate: func(d *Definition) {
				d.Steps[0].If = "yes"
			},
			wantErr: true,
		},
		{
			name: "rejects a malformed if expression shape",
			mutate: func(d *Definition) {
				d.Steps[0].If = "${{ nonsense }}"
			},
			wantErr: true,
		},
		{
			name: "rejects an if expression referencing a step defined later",
			mutate: func(d *Definition) {
				d.Steps[0].If = "${{ steps.second.outputs.value }}"
			},
			wantErr: true,
		},
		{
			name: "accepts a literal foreach list with each referenced in with",
			mutate: func(d *Definition) {
				d.Steps[0].Foreach = []any{"a", "b"}
				d.Steps[0].With["value"] = "${{ each }}"
			},
		},
		{
			name: "accepts a foreach bound via a steps expression",
			mutate: func(d *Definition) {
				d.Steps[1].Foreach = "${{ steps.first.outputs.value }}"
				d.Steps[1].With["value"] = "${{ each }}"
			},
		},
		{
			name: "rejects a non-list, non-expression foreach",
			mutate: func(d *Definition) {
				d.Steps[0].Foreach = "not_a_list_or_expression"
			},
			wantErr: true,
		},
		{
			name: "rejects a malformed foreach expression shape",
			mutate: func(d *Definition) {
				d.Steps[0].Foreach = "${{ nonsense }}"
			},
			wantErr: true,
		},
		{
			name: "rejects each referenced in with when the step has no foreach",
			mutate: func(d *Definition) {
				d.Steps[0].With["value"] = "${{ each }}"
			},
			wantErr: true,
		},
		{
			name: "rejects each referenced in another step's with",
			mutate: func(d *Definition) {
				d.Steps[0].Foreach = []any{"a", "b"}
				d.Steps[1].With["value"] = "${{ each }}"
			},
			wantErr: true,
		},
		{
			name: "rejects each referenced in if",
			mutate: func(d *Definition) {
				d.Steps[0].Foreach = []any{"a", "b"}
				d.Steps[0].If = "${{ each }}"
			},
			wantErr: true,
		},
		{
			name: "rejects each referenced in connector",
			mutate: func(d *Definition) {
				d.Steps[0].Foreach = []any{"a", "b"}
				d.Steps[0].Connector = "${{ each }}"
			},
			wantErr: true,
		},
		{
			name: "accepts an if comparison expression",
			mutate: func(d *Definition) {
				d.Steps[1].If = "${{ steps.first.outputs.value == 'hello' }}"
			},
		},
		{
			name: "rejects an if comparison with a malformed literal",
			mutate: func(d *Definition) {
				d.Steps[1].If = "${{ steps.first.outputs.value >= not_a_literal }}"
			},
			wantErr: true,
		},
		{
			name: "accepts stop_if_false alongside if",
			mutate: func(d *Definition) {
				d.Steps[0].If = false
				d.Steps[0].StopIfFalse = true
			},
		},
		{
			name: "rejects stop_if_false without if",
			mutate: func(d *Definition) {
				d.Steps[0].StopIfFalse = true
			},
			wantErr: true,
		},
		{
			name: "accepts else_of referencing an earlier step",
			mutate: func(d *Definition) {
				d.Steps[1].ElseOf = "first"
			},
		},
		{
			name: "rejects else_of referencing an undefined step",
			mutate: func(d *Definition) {
				d.Steps[0].ElseOf = "unknown"
			},
			wantErr: true,
		},
		{
			name: "rejects else_of referencing a step defined later",
			mutate: func(d *Definition) {
				d.Steps[0].ElseOf = "second"
			},
			wantErr: true,
		},
		{
			name: "accepts a well-formed input schema",
			mutate: func(d *Definition) {
				d.Inputs = []InputDef{
					{Name: "name", Type: "string", Required: true},
					{Name: "shout", Type: "boolean", Default: false},
					{Name: "greeting", Type: "enum", Enum: []string{"hi", "hello"}, Default: "hi"},
				}
			},
		},
		{
			name: "rejects a duplicate input name",
			mutate: func(d *Definition) {
				d.Inputs = []InputDef{{Name: "name"}, {Name: "name"}}
			},
			wantErr: true,
		},
		{
			name: "rejects an input with an unsupported type",
			mutate: func(d *Definition) {
				d.Inputs = []InputDef{{Name: "name", Type: "array"}}
			},
			wantErr: true,
		},
		{
			name: "rejects an input declaring both required and default",
			mutate: func(d *Definition) {
				d.Inputs = []InputDef{{Name: "name", Required: true, Default: "world"}}
			},
			wantErr: true,
		},
		{
			name: "accepts a well-formed schedule trigger",
			mutate: func(d *Definition) {
				d.Trigger = Trigger{Type: "schedule", Cron: "*/5 * * * *"}
			},
		},
		{
			name: "accepts a schedule trigger with an explicit on_missed policy",
			mutate: func(d *Definition) {
				d.Trigger = Trigger{Type: "schedule", Cron: "*/5 * * * *", OnMissed: "fire_once"}
			},
		},
		{
			name: "rejects a schedule trigger with a malformed cron expression",
			mutate: func(d *Definition) {
				d.Trigger = Trigger{Type: "schedule", Cron: "not a cron expression"}
			},
			wantErr: true,
		},
		{
			name: "rejects a schedule trigger with an empty cron expression",
			mutate: func(d *Definition) {
				d.Trigger = Trigger{Type: "schedule"}
			},
			wantErr: true,
		},
		{
			name: "rejects a schedule trigger with an unrecognized on_missed policy",
			mutate: func(d *Definition) {
				d.Trigger = Trigger{Type: "schedule", Cron: "*/5 * * * *", OnMissed: "catch_up_all"}
			},
			wantErr: true,
		},
		{
			name: "rejects a manual trigger declaring a cron expression",
			mutate: func(d *Definition) {
				d.Trigger = Trigger{Type: "manual", Cron: "*/5 * * * *"}
			},
			wantErr: true,
		},
		{
			name: "rejects a manual trigger declaring an on_missed policy",
			mutate: func(d *Definition) {
				d.Trigger = Trigger{Type: "manual", OnMissed: "skip"}
			},
			wantErr: true,
		},
		{
			name: "rejects a schedule trigger with a required input lacking a default",
			mutate: func(d *Definition) {
				d.Trigger = Trigger{Type: "schedule", Cron: "*/5 * * * *"}
				d.Inputs = []InputDef{{Name: "name", Required: true}}
			},
			wantErr: true,
		},
		{
			name: "accepts a schedule trigger with a non-required input that has a default",
			mutate: func(d *Definition) {
				d.Trigger = Trigger{Type: "schedule", Cron: "*/5 * * * *"}
				d.Inputs = []InputDef{{Name: "name", Default: "world"}}
			},
		},
		{
			name: "rejects a schedule trigger with a connector-bound step",
			mutate: func(d *Definition) {
				d.Trigger = Trigger{Type: "schedule", Cron: "*/5 * * * *"}
				d.Steps[0].Connector = "${{ bindings.ai_provider }}"
			},
			wantErr: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			def := validDefinition()
			if tt.mutate != nil {
				tt.mutate(def)
			}

			err := Validate(def, knownActions)

			if tt.wantErr && err == nil {
				t.Fatal("expected an error, got nil")
			}
			if !tt.wantErr && err != nil {
				t.Fatalf("Validate() error = %v", err)
			}
		})
	}
}
