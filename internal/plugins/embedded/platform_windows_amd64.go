//go:build windows && amd64

package embedded

import "embed"

// platformFS embeds bin/windows_amd64 — every reference plugin executable built
// for windows/amd64, plus the .gitkeep placeholder that keeps this
// directory (and this go:embed directive) valid before any real plugin has
// been built into it. "all:" is required so the directory is accepted even
// when .gitkeep is the only file present — see ADR-0059.
//
//go:embed all:bin/windows_amd64
var platformFS embed.FS

const platformDir = "bin/windows_amd64"
