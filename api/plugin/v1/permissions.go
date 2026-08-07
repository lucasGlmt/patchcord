// Not generated: a hand-written companion to plugin.pb.go, giving the wire
// field HandshakeResponse.permissions (repeated string, plugin.proto) a
// defined, validated vocabulary (ADR-0072). Kept in this package rather
// than in internal/plugins or sdk/go-plugin because both already depend on
// it independently — the core imports it directly, and sdk/go-plugin (its
// own Go module, ADR-0066) imports it too — making it the one place a
// plugin author and the agent share a permission vocabulary without a
// plugin ever depending on internal/ (non-negotiable #4).
package pluginv1

import (
	"fmt"
	"strings"
)

// Permission is a scope string a plugin declares it needs from the agent
// (vision document §9.1). It travels on the wire as a plain string
// (HandshakeResponse.permissions) — kept a defined Go type, not a proto
// enum, so a new recognized scope can be added without a protocol version
// bump: it changes nothing about the wire shape, only what ValidatePermission
// accepts.
type Permission string

const (
	// PermissionNetworkOutbound declares that the plugin makes outbound
	// network connections. Declared today by every connector-consuming
	// example plugin (http, postgresql, mysql, openai).
	PermissionNetworkOutbound Permission = "network.outbound"
)

// PermissionSecretsReadPrefix is the prefix of a parameterized permission
// scope, "secrets.read:<connector-type>" (vision document §9.1's own
// example, "secrets.read:postgresql"). Not declared by any example plugin
// today — codified because the vision document specifies it. The suffix is
// not cross-checked against the plugin's own declared connector types:
// those aren't known until this same handshake finishes parsing, and
// building that cross-check isn't worth it for a field that stays
// declarative (see the package doc comment on why enforcement stops at
// shape validation).
const PermissionSecretsReadPrefix = "secrets.read"

// ValidatePermission reports whether s is a recognized permission scope:
// either an exact match against a known flat scope (PermissionNetworkOutbound),
// or "<known-prefix>:<non-empty-suffix>" for a parameterized one
// (PermissionSecretsReadPrefix).
func ValidatePermission(s string) error {
	if s == "" {
		return fmt.Errorf("permission must not be empty")
	}
	if Permission(s) == PermissionNetworkOutbound {
		return nil
	}
	prefix := PermissionSecretsReadPrefix + ":"
	if suffix, ok := strings.CutPrefix(s, prefix); ok {
		if suffix == "" {
			return fmt.Errorf("permission %q: %q must be followed by a non-empty connector type", s, PermissionSecretsReadPrefix)
		}
		return nil
	}
	return fmt.Errorf("permission %q is not a recognized scope", s)
}
