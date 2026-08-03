package workflow

import (
	"reflect"
	"testing"
)

func TestResolveInputs(t *testing.T) {
	ctx := ExprContext{
		Inputs: map[string]any{"value": "hello"},
		StepOutputs: map[string]map[string]any{
			"first": {"value": "HELLO"},
		},
	}

	tests := []struct {
		name    string
		with    map[string]any
		key     string
		want    any
		wantErr bool
	}{
		{
			name: "resolves a workflow input expression",
			with: map[string]any{"value": "${{ workflow.inputs.value }}"},
			key:  "value",
			want: "hello",
		},
		{
			name: "resolves a step output expression",
			with: map[string]any{"value": "${{ steps.first.outputs.value }}"},
			key:  "value",
			want: "HELLO",
		},
		{
			name: "passes through a literal value unchanged",
			with: map[string]any{"value": "literal"},
			key:  "value",
			want: "literal",
		},
		{
			name: "passes through a non-string value unchanged",
			with: map[string]any{"value": 42},
			key:  "value",
			want: 42,
		},
		{
			name: "resolves a step output expression nested inside a list value",
			with: map[string]any{"values": []any{"Salut, ", "${{ steps.first.outputs.value }}"}},
			key:  "values",
			want: []any{"Salut, ", "HELLO"},
		},
		{
			name: "resolves a step output expression nested inside a map value",
			with: map[string]any{"headers": map[string]any{"Authorization": "${{ steps.first.outputs.value }}"}},
			key:  "headers",
			want: map[string]any{"Authorization": "HELLO"},
		},
		{
			name: "leaves a value with embedded (non-whole) syntax unresolved",
			with: map[string]any{"value": "prefix-${{ workflow.inputs.value }}"},
			key:  "value",
			want: "prefix-${{ workflow.inputs.value }}",
		},
		{
			name:    "fails when the referenced workflow input is missing",
			with:    map[string]any{"value": "${{ workflow.inputs.missing }}"},
			wantErr: true,
		},
		{
			name:    "fails when the referenced step has no output yet",
			with:    map[string]any{"value": "${{ steps.unknown.outputs.value }}"},
			wantErr: true,
		},
		{
			name:    "fails when the referenced output key is missing",
			with:    map[string]any{"value": "${{ steps.first.outputs.missing }}"},
			wantErr: true,
		},
		{
			name:    "fails when an expression nested inside a list value is unresolvable",
			with:    map[string]any{"values": []any{"${{ steps.first.outputs.missing }}"}},
			wantErr: true,
		},
		{
			name:    "fails to resolve each outside a foreach iteration",
			with:    map[string]any{"value": "${{ each }}"},
			wantErr: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := ResolveInputs(tt.with, ctx)

			if tt.wantErr {
				if err == nil {
					t.Fatal("expected an error, got nil")
				}
				return
			}
			if err != nil {
				t.Fatalf("ResolveInputs() error = %v", err)
			}
			if !reflect.DeepEqual(got[tt.key], tt.want) {
				t.Fatalf("%s = %v, want %v", tt.key, got[tt.key], tt.want)
			}
		})
	}
}

func TestResolveInputs_ResolvesEachDuringAnIteration(t *testing.T) {
	ctx := ExprContext{Each: "bob", HasEach: true}

	got, err := ResolveInputs(map[string]any{"value": "${{ each }}"}, ctx)
	if err != nil {
		t.Fatalf("ResolveInputs() error = %v", err)
	}
	if got["value"] != "bob" {
		t.Fatalf(`got["value"] = %v, want "bob"`, got["value"])
	}
}

func TestResolveForeach(t *testing.T) {
	ctx := ExprContext{
		Inputs: map[string]any{"not_a_list": "hello"},
		StepOutputs: map[string]map[string]any{
			"first": {"values": []any{"alice", "bob"}},
		},
	}

	tests := []struct {
		name    string
		foreach any
		want    []any
		wantErr bool
	}{
		{
			name:    "nil means no iteration",
			foreach: nil,
			want:    nil,
		},
		{
			name:    "resolves a literal list",
			foreach: []any{"a", "b"},
			want:    []any{"a", "b"},
		},
		{
			name:    "resolves a steps.outputs expression",
			foreach: "${{ steps.first.outputs.values }}",
			want:    []any{"alice", "bob"},
		},
		{
			name:    "fails when the referenced output was not produced",
			foreach: "${{ steps.unknown.outputs.values }}",
			wantErr: true,
		},
		{
			name:    "fails when the expression resolves to a non-list",
			foreach: "${{ workflow.inputs.not_a_list }}",
			wantErr: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := ResolveForeach(tt.foreach, ctx)

			if tt.wantErr {
				if err == nil {
					t.Fatal("expected an error, got nil")
				}
				return
			}
			if err != nil {
				t.Fatalf("ResolveForeach() error = %v", err)
			}
			if !reflect.DeepEqual(got, tt.want) {
				t.Fatalf("ResolveForeach() = %v, want %v", got, tt.want)
			}
		})
	}
}

func TestResolveConnector(t *testing.T) {
	ctx := ExprContext{
		Inputs:      map[string]any{"connector_id": "from_input", "not_a_string": 42},
		StepOutputs: map[string]map[string]any{"first": {"connector_id": "from_step_output"}},
		Bindings:    map[string]string{"ai_provider": "my_openai"},
	}

	tests := []struct {
		name      string
		connector string
		want      string
		wantErr   bool
	}{
		{
			name:      "empty connector resolves to no connector",
			connector: "",
			want:      "",
		},
		{
			name:      "resolves a bindings expression",
			connector: "${{ bindings.ai_provider }}",
			want:      "my_openai",
		},
		{
			name:      "resolves a workflow.inputs expression",
			connector: "${{ workflow.inputs.connector_id }}",
			want:      "from_input",
		},
		{
			name:      "resolves a steps.outputs expression",
			connector: "${{ steps.first.outputs.connector_id }}",
			want:      "from_step_output",
		},
		{
			name:      "rejects a literal value",
			connector: "my_connector",
			wantErr:   true,
		},
		{
			name:      "fails when the referenced binding was not provided",
			connector: "${{ bindings.missing }}",
			wantErr:   true,
		},
		{
			name:      "fails when the expression resolves to a non-string",
			connector: "${{ workflow.inputs.not_a_string }}",
			wantErr:   true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := ResolveConnector(tt.connector, ctx)

			if tt.wantErr {
				if err == nil {
					t.Fatal("expected an error, got nil")
				}
				return
			}
			if err != nil {
				t.Fatalf("ResolveConnector() error = %v", err)
			}
			if got != tt.want {
				t.Fatalf("ResolveConnector() = %q, want %q", got, tt.want)
			}
		})
	}
}

func TestResolveIf(t *testing.T) {
	ctx := ExprContext{
		Inputs: map[string]any{"enabled": true, "not_a_bool": "yes"},
		StepOutputs: map[string]map[string]any{
			"first": {"proceed": false},
		},
	}

	tests := []struct {
		name    string
		ifValue any
		want    bool
		wantErr bool
	}{
		{
			name:    "nil defaults to true",
			ifValue: nil,
			want:    true,
		},
		{
			name:    "literal true",
			ifValue: true,
			want:    true,
		},
		{
			name:    "literal false",
			ifValue: false,
			want:    false,
		},
		{
			name:    "resolves a workflow.inputs expression",
			ifValue: "${{ workflow.inputs.enabled }}",
			want:    true,
		},
		{
			name:    "resolves a steps.outputs expression",
			ifValue: "${{ steps.first.outputs.proceed }}",
			want:    false,
		},
		{
			name:    "fails when the referenced input was not provided",
			ifValue: "${{ workflow.inputs.missing }}",
			wantErr: true,
		},
		{
			name:    "fails when the expression resolves to a non-boolean",
			ifValue: "${{ workflow.inputs.not_a_bool }}",
			wantErr: true,
		},
		{
			name:    "fails when the literal value is not a boolean",
			ifValue: "yes",
			wantErr: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := ResolveIf(tt.ifValue, ctx)

			if tt.wantErr {
				if err == nil {
					t.Fatal("expected an error, got nil")
				}
				return
			}
			if err != nil {
				t.Fatalf("ResolveIf() error = %v", err)
			}
			if got != tt.want {
				t.Fatalf("ResolveIf() = %v, want %v", got, tt.want)
			}
		})
	}
}

func TestResolveExpression_Comparisons(t *testing.T) {
	ctx := ExprContext{
		Inputs: map[string]any{"count": float64(3)},
		StepOutputs: map[string]map[string]any{
			"first": {"status": "active", "value": float64(2), "flag": true},
		},
	}

	tests := []struct {
		name    string
		expr    string
		want    bool
		wantErr bool
	}{
		{name: "numeric >=, true", expr: "steps.first.outputs.value >= 2", want: true},
		{name: "numeric >=, false", expr: "steps.first.outputs.value >= 3", want: false},
		{name: "numeric >", expr: "workflow.inputs.count > 2", want: true},
		{name: "numeric <", expr: "workflow.inputs.count < 2", want: false},
		{name: "numeric <=", expr: "steps.first.outputs.value <= 2", want: true},
		{name: "numeric ==", expr: "steps.first.outputs.value == 2", want: true},
		{name: "numeric !=", expr: "steps.first.outputs.value != 2", want: false},
		{name: "string == with single quotes", expr: "steps.first.outputs.status == 'active'", want: true},
		{name: "string == with double quotes", expr: `steps.first.outputs.status == "inactive"`, want: false},
		{name: "string !=", expr: "steps.first.outputs.status != 'inactive'", want: true},
		{name: "bool ==", expr: "steps.first.outputs.flag == true", want: true},
		{name: "no surrounding whitespace", expr: "steps.first.outputs.value>=2", want: true},
		{
			name:    "fails when an ordering operator compares a non-numeric operand",
			expr:    "steps.first.outputs.status >= 2",
			wantErr: true,
		},
		{
			name:    "fails when the left-hand path cannot resolve",
			expr:    "steps.first.outputs.missing == 2",
			wantErr: true,
		},
		{
			name:    "fails on a malformed literal",
			expr:    "steps.first.outputs.value >= not_a_literal",
			wantErr: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := resolveExpression(tt.expr, ctx)

			if tt.wantErr {
				if err == nil {
					t.Fatal("expected an error, got nil")
				}
				return
			}
			if err != nil {
				t.Fatalf("resolveExpression(%q) error = %v", tt.expr, err)
			}
			if got != tt.want {
				t.Fatalf("resolveExpression(%q) = %v, want %v", tt.expr, got, tt.want)
			}
		})
	}
}

func TestValidateExpression_Comparisons(t *testing.T) {
	seenSteps := map[string]struct{}{"first": {}}

	tests := []struct {
		name    string
		expr    string
		wantErr bool
	}{
		{name: "accepts a well-formed numeric comparison", expr: "steps.first.outputs.value >= 2"},
		{name: "accepts a well-formed string comparison", expr: "steps.first.outputs.status == 'active'"},
		{name: "accepts a well-formed bool comparison", expr: "steps.first.outputs.flag == true"},
		{
			name:    "rejects a comparison whose left-hand side references an undefined step",
			expr:    "steps.unknown.outputs.value >= 2",
			wantErr: true,
		},
		{
			name:    "rejects a comparison with a malformed literal",
			expr:    "steps.first.outputs.value >= not_a_literal",
			wantErr: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := validateExpression(tt.expr, seenSteps, false)

			if tt.wantErr {
				if err == nil {
					t.Fatal("expected an error, got nil")
				}
				return
			}
			if err != nil {
				t.Fatalf("validateExpression(%q) error = %v", tt.expr, err)
			}
		})
	}
}
