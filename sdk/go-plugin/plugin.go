// Package patchcord is the official SDK for writing Patchcord plugins in
// Go. It hides the transport, the protocol, and the handshake behind a
// small interface: implement Action, list them in a Plugin, and call Serve.
//
// A plugin built with this SDK depends only on this package and the public
// protocol it wraps (github.com/lucasglmt/patchcord/api/plugin/v1) — never
// on any internal/ package of the agent.
package patchcord

import (
	"context"

	pluginv1 "github.com/lucasglmt/patchcord/api/plugin/v1"
)

// Permission is a scope string a plugin declares it needs from the agent —
// an alias of api/plugin/v1's type, the shared contract both this SDK and
// the agent validate a plugin's declared permissions against
// (pluginv1.ValidatePermission, ADR-0072).
type Permission = pluginv1.Permission

// PermissionNetworkOutbound declares that the plugin makes outbound network
// connections.
const PermissionNetworkOutbound = pluginv1.PermissionNetworkOutbound

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

// Schema is a JSON Schema, expressed as a Go map so it crosses the plugin
// protocol as a google.protobuf.Struct — the same encoding ActionInput and
// ActionOutput already use for the values themselves at execution time.
// Both a human building a workflow by hand and a coding agent generating
// one can use it to discover an action's or a connector's shape without
// reading the plugin's source (ADR-0062).
type Schema map[string]any

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
// "text.uppercase@1". Description, InputSchema and OutputSchema are
// mandatory, not an optional interface like ConnectorTester below: the
// vision document (section 7.4) specifies them as part of what an action
// declares, and this SDK enforces it so no action can ship undocumented
// (ADR-0062) — the same "no silent trapdoor" stance internal/workflow's
// action-id validation already takes.
type Action interface {
	// ID is the action's stable, versioned identifier.
	ID() string
	// Description is one human-readable sentence: what this action does.
	Description() string
	// InputSchema is the JSON Schema ActionInput must match.
	InputSchema() Schema
	// OutputSchema is the JSON Schema ActionOutput will match.
	OutputSchema() Schema
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

// Connector describes one connector type a plugin's actions can be bound
// to, e.g. "http.connection@1" — declarative only: the plugin's actions
// decide for themselves whether they require a bound connector, this just
// advertises the type and its configuration shape in the manifest. It has
// no behavior of its own (that's what the optional ConnectorTester is
// for) — a plain struct is enough, simpler than an interface with nothing
// to run.
type Connector struct {
	// Type is the connector type's stable, versioned identifier.
	Type string
	// Description is one human-readable sentence: what system this
	// connector reaches.
	Description string
	// ConfigSchema is the JSON Schema of this connector's non-secret
	// configuration. Never describe secret fields here — a connector's
	// secrets never appear in a schema meant to be shown or stored
	// (ADR-0009).
	ConfigSchema Schema
}

// Plugin describes everything a plugin contributes to the agent.
type Plugin struct {
	Manifest   Manifest
	Actions    []Action
	Connectors []Connector
	// Tester, if set, lets this plugin's connector(s) be checked with
	// `patchcord connector test` without running a full action.
	Tester ConnectorTester
	// Permissions this plugin requires from the agent, e.g.
	// PermissionNetworkOutbound. Validated for shape (a recognized scope
	// string) by Serve, and again by the agent at handshake time
	// (ADR-0072) — neither check enforces anything about what the running
	// process actually does; there is no sandboxing or capability gating
	// yet (vision document §15.5's "capability broker" remains a separate,
	// later decision).
	Permissions []Permission
}
