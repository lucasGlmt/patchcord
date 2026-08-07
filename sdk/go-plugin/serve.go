package patchcord

import (
	"context"
	"encoding/json"
	"fmt"
	"net"

	"google.golang.org/grpc"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/health"
	"google.golang.org/grpc/health/grpc_health_v1"
	"google.golang.org/grpc/status"
	"google.golang.org/protobuf/types/known/structpb"

	pluginv1 "github.com/lucasglmt/patchcord/api/plugin/v1"
)

// protocolVersion is the highest plugin protocol version this SDK speaks.
// Bumped to 2 by ADR-0062, which changed Contributions from bare
// identifier lists to full descriptors (description + JSON Schema) — a
// wire-incompatible change, not just an additive one, so a plugin built
// against this SDK cannot speak v1.
const protocolVersion = 2

// Serve starts the plugin's gRPC server on a local TCP port, prints its
// bootstrap ready message on stdout so the agent can discover it, and
// blocks serving requests until the process is terminated.
func Serve(plugin Plugin) error {
	srv, err := newServer(plugin)
	if err != nil {
		return err
	}

	listener, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		return fmt.Errorf("listen: %w", err)
	}

	grpcServer := grpc.NewServer()
	pluginv1.RegisterPluginServiceServer(grpcServer, srv)

	// The agent's Plugin Supervisor polls the standard gRPC health protocol
	// to detect an unresponsive plugin. Report SERVING as soon as the
	// server starts: this plugin has no sub-components whose health could
	// diverge from "process is up and the gRPC server is running".
	healthServer := health.NewServer()
	healthServer.SetServingStatus("", grpc_health_v1.HealthCheckResponse_SERVING)
	grpc_health_v1.RegisterHealthServer(grpcServer, healthServer)

	ready, err := json.Marshal(struct {
		Address string `json:"address"`
	}{Address: listener.Addr().String()})
	if err != nil {
		return fmt.Errorf("encode ready message: %w", err)
	}
	fmt.Println(string(ready))

	return grpcServer.Serve(listener)
}

// server implements pluginv1.PluginServiceServer on top of a Plugin.
type server struct {
	pluginv1.UnimplementedPluginServiceServer

	manifest    Manifest
	actionDescs []*pluginv1.ActionDescriptor
	actions     map[string]Action
	connDescs   []*pluginv1.ConnectorDescriptor
	tester      ConnectorTester
	permissions []string
}

func newServer(plugin Plugin) (*server, error) {
	if plugin.Manifest.ID == "" {
		return nil, fmt.Errorf("plugin manifest is missing an id")
	}
	if plugin.Manifest.Version == "" {
		return nil, fmt.Errorf("plugin manifest is missing a version")
	}

	permissions := make([]string, len(plugin.Permissions))
	for i, perm := range plugin.Permissions {
		if err := pluginv1.ValidatePermission(string(perm)); err != nil {
			return nil, fmt.Errorf("permissions[%d]: %w", i, err)
		}
		permissions[i] = string(perm)
	}

	actions := make(map[string]Action, len(plugin.Actions))
	actionDescs := make([]*pluginv1.ActionDescriptor, 0, len(plugin.Actions))
	for _, action := range plugin.Actions {
		if _, exists := actions[action.ID()]; exists {
			return nil, fmt.Errorf("duplicate action id %q", action.ID())
		}
		actions[action.ID()] = action

		inputSchema, err := structpb.NewStruct(action.InputSchema())
		if err != nil {
			return nil, fmt.Errorf("encode input schema for action %q: %w", action.ID(), err)
		}
		outputSchema, err := structpb.NewStruct(action.OutputSchema())
		if err != nil {
			return nil, fmt.Errorf("encode output schema for action %q: %w", action.ID(), err)
		}
		actionDescs = append(actionDescs, &pluginv1.ActionDescriptor{
			Id:           action.ID(),
			Description:  action.Description(),
			InputSchema:  inputSchema,
			OutputSchema: outputSchema,
		})
	}

	connDescs := make([]*pluginv1.ConnectorDescriptor, 0, len(plugin.Connectors))
	for _, connector := range plugin.Connectors {
		configSchema, err := structpb.NewStruct(connector.ConfigSchema)
		if err != nil {
			return nil, fmt.Errorf("encode config schema for connector %q: %w", connector.Type, err)
		}
		connDescs = append(connDescs, &pluginv1.ConnectorDescriptor{
			Type:         connector.Type,
			Description:  connector.Description,
			ConfigSchema: configSchema,
		})
	}

	return &server{
		manifest:    plugin.Manifest,
		actionDescs: actionDescs,
		actions:     actions,
		connDescs:   connDescs,
		tester:      plugin.Tester,
		permissions: permissions,
	}, nil
}

func (s *server) Handshake(_ context.Context, req *pluginv1.HandshakeRequest) (*pluginv1.HandshakeResponse, error) {
	// Unlike protocol version 1, this SDK cannot negotiate down to a lower
	// version on request: Contributions' wire shape changed (ADR-0062), not
	// just its content, so there is no lower-version response this server
	// could still produce. An agent that requests less than protocolVersion
	// is simply too old for this plugin.
	if req.GetProtocolVersion() < protocolVersion {
		return nil, status.Errorf(codes.FailedPrecondition,
			"agent supports up to protocol version %d, this plugin requires %d — upgrade the agent",
			req.GetProtocolVersion(), protocolVersion)
	}

	return &pluginv1.HandshakeResponse{
		ProtocolVersion: protocolVersion,
		PluginId:        s.manifest.ID,
		PluginVersion:   s.manifest.Version,
		Contributes: &pluginv1.Contributions{
			Actions:    s.actionDescs,
			Connectors: s.connDescs,
		},
		Permissions: s.permissions,
	}, nil
}

func (s *server) ExecuteAction(ctx context.Context, req *pluginv1.ExecuteActionRequest) (*pluginv1.ExecuteActionResponse, error) {
	action, ok := s.actions[req.GetAction()]
	if !ok {
		return nil, status.Errorf(codes.NotFound, "unknown action %q", req.GetAction())
	}

	output, err := action.Run(ctx, ActionInput(req.GetInput().AsMap()), connectorConfigFromProto(req.GetConnector()))
	if err != nil {
		return nil, status.Errorf(codes.Internal, "action %q failed: %s", req.GetAction(), err)
	}

	outputStruct, err := structpb.NewStruct(output)
	if err != nil {
		return nil, status.Errorf(codes.Internal, "encode action %q output: %s", req.GetAction(), err)
	}

	return &pluginv1.ExecuteActionResponse{Output: outputStruct}, nil
}

func (s *server) TestConnector(ctx context.Context, req *pluginv1.TestConnectorRequest) (*pluginv1.TestConnectorResponse, error) {
	if s.tester == nil {
		return nil, status.Errorf(codes.Unimplemented, "plugin %q does not support connector testing", s.manifest.ID)
	}

	connector := connectorConfigFromProto(req.GetConnector())
	if connector == nil {
		return nil, status.Errorf(codes.InvalidArgument, "test connector requires a connector")
	}

	if err := s.tester.TestConnector(ctx, *connector); err != nil {
		return &pluginv1.TestConnectorResponse{Ok: false, Message: err.Error()}, nil
	}
	return &pluginv1.TestConnectorResponse{Ok: true}, nil
}

// connectorConfigFromProto decodes the wire form of a bound connector into
// the SDK's ConnectorConfig, or returns nil unchanged when the calling step
// bound no connector.
func connectorConfigFromProto(pb *pluginv1.ConnectorConfig) *ConnectorConfig {
	if pb == nil {
		return nil
	}
	return &ConnectorConfig{
		Type:    pb.GetType(),
		Config:  pb.GetConfig().AsMap(),
		Secrets: pb.GetSecrets().AsMap(),
	}
}
