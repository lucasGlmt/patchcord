// Package migrations embeds the SQL files internal/persistence applies to
// bring the agent's SQLite schema up to date.
package migrations

import "embed"

// FS holds every "NNNN_name.sql" migration file next to this package.
//
//go:embed *.sql
var FS embed.FS
