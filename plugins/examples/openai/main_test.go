package main

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	patchcord "github.com/lucasglmt/patchcord/sdk/go-plugin"
)

func TestResolveBaseURL(t *testing.T) {
	tests := []struct {
		name   string
		config map[string]any
		want   string
	}{
		{
			name:   "defaults when base_url is absent",
			config: map[string]any{},
			want:   defaultBaseURL,
		},
		{
			name:   "defaults when base_url is empty",
			config: map[string]any{"base_url": ""},
			want:   defaultBaseURL,
		},
		{
			name:   "defaults when base_url is not a string",
			config: map[string]any{"base_url": 42},
			want:   defaultBaseURL,
		},
		{
			name:   "uses the configured override",
			config: map[string]any{"base_url": "https://my-proxy.internal/v1"},
			want:   "https://my-proxy.internal/v1",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := resolveBaseURL(tt.config); got != tt.want {
				t.Fatalf("resolveBaseURL() = %q, want %q", got, tt.want)
			}
		})
	}
}

func TestGenerateTextAction_Run(t *testing.T) {
	var lastRequest *http.Request
	var lastBody chatCompletionRequest

	successServer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		lastRequest = r
		_ = json.NewDecoder(r.Body).Decode(&lastBody)

		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(chatCompletionResponse{
			Choices: []struct {
				Message      chatMessage `json:"message"`
				FinishReason string      `json:"finish_reason"`
			}{
				{Message: chatMessage{Role: "assistant", Content: "hi there"}, FinishReason: "stop"},
			},
			Usage: struct {
				PromptTokens     int `json:"prompt_tokens"`
				CompletionTokens int `json:"completion_tokens"`
				TotalTokens      int `json:"total_tokens"`
			}{PromptTokens: 3, CompletionTokens: 2, TotalTokens: 5},
		})
	}))
	defer successServer.Close()

	errorServer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusUnauthorized)
		_, _ = w.Write([]byte(`{"error":{"message":"Invalid API key"}}`))
	}))
	defer errorServer.Close()

	validConnector := &patchcord.ConnectorConfig{
		Type:    "openai.connection@1",
		Config:  map[string]any{"base_url": successServer.URL},
		Secrets: map[string]any{"api_key": "sk-test"},
	}

	tests := []struct {
		name      string
		input     patchcord.ActionInput
		connector *patchcord.ConnectorConfig
		wantErr   bool
		check     func(t *testing.T, output patchcord.ActionOutput)
	}{
		{
			name:      "requires a bound connector",
			input:     patchcord.ActionInput{"model": "gpt-4o-mini", "prompt": "hi"},
			connector: nil,
			wantErr:   true,
		},
		{
			name:  "requires an api_key secret",
			input: patchcord.ActionInput{"model": "gpt-4o-mini", "prompt": "hi"},
			connector: &patchcord.ConnectorConfig{
				Config: map[string]any{"base_url": successServer.URL},
			},
			wantErr: true,
		},
		{
			name:      "requires a model input",
			input:     patchcord.ActionInput{"prompt": "hi"},
			connector: validConnector,
			wantErr:   true,
		},
		{
			name:      "requires a prompt input",
			input:     patchcord.ActionInput{"model": "gpt-4o-mini"},
			connector: validConnector,
			wantErr:   true,
		},
		{
			name: "sends the request and decodes a successful response",
			input: patchcord.ActionInput{
				"model":  "gpt-4o-mini",
				"prompt": "hello",
				"system": "be terse",
			},
			connector: validConnector,
			check: func(t *testing.T, output patchcord.ActionOutput) {
				if output["text"] != "hi there" {
					t.Fatalf("text = %v, want %q", output["text"], "hi there")
				}
				if output["finish_reason"] != "stop" {
					t.Fatalf("finish_reason = %v, want %q", output["finish_reason"], "stop")
				}
				usage, ok := output["usage"].(map[string]any)
				if !ok || usage["total_tokens"] != 5 {
					t.Fatalf("usage = %v, want total_tokens=5", output["usage"])
				}

				if got := lastRequest.Header.Get("Authorization"); got != "Bearer sk-test" {
					t.Fatalf("Authorization header = %q, want %q", got, "Bearer sk-test")
				}
				if lastBody.Model != "gpt-4o-mini" {
					t.Fatalf("request model = %q, want %q", lastBody.Model, "gpt-4o-mini")
				}
				if len(lastBody.Messages) != 2 || lastBody.Messages[0].Role != "system" || lastBody.Messages[1].Role != "user" {
					t.Fatalf("request messages = %+v, want [system, user]", lastBody.Messages)
				}
			},
		},
		{
			name: "converts max_tokens from the float64 protobuf number to an int",
			input: patchcord.ActionInput{
				"model":      "gpt-4o-mini",
				"prompt":     "hello",
				"max_tokens": float64(128),
			},
			connector: validConnector,
			check: func(t *testing.T, _ patchcord.ActionOutput) {
				if lastBody.MaxTokens == nil || *lastBody.MaxTokens != 128 {
					t.Fatalf("request max_tokens = %v, want 128", lastBody.MaxTokens)
				}
			},
		},
		{
			name:  "turns a non-2xx response into a Go error",
			input: patchcord.ActionInput{"model": "gpt-4o-mini", "prompt": "hi"},
			connector: &patchcord.ConnectorConfig{
				Config:  map[string]any{"base_url": errorServer.URL},
				Secrets: map[string]any{"api_key": "sk-test"},
			},
			wantErr: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			output, err := generateTextAction{}.Run(context.Background(), tt.input, tt.connector)

			if tt.wantErr {
				if err == nil {
					t.Fatal("expected an error, got nil")
				}
				return
			}
			if err != nil {
				t.Fatalf("Run() error = %v", err)
			}
			if tt.check != nil {
				tt.check(t, output)
			}
		})
	}
}

func TestGenerateTextAction_Run_ErrorMessageFromBody(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusUnauthorized)
		_, _ = w.Write([]byte(`{"error":{"message":"Invalid API key"}}`))
	}))
	defer server.Close()

	connector := &patchcord.ConnectorConfig{
		Config:  map[string]any{"base_url": server.URL},
		Secrets: map[string]any{"api_key": "sk-test"},
	}

	_, err := generateTextAction{}.Run(context.Background(), patchcord.ActionInput{
		"model": "gpt-4o-mini", "prompt": "hi",
	}, connector)
	if err == nil {
		t.Fatal("expected an error, got nil")
	}
	if got := err.Error(); !strings.Contains(got, "Invalid API key") {
		t.Fatalf("error = %q, want it to contain the OpenAI error message", got)
	}
}

func TestGenerateTextAction_Run_RespectsContextCancellation(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		select {
		case <-time.After(500 * time.Millisecond):
		case <-r.Context().Done():
		}
	}))
	defer server.Close()

	connector := &patchcord.ConnectorConfig{
		Config:  map[string]any{"base_url": server.URL},
		Secrets: map[string]any{"api_key": "sk-test"},
	}

	ctx, cancel := context.WithTimeout(context.Background(), 20*time.Millisecond)
	defer cancel()

	_, err := generateTextAction{}.Run(ctx, patchcord.ActionInput{"model": "gpt-4o-mini", "prompt": "hi"}, connector)
	if err == nil {
		t.Fatal("expected an error when ctx times out before the response, got nil")
	}
}
