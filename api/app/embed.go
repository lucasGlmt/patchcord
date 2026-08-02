// Package app holds the public contract for the application package
// format: the JSON Schema of patchcord-app.yaml (vision doc §7.6, §9.3,
// §15.4). Never hand-edit v1/manifest.schema.json's meaning without
// bumping to a v2 directory — see ADR-0026 for what v1 deliberately
// leaves out (connectors.use, capabilities) and why.
package app

import _ "embed"

// ManifestSchemaV1 holds the v1 JSON Schema for patchcord-app.yaml,
// documenting the contract that internal/apps.LoadManifest implements by
// hand (no runtime JSON Schema validation yet — same choice already made
// for connector config, see internal/connectors).
//
//go:embed v1/manifest.schema.json
var ManifestSchemaV1 []byte
