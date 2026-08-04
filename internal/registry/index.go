package registry

import (
	"encoding/json"
	"fmt"
	"strings"
)

// IndexFileName is the file every registry (local directory or http(s)
// location) must serve at its root.
const IndexFileName = "index.json"

// knownKinds are the only values IndexEntry.Kind may take — the package
// vocabulary fixed by CLAUDE.md §3 and vision document §9.3. An index
// declaring anything else is a hard decode error: this vocabulary must
// never silently drift.
var knownKinds = map[string]struct{}{
	"plugin":   {},
	"app":      {},
	"workflow": {},
	"bundle":   {},
}

// Index is a registry's index.json: every package it serves, by id.
type Index struct {
	SchemaVersion int                   `json:"schemaVersion"`
	Packages      map[string]IndexEntry `json:"packages"`
}

// IndexEntry is one package's entry in a registry index. Versions maps a
// version string to the package file's path, relative to the registry's
// own location. Latest names the version Resolve picks when the caller
// does not pin one — an explicit declaration by the index's author, never
// inferred by comparing version strings (nothing in this codebase parses
// or orders versions numerically; see bundles.splitPluginDependency's
// exact-string-equality dependency check for the same choice).
type IndexEntry struct {
	Kind     string            `json:"kind"`
	Latest   string            `json:"latest"`
	Versions map[string]string `json:"versions"`
}

// decodeIndex parses and validates an index.json's raw bytes.
func decodeIndex(data []byte) (Index, error) {
	var idx Index
	if err := json.Unmarshal(data, &idx); err != nil {
		return Index{}, fmt.Errorf("decode %s: %w", IndexFileName, err)
	}

	for id, entry := range idx.Packages {
		if _, ok := knownKinds[entry.Kind]; !ok {
			return Index{}, fmt.Errorf("decode %s: package %q has unknown kind %q", IndexFileName, id, entry.Kind)
		}
	}

	return idx, nil
}

// ParseRef splits ref into an id and an optional version: "id@version" or
// a bare "id". An empty version means "resolve to the registry's declared
// latest" — unlike bundles.splitPluginDependency's requires_plugins
// dependencies (which must always pin an exact version), a bare id is a
// valid, meaningful reference here.
func ParseRef(ref string) (id, version string) {
	id, version, _ = strings.Cut(ref, "@")
	return id, version
}
