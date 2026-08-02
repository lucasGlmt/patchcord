package main

import (
	"context"
	"reflect"
	"testing"

	patchcord "github.com/lucasglmt/patchcord/sdk/go-plugin"
)

func TestParseAction_Run(t *testing.T) {
	tests := []struct {
		name    string
		input   patchcord.ActionInput
		want    any
		wantErr bool
	}{
		{
			name:  "parses an object",
			input: patchcord.ActionInput{"value": `{"a": 1, "b": "two"}`},
			want:  map[string]any{"a": 1.0, "b": "two"},
		},
		{
			name:  "parses an array",
			input: patchcord.ActionInput{"value": `[1, 2, 3]`},
			want:  []any{1.0, 2.0, 3.0},
		},
		{
			name:    "rejects a missing value",
			input:   patchcord.ActionInput{},
			wantErr: true,
		},
		{
			name:    "rejects invalid json",
			input:   patchcord.ActionInput{"value": `{not json`},
			wantErr: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			output, err := parseAction{}.Run(context.Background(), tt.input, nil)

			if tt.wantErr {
				if err == nil {
					t.Fatal("expected an error, got nil")
				}
				return
			}
			if err != nil {
				t.Fatalf("Run() error = %v", err)
			}
			if got := output["value"]; !reflect.DeepEqual(got, tt.want) {
				t.Fatalf("output[value] = %#v, want %#v", got, tt.want)
			}
		})
	}
}

func TestStringifyAction_Run(t *testing.T) {
	tests := []struct {
		name    string
		input   patchcord.ActionInput
		want    string
		wantErr bool
	}{
		{
			name:  "stringifies an object compactly by default",
			input: patchcord.ActionInput{"value": map[string]any{"a": 1.0}},
			want:  `{"a":1}`,
		},
		{
			name:  "stringifies an object with indentation when pretty is true",
			input: patchcord.ActionInput{"value": map[string]any{"a": 1.0}, "pretty": true},
			want:  "{\n  \"a\": 1\n}",
		},
		{
			name:    "rejects a missing value",
			input:   patchcord.ActionInput{},
			wantErr: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			output, err := stringifyAction{}.Run(context.Background(), tt.input, nil)

			if tt.wantErr {
				if err == nil {
					t.Fatal("expected an error, got nil")
				}
				return
			}
			if err != nil {
				t.Fatalf("Run() error = %v", err)
			}
			if output["value"] != tt.want {
				t.Fatalf("output[value] = %v, want %q", output["value"], tt.want)
			}
		})
	}
}

func TestMergeAction_Run(t *testing.T) {
	tests := []struct {
		name    string
		input   patchcord.ActionInput
		want    map[string]any
		wantErr bool
	}{
		{
			name: "patch keys override base keys at the top level",
			input: patchcord.ActionInput{
				"base":  map[string]any{"a": 1.0, "b": 2.0},
				"patch": map[string]any{"b": 3.0, "c": 4.0},
			},
			want: map[string]any{"a": 1.0, "b": 3.0, "c": 4.0},
		},
		{
			name:    "rejects a non-object base",
			input:   patchcord.ActionInput{"base": "not an object", "patch": map[string]any{}},
			wantErr: true,
		},
		{
			name:    "rejects a non-object patch",
			input:   patchcord.ActionInput{"base": map[string]any{}, "patch": "not an object"},
			wantErr: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			output, err := mergeAction{}.Run(context.Background(), tt.input, nil)

			if tt.wantErr {
				if err == nil {
					t.Fatal("expected an error, got nil")
				}
				return
			}
			if err != nil {
				t.Fatalf("Run() error = %v", err)
			}
			got, ok := output["value"].(map[string]any)
			if !ok || !reflect.DeepEqual(got, tt.want) {
				t.Fatalf("output[value] = %#v, want %#v", output["value"], tt.want)
			}
		})
	}
}

func TestJsonpathExtractAction_Run(t *testing.T) {
	sample := map[string]any{
		"store": map[string]any{
			"name": "Patchcord",
			"books": []any{
				map[string]any{"title": "Alpha", "price": 10.0},
				map[string]any{"title": "Beta", "price": 20.0},
			},
		},
	}

	tests := []struct {
		name      string
		input     patchcord.ActionInput
		wantFound bool
		wantValue any
		wantErr   bool
	}{
		{
			name:      "extracts a nested key with dot access",
			input:     patchcord.ActionInput{"value": sample, "path": "$.store.name"},
			wantFound: true,
			wantValue: "Patchcord",
		},
		{
			name:      "extracts an array element by index",
			input:     patchcord.ActionInput{"value": sample, "path": "$.store.books[1].title"},
			wantFound: true,
			wantValue: "Beta",
		},
		{
			name:      "extracts a key with bracket notation",
			input:     patchcord.ActionInput{"value": sample, "path": "$['store']['name']"},
			wantFound: true,
			wantValue: "Patchcord",
		},
		{
			name:      "reports not found for a missing key, without erroring",
			input:     patchcord.ActionInput{"value": sample, "path": "$.store.missing"},
			wantFound: false,
			wantValue: nil,
		},
		{
			name:    "rejects a missing path",
			input:   patchcord.ActionInput{"value": sample},
			wantErr: true,
		},
		{
			name:    "rejects a missing value",
			input:   patchcord.ActionInput{"path": "$.store.name"},
			wantErr: true,
		},
		{
			name:    "rejects a path that doesn't start with $",
			input:   patchcord.ActionInput{"value": sample, "path": "store.name"},
			wantErr: true,
		},
		{
			name:    "rejects a malformed bracket segment",
			input:   patchcord.ActionInput{"value": sample, "path": "$.store["},
			wantErr: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			output, err := jsonpathExtractAction{}.Run(context.Background(), tt.input, nil)

			if tt.wantErr {
				if err == nil {
					t.Fatal("expected an error, got nil")
				}
				return
			}
			if err != nil {
				t.Fatalf("Run() error = %v", err)
			}
			if output["found"] != tt.wantFound {
				t.Fatalf("output[found] = %v, want %v", output["found"], tt.wantFound)
			}
			if output["value"] != tt.wantValue {
				t.Fatalf("output[value] = %v, want %v", output["value"], tt.wantValue)
			}
		})
	}

	t.Run("wildcard collects every match into values", func(t *testing.T) {
		output, err := jsonpathExtractAction{}.Run(context.Background(), patchcord.ActionInput{
			"value": sample,
			"path":  "$.store.books[*].title",
		}, nil)
		if err != nil {
			t.Fatalf("Run() error = %v", err)
		}
		values, ok := output["values"].([]any)
		if !ok || len(values) != 2 || values[0] != "Alpha" || values[1] != "Beta" {
			t.Fatalf("output[values] = %#v, want [Alpha Beta]", output["values"])
		}
	})
}
