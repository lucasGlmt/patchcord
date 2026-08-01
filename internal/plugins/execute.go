package plugins

import (
	"context"
	"fmt"

	"google.golang.org/protobuf/types/known/structpb"

	pluginv1 "github.com/lucasglmt/patchcord/api/plugin/v1"
)

// ExecuteAction calls the plugin's ExecuteAction RPC for the given action
// id, passing input as its arguments, and returns its output.
func ExecuteAction(ctx context.Context, client pluginv1.PluginServiceClient, action string, input map[string]any) (map[string]any, error) {
	inputStruct, err := structpb.NewStruct(input)
	if err != nil {
		return nil, fmt.Errorf("encode action %q input: %w", action, err)
	}

	resp, err := client.ExecuteAction(ctx, &pluginv1.ExecuteActionRequest{
		Action: action,
		Input:  inputStruct,
	})
	if err != nil {
		return nil, fmt.Errorf("call execute action %q: %w", action, err)
	}

	return resp.GetOutput().AsMap(), nil
}
