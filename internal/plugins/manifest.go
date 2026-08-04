package plugins

import (
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
)

// PackageManifestFileName is the file a plugin package's source directory
// must contain at its root (vision document, section 9.1).
const PackageManifestFileName = "manifest.json"

// packageManifestKind is the only value PackageManifest.Kind accepts —
// distinguishes a plugin package manifest from other package kinds
// (.patchcord-app's patchcord-app.yaml, .patchcord-bundle's bundle.yaml)
// even though none of them share a file name today.
const packageManifestKind = "plugin"

// ErrInvalidPackageManifest is returned by ParsePackageManifest and
// LoadPackageManifest when the manifest is malformed or missing a required
// field.
var ErrInvalidPackageManifest = errors.New("invalid plugin package manifest")

// PackageManifest is the parsed content of a .patchcord-plugin package's
// manifest.json — declared statically, before the plugin process is ever
// launched, so its id, version and permissions can be shown to the user
// (vision document, section 9.2, step 5) and the right platform executable
// can be selected (step 7). It is distinct from Manifest (handshake.go),
// which a running plugin process returns over RPC once launched and
// remains the source of truth for the actions/connectors it actually
// contributes.
type PackageManifest struct {
	SchemaVersion   int
	ID              string
	Version         string
	ProtocolVersion uint32
	Permissions     []string
	// Executables maps a "GOOS-GOARCH" platform key (e.g. "darwin-arm64",
	// matching runtime.GOOS+"-"+runtime.GOARCH) to the executable's path
	// relative to the package root.
	Executables map[string]string
}

// packageManifestJSON mirrors manifest.json's on-disk shape exactly, kept
// separate from PackageManifest so callers work with typed, validated
// fields rather than the raw JSON tags.
type packageManifestJSON struct {
	SchemaVersion   int               `json:"schemaVersion"`
	Kind            string            `json:"kind"`
	ID              string            `json:"id"`
	Version         string            `json:"version"`
	ProtocolVersion uint32            `json:"protocolVersion"`
	Permissions     []string          `json:"permissions"`
	Executables     map[string]string `json:"executables"`
}

// ParsePackageManifest parses and validates a plugin package manifest from
// its JSON source, returning ErrInvalidPackageManifest if a required field
// is missing, empty, or malformed.
func ParsePackageManifest(source []byte) (*PackageManifest, error) {
	var raw packageManifestJSON
	if err := json.Unmarshal(source, &raw); err != nil {
		return nil, fmt.Errorf("%w: %s", ErrInvalidPackageManifest, err)
	}
	if raw.Kind != packageManifestKind {
		return nil, fmt.Errorf("%w: kind must be %q, got %q", ErrInvalidPackageManifest, packageManifestKind, raw.Kind)
	}
	if raw.ID == "" {
		return nil, fmt.Errorf("%w: id must not be empty", ErrInvalidPackageManifest)
	}
	if raw.Version == "" {
		return nil, fmt.Errorf("%w: version must not be empty", ErrInvalidPackageManifest)
	}
	if raw.ProtocolVersion == 0 {
		return nil, fmt.Errorf("%w: protocolVersion must be greater than zero", ErrInvalidPackageManifest)
	}
	if len(raw.Executables) == 0 {
		return nil, fmt.Errorf("%w: executables must declare at least one platform", ErrInvalidPackageManifest)
	}
	for platform, path := range raw.Executables {
		if platform == "" {
			return nil, fmt.Errorf("%w: executables has an empty platform key", ErrInvalidPackageManifest)
		}
		if path == "" {
			return nil, fmt.Errorf("%w: executables[%q] must not be empty", ErrInvalidPackageManifest, platform)
		}
	}

	return &PackageManifest{
		SchemaVersion:   raw.SchemaVersion,
		ID:              raw.ID,
		Version:         raw.Version,
		ProtocolVersion: raw.ProtocolVersion,
		Permissions:     raw.Permissions,
		Executables:     raw.Executables,
	}, nil
}

// LoadPackageManifest reads and parses dir's manifest.json.
func LoadPackageManifest(dir string) (*PackageManifest, error) {
	path := filepath.Join(dir, PackageManifestFileName)
	source, err := os.ReadFile(path)
	if err != nil {
		return nil, fmt.Errorf("read %s: %w", path, err)
	}
	return ParsePackageManifest(source)
}
