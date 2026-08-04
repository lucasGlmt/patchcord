// Package bundles installs bundles: packages that group an application, its
// workflows, and its plugin dependencies into one .patchcord-bundle archive
// (vision document, section 9.3). A bundle install delegates to
// internal/apps and internal/runs rather than duplicating their logic —
// this package only orchestrates, in order: checking declared plugin
// dependencies are present, installing the embedded app (if any), and
// installing the embedded workflows.
//
// Embedding connectors ("configuration" in the vision document's wording)
// is not modeled yet: internal/connectors has no file-based export/template
// mechanism today (ADR-0020), so there is nothing a bundle could portably
// carry beyond a non-secret connector's id and type — deferred to a later
// pass, not forgotten.
package bundles

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"gopkg.in/yaml.v3"
)

// ManifestFileName is the file a bundle's source/staging directory must
// contain at its root.
const ManifestFileName = "bundle.yaml"

// ErrInvalidManifest is returned by ParseManifest and LoadManifest when the
// manifest is malformed or missing a required field.
var ErrInvalidManifest = errors.New("invalid bundle manifest")

// Manifest is the parsed content of a bundle's bundle.yaml.
type Manifest struct {
	ID        string
	Version   string
	App       string   // relative path to the embedded app's source directory; empty if the bundle has no app.
	Workflows []string // relative paths to embedded workflow YAML files.
	// RequiresPlugins lists "id@version" plugin dependencies InstallPackage
	// checks are already installed before proceeding — see the package doc
	// comment: v1 validates, it does not auto-install.
	RequiresPlugins []string
}

type manifestYAML struct {
	ID              string   `yaml:"id"`
	Version         string   `yaml:"version"`
	App             string   `yaml:"app"`
	Workflows       []string `yaml:"workflows"`
	RequiresPlugins []string `yaml:"requires_plugins"`
}

// ParseManifest parses and validates a bundle manifest from its YAML
// source, returning ErrInvalidManifest if a required field is missing,
// empty, or malformed.
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
	for i, w := range raw.Workflows {
		if w == "" {
			return nil, fmt.Errorf("%w: workflows[%d] must not be empty", ErrInvalidManifest, i)
		}
	}
	for i, dep := range raw.RequiresPlugins {
		if _, _, err := splitPluginDependency(dep); err != nil {
			return nil, fmt.Errorf("%w: requires_plugins[%d]: %s", ErrInvalidManifest, i, err)
		}
	}

	return &Manifest{
		ID:              raw.ID,
		Version:         raw.Version,
		App:             raw.App,
		Workflows:       raw.Workflows,
		RequiresPlugins: raw.RequiresPlugins,
	}, nil
}

// LoadManifest reads and parses dir's bundle.yaml.
func LoadManifest(dir string) (*Manifest, error) {
	path := filepath.Join(dir, ManifestFileName)
	source, err := os.ReadFile(path)
	if err != nil {
		return nil, fmt.Errorf("read %s: %w", path, err)
	}
	return ParseManifest(source)
}

// splitPluginDependency splits an "id@version" dependency string. version
// must be non-empty: an unpinned dependency ("io.patchcord.example-text",
// no "@") would make what a bundle needs ambiguous at install time.
func splitPluginDependency(dep string) (id, version string, err error) {
	id, version, ok := strings.Cut(dep, "@")
	if !ok || id == "" || version == "" {
		return "", "", fmt.Errorf("%q must be in the form \"id@version\"", dep)
	}
	return id, version, nil
}
