package patchcord

import (
	"context"
	"encoding/json"
	"fmt"
	"net"

	"google.golang.org/grpc"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
	"google.golang.org/protobuf/types/known/structpb"

	pluginv1 "github.com/lucasglmt/patchcord/api/plugin/v1"
)

// protocolVersion is the highest plugin protocol version this SDK speaks.
const protocolVersion = 1

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
	actionIDs   []string
	actions     map[string]Action
	permissions []string
}

func newServer(plugin Plugin) (*server, error) {
	if plugin.Manifest.ID == "" {
		return nil, fmt.Errorf("plugin manifest is missing an id")
	}
	if plugin.Manifest.Version == "" {
		return nil, fmt.Errorf("plugin manifest is missing a version")
	}

	actions := make(map[string]Action, len(plugin.Actions))
	actionIDs := make([]string, 0, len(plugin.Actions))
	for _, action := range plugin.Actions {
		if _, exists := actions[action.ID()]; exists {
			return nil, fmt.Errorf("duplicate action id %q", action.ID())
		}
		actions[action.ID()] = action
		actionIDs = append(actionIDs, action.ID())
	}

	return &server{
		manifest:    plugin.Manifest,
		actionIDs:   actionIDs,
		actions:     actions,
		permissions: plugin.Permissions,
	}, nil
}

func (s *server) Handshake(_ context.Context, req *pluginv1.HandshakeRequest) (*pluginv1.HandshakeResponse, error) {
	negotiated := uint32(protocolVersion)
	if req.GetProtocolVersion() < negotiated {
		negotiated = req.GetProtocolVersion()
	}
	if negotiated == 0 {
		return nil, status.Errorf(codes.FailedPrecondition,
			"no compatible protocol version: agent supports up to %d, plugin supports %d",
			req.GetProtocolVersion(), protocolVersion)
	}

	return &pluginv1.HandshakeResponse{
		ProtocolVersion: negotiated,
		PluginId:        s.manifest.ID,
		PluginVersion:   s.manifest.Version,
		Contributes: &pluginv1.Contributions{
			Actions: s.actionIDs,
		},
		Permissions: s.permissions,
	}, nil
}

func (s *server) ExecuteAction(ctx context.Context, req *pluginv1.ExecuteActionRequest) (*pluginv1.ExecuteActionResponse, error) {
	action, ok := s.actions[req.GetAction()]
	if !ok {
		return nil, status.Errorf(codes.NotFound, "unknown action %q", req.GetAction())
	}

	output, err := action.Run(ctx, ActionInput(req.GetInput().AsMap()))
	if err != nil {
		return nil, status.Errorf(codes.Internal, "action %q failed: %s", req.GetAction(), err)
	}

	outputStruct, err := structpb.NewStruct(output)
	if err != nil {
		return nil, status.Errorf(codes.Internal, "encode action %q output: %s", req.GetAction(), err)
	}

	return &pluginv1.ExecuteActionResponse{Output: outputStruct}, nil
}
