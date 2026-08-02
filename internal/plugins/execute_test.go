package plugins

import (
	"context"
	"errors"
	"testing"

	"google.golang.org/protobuf/types/known/structpb"

	pluginv1 "github.com/lucasglmt/patchcord/api/plugin/v1"
	"github.com/lucasglmt/patchcord/internal/connectors"
)

func TestExecuteAction(t *testing.T) {
	outputStruct, err := structpb.NewStruct(map[string]any{"value": "HELLO"})
	if err != nil {
		t.Fatalf("build output struct: %v", err)
	}

	tests := []struct {
		name       string
		executeErr error
		response   *pluginv1.ExecuteActionResponse
		wantErr    bool
		want       map[string]any
	}{
		{
			name:     "returns the plugin's output",
			response: &pluginv1.ExecuteActionResponse{Output: outputStruct},
			want:     map[string]any{"value": "HELLO"},
		},
		{
			name:       "propagates an RPC error",
			executeErr: errors.New("boom"),
			wantErr:    true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			client := dialFakePlugin(t, &fakePluginServer{
				executeResponse: tt.response,
				executeErr:      tt.executeErr,
			})

			got, err := ExecuteAction(context.Background(), client, "text.uppercase@1", map[string]any{"value": "hello"}, nil)

			if tt.wantErr {
				if err == nil {
					t.Fatal("expected an error, got nil")
				}
				return
			}
			if err != nil {
				t.Fatalf("ExecuteAction() error = %v", err)
			}
			if got["value"] != tt.want["value"] {
				t.Fatalf("output = %v, want %v", got, tt.want)
			}
		})
	}
}

func TestExecuteAction_EncodesABoundConnector(t *testing.T) {
	outputStruct, err := structpb.NewStruct(map[string]any{"bound": true})
	if err != nil {
		t.Fatalf("build output struct: %v", err)
	}
	client := dialFakePlugin(t, &fakePluginServer{
		executeResponse: &pluginv1.ExecuteActionResponse{Output: outputStruct},
	})

	connector := &connectors.ResolvedConnector{
		Type:    "postgresql.connection@1",
		Config:  map[string]any{"host": "db.internal"},
		Secrets: map[string]any{"password": "s3cr3t"},
	}

	got, err := ExecuteAction(context.Background(), client, "postgresql.query@1", map[string]any{"query": "select 1"}, connector)
	if err != nil {
		t.Fatalf("ExecuteAction() error = %v", err)
	}
	if got["bound"] != true {
		t.Fatalf("output = %v, want bound=true", got)
	}
}

func TestTestConnector(t *testing.T) {
	connector := &connectors.ResolvedConnector{
		Type:    "postgresql.connection@1",
		Config:  map[string]any{"host": "db.internal"},
		Secrets: map[string]any{"password": "s3cr3t"},
	}

	t.Run("reports a successful test", func(t *testing.T) {
		client := dialFakePlugin(t, &fakePluginServer{
			testConnectorResponse: &pluginv1.TestConnectorResponse{Ok: true},
		})

		ok, message, err := TestConnector(context.Background(), client, connector)
		if err != nil {
			t.Fatalf("TestConnector() error = %v", err)
		}
		if !ok {
			t.Fatalf("ok = %v, want true", ok)
		}
		if message != "" {
			t.Fatalf("message = %q, want empty", message)
		}
	})

	t.Run("reports a failed test without an error", func(t *testing.T) {
		client := dialFakePlugin(t, &fakePluginServer{
			testConnectorResponse: &pluginv1.TestConnectorResponse{Ok: false, Message: "connection refused"},
		})

		ok, message, err := TestConnector(context.Background(), client, connector)
		if err != nil {
			t.Fatalf("TestConnector() error = %v, want nil (a failed test is a legitimate result)", err)
		}
		if ok {
			t.Fatal("ok = true, want false")
		}
		if message != "connection refused" {
			t.Fatalf("message = %q, want %q", message, "connection refused")
		}
	})

	t.Run("propagates an RPC error", func(t *testing.T) {
		client := dialFakePlugin(t, &fakePluginServer{testConnectorErr: errors.New("boom")})

		_, _, err := TestConnector(context.Background(), client, connector)
		if err == nil {
			t.Fatal("expected an error, got nil")
		}
	})
}
