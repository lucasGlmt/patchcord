package secrets

import (
	"context"
	"testing"
)

func TestValidateType(t *testing.T) {
	tests := []struct {
		name    string
		typ     string
		wantErr bool
	}{
		{name: "accepts env", typ: "env"},
		{name: "rejects an unknown type", typ: "vault", wantErr: true},
		{name: "rejects an empty type", typ: "", wantErr: true},
		{name: "rejects a near-miss typo", typ: "emv", wantErr: true},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := ValidateType(tt.typ)
			if tt.wantErr && err == nil {
				t.Fatal("expected an error, got nil")
			}
			if !tt.wantErr && err != nil {
				t.Fatalf("ValidateType() error = %v", err)
			}
		})
	}
}

func TestEnvStore_Resolve(t *testing.T) {
	tests := []struct {
		name    string
		ref     Reference
		setEnv  bool
		envVal  string
		want    string
		wantErr bool
	}{
		{
			name:   "resolves a set environment variable",
			ref:    Reference{Type: "env", Key: "PATCHCORD_TEST_SECRET"},
			setEnv: true,
			envVal: "s3cr3t",
			want:   "s3cr3t",
		},
		{
			name:    "errors when the environment variable is not set",
			ref:     Reference{Type: "env", Key: "PATCHCORD_TEST_SECRET_UNSET"},
			wantErr: true,
		},
		{
			name:    "errors on an unsupported reference type",
			ref:     Reference{Type: "vault", Key: "PATCHCORD_TEST_SECRET"},
			setEnv:  true,
			envVal:  "s3cr3t",
			wantErr: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if tt.setEnv {
				t.Setenv(tt.ref.Key, tt.envVal)
			}

			got, err := EnvStore{}.Resolve(context.Background(), tt.ref)
			if tt.wantErr {
				if err == nil {
					t.Fatal("expected an error, got nil")
				}
				return
			}
			if err != nil {
				t.Fatalf("Resolve() error = %v", err)
			}
			if got != tt.want {
				t.Fatalf("Resolve() = %q, want %q", got, tt.want)
			}
		})
	}
}
