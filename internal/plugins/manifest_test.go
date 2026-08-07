package plugins

import (
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"testing"
)

func validPackageManifestJSON(id, version string) []byte {
	body, _ := json.Marshal(packageManifestJSON{
		SchemaVersion:   1,
		Kind:            "plugin",
		ID:              id,
		Version:         version,
		ProtocolVersion: 1,
		Permissions:     []string{"network.outbound"},
		Executables: map[string]string{
			"darwin-arm64": "binaries/darwin-arm64/plugin",
			"linux-amd64":  "binaries/linux-amd64/plugin",
		},
	})
	return body
}

func TestParsePackageManifest(t *testing.T) {
	t.Run("parses a valid manifest", func(t *testing.T) {
		m, err := ParsePackageManifest(validPackageManifestJSON("io.patchcord.example-text", "1.0.0"))
		if err != nil {
			t.Fatalf("ParsePackageManifest() error = %v", err)
		}
		if m.ID != "io.patchcord.example-text" {
			t.Fatalf("ID = %q, want %q", m.ID, "io.patchcord.example-text")
		}
		if m.Version != "1.0.0" {
			t.Fatalf("Version = %q, want %q", m.Version, "1.0.0")
		}
		if m.ProtocolVersion != 1 {
			t.Fatalf("ProtocolVersion = %d, want 1", m.ProtocolVersion)
		}
		if len(m.Permissions) != 1 || m.Permissions[0] != "network.outbound" {
			t.Fatalf("Permissions = %v, want [network.outbound]", m.Permissions)
		}
		if m.Executables["darwin-arm64"] != "binaries/darwin-arm64/plugin" {
			t.Fatalf("Executables[darwin-arm64] = %q, want %q", m.Executables["darwin-arm64"], "binaries/darwin-arm64/plugin")
		}
	})

	t.Run("rejects malformed JSON", func(t *testing.T) {
		_, err := ParsePackageManifest([]byte("{not json"))
		if !errors.Is(err, ErrInvalidPackageManifest) {
			t.Fatalf("ParsePackageManifest() error = %v, want ErrInvalidPackageManifest", err)
		}
	})

	t.Run("rejects a kind other than plugin", func(t *testing.T) {
		body, _ := json.Marshal(packageManifestJSON{
			Kind: "app", ID: "x", Version: "1.0.0", ProtocolVersion: 1,
			Executables: map[string]string{"darwin-arm64": "binaries/darwin-arm64/plugin"},
		})
		if _, err := ParsePackageManifest(body); !errors.Is(err, ErrInvalidPackageManifest) {
			t.Fatalf("ParsePackageManifest() error = %v, want ErrInvalidPackageManifest", err)
		}
	})

	t.Run("rejects an empty id", func(t *testing.T) {
		body, _ := json.Marshal(packageManifestJSON{
			Kind: "plugin", Version: "1.0.0", ProtocolVersion: 1,
			Executables: map[string]string{"darwin-arm64": "binaries/darwin-arm64/plugin"},
		})
		if _, err := ParsePackageManifest(body); !errors.Is(err, ErrInvalidPackageManifest) {
			t.Fatalf("ParsePackageManifest() error = %v, want ErrInvalidPackageManifest", err)
		}
	})

	t.Run("rejects an empty version", func(t *testing.T) {
		body, _ := json.Marshal(packageManifestJSON{
			Kind: "plugin", ID: "x", ProtocolVersion: 1,
			Executables: map[string]string{"darwin-arm64": "binaries/darwin-arm64/plugin"},
		})
		if _, err := ParsePackageManifest(body); !errors.Is(err, ErrInvalidPackageManifest) {
			t.Fatalf("ParsePackageManifest() error = %v, want ErrInvalidPackageManifest", err)
		}
	})

	t.Run("rejects a zero protocolVersion", func(t *testing.T) {
		body, _ := json.Marshal(packageManifestJSON{
			Kind: "plugin", ID: "x", Version: "1.0.0",
			Executables: map[string]string{"darwin-arm64": "binaries/darwin-arm64/plugin"},
		})
		if _, err := ParsePackageManifest(body); !errors.Is(err, ErrInvalidPackageManifest) {
			t.Fatalf("ParsePackageManifest() error = %v, want ErrInvalidPackageManifest", err)
		}
	})

	t.Run("rejects an empty executables map", func(t *testing.T) {
		body, _ := json.Marshal(packageManifestJSON{
			Kind: "plugin", ID: "x", Version: "1.0.0", ProtocolVersion: 1,
		})
		if _, err := ParsePackageManifest(body); !errors.Is(err, ErrInvalidPackageManifest) {
			t.Fatalf("ParsePackageManifest() error = %v, want ErrInvalidPackageManifest", err)
		}
	})

	t.Run("rejects an empty executable path", func(t *testing.T) {
		body, _ := json.Marshal(packageManifestJSON{
			Kind: "plugin", ID: "x", Version: "1.0.0", ProtocolVersion: 1,
			Executables: map[string]string{"darwin-arm64": ""},
		})
		if _, err := ParsePackageManifest(body); !errors.Is(err, ErrInvalidPackageManifest) {
			t.Fatalf("ParsePackageManifest() error = %v, want ErrInvalidPackageManifest", err)
		}
	})

	t.Run("accepts an empty permissions list", func(t *testing.T) {
		body, _ := json.Marshal(packageManifestJSON{
			Kind: "plugin", ID: "x", Version: "1.0.0", ProtocolVersion: 1,
			Executables: map[string]string{"darwin-arm64": "binaries/darwin-arm64/plugin"},
		})
		m, err := ParsePackageManifest(body)
		if err != nil {
			t.Fatalf("ParsePackageManifest() error = %v", err)
		}
		if len(m.Permissions) != 0 {
			t.Fatalf("Permissions = %v, want empty", m.Permissions)
		}
	})

	t.Run("accepts a parameterized secrets.read permission", func(t *testing.T) {
		body, _ := json.Marshal(packageManifestJSON{
			Kind: "plugin", ID: "x", Version: "1.0.0", ProtocolVersion: 1,
			Permissions: []string{"secrets.read:postgresql"},
			Executables: map[string]string{"darwin-arm64": "binaries/darwin-arm64/plugin"},
		})
		if _, err := ParsePackageManifest(body); err != nil {
			t.Fatalf("ParsePackageManifest() error = %v", err)
		}
	})

	t.Run("rejects an unrecognized permission scope", func(t *testing.T) {
		body, _ := json.Marshal(packageManifestJSON{
			Kind: "plugin", ID: "x", Version: "1.0.0", ProtocolVersion: 1,
			Permissions: []string{"filesystem.read"},
			Executables: map[string]string{"darwin-arm64": "binaries/darwin-arm64/plugin"},
		})
		if _, err := ParsePackageManifest(body); !errors.Is(err, ErrInvalidPackageManifest) {
			t.Fatalf("ParsePackageManifest() error = %v, want ErrInvalidPackageManifest", err)
		}
	})

	t.Run("rejects a malformed parameterized permission", func(t *testing.T) {
		body, _ := json.Marshal(packageManifestJSON{
			Kind: "plugin", ID: "x", Version: "1.0.0", ProtocolVersion: 1,
			Permissions: []string{"secrets.read:"},
			Executables: map[string]string{"darwin-arm64": "binaries/darwin-arm64/plugin"},
		})
		if _, err := ParsePackageManifest(body); !errors.Is(err, ErrInvalidPackageManifest) {
			t.Fatalf("ParsePackageManifest() error = %v, want ErrInvalidPackageManifest", err)
		}
	})
}

func TestLoadPackageManifest(t *testing.T) {
	t.Run("reads and parses manifest.json from a directory", func(t *testing.T) {
		dir := t.TempDir()
		if err := os.WriteFile(filepath.Join(dir, PackageManifestFileName), validPackageManifestJSON("io.patchcord.example-text", "1.0.0"), 0o644); err != nil {
			t.Fatalf("write manifest.json: %v", err)
		}

		m, err := LoadPackageManifest(dir)
		if err != nil {
			t.Fatalf("LoadPackageManifest() error = %v", err)
		}
		if m.ID != "io.patchcord.example-text" {
			t.Fatalf("ID = %q, want %q", m.ID, "io.patchcord.example-text")
		}
	})

	t.Run("fails when the manifest file is missing", func(t *testing.T) {
		if _, err := LoadPackageManifest(t.TempDir()); err == nil {
			t.Fatal("expected an error for a missing manifest, got nil")
		}
	})
}
