package workflow

import "testing"

func TestParse(t *testing.T) {
	tests := []struct {
		name    string
		source  string
		wantErr bool
	}{
		{
			name: "parses the hello_patchcord reference example",
			source: `
schema_version: 1
id: hello_patchcord
version: 1
trigger:
  type: manual
steps:
  - id: transform
    uses: text.uppercase@1
    with:
      value: "Welcome Patchcord"
`,
		},
		{
			name:    "rejects malformed YAML",
			source:  "steps: [this is not valid yaml: [",
			wantErr: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			def, err := Parse([]byte(tt.source))

			if tt.wantErr {
				if err == nil {
					t.Fatal("expected an error, got nil")
				}
				return
			}
			if err != nil {
				t.Fatalf("Parse() error = %v", err)
			}
			if def.ID != "hello_patchcord" {
				t.Fatalf("ID = %q, want %q", def.ID, "hello_patchcord")
			}
			if len(def.Steps) != 1 || def.Steps[0].Uses != "text.uppercase@1" {
				t.Fatalf("Steps = %+v, want one step using text.uppercase@1", def.Steps)
			}
		})
	}
}
