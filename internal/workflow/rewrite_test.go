package workflow

import "testing"

func TestRewriteVersion(t *testing.T) {
	tests := []struct {
		name    string
		source  string
		version int
		want    string
	}{
		{
			name:    "replaces a simple top-level version",
			source:  "schema_version: 1\nid: demo\nversion: 1\ntrigger:\n  type: manual\n",
			version: 2,
			want:    "schema_version: 1\nid: demo\nversion: 2\ntrigger:\n  type: manual\n",
		},
		{
			name:    "is a no-op when the declared version already matches",
			source:  "schema_version: 1\nid: demo\nversion: 3\ntrigger:\n  type: manual\n",
			version: 3,
			want:    "schema_version: 1\nid: demo\nversion: 3\ntrigger:\n  type: manual\n",
		},
		{
			name:    "preserves a trailing inline comment on the version line",
			source:  "id: demo\nversion: 1  # bump me\ntrigger:\n  type: manual\n",
			version: 5,
			want:    "id: demo\nversion: 5  # bump me\ntrigger:\n  type: manual\n",
		},
		{
			name:    "leaves everything else in the file untouched",
			source:  "# a header comment\nschema_version: 1\nid: demo\nversion: 1\nsteps:\n  - id: step\n    uses: text.uppercase@1\n    with:\n      value: \"hello\"\n",
			version: 10,
			want:    "# a header comment\nschema_version: 1\nid: demo\nversion: 10\nsteps:\n  - id: step\n    uses: text.uppercase@1\n    with:\n      value: \"hello\"\n",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := string(RewriteVersion([]byte(tt.source), tt.version))
			if got != tt.want {
				t.Fatalf("RewriteVersion() = %q, want %q", got, tt.want)
			}
		})
	}
}
