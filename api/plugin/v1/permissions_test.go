package pluginv1

import "testing"

func TestValidatePermission(t *testing.T) {
	tests := []struct {
		name    string
		perm    string
		wantErr bool
	}{
		{name: "network.outbound is recognized", perm: "network.outbound", wantErr: false},
		{name: "a parameterized secrets.read scope is recognized", perm: "secrets.read:postgresql", wantErr: false},
		{name: "an empty string is rejected", perm: "", wantErr: true},
		{name: "an unknown flat scope is rejected", perm: "filesystem.read", wantErr: true},
		{name: "secrets.read with an empty suffix is rejected", perm: "secrets.read:", wantErr: true},
		{name: "secrets.read with no suffix at all is rejected", perm: "secrets.read", wantErr: true},
		{name: "a scope that merely contains a known prefix is rejected", perm: "not.secrets.read:postgresql", wantErr: true},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := ValidatePermission(tt.perm)
			if tt.wantErr && err == nil {
				t.Fatalf("ValidatePermission(%q) = nil, want an error", tt.perm)
			}
			if !tt.wantErr && err != nil {
				t.Fatalf("ValidatePermission(%q) error = %v, want nil", tt.perm, err)
			}
		})
	}
}
