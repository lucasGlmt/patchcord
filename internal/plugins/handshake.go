// Package plugins launches Patchcord plugin processes and performs the
// protocol handshake described in the vision document (section 8.3).
package plugins

import (
	"context"
	"errors"
	"fmt"

	pluginv1 "github.com/lucasglmt/patchcord/api/plugin/v1"
)

// CurrentProtocolVersion is the highest plugin protocol version this agent
// speaks. It is sent in every HandshakeRequest.
const CurrentProtocolVersion uint32 = 1

// Manifest is what a plugin declares about itself during the handshake.
type Manifest struct {
	ProtocolVersion uint32
	PluginID        string
	PluginVersion   string
	Connectors      []string
	Actions         []string
	Permissions     []string
}

// Handshake calls the plugin's Handshake RPC, validates its response, and
// returns the resulting manifest.
//
// It takes an already-connected client rather than launching a process
// itself, which keeps the negotiation logic testable against an in-memory
// gRPC server instead of a real plugin binary (see handshake_test.go).
func Handshake(ctx context.Context, client pluginv1.PluginServiceClient) (*Manifest, error) {
	resp, err := client.Handshake(ctx, &pluginv1.HandshakeRequest{
		ProtocolVersion: CurrentProtocolVersion,
	})
	if err != nil {
		return nil, fmt.Errorf("call handshake: %w", err)
	}

	if resp.GetProtocolVersion() == 0 || resp.GetProtocolVersion() > CurrentProtocolVersion {
		return nil, fmt.Errorf("plugin requires protocol version %d, agent supports up to %d",
			resp.GetProtocolVersion(), CurrentProtocolVersion)
	}
	if resp.GetPluginId() == "" {
		return nil, errors.New("plugin manifest is missing a plugin id")
	}
	if resp.GetPluginVersion() == "" {
		return nil, errors.New("plugin manifest is missing a plugin version")
	}

	return &Manifest{
		ProtocolVersion: resp.GetProtocolVersion(),
		PluginID:        resp.GetPluginId(),
		PluginVersion:   resp.GetPluginVersion(),
		Connectors:      resp.GetContributes().GetConnectors(),
		Actions:         resp.GetContributes().GetActions(),
		Permissions:     resp.GetPermissions(),
	}, nil
}
