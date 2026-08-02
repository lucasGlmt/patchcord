package main

import (
	"context"
	"io"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	patchcord "github.com/lucasglmt/patchcord/sdk/go-plugin"
)

func TestHTTPRequestAction_Run(t *testing.T) {
	var lastRequest *http.Request
	var lastBody string

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		lastRequest = r
		body, _ := io.ReadAll(r.Body)
		lastBody = string(body)
		w.Header().Set("X-Test", "1")
		w.WriteHeader(http.StatusTeapot)
		_, _ = w.Write([]byte("ok"))
	}))
	defer server.Close()

	connector := &patchcord.ConnectorConfig{
		Type:    "http.connection@1",
		Config:  map[string]any{"base_url": server.URL},
		Secrets: map[string]any{"authorization": "Bearer s3cr3t"},
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
			input:     patchcord.ActionInput{},
			connector: nil,
			wantErr:   true,
		},
		{
			name:      "requires base_url in the connector config",
			input:     patchcord.ActionInput{},
			connector: &patchcord.ConnectorConfig{Type: "http.connection@1", Config: map[string]any{}},
			wantErr:   true,
		},
		{
			name:      "performs a GET request by default and returns status/headers/body",
			input:     patchcord.ActionInput{},
			connector: connector,
			check: func(t *testing.T, output patchcord.ActionOutput) {
				if output["status"] != http.StatusTeapot {
					t.Fatalf("status = %v, want %d", output["status"], http.StatusTeapot)
				}
				if output["body"] != "ok" {
					t.Fatalf("body = %v, want %q", output["body"], "ok")
				}
				headers, ok := output["headers"].(map[string]any)
				if !ok || headers["X-Test"] != "1" {
					t.Fatalf("headers = %v, want X-Test=1", output["headers"])
				}
				if lastRequest.Method != http.MethodGet {
					t.Fatalf("request method = %s, want GET", lastRequest.Method)
				}
			},
		},
		{
			name:      "appends path to base_url",
			input:     patchcord.ActionInput{"path": "/users/1"},
			connector: connector,
			check: func(t *testing.T, _ patchcord.ActionOutput) {
				if lastRequest.URL.Path != "/users/1" {
					t.Fatalf("request path = %s, want /users/1", lastRequest.URL.Path)
				}
			},
		},
		{
			name: "sends custom headers from input",
			input: patchcord.ActionInput{
				"headers": map[string]any{"X-Custom": "value"},
			},
			connector: connector,
			check: func(t *testing.T, _ patchcord.ActionOutput) {
				if got := lastRequest.Header.Get("X-Custom"); got != "value" {
					t.Fatalf("request header X-Custom = %q, want %q", got, "value")
				}
			},
		},
		{
			name:      "sends the connector's authorization secret as a header",
			input:     patchcord.ActionInput{},
			connector: connector,
			check: func(t *testing.T, _ patchcord.ActionOutput) {
				if got := lastRequest.Header.Get("Authorization"); got != "Bearer s3cr3t" {
					t.Fatalf("request header Authorization = %q, want %q", got, "Bearer s3cr3t")
				}
			},
		},
		{
			name: "sends a request body for POST",
			input: patchcord.ActionInput{
				"method": "POST",
				"body":   `{"hello":"world"}`,
			},
			connector: connector,
			check: func(t *testing.T, _ patchcord.ActionOutput) {
				if lastRequest.Method != http.MethodPost {
					t.Fatalf("request method = %s, want POST", lastRequest.Method)
				}
				if lastBody != `{"hello":"world"}` {
					t.Fatalf("request body = %q, want %q", lastBody, `{"hello":"world"}`)
				}
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			output, err := httpRequestAction{}.Run(context.Background(), tt.input, tt.connector)

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

func TestHTTPRequestAction_Run_RespectsContextCancellation(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		select {
		case <-time.After(500 * time.Millisecond):
		case <-r.Context().Done():
		}
	}))
	defer server.Close()

	connector := &patchcord.ConnectorConfig{Config: map[string]any{"base_url": server.URL}}

	ctx, cancel := context.WithTimeout(context.Background(), 20*time.Millisecond)
	defer cancel()

	_, err := httpRequestAction{}.Run(ctx, patchcord.ActionInput{}, connector)
	if err == nil {
		t.Fatal("expected an error when ctx times out before the response, got nil")
	}
}
