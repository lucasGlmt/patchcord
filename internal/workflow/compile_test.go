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
