module github.com/lucasglmt/patchcord/sdk/go-plugin

go 1.25.3

require (
	google.golang.org/grpc v1.83.0
	google.golang.org/protobuf v1.36.11
)

require (
	golang.org/x/net v0.55.0 // indirect
	golang.org/x/sys v0.45.0 // indirect
	golang.org/x/text v0.37.0 // indirect
	google.golang.org/genproto/googleapis/rpc v0.0.0-20260526163538-3dc84a4a5aaa // indirect
)

// api/plugin has no published tag yet, so this module's own build resolves
// it from the local checkout (see docs/adr/0066). A published plugin
// consuming this SDK from outside the monorepo will instead pick up a real
// tagged version of api/plugin transitively — this replace never applies
// to that downstream build.
require github.com/lucasglmt/patchcord/api/plugin v0.0.0-00010101000000-000000000000

replace github.com/lucasglmt/patchcord/api/plugin => ../../api/plugin
