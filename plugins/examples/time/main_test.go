package main

import (
	"context"
	"errors"
	"testing"
	"time"

	patchcord "github.com/lucasglmt/patchcord/sdk/go-plugin"
)

func TestNowAction_Run(t *testing.T) {
	before := time.Now().UTC()
	output, err := nowAction{}.Run(context.Background(), nil, nil)
	after := time.Now().UTC()
	if err != nil {
		t.Fatalf("Run() error = %v", err)
	}

	value, ok := output["value"].(string)
	if !ok {
		t.Fatalf("output[value] = %#v, want a string", output["value"])
	}
	parsed, err := time.Parse(time.RFC3339, value)
	if err != nil {
		t.Fatalf("output[value] = %q is not RFC3339: %v", value, err)
	}
	if parsed.Before(before.Add(-time.Second)) || parsed.After(after.Add(time.Second)) {
		t.Fatalf("output[value] = %v, want a time between %v and %v", parsed, before, after)
	}

	unix, ok := output["unix"].(float64)
	if !ok {
		t.Fatalf("output[unix] = %#v, want a float64", output["unix"])
	}
	if int64(unix) != parsed.Unix() {
		t.Fatalf("output[unix] = %v, want %v", unix, parsed.Unix())
	}
}

func TestFormatAction_Run(t *testing.T) {
	tests := []struct {
		name    string
		input   patchcord.ActionInput
		want    string
		wantErr bool
	}{
		{
			name:  "formats an RFC3339 value with the given layout",
			input: patchcord.ActionInput{"value": "2026-08-02T15:04:05Z", "layout": "2006-01-02"},
			want:  "2026-08-02",
		},
		{
			name:    "rejects a missing value",
			input:   patchcord.ActionInput{"layout": "2006-01-02"},
			wantErr: true,
		},
		{
			name:    "rejects a missing layout",
			input:   patchcord.ActionInput{"value": "2026-08-02T15:04:05Z"},
			wantErr: true,
		},
		{
			name:    "rejects a non-RFC3339 value",
			input:   patchcord.ActionInput{"value": "not a time", "layout": "2006-01-02"},
			wantErr: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			output, err := formatAction{}.Run(context.Background(), tt.input, nil)

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

func TestParseAction_Run(t *testing.T) {
	tests := []struct {
		name     string
		input    patchcord.ActionInput
		want     string
		wantUnix int64
		wantErr  bool
	}{
		{
			name:     "parses a custom layout into RFC3339 UTC",
			input:    patchcord.ActionInput{"value": "2026-08-02", "layout": "2006-01-02"},
			want:     "2026-08-02T00:00:00Z",
			wantUnix: 1785628800,
		},
		{
			name:    "rejects a missing value",
			input:   patchcord.ActionInput{"layout": "2006-01-02"},
			wantErr: true,
		},
		{
			name:    "rejects a missing layout",
			input:   patchcord.ActionInput{"value": "2026-08-02"},
			wantErr: true,
		},
		{
			name:    "rejects a value that doesn't match the layout",
			input:   patchcord.ActionInput{"value": "not a date", "layout": "2006-01-02"},
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
			if output["value"] != tt.want {
				t.Fatalf("output[value] = %v, want %q", output["value"], tt.want)
			}
			if output["unix"] != float64(tt.wantUnix) {
				t.Fatalf("output[unix] = %v, want %v", output["unix"], tt.wantUnix)
			}
		})
	}
}

func TestSleepAction_Run(t *testing.T) {
	tests := []struct {
		name    string
		input   patchcord.ActionInput
		want    string
		wantErr bool
	}{
		{
			name:  "sleeps for the given duration",
			input: patchcord.ActionInput{"duration": "10ms"},
			want:  "10ms",
		},
		{
			name:  "accepts a zero duration",
			input: patchcord.ActionInput{"duration": "0s"},
			want:  "0s",
		},
		{
			name:    "rejects a missing duration",
			input:   patchcord.ActionInput{},
			wantErr: true,
		},
		{
			name:    "rejects an invalid duration",
			input:   patchcord.ActionInput{"duration": "not a duration"},
			wantErr: true,
		},
		{
			name:    "rejects a negative duration",
			input:   patchcord.ActionInput{"duration": "-1s"},
			wantErr: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			before := time.Now()
			output, err := sleepAction{}.Run(context.Background(), tt.input, nil)
			elapsed := time.Since(before)

			if tt.wantErr {
				if err == nil {
					t.Fatal("expected an error, got nil")
				}
				return
			}
			if err != nil {
				t.Fatalf("Run() error = %v", err)
			}
			if output["slept_for"] != tt.want {
				t.Fatalf("output[slept_for] = %v, want %q", output["slept_for"], tt.want)
			}
			want, _ := time.ParseDuration(tt.want)
			if elapsed < want {
				t.Fatalf("Run() returned after %v, want at least %v", elapsed, want)
			}
		})
	}
}

func TestSleepAction_Run_ContextCancelled(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	cancel()

	before := time.Now()
	output, err := sleepAction{}.Run(ctx, patchcord.ActionInput{"duration": "1h"}, nil)
	elapsed := time.Since(before)

	if !errors.Is(err, context.Canceled) {
		t.Fatalf("Run() error = %v, want context.Canceled", err)
	}
	if output != nil {
		t.Fatalf("output = %#v, want nil", output)
	}
	if elapsed > time.Second {
		t.Fatalf("Run() took %v, want to return immediately on a cancelled context", elapsed)
	}
}

func TestAddAction_Run(t *testing.T) {
	tests := []struct {
		name    string
		input   patchcord.ActionInput
		want    string
		wantErr bool
	}{
		{
			name:  "adds a positive duration",
			input: patchcord.ActionInput{"value": "2026-08-02T00:00:00Z", "duration": "24h"},
			want:  "2026-08-03T00:00:00Z",
		},
		{
			name:  "adds a negative duration",
			input: patchcord.ActionInput{"value": "2026-08-02T00:00:00Z", "duration": "-30m"},
			want:  "2026-08-01T23:30:00Z",
		},
		{
			name:    "rejects a missing value",
			input:   patchcord.ActionInput{"duration": "24h"},
			wantErr: true,
		},
		{
			name:    "rejects a missing duration",
			input:   patchcord.ActionInput{"value": "2026-08-02T00:00:00Z"},
			wantErr: true,
		},
		{
			name:    "rejects a non-RFC3339 value",
			input:   patchcord.ActionInput{"value": "not a time", "duration": "24h"},
			wantErr: true,
		},
		{
			name:    "rejects an invalid duration",
			input:   patchcord.ActionInput{"value": "2026-08-02T00:00:00Z", "duration": "not a duration"},
			wantErr: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			output, err := addAction{}.Run(context.Background(), tt.input, nil)

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
