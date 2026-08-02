// Command openai is the second reference example plugin demonstrating a
// real connector-consuming action, this time targeting the OpenAI Chat
// Completions API specifically: it contributes one connector type,
// "openai.connection@1", and one action, "ai.generate_text@1" (the action
// id is deliberately generic — see the vision document's own example list,
// section 7.4 — so a future provider plugin could contribute the same
// action bound to a different connector, without workflows changing).
//
// Unlike plugins/examples/http's http.request@1, which is a generic,
// untyped passthrough that never turns a non-2xx response into a Go error
// (it's a legitimate result the workflow can branch on), this action knows
// the exact shape of what it's calling: a non-2xx response *is* a Go
// error here, since there is no meaningful "text" to hand back on failure.
// That matches the vision document's own framing (section 16): "L'IA doit
// rester encapsulée derrière des actions typées et auditables."
//
// It depends only on the SDK (sdk/go-plugin), never on any internal/
// package of the agent.
package main

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"os"
	"strings"

	patchcord "github.com/lucasglmt/patchcord/sdk/go-plugin"
)

// defaultBaseURL is used when the connector's config omits base_url —
// overriding it is how this plugin also reaches Azure OpenAI or another
// OpenAI-compatible proxy without any code change.
const defaultBaseURL = "https://api.openai.com/v1"

// maxResponseBodyBytes caps how much of a response body Run reads into
// memory — basic hygiene, not a security boundary (see
// plugins/examples/http for the same reasoning).
const maxResponseBodyBytes = 10 << 20 // 10 MiB

var httpClient = &http.Client{}

type chatMessage struct {
	Role    string `json:"role"`
	Content string `json:"content"`
}

type chatCompletionRequest struct {
	Model       string        `json:"model"`
	Messages    []chatMessage `json:"messages"`
	Temperature *float64      `json:"temperature,omitempty"`
	MaxTokens   *int          `json:"max_tokens,omitempty"`
}

type chatCompletionResponse struct {
	Choices []struct {
		Message      chatMessage `json:"message"`
		FinishReason string      `json:"finish_reason"`
	} `json:"choices"`
	Usage struct {
		PromptTokens     int `json:"prompt_tokens"`
		CompletionTokens int `json:"completion_tokens"`
		TotalTokens      int `json:"total_tokens"`
	} `json:"usage"`
}

type openAIErrorResponse struct {
	Error struct {
		Message string `json:"message"`
	} `json:"error"`
}

// resolveBaseURL returns config's base_url, or defaultBaseURL if it is
// absent, empty, or not a string. Kept pure (no I/O) so the "no override"
// path can be tested without ever calling the real OpenAI API.
func resolveBaseURL(config map[string]any) string {
	if baseURL, ok := config["base_url"].(string); ok && baseURL != "" {
		return baseURL
	}
	return defaultBaseURL
}

type generateTextAction struct{}

func (generateTextAction) ID() string { return "ai.generate_text@1" }

func (generateTextAction) Run(ctx context.Context, input patchcord.ActionInput, connector *patchcord.ConnectorConfig) (patchcord.ActionOutput, error) {
	if connector == nil {
		return nil, fmt.Errorf("action %q requires a bound connector", "ai.generate_text@1")
	}
	apiKey, ok := connector.Secrets["api_key"].(string)
	if !ok || apiKey == "" {
		return nil, fmt.Errorf("connector secret %q must be a non-empty string", "api_key")
	}

	model, ok := input["model"].(string)
	if !ok || model == "" {
		return nil, fmt.Errorf("input %q must be a non-empty string", "model")
	}
	prompt, ok := input["prompt"].(string)
	if !ok || prompt == "" {
		return nil, fmt.Errorf("input %q must be a non-empty string", "prompt")
	}

	var messages []chatMessage
	if system, _ := input["system"].(string); system != "" {
		messages = append(messages, chatMessage{Role: "system", Content: system})
	}
	messages = append(messages, chatMessage{Role: "user", Content: prompt})

	reqBody := chatCompletionRequest{Model: model, Messages: messages}
	if temperature, ok := input["temperature"].(float64); ok {
		reqBody.Temperature = &temperature
	}
	if maxTokens, ok := input["max_tokens"].(float64); ok {
		// Every number crossing the plugin protocol goes through a
		// protobuf Struct, whose only numeric kind is NumberValue
		// (float64) — an integer in the workflow's YAML still arrives
		// here as a float64, never as an int.
		n := int(maxTokens)
		reqBody.MaxTokens = &n
	}

	payload, err := json.Marshal(reqBody)
	if err != nil {
		return nil, fmt.Errorf("encode request: %w", err)
	}

	url := strings.TrimRight(resolveBaseURL(connector.Config), "/") + "/chat/completions"
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, url, bytes.NewReader(payload))
	if err != nil {
		return nil, fmt.Errorf("build request: %w", err)
	}
	req.Header.Set("Authorization", "Bearer "+apiKey)
	req.Header.Set("Content-Type", "application/json")

	resp, err := httpClient.Do(req)
	if err != nil {
		return nil, fmt.Errorf("perform request: %w", err)
	}
	defer resp.Body.Close()

	respBody, err := io.ReadAll(io.LimitReader(resp.Body, maxResponseBodyBytes))
	if err != nil {
		return nil, fmt.Errorf("read response body: %w", err)
	}

	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		var errResp openAIErrorResponse
		if json.Unmarshal(respBody, &errResp) == nil && errResp.Error.Message != "" {
			return nil, fmt.Errorf("openai request failed (%d): %s", resp.StatusCode, errResp.Error.Message)
		}
		return nil, fmt.Errorf("openai request failed (%d): %s", resp.StatusCode, string(respBody))
	}

	var completion chatCompletionResponse
	if err := json.Unmarshal(respBody, &completion); err != nil {
		return nil, fmt.Errorf("decode response: %w", err)
	}
	if len(completion.Choices) == 0 {
		return nil, fmt.Errorf("openai response contained no choices")
	}

	return patchcord.ActionOutput{
		"text":          completion.Choices[0].Message.Content,
		"finish_reason": completion.Choices[0].FinishReason,
		"usage": map[string]any{
			"prompt_tokens":     completion.Usage.PromptTokens,
			"completion_tokens": completion.Usage.CompletionTokens,
			"total_tokens":      completion.Usage.TotalTokens,
		},
	}, nil
}

func main() {
	plugin := patchcord.Plugin{
		Manifest: patchcord.Manifest{
			ID:      "io.patchcord.example-openai",
			Version: "1.0.0",
		},
		Actions:     []patchcord.Action{generateTextAction{}},
		Connectors:  []string{"openai.connection@1"},
		Permissions: []string{"network.outbound"},
	}

	if err := patchcord.Serve(plugin); err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
}
