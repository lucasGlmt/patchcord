module github.com/lucasglmt/patchcord

go 1.25.3

require (
	github.com/DATA-DOG/go-sqlmock v1.5.2
	github.com/fsnotify/fsnotify v1.10.1
	github.com/go-sql-driver/mysql v1.10.0
	github.com/google/uuid v1.6.0
	github.com/jackc/pgx/v5 v5.10.0
	github.com/lucasglmt/patchcord/api/plugin v0.0.0-00010101000000-000000000000
	github.com/lucasglmt/patchcord/sdk/go-plugin v0.0.0-00010101000000-000000000000
	github.com/modelcontextprotocol/go-sdk v1.7.0
	github.com/robfig/cron/v3 v3.0.1
	github.com/santhosh-tekuri/jsonschema/v5 v5.3.1
	github.com/spf13/cobra v1.10.2
	github.com/zalando/go-keyring v0.2.8
	golang.org/x/sync v0.21.0
	google.golang.org/grpc v1.83.0
	google.golang.org/protobuf v1.36.11
	gopkg.in/yaml.v3 v3.0.1
	modernc.org/sqlite v1.55.0
)

// api/plugin and sdk/go-plugin are nested modules of this monorepo (see
// docs/adr/0066); they have no published tag yet at this module version, so
// they are resolved from the local checkout instead of the module proxy.
// This replace only affects builds of this module itself — it never
// applies to downstream consumers that import api/plugin or sdk/go-plugin
// directly.
replace (
	github.com/lucasglmt/patchcord/api/plugin => ./api/plugin
	github.com/lucasglmt/patchcord/sdk/go-plugin => ./sdk/go-plugin
)

require (
	filippo.io/edwards25519 v1.2.0 // indirect
	github.com/danieljoos/wincred v1.2.3 // indirect
	github.com/dustin/go-humanize v1.0.1 // indirect
	github.com/godbus/dbus/v5 v5.2.2 // indirect
	github.com/google/jsonschema-go v0.4.3 // indirect
	github.com/inconshreveable/mousetrap v1.1.0 // indirect
	github.com/jackc/pgpassfile v1.0.0 // indirect
	github.com/jackc/pgservicefile v0.0.0-20240606120523-5a60cdf6a761 // indirect
	github.com/jackc/puddle/v2 v2.2.2 // indirect
	github.com/kr/text v0.2.0 // indirect
	github.com/mattn/go-isatty v0.0.20 // indirect
	github.com/ncruces/go-strftime v1.0.0 // indirect
	github.com/remyoudompheng/bigfft v0.0.0-20230129092748-24d4a6f8daec // indirect
	github.com/rogpeppe/go-internal v1.15.0 // indirect
	github.com/segmentio/asm v1.1.3 // indirect
	github.com/segmentio/encoding v0.5.4 // indirect
	github.com/spf13/pflag v1.0.9 // indirect
	github.com/yosida95/uritemplate/v3 v3.0.2 // indirect
	golang.org/x/net v0.55.0 // indirect
	golang.org/x/oauth2 v0.36.0 // indirect
	golang.org/x/sys v0.46.0 // indirect
	golang.org/x/text v0.37.0 // indirect
	golang.org/x/time v0.15.0 // indirect
	google.golang.org/genproto/googleapis/rpc v0.0.0-20260526163538-3dc84a4a5aaa // indirect
	modernc.org/libc v1.74.1 // indirect
	modernc.org/mathutil v1.7.1 // indirect
	modernc.org/memory v1.11.0 // indirect
)
