package plugins

import (
	"context"
	"errors"
	"testing"

	"google.golang.org/protobuf/types/known/structpb"

	pluginv1 "github.com/lucasglmt/patchcord/api/plugin/v1"
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

			got, err := ExecuteAction(context.Background(), client, "text.uppercase@1", map[string]any{"value": "hello"})

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
