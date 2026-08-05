// Package plugins launches Patchcord plugin processes and performs the
// protocol handshake described in the vision document (section 8.3).
package plugins

import (
	"context"
	"errors"
	"fmt"

	pluginv1 "github.com/lucasglmt/patchcord/api/plugin/v1"
)

// CurrentProtocolVersion is the plugin protocol version this agent speaks.
// It is sent in every HandshakeRequest. Bumped to 2 by ADR-0062, which
// changed Contributions from bare identifier lists to full descriptors
// (description + JSON Schema) — a wire-incompatible change, so unlike
// version 1 there is no range of versions to negotiate within: a plugin
// must speak exactly this version.
const CurrentProtocolVersion uint32 = 2

// ActionDescriptor is what a plugin declares about one action it
// contributes — its shape, not just its identifier (ADR-0062, closing the
// gap with the vision document's section 7.4).
type ActionDescriptor struct {
	ID                    string
	Description           string
	InputSchema           map[string]any
	OutputSchema          map[string]any
	DefaultTimeoutSeconds uint32
}

// ConnectorDescriptor is what a plugin declares about one connector type
// it contributes — its configuration shape, not just its identifier
// (ADR-0062, closing the gap with the vision document's section 7.3).
type ConnectorDescriptor struct {
	Type         string
	Description  string
	ConfigSchema map[string]any
}

// Manifest is what a plugin declares about itself during the handshake.
type Manifest struct {
	ProtocolVersion uint32
	PluginID        string
	PluginVersion   string
	Connectors      []ConnectorDescriptor
	Actions         []ActionDescriptor
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

	// No coexistence is possible once a protocol version's message shape
	// itself changes (ADR-0062): a plugin speaking any version other than
	// CurrentProtocolVersion must fail the handshake with a clear reason,
	// rather than have its response silently misread as an empty
	// Contributions.
	if resp.GetProtocolVersion() == 0 {
		return nil, errors.New("plugin manifest is missing a protocol version")
	}
	if resp.GetProtocolVersion() != CurrentProtocolVersion {
		return nil, fmt.Errorf("plugin speaks protocol version %d, agent requires %d — recompile the plugin against a matching sdk/go-plugin release",
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
		Connectors:      connectorDescriptorsFromProto(resp.GetContributes().GetConnectors()),
		Actions:         actionDescriptorsFromProto(resp.GetContributes().GetActions()),
		Permissions:     resp.GetPermissions(),
	}, nil
}

func actionDescriptorsFromProto(pb []*pluginv1.ActionDescriptor) []ActionDescriptor {
	if len(pb) == 0 {
		return nil
	}
	descs := make([]ActionDescriptor, len(pb))
	for i, a := range pb {
		descs[i] = ActionDescriptor{
			ID:                    a.GetId(),
			Description:           a.GetDescription(),
			InputSchema:           a.GetInputSchema().AsMap(),
			OutputSchema:          a.GetOutputSchema().AsMap(),
			DefaultTimeoutSeconds: a.GetDefaultTimeoutSeconds(),
		}
	}
	return descs
}

func connectorDescriptorsFromProto(pb []*pluginv1.ConnectorDescriptor) []ConnectorDescriptor {
	if len(pb) == 0 {
		return nil
	}
	descs := make([]ConnectorDescriptor, len(pb))
	for i, c := range pb {
		descs[i] = ConnectorDescriptor{
			Type:         c.GetType(),
			Description:  c.GetDescription(),
			ConfigSchema: c.GetConfigSchema().AsMap(),
		}
	}
	return descs
}
