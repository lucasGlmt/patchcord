package registry

import "testing"

func TestParseRef(t *testing.T) {
	tests := []struct {
		ref         string
		wantID      string
		wantVersion string
	}{
		{"io.patchcord.example-bundle", "io.patchcord.example-bundle", ""},
		{"io.patchcord.example-bundle@1.0.0", "io.patchcord.example-bundle", "1.0.0"},
		// strings.Cut splits on the first "@" only — a version string that
		// itself contains "@" (unusual, but not forbidden by any manifest
		// validation in this codebase) is preserved whole in the version
		// half, not further split.
		{"a@b@c", "a", "b@c"},
		{"", "", ""},
	}

	for _, tt := range tests {
		id, version := ParseRef(tt.ref)
		if id != tt.wantID || version != tt.wantVersion {
			t.Errorf("ParseRef(%q) = (%q, %q), want (%q, %q)", tt.ref, id, version, tt.wantID, tt.wantVersion)
		}
	}
}

func TestDecodeIndex(t *testing.T) {
	t.Run("parses a valid index", func(t *testing.T) {
		data := []byte(`{
			"schemaVersion": 1,
			"packages": {
				"io.patchcord.example-bundle": {
					"kind": "bundle",
					"latest": "1.1.0",
					"versions": {
						"1.0.0": "packages/example-bundle-1.0.0.patchcord-bundle",
						"1.1.0": "packages/example-bundle-1.1.0.patchcord-bundle"
					}
				}
			}
		}`)

		idx, err := decodeIndex(data)
		if err != nil {
			t.Fatalf("decodeIndex() error = %v", err)
		}
		entry, ok := idx.Packages["io.patchcord.example-bundle"]
		if !ok {
			t.Fatal("decodeIndex() did not include io.patchcord.example-bundle")
		}
		if entry.Kind != "bundle" || entry.Latest != "1.1.0" {
			t.Fatalf("entry = %+v, want kind=bundle latest=1.1.0", entry)
		}
	})

	t.Run("rejects malformed JSON", func(t *testing.T) {
		if _, err := decodeIndex([]byte("not json")); err == nil {
			t.Fatal("expected an error for malformed JSON, got nil")
		}
	})

	t.Run("rejects an unknown kind", func(t *testing.T) {
		data := []byte(`{"schemaVersion": 1, "packages": {"x": {"kind": "container", "latest": "1.0.0", "versions": {"1.0.0": "x.tar"}}}}`)
		if _, err := decodeIndex(data); err == nil {
			t.Fatal("expected an error for an unknown kind, got nil")
		}
	})
}
