package workflow

import "testing"

func TestValidateInputSchema(t *testing.T) {
	tests := []struct {
		name        string
		with        map[string]any
		inputSchema map[string]any
		wantErr     bool
	}{
		{
			name:        "nil input_schema constrains nothing",
			with:        map[string]any{"anything": "goes"},
			inputSchema: nil,
		},
		{
			name:        "input_schema with no properties/required constrains nothing",
			with:        map[string]any{"value": 42},
			inputSchema: map[string]any{"type": "object"},
		},
		{
			name: "accepts a literal matching its declared string type",
			with: map[string]any{"value": "hello"},
			inputSchema: map[string]any{
				"properties": map[string]any{"value": map[string]any{"type": "string"}},
				"required":   []any{"value"},
			},
		},
		{
			name: "rejects a literal not matching its declared string type",
			with: map[string]any{"value": 42},
			inputSchema: map[string]any{
				"properties": map[string]any{"value": map[string]any{"type": "string"}},
				"required":   []any{"value"},
			},
			wantErr: true,
		},
		{
			name: "rejects a missing required field",
			with: map[string]any{},
			inputSchema: map[string]any{
				"properties": map[string]any{"value": map[string]any{"type": "string"}},
				"required":   []any{"value"},
			},
			wantErr: true,
		},
		{
			name: "a required field is present even though its value is an expression",
			with: map[string]any{"value": "${{ steps.first.outputs.value }}"},
			inputSchema: map[string]any{
				"properties": map[string]any{"value": map[string]any{"type": "string"}},
				"required":   []any{"value"},
			},
		},
		{
			name: "an expression value is never type-checked, even against an incompatible type",
			with: map[string]any{"value": "${{ steps.first.outputs.value }}"},
			inputSchema: map[string]any{
				"properties": map[string]any{"value": map[string]any{"type": "integer"}},
			},
		},
		{
			name: "an expression nested inside a list skips the whole field, not just that item",
			with: map[string]any{"values": []any{"a literal", "${{ steps.first.outputs.value }}"}},
			inputSchema: map[string]any{
				"properties": map[string]any{
					"values": map[string]any{"type": "array", "items": map[string]any{"type": "integer"}},
				},
			},
		},
		{
			name: "a sibling field with no expression is still checked",
			with: map[string]any{
				"value":      42,
				"unresolved": "${{ steps.first.outputs.value }}",
			},
			inputSchema: map[string]any{
				"properties": map[string]any{
					"value":      map[string]any{"type": "string"},
					"unresolved": map[string]any{"type": "string"},
				},
				"required": []any{"value", "unresolved"},
			},
			wantErr: true,
		},
		{
			name: "accepts int for a declared number property",
			with: map[string]any{"value": 42},
			inputSchema: map[string]any{
				"properties": map[string]any{"value": map[string]any{"type": "number"}},
			},
		},
		{
			name: "accepts int for a declared integer property",
			with: map[string]any{"value": 42},
			inputSchema: map[string]any{
				"properties": map[string]any{"value": map[string]any{"type": "integer"}},
			},
		},
		{
			name: "rejects a fractional value for a declared integer property",
			with: map[string]any{"value": 4.2},
			inputSchema: map[string]any{
				"properties": map[string]any{"value": map[string]any{"type": "integer"}},
			},
			wantErr: true,
		},
		{
			name: "accepts a nested object matching its own properties/required",
			with: map[string]any{"config": map[string]any{"host": "db.internal"}},
			inputSchema: map[string]any{
				"properties": map[string]any{
					"config": map[string]any{
						"type":       "object",
						"properties": map[string]any{"host": map[string]any{"type": "string"}},
						"required":   []any{"host"},
					},
				},
			},
		},
		{
			name: "rejects a nested object missing its own required field",
			with: map[string]any{"config": map[string]any{}},
			inputSchema: map[string]any{
				"properties": map[string]any{
					"config": map[string]any{
						"type":       "object",
						"properties": map[string]any{"host": map[string]any{"type": "string"}},
						"required":   []any{"host"},
					},
				},
			},
			wantErr: true,
		},
		{
			name: "rejects an array item not matching its items schema",
			with: map[string]any{"values": []any{"a", 2, "c"}},
			inputSchema: map[string]any{
				"properties": map[string]any{
					"values": map[string]any{"type": "array", "items": map[string]any{"type": "string"}},
				},
			},
			wantErr: true,
		},
		{
			name: "a property with no recognized type accepts anything",
			with: map[string]any{"value": map[string]any{"anything": []any{"goes", 1, true}}},
			inputSchema: map[string]any{
				"properties": map[string]any{"value": map[string]any{"description": "no type declared"}},
			},
		},
		{
			name: "a with key not declared in properties is ignored",
			with: map[string]any{"undeclared": 42},
			inputSchema: map[string]any{
				"properties": map[string]any{"value": map[string]any{"type": "string"}},
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := validateInputSchema(tt.with, tt.inputSchema)
			if tt.wantErr && err == nil {
				t.Fatal("expected an error, got nil")
			}
			if !tt.wantErr && err != nil {
				t.Fatalf("validateInputSchema() error = %v", err)
			}
		})
	}
}

func TestContainsExpression(t *testing.T) {
	tests := []struct {
		name  string
		value any
		want  bool
	}{
		{name: "a plain string literal", value: "hello", want: false},
		{name: "a whole expression string", value: "${{ steps.first.outputs.value }}", want: true},
		{name: "a non-string literal", value: 42, want: false},
		{name: "an expression nested inside a list", value: []any{"a", "${{ x }}"}, want: true},
		{name: "a list with no expression", value: []any{"a", "b"}, want: false},
		{name: "an expression nested inside a map", value: map[string]any{"k": "${{ x }}"}, want: true},
		{name: "a map with no expression", value: map[string]any{"k": "v"}, want: false},
		{
			name:  "an expression nested several levels deep",
			value: map[string]any{"k": []any{map[string]any{"deep": "${{ x }}"}}},
			want:  true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := containsExpression(tt.value); got != tt.want {
				t.Fatalf("containsExpression(%#v) = %v, want %v", tt.value, got, tt.want)
			}
		})
	}
}

func TestNormalizeNumbers(t *testing.T) {
	got := normalizeNumbers(map[string]any{
		"int":    42,
		"float":  4.2,
		"nested": []any{1, 2, map[string]any{"deep": 3}},
	}).(map[string]any)

	if _, ok := got["int"].(float64); !ok {
		t.Fatalf("normalizeNumbers()[int] = %#v (%T), want float64", got["int"], got["int"])
	}
	if got["int"].(float64) != 42 {
		t.Fatalf("normalizeNumbers()[int] = %v, want 42", got["int"])
	}
	if got["float"].(float64) != 4.2 {
		t.Fatalf("normalizeNumbers()[float] = %v, want 4.2", got["float"])
	}

	nested := got["nested"].([]any)
	for i, v := range nested[:2] {
		if _, ok := v.(float64); !ok {
			t.Fatalf("normalizeNumbers()[nested][%d] = %#v (%T), want float64", i, v, v)
		}
	}
	deep := nested[2].(map[string]any)
	if _, ok := deep["deep"].(float64); !ok {
		t.Fatalf("normalizeNumbers()[nested][2][deep] = %#v (%T), want float64", deep["deep"], deep["deep"])
	}
}
