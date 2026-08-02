// General API metadata swag (`make swagger`) reads to generate the OpenAPI
// spec committed at api/agent/openapi.json — see that package's doc
// comment. Never hand-edit the generated spec; change the annotations on
// the handlers below (or here) and regenerate instead.
//
// @title           Patchcord Agent API
// @version         1
// @description     Public HTTP API for the Patchcord agent (vision document, section 10.1). Covers workflow triggering and run observability; the rest of the vision document's API surface (plugins, connectors, actions, apps) is not implemented yet.
// @BasePath        /v1
package api
