package plugins

import (
	"context"
	"fmt"

	"google.golang.org/protobuf/types/known/structpb"

	pluginv1 "github.com/lucasglmt/patchcord/api/plugin/v1"
	"github.com/lucasglmt/patchcord/internal/connectors"
)

// ExecuteAction calls the plugin's ExecuteAction RPC for the given action
// id, passing input as its arguments and connector as the resolved
// connector bound to it (nil if none), and returns its output.
func ExecuteAction(ctx context.Context, client pluginv1.PluginServiceClient, action string, input map[string]any, connector *connectors.ResolvedConnector) (map[string]any, error) {
	inputStruct, err := structpb.NewStruct(input)
	if err != nil {
		return nil, fmt.Errorf("encode action %q input: %w", action, err)
	}

	connectorConfig, err := connectorConfigToProto(connector)
	if err != nil {
		return nil, fmt.Errorf("encode action %q connector: %w", action, err)
	}

	resp, err := client.ExecuteAction(ctx, &pluginv1.ExecuteActionRequest{
		Action:    action,
		Input:     inputStruct,
		Connector: connectorConfig,
	})
	if err != nil {
		return nil, fmt.Errorf("call execute action %q: %w", action, err)
	}

	return resp.GetOutput().AsMap(), nil
}

// connectorConfigToProto encodes a resolved connector for the wire, or
// returns nil unchanged when the calling step bound no connector.
func connectorConfigToProto(rc *connectors.ResolvedConnector) (*pluginv1.ConnectorConfig, error) {
	if rc == nil {
		return nil, nil
	}

	configStruct, err := structpb.NewStruct(rc.Config)
	if err != nil {
		return nil, fmt.Errorf("encode connector config: %w", err)
	}
	secretsStruct, err := structpb.NewStruct(rc.Secrets)
	if err != nil {
		return nil, fmt.Errorf("encode connector secrets: %w", err)
	}

	return &pluginv1.ConnectorConfig{
		Type:    rc.Type,
		Config:  configStruct,
		Secrets: secretsStruct,
	}, nil
}
