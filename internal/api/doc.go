// General API metadata swag (`make swagger`) reads to generate the OpenAPI
// spec committed at api/agent/openapi.json — see that package's doc
// comment. Never hand-edit the generated spec; change the annotations on
// the handlers below (or here) and regenerate instead.
//
// @title           Patchcord Agent API
// @version         1
// @description     Public HTTP API for the Patchcord agent (vision document, section 10.1). Covers workflow triggering and run observability, connector CRUD and testing, application hosting, and a read-only plugin catalog listing; the rest of the vision document's API surface (actions, full plugin management) is not implemented yet.
// @description     Every route marked with a lock icon below requires "Authorization: Bearer <admin token>" — but only once at least one admin token has been created (`patchcord auth token create`); a fresh agent answers every request unauthenticated, exactly as before this existed (ADR-0036).
// @BasePath        /v1
// @securityDefinitions.apikey  BearerAuth
// @in                          header
// @name                        Authorization
// @description                 Admin token, if any has been created (see the top-level description). Pass as "Bearer <token>".
package api
