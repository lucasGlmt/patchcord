package bundles

import (
	"errors"
	"os"
	"path/filepath"
	"testing"
)

func TestParseManifest(t *testing.T) {
	t.Run("parses a valid manifest", func(t *testing.T) {
		source := []byte(`
id: io.patchcord.example-bundle
version: "1.0.0"
app: app/
workflows:
  - workflows/main.yaml
requires_plugins:
  - io.patchcord.example-text@1.0.0
`)
		m, err := ParseManifest(source)
		if err != nil {
			t.Fatalf("ParseManifest() error = %v", err)
		}
		if m.ID != "io.patchcord.example-bundle" {
			t.Fatalf("ID = %q, want %q", m.ID, "io.patchcord.example-bundle")
		}
		if m.Version != "1.0.0" {
			t.Fatalf("Version = %q, want %q", m.Version, "1.0.0")
		}
		if m.App != "app/" {
			t.Fatalf("App = %q, want %q", m.App, "app/")
		}
		if len(m.Workflows) != 1 || m.Workflows[0] != "workflows/main.yaml" {
			t.Fatalf("Workflows = %v, want [workflows/main.yaml]", m.Workflows)
		}
		if len(m.RequiresPlugins) != 1 || m.RequiresPlugins[0] != "io.patchcord.example-text@1.0.0" {
			t.Fatalf("RequiresPlugins = %v, want [io.patchcord.example-text@1.0.0]", m.RequiresPlugins)
		}
	})

	t.Run("accepts a bundle with no app and no dependencies", func(t *testing.T) {
		source := []byte(`
id: io.patchcord.example-bundle
version: "1.0.0"
workflows:
  - workflows/main.yaml
`)
		m, err := ParseManifest(source)
		if err != nil {
			t.Fatalf("ParseManifest() error = %v", err)
		}
		if m.App != "" {
			t.Fatalf("App = %q, want empty", m.App)
		}
	})

	t.Run("rejects malformed YAML", func(t *testing.T) {
		_, err := ParseManifest([]byte("id: [this is not"))
		if !errors.Is(err, ErrInvalidManifest) {
			t.Fatalf("ParseManifest() error = %v, want ErrInvalidManifest", err)
		}
	})

	t.Run("rejects an empty id", func(t *testing.T) {
		source := []byte(`version: "1.0.0"`)
		if _, err := ParseManifest(source); !errors.Is(err, ErrInvalidManifest) {
			t.Fatalf("ParseManifest() error = %v, want ErrInvalidManifest", err)
		}
	})

	t.Run("rejects an empty version", func(t *testing.T) {
		source := []byte(`id: io.patchcord.example-bundle`)
		if _, err := ParseManifest(source); !errors.Is(err, ErrInvalidManifest) {
			t.Fatalf("ParseManifest() error = %v, want ErrInvalidManifest", err)
		}
	})

	t.Run("rejects an empty workflows entry", func(t *testing.T) {
		source := []byte(`
id: io.patchcord.example-bundle
version: "1.0.0"
workflows:
  - ""
`)
		if _, err := ParseManifest(source); !errors.Is(err, ErrInvalidManifest) {
			t.Fatalf("ParseManifest() error = %v, want ErrInvalidManifest", err)
		}
	})

	t.Run("rejects a requires_plugins entry with no version pin", func(t *testing.T) {
		source := []byte(`
id: io.patchcord.example-bundle
version: "1.0.0"
requires_plugins:
  - io.patchcord.example-text
`)
		if _, err := ParseManifest(source); !errors.Is(err, ErrInvalidManifest) {
			t.Fatalf("ParseManifest() error = %v, want ErrInvalidManifest", err)
		}
	})
}

func TestLoadManifest(t *testing.T) {
	t.Run("reads and parses bundle.yaml from a directory", func(t *testing.T) {
		dir := t.TempDir()
		content := "id: io.patchcord.example-bundle\nversion: \"1.0.0\"\n"
		if err := os.WriteFile(filepath.Join(dir, ManifestFileName), []byte(content), 0o644); err != nil {
			t.Fatalf("write bundle.yaml: %v", err)
		}

		m, err := LoadManifest(dir)
		if err != nil {
			t.Fatalf("LoadManifest() error = %v", err)
		}
		if m.ID != "io.patchcord.example-bundle" {
			t.Fatalf("ID = %q, want %q", m.ID, "io.patchcord.example-bundle")
		}
	})

	t.Run("fails when the manifest file is missing", func(t *testing.T) {
		if _, err := LoadManifest(t.TempDir()); err == nil {
			t.Fatal("expected an error for a missing manifest, got nil")
		}
	})
}
