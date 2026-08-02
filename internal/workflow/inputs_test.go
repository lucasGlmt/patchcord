package workflow

import (
	"errors"
	"testing"
)

func TestValidateInputDefs(t *testing.T) {
	tests := []struct {
		name    string
		defs    []InputDef
		wantErr bool
	}{
		{name: "accepts no inputs"},
		{
			name: "accepts string, number, boolean and enum types",
			defs: []InputDef{
				{Name: "a", Type: "string"},
				{Name: "b", Type: "number"},
				{Name: "c", Type: "boolean"},
				{Name: "d", Type: "enum", Enum: []string{"x", "y"}},
			},
		},
		{
			name: "empty type defaults to string",
			defs: []InputDef{{Name: "a", Default: "hello"}},
		},
		{
			name:    "rejects an empty name",
			defs:    []InputDef{{Name: ""}},
			wantErr: true,
		},
		{
			name:    "rejects a duplicate name",
			defs:    []InputDef{{Name: "a"}, {Name: "a"}},
			wantErr: true,
		},
		{
			name:    "rejects an unknown type",
			defs:    []InputDef{{Name: "a", Type: "array"}},
			wantErr: true,
		},
		{
			name:    "rejects enum without an enum list",
			defs:    []InputDef{{Name: "a", Type: "enum"}},
			wantErr: true,
		},
		{
			name:    "rejects a duplicate enum value",
			defs:    []InputDef{{Name: "a", Type: "enum", Enum: []string{"x", "x"}}},
			wantErr: true,
		},
		{
			name:    "rejects enum set on a non-enum type",
			defs:    []InputDef{{Name: "a", Type: "string", Enum: []string{"x"}}},
			wantErr: true,
		},
		{
			name:    "rejects required and default together",
			defs:    []InputDef{{Name: "a", Required: true, Default: "hello"}},
			wantErr: true,
		},
		{
			name:    "rejects a default that doesn't match the declared type",
			defs:    []InputDef{{Name: "a", Type: "number", Default: "not a number"}},
			wantErr: true,
		},
		{
			name:    "rejects an enum default not in the enum list",
			defs:    []InputDef{{Name: "a", Type: "enum", Enum: []string{"x", "y"}, Default: "z"}},
			wantErr: true,
		},
		{
			name: "accepts a matching default for every type",
			defs: []InputDef{
				{Name: "a", Type: "string", Default: "hello"},
				{Name: "b", Type: "number", Default: 42},
				{Name: "c", Type: "boolean", Default: true},
				{Name: "d", Type: "enum", Enum: []string{"x", "y"}, Default: "x"},
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := validateInputDefs(tt.defs)
			if tt.wantErr && err == nil {
				t.Fatal("expected an error, got nil")
			}
			if !tt.wantErr && err != nil {
				t.Fatalf("validateInputDefs() error = %v", err)
			}
		})
	}
}

func TestPrepareInputs(t *testing.T) {
	t.Run("passes provided through unchanged when no inputs are declared", func(t *testing.T) {
		provided := map[string]any{"anything": "goes"}
		got, err := PrepareInputs(nil, provided)
		if err != nil {
			t.Fatalf("PrepareInputs() error = %v", err)
		}
		if got["anything"] != "goes" {
			t.Fatalf("got %v, want provided passed through unchanged", got)
		}
	})

	t.Run("fills in a default when the input is omitted", func(t *testing.T) {
		defs := []InputDef{{Name: "name", Type: "string", Default: "world"}}
		got, err := PrepareInputs(defs, map[string]any{})
		if err != nil {
			t.Fatalf("PrepareInputs() error = %v", err)
		}
		if got["name"] != "world" {
			t.Fatalf("got %v, want default applied", got)
		}
	})

	t.Run("rejects a missing required input", func(t *testing.T) {
		defs := []InputDef{{Name: "name", Type: "string", Required: true}}
		_, err := PrepareInputs(defs, map[string]any{})
		if !errors.Is(err, ErrInvalidInputs) {
			t.Fatalf("PrepareInputs() error = %v, want ErrInvalidInputs", err)
		}
	})

	t.Run("omits an optional input with no default and no provided value", func(t *testing.T) {
		defs := []InputDef{{Name: "name", Type: "string"}}
		got, err := PrepareInputs(defs, map[string]any{})
		if err != nil {
			t.Fatalf("PrepareInputs() error = %v", err)
		}
		if _, ok := got["name"]; ok {
			t.Fatalf("got %v, want no \"name\" key", got)
		}
	})

	t.Run("rejects an undeclared input key", func(t *testing.T) {
		defs := []InputDef{{Name: "name", Type: "string"}}
		_, err := PrepareInputs(defs, map[string]any{"name": "world", "extra": "nope"})
		if !errors.Is(err, ErrInvalidInputs) {
			t.Fatalf("PrepareInputs() error = %v, want ErrInvalidInputs", err)
		}
	})

	t.Run("coerces a CLI-style string into a number", func(t *testing.T) {
		defs := []InputDef{{Name: "count", Type: "number"}}
		got, err := PrepareInputs(defs, map[string]any{"count": "42"})
		if err != nil {
			t.Fatalf("PrepareInputs() error = %v", err)
		}
		if got["count"] != float64(42) {
			t.Fatalf("got %v (%T), want float64(42)", got["count"], got["count"])
		}
	})

	t.Run("coerces a CLI-style string into a boolean", func(t *testing.T) {
		defs := []InputDef{{Name: "shout", Type: "boolean"}}
		got, err := PrepareInputs(defs, map[string]any{"shout": "true"})
		if err != nil {
			t.Fatalf("PrepareInputs() error = %v", err)
		}
		if got["shout"] != true {
			t.Fatalf("got %v (%T), want true", got["shout"], got["shout"])
		}
	})

	t.Run("accepts an already-typed JSON value unchanged", func(t *testing.T) {
		defs := []InputDef{{Name: "count", Type: "number"}}
		got, err := PrepareInputs(defs, map[string]any{"count": float64(7)})
		if err != nil {
			t.Fatalf("PrepareInputs() error = %v", err)
		}
		if got["count"] != float64(7) {
			t.Fatalf("got %v, want float64(7)", got["count"])
		}
	})

	t.Run("rejects a value not matching the declared type", func(t *testing.T) {
		defs := []InputDef{{Name: "count", Type: "number"}}
		_, err := PrepareInputs(defs, map[string]any{"count": "not a number"})
		if !errors.Is(err, ErrInvalidInputs) {
			t.Fatalf("PrepareInputs() error = %v, want ErrInvalidInputs", err)
		}
	})

	t.Run("rejects an enum value not in the list", func(t *testing.T) {
		defs := []InputDef{{Name: "greeting", Type: "enum", Enum: []string{"hi", "hello"}}}
		_, err := PrepareInputs(defs, map[string]any{"greeting": "yo"})
		if !errors.Is(err, ErrInvalidInputs) {
			t.Fatalf("PrepareInputs() error = %v, want ErrInvalidInputs", err)
		}
	})

	t.Run("accepts a valid enum value", func(t *testing.T) {
		defs := []InputDef{{Name: "greeting", Type: "enum", Enum: []string{"hi", "hello"}}}
		got, err := PrepareInputs(defs, map[string]any{"greeting": "hi"})
		if err != nil {
			t.Fatalf("PrepareInputs() error = %v", err)
		}
		if got["greeting"] != "hi" {
			t.Fatalf("got %v, want \"hi\"", got["greeting"])
		}
	})

	t.Run("does not mutate provided", func(t *testing.T) {
		defs := []InputDef{{Name: "name", Type: "string", Required: true}}
		provided := map[string]any{"name": "world"}
		if _, err := PrepareInputs(defs, provided); err != nil {
			t.Fatalf("PrepareInputs() error = %v", err)
		}
		if len(provided) != 1 {
			t.Fatalf("provided was mutated: %v", provided)
		}
	})
}
