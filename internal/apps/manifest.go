// Package apps manages installed applications: web frontends built on top
// of the agent's public API (vision document, section 7.6). An application
// never receives the agent's full privileges — it declares the permissions
// it needs in a manifest, and receives a session limited to exactly those
// (vision document, section 15.4; internal/auth issues and checks such
// sessions).
//
// This is the minimal first slice (ADR-0026): the manifest only covers
// workflows.run. connectors.use and capabilities from the vision document
// are deliberately not modeled yet — there is no enforcement point for
// them, the same situation plugin permissions are in today
// (internal/plugins.CatalogEntry.Permissions is recorded but unchecked).
package apps

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"

	"gopkg.in/yaml.v3"
)

// ManifestFileName is the file an application's source/static directory
// must contain at its root (vision document, section 10.3).
const ManifestFileName = "patchcord-app.yaml"

// ErrInvalidManifest is returned by ParseManifest and LoadManifest when the
// manifest is malformed or missing a required field.
var ErrInvalidManifest = errors.New("invalid app manifest")

// AppPermissions is the permission set an application's sessions are
// limited to (api/app/v1/manifest.schema.json). WorkflowsRun is the only
// kind enforced today — see the package doc comment.
type AppPermissions struct {
	WorkflowsRun []string `json:"workflows_run"`
}

// Manifest is the parsed content of an application's patchcord-app.yaml.
type Manifest struct {
	ID          string
	Version     string
	Permissions AppPermissions
}

// manifestYAML mirrors patchcord-app.yaml's nested shape exactly, as fixed
// by api/app/v1/manifest.schema.json (permissions.workflows.run) — kept
// separate from Manifest so callers work with a flat, convenient
// AppPermissions rather than the nesting the public YAML contract commits
// to.
type manifestYAML struct {
	ID          string `yaml:"id"`
	Version     string `yaml:"version"`
	Permissions struct {
		Workflows struct {
			Run []string `yaml:"run"`
		} `yaml:"workflows"`
	} `yaml:"permissions"`
}

// ParseManifest parses and validates an application manifest from its YAML
// source, returning ErrInvalidManifest if a required field is missing or
// empty.
func ParseManifest(source []byte) (*Manifest, error) {
	var raw manifestYAML
	if err := yaml.Unmarshal(source, &raw); err != nil {
		return nil, fmt.Errorf("%w: %s", ErrInvalidManifest, err)
	}
	if raw.ID == "" {
		return nil, fmt.Errorf("%w: id must not be empty", ErrInvalidManifest)
	}
	if raw.Version == "" {
		return nil, fmt.Errorf("%w: version must not be empty", ErrInvalidManifest)
	}
	for i, workflowID := range raw.Permissions.Workflows.Run {
		if workflowID == "" {
			return nil, fmt.Errorf("%w: permissions.workflows.run[%d] must not be empty", ErrInvalidManifest, i)
		}
	}

	return &Manifest{
		ID:      raw.ID,
		Version: raw.Version,
		Permissions: AppPermissions{
			WorkflowsRun: raw.Permissions.Workflows.Run,
		},
	}, nil
}

// LoadManifest reads and parses dir's patchcord-app.yaml.
func LoadManifest(dir string) (*Manifest, error) {
	path := filepath.Join(dir, ManifestFileName)
	source, err := os.ReadFile(path)
	if err != nil {
		return nil, fmt.Errorf("read %s: %w", path, err)
	}
	return ParseManifest(source)
}
