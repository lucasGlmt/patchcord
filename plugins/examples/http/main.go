// Command http is the reference example plugin demonstrating a real
// connector-consuming action (vision document section 7.4's http.request
// example): it contributes one connector type, "http.connection@1", and
// one action, "http.request@1", which requires that connector to be bound.
// It proves a plugin can use a connector's resolved configuration and
// secrets (ADR-0021) to talk to a real external system, without the core
// ever knowing what "HTTP" or a base URL is (non-negotiable #3).
//
// It depends only on the SDK (sdk/go-plugin), never on any internal/
// package of the agent.
package main

import (
	"context"
	"fmt"
	"io"
	"net/http"
	"os"
	"strings"

	patchcord "github.com/lucasglmt/patchcord/sdk/go-plugin"
)

// maxResponseBodyBytes caps how much of a response body Run reads into
// memory — basic hygiene against an unexpectedly large response, not a
// security boundary: the target is a base_url the user configured
// themselves via `connector create`, not untrusted input.
const maxResponseBodyBytes = 10 << 20 // 10 MiB

var httpClient = &http.Client{}

// schema and stringProp are tiny local helpers to keep the JSON Schema
// literals below readable — every example plugin in this directory repeats
// this pattern rather than sharing it, since a plugin depends only on the
// SDK, never on another plugin or an internal/ package (CLAUDE.md §2).
func schema(properties map[string]any, required ...string) patchcord.Schema {
	s := patchcord.Schema{
		"type":       "object",
		"properties": properties,
	}
	if len(required) > 0 {
		req := make([]any, len(required))
		for i, r := range required {
			req[i] = r
		}
		s["required"] = req
	}
	return s
}

func stringProp(description string) map[string]any {
	return map[string]any{"type": "string", "description": description}
}

type httpRequestAction struct{}

func (httpRequestAction) ID() string { return "http.request@1" }
func (httpRequestAction) Description() string {
	return "Performs one HTTP request against the bound connector's base_url."
}

func (httpRequestAction) InputSchema() patchcord.Schema {
	return schema(map[string]any{
		"method":  stringProp("The HTTP method to use, e.g. \"GET\", \"POST\". Defaults to \"GET\"."),
		"path":    stringProp("Appended to the connector's base_url, e.g. \"/users/42\"."),
		"body":    stringProp("The request body, sent as-is. Omit for no body."),
		"headers": map[string]any{"type": "object", "description": "Extra request headers, name to string value."},
	})
}

func (httpRequestAction) OutputSchema() patchcord.Schema {
	return schema(map[string]any{
		"status":  map[string]any{"type": "integer", "description": "The response's HTTP status code."},
		"headers": map[string]any{"type": "object", "description": "The response headers, name to string value."},
		"body":    stringProp("The response body."),
	}, "status", "headers", "body")
}

// Run performs one HTTP request against the bound connector's base_url. It
// only returns an error for a request that never completed (bad
// configuration, network failure, ctx cancelled/timed out) — a non-2xx
// response is a legitimate result reported via the "status" output, not a
// Go error, so a workflow can branch on it instead of the run failing.
func (httpRequestAction) Run(ctx context.Context, input patchcord.ActionInput, connector *patchcord.ConnectorConfig) (patchcord.ActionOutput, error) {
	if connector == nil {
		return nil, fmt.Errorf("action %q requires a bound connector", "http.request@1")
	}
	baseURL, ok := connector.Config["base_url"].(string)
	if !ok || baseURL == "" {
		return nil, fmt.Errorf("connector config %q must be a non-empty string", "base_url")
	}

	method, _ := input["method"].(string)
	if method == "" {
		method = http.MethodGet
	}

	url := baseURL
	if path, _ := input["path"].(string); path != "" {
		url = strings.TrimRight(baseURL, "/") + "/" + strings.TrimLeft(path, "/")
	}

	var bodyReader io.Reader
	if body, _ := input["body"].(string); body != "" {
		bodyReader = strings.NewReader(body)
	}

	req, err := http.NewRequestWithContext(ctx, method, url, bodyReader)
	if err != nil {
		return nil, fmt.Errorf("build request: %w", err)
	}

	if headers, ok := input["headers"].(map[string]any); ok {
		for key, value := range headers {
			if s, ok := value.(string); ok {
				req.Header.Set(key, s)
			}
		}
	}
	if token, ok := connector.Secrets["authorization"].(string); ok && token != "" {
		req.Header.Set("Authorization", token)
	}

	resp, err := httpClient.Do(req)
	if err != nil {
		return nil, fmt.Errorf("perform request: %w", err)
	}
	defer resp.Body.Close()

	respBody, err := io.ReadAll(io.LimitReader(resp.Body, maxResponseBodyBytes))
	if err != nil {
		return nil, fmt.Errorf("read response body: %w", err)
	}

	respHeaders := make(map[string]any, len(resp.Header))
	for key := range resp.Header {
		respHeaders[key] = resp.Header.Get(key)
	}

	return patchcord.ActionOutput{
		"status":  resp.StatusCode,
		"headers": respHeaders,
		"body":    string(respBody),
	}, nil
}

func main() {
	plugin := patchcord.Plugin{
		Manifest: patchcord.Manifest{
			ID:      "io.patchcord.example-http",
			Version: "1.0.0",
		},
		Actions: []patchcord.Action{httpRequestAction{}},
		Connectors: []patchcord.Connector{
			{
				Type:        "http.connection@1",
				Description: "An HTTP base URL, with an optional bearer/authorization secret.",
				ConfigSchema: schema(map[string]any{
					"base_url": stringProp("The base URL every request is made against, e.g. \"https://api.example.com\"."),
				}, "base_url"),
			},
		},
		Permissions: []patchcord.Permission{patchcord.PermissionNetworkOutbound},
	}

	if err := patchcord.Serve(plugin); err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
}
