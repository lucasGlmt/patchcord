package main

import (
	"context"
	"slices"
	"testing"

	patchcord "github.com/lucasglmt/patchcord/sdk/go-plugin"
)

func TestUppercaseAction_Run(t *testing.T) {
	tests := []struct {
		name    string
		input   patchcord.ActionInput
		want    string
		wantErr bool
	}{
		{
			name:  "uppercases a string value",
			input: patchcord.ActionInput{"value": "Welcome Patchcord"},
			want:  "WELCOME PATCHCORD",
		},
		{
			name:    "rejects a missing value",
			input:   patchcord.ActionInput{},
			wantErr: true,
		},
		{
			name:    "rejects a non-string value",
			input:   patchcord.ActionInput{"value": 42},
			wantErr: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			output, err := uppercaseAction{}.Run(context.Background(), tt.input, nil)

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

func TestLowercaseAction_Run(t *testing.T) {
	tests := []struct {
		name    string
		input   patchcord.ActionInput
		want    string
		wantErr bool
	}{
		{
			name:  "lowercases a string value",
			input: patchcord.ActionInput{"value": "Welcome Patchcord"},
			want:  "welcome patchcord",
		},
		{
			name:    "rejects a missing value",
			input:   patchcord.ActionInput{},
			wantErr: true,
		},
		{
			name:    "rejects a non-string value",
			input:   patchcord.ActionInput{"value": 42},
			wantErr: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			output, err := lowercaseAction{}.Run(context.Background(), tt.input, nil)

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

func TestJoinAction_Run(t *testing.T) {
	tests := []struct {
		name    string
		input   patchcord.ActionInput
		want    string
		wantErr bool
	}{
		{
			name:  "joins values with the separator",
			input: patchcord.ActionInput{"values": []any{"welcome", "to", "patchcord"}, "separator": " "},
			want:  "welcome to patchcord",
		},
		{
			name:  "joins with an empty separator when none is given",
			input: patchcord.ActionInput{"values": []any{"a", "b", "c"}},
			want:  "abc",
		},
		{
			name:  "joins a single value",
			input: patchcord.ActionInput{"values": []any{"solo"}, "separator": ", "},
			want:  "solo",
		},
		{
			name:    "rejects a missing values list",
			input:   patchcord.ActionInput{},
			wantErr: true,
		},
		{
			name:    "rejects a non-list values input",
			input:   patchcord.ActionInput{"values": "not a list"},
			wantErr: true,
		},
		{
			name:    "rejects a values list containing a non-string element",
			input:   patchcord.ActionInput{"values": []any{"a", 42}},
			wantErr: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			output, err := joinAction{}.Run(context.Background(), tt.input, nil)

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

func TestSplitAction_Run(t *testing.T) {
	tests := []struct {
		name    string
		input   patchcord.ActionInput
		want    []string
		wantErr bool
	}{
		{
			name:  "splits on the separator",
			input: patchcord.ActionInput{"value": "welcome to patchcord", "separator": " "},
			want:  []string{"welcome", "to", "patchcord"},
		},
		{
			name:  "splits on a multi-character separator",
			input: patchcord.ActionInput{"value": "a, b, c", "separator": ", "},
			want:  []string{"a", "b", "c"},
		},
		{
			name:  "returns the whole value when the separator never occurs",
			input: patchcord.ActionInput{"value": "solo", "separator": ","},
			want:  []string{"solo"},
		},
		{
			name:    "rejects a missing value",
			input:   patchcord.ActionInput{"separator": " "},
			wantErr: true,
		},
		{
			name:    "rejects a non-string value",
			input:   patchcord.ActionInput{"value": 42, "separator": " "},
			wantErr: true,
		},
		{
			name:    "rejects a missing separator",
			input:   patchcord.ActionInput{"value": "welcome to patchcord"},
			wantErr: true,
		},
		{
			name:    "rejects a non-string separator",
			input:   patchcord.ActionInput{"value": "welcome to patchcord", "separator": 42},
			wantErr: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			output, err := splitAction{}.Run(context.Background(), tt.input, nil)

			if tt.wantErr {
				if err == nil {
					t.Fatal("expected an error, got nil")
				}
				return
			}
			if err != nil {
				t.Fatalf("Run() error = %v", err)
			}
			got, ok := output["values"].([]any)
			if !ok {
				t.Fatalf("output[values] = %#v, want []any", output["values"])
			}
			want := make([]any, len(tt.want))
			for i, v := range tt.want {
				want[i] = v
			}
			if !slices.Equal(got, want) {
				t.Fatalf("output[values] = %v, want %v", got, want)
			}
		})
	}
}

func TestEchoConnectorAction_Run(t *testing.T) {
	t.Run("reports unbound when no connector is given", func(t *testing.T) {
		output, err := echoConnectorAction{}.Run(context.Background(), nil, nil)
		if err != nil {
			t.Fatalf("Run() error = %v", err)
		}
		if output["bound"] != false {
			t.Fatalf("output[bound] = %v, want false", output["bound"])
		}
	})

	t.Run("echoes type and config, never secrets", func(t *testing.T) {
		connector := &patchcord.ConnectorConfig{
			Type:    "demo.connection@1",
			Config:  map[string]any{"greeting": "hello"},
			Secrets: map[string]any{"token": "s3cr3t"},
		}

		output, err := echoConnectorAction{}.Run(context.Background(), nil, connector)
		if err != nil {
			t.Fatalf("Run() error = %v", err)
		}
		if output["bound"] != true {
			t.Fatalf("output[bound] = %v, want true", output["bound"])
		}
		if output["type"] != "demo.connection@1" {
			t.Fatalf("output[type] = %v, want %q", output["type"], "demo.connection@1")
		}
		config, ok := output["config"].(map[string]any)
		if !ok || config["greeting"] != "hello" {
			t.Fatalf("output[config] = %v, want {\"greeting\":\"hello\"}", output["config"])
		}

		if _, leaked := output["secrets"]; leaked {
			t.Fatal("output must never include \"secrets\"")
		}
		for _, v := range output {
			if v == "s3cr3t" {
				t.Fatalf("output = %v, must never leak the resolved secret value", output)
			}
		}
	})
}
