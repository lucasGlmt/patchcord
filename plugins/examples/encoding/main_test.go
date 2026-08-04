package main

import (
	"context"
	"testing"

	"github.com/google/uuid"

	patchcord "github.com/lucasglmt/patchcord/sdk/go-plugin"
)

func TestBase64EncodeAction_Run(t *testing.T) {
	tests := []struct {
		name    string
		input   patchcord.ActionInput
		want    string
		wantErr bool
	}{
		{
			name:  "encodes a string",
			input: patchcord.ActionInput{"value": "Welcome Patchcord"},
			want:  "V2VsY29tZSBQYXRjaGNvcmQ=",
		},
		{
			name:  "encodes an empty string",
			input: patchcord.ActionInput{"value": ""},
			want:  "",
		},
		{
			name:    "rejects a missing value",
			input:   patchcord.ActionInput{},
			wantErr: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			output, err := base64EncodeAction{}.Run(context.Background(), tt.input, nil)

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

func TestBase64DecodeAction_Run(t *testing.T) {
	tests := []struct {
		name    string
		input   patchcord.ActionInput
		want    string
		wantErr bool
	}{
		{
			name:  "decodes a valid base64 string",
			input: patchcord.ActionInput{"value": "V2VsY29tZSBQYXRjaGNvcmQ="},
			want:  "Welcome Patchcord",
		},
		{
			name:    "rejects a missing value",
			input:   patchcord.ActionInput{},
			wantErr: true,
		},
		{
			name:    "rejects invalid base64",
			input:   patchcord.ActionInput{"value": "not valid base64!!"},
			wantErr: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			output, err := base64DecodeAction{}.Run(context.Background(), tt.input, nil)

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

func TestSha256Action_Run(t *testing.T) {
	tests := []struct {
		name    string
		input   patchcord.ActionInput
		want    string
		wantErr bool
	}{
		{
			name:  "hashes a string",
			input: patchcord.ActionInput{"value": "Welcome Patchcord"},
			want:  "f1ee448b5c6c58cc84f58103378684f2030aa3161ef1716a1e54135fed50d6b3",
		},
		{
			name:  "hashes an empty string",
			input: patchcord.ActionInput{"value": ""},
			want:  "e3b0c44298fc1c149afbf4c8996fb92427ae41e4649b934ca495991b7852b855",
		},
		{
			name:    "rejects a missing value",
			input:   patchcord.ActionInput{},
			wantErr: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			output, err := sha256Action{}.Run(context.Background(), tt.input, nil)

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

func TestUUIDGenerateAction_Run(t *testing.T) {
	output, err := uuidGenerateAction{}.Run(context.Background(), nil, nil)
	if err != nil {
		t.Fatalf("Run() error = %v", err)
	}

	value, ok := output["value"].(string)
	if !ok {
		t.Fatalf("output[value] = %#v, want a string", output["value"])
	}
	if _, err := uuid.Parse(value); err != nil {
		t.Fatalf("output[value] = %q is not a valid UUID: %v", value, err)
	}

	other, err := uuidGenerateAction{}.Run(context.Background(), nil, nil)
	if err != nil {
		t.Fatalf("Run() error = %v", err)
	}
	if output["value"] == other["value"] {
		t.Fatal("two calls to Run() produced the same UUID")
	}
}

// TestActionIDs guards against an ID() left stale after a copy-paste: the
// plugin routes an incoming call by matching the action ID a workflow's
// `uses:` declares against Action.ID() (see sdk/go-plugin), so an action
// whose ID() doesn't match its own doc comment/registration would silently
// run under the wrong name — or never run at all, shadowed by whichever
// other action does claim that ID.
func TestActionIDs(t *testing.T) {
	tests := []struct {
		action patchcord.Action
		want   string
	}{
		{base64EncodeAction{}, "base64.encode@1"},
		{base64DecodeAction{}, "base64.decode@1"},
		{sha256Action{}, "hash.sha256@1"},
		{uuidGenerateAction{}, "uuid.generate@1"},
	}

	for _, tt := range tests {
		if got := tt.action.ID(); got != tt.want {
			t.Errorf("%T.ID() = %q, want %q", tt.action, got, tt.want)
		}
	}
}
