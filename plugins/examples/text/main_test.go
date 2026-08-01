package main

import (
	"context"
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
			output, err := uppercaseAction{}.Run(context.Background(), tt.input)

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
			output, err := lowercaseAction{}.Run(context.Background(), tt.input)

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
			output, err := joinAction{}.Run(context.Background(), tt.input)

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
