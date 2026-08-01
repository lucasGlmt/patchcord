// Package patchcord is the official SDK for writing Patchcord plugins in
// Go. It hides the transport, the protocol, and the handshake behind a
// small interface: implement Action, list them in a Plugin, and call Serve.
//
// A plugin built with this SDK depends only on this package and the public
// protocol it wraps (github.com/lucasglmt/patchcord/api/plugin/v1) — never
// on any internal/ package of the agent.
package patchcord

import "context"

// Manifest identifies a plugin and its version, as introduced in the
// vision document (section 8.3).
type Manifest struct {
	ID      string
	Version string
}

// ActionInput holds the input values passed to an action's Run method.
type ActionInput map[string]any

// ActionOutput holds the output values an action returns.
type ActionOutput map[string]any

// Action is one atomic operation a plugin contributes to the agent, such as
// "text.uppercase@1".
type Action interface {
	// ID is the action's stable, versioned identifier.
	ID() string
	// Run executes the action.
	Run(ctx context.Context, input ActionInput) (ActionOutput, error)
}

// Plugin describes everything a plugin contributes to the agent.
type Plugin struct {
	Manifest Manifest
	Actions  []Action
	// Permissions this plugin requires from the agent, e.g.
	// "network.outbound". Declarative only in this version: the agent does
	// not yet enforce them.
	Permissions []string
}
