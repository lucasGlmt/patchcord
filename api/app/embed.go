// Package app holds the public contract for the application package
// format: the JSON Schema of patchcord-app.yaml (vision doc §7.6, §9.3,
// §15.4). Never hand-edit a schema file's meaning in place without bumping
// to a new version directory — v1 is kept byte-for-byte as the historical
// record of what shipped (see ADR-0026 for what it deliberately left out,
// and ADR-0071 for why v2 adds connectors.use). v2 is current; capabilities
// from the vision document is still not part of either — there is still no
// enforcement point for it.
package app

import _ "embed"

// ManifestSchemaV1 holds the v1 JSON Schema for patchcord-app.yaml
// (workflows.run only). Superseded by ManifestSchemaV2 — kept only as the
// historical record of what v1 shipped.
//
//go:embed v1/manifest.schema.json
var ManifestSchemaV1 []byte

// ManifestSchemaV2 holds the current JSON Schema for patchcord-app.yaml
// (workflows.run, connectors.use — ADR-0071), documenting the contract
// that internal/apps.ParseManifest implements by hand (no runtime JSON
// Schema validation yet — same choice already made for connector config,
// see internal/connectors).
//
//go:embed v2/manifest.schema.json
var ManifestSchemaV2 []byte
