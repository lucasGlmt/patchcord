// Package agent holds the agent's public HTTP API contract: the OpenAPI
// (Swagger 2.0) specification generated from internal/api's swag
// annotations. Never hand-edit swagger.json/swagger.yaml — change the
// annotations on the handlers in internal/api and run `make swagger`
// instead, exactly like api/plugin/v1's generated stubs are regenerated
// with `make proto` rather than patched by hand.
package agent

import _ "embed"

// Spec holds the generated OpenAPI document as JSON, served by the agent
// itself at GET /v1/openapi.json (internal/api/router.go).
//
//go:embed swagger.json
var Spec []byte
