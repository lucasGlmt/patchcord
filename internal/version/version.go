// Package version holds the agent binary's build-time version metadata.
//
// Version, Commit and Date are injected via -ldflags "-X ..." at build
// time — see the "build" target in the Makefile, the Dockerfile's build
// args, and .goreleaser.yaml, all three of which must target these exact
// variable names. A plain `go build ./cmd/patchcord` without ldflags falls
// back to the defaults below; that is expected for local development
// builds and is not an error.
//
// This is the agent release version — orthogonal to the plugin protocol
// version (internal/plugins.CurrentProtocolVersion), workflow
// schema_version, and package/bundle format versions, which each evolve on
// their own cycle. See docs/adr/0056-versionnement-du-binaire-agent.md.
package version

var (
	// Version is the agent's release version, e.g. "0.1.0" or a
	// `git describe` output such as "0.1.0-3-gabcdef" for a build that
	// sits ahead of the last tag. "dev" outside of a version-injected
	// build.
	Version = "dev"
	// Commit is the short git commit hash the binary was built from.
	Commit = "none"
	// Date is the UTC build timestamp, RFC 3339.
	Date = "unknown"
)

// String returns a single-line, human-readable summary of the build
// metadata, e.g. "0.1.0 (commit abc1234, built 2026-08-05T12:00:00Z)".
func String() string {
	return Version + " (commit " + Commit + ", built " + Date + ")"
}
