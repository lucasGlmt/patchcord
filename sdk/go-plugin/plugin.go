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

// ConnectorConfig carries one connector's resolved configuration and secret
// values, as bound to an action call by the workflow step that invoked it.
// Secrets are resolved fresh for this one call and never persisted by the
// agent — an action must never include them in its ActionOutput, since
// outputs are recorded in run history in the clear.
type ConnectorConfig struct {
	Type    string
	Config  map[string]any
	Secrets map[string]any
}

// Action is one atomic operation a plugin contributes to the agent, such as
// "text.uppercase@1".
type Action interface {
	// ID is the action's stable, versioned identifier.
	ID() string
	// Run executes the action. connector is nil unless the workflow step
	// that invoked this action bound a connector to it.
	Run(ctx context.Context, input ActionInput, connector *ConnectorConfig) (ActionOutput, error)
}

// ConnectorTester is implemented by a plugin that can attempt a real
// connection using a connector's resolved configuration and secrets,
// without running any action — what `patchcord connector test` calls into.
// Implementing it is optional: a plugin with no Tester set responds
// UNIMPLEMENTED to TestConnector, which the agent reports distinctly from a
// test that ran and failed.
type ConnectorTester interface {
	// TestConnector attempts to reach the external system connector
	// describes. A returned error means the attempt failed (e.g. wrong
	// password, host unreachable) — a legitimate, expected outcome, not a
	// sign the plugin itself is broken. err's message is what the caller
	// sees; never include a secret value in it.
	TestConnector(ctx context.Context, connector ConnectorConfig) error
}

// Plugin describes everything a plugin contributes to the agent.
type Plugin struct {
	Manifest Manifest
	Actions  []Action
	// Connectors lists the connector type identifiers this plugin's
	// actions can be bound to, e.g. "http.connection@1" — declarative
	// only: the plugin's actions decide for themselves whether they
	// require a bound connector, this just advertises it in the manifest.
	Connectors []string
	// Tester, if set, lets this plugin's connector(s) be checked with
	// `patchcord connector test` without running a full action.
	Tester ConnectorTester
	// Permissions this plugin requires from the agent, e.g.
	// "network.outbound". Declarative only in this version: the agent does
	// not yet enforce them.
	Permissions []string
}
