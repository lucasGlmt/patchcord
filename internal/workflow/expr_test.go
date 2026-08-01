package workflow

import (
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
		want    any
		wantErr bool
	}{
		{
			name: "resolves a workflow input expression",
			with: map[string]any{"value": "${{ workflow.inputs.value }}"},
			want: "hello",
		},
		{
			name: "resolves a step output expression",
			with: map[string]any{"value": "${{ steps.first.outputs.value }}"},
			want: "HELLO",
		},
		{
			name: "passes through a literal value unchanged",
			with: map[string]any{"value": "literal"},
			want: "literal",
		},
		{
			name: "passes through a non-string value unchanged",
			with: map[string]any{"value": 42},
			want: 42,
		},
		{
			name: "leaves a value with embedded (non-whole) syntax unresolved",
			with: map[string]any{"value": "prefix-${{ workflow.inputs.value }}"},
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
			if got["value"] != tt.want {
				t.Fatalf("value = %v, want %v", got["value"], tt.want)
			}
		})
	}
}
