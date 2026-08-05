package cli

import (
	"fmt"
	"strings"
)

// scaffoldDirName derives a default output directory (or file basename)
// from an id by taking its last "."-separated segment, so
// "io.patchcord.example-text" defaults to "example-text" rather than the
// full reverse-DNS id — shared by `plugin new`/`app new`/`bundle new`.
func scaffoldDirName(id string) string {
	if i := strings.LastIndex(id, "."); i >= 0 && i+1 < len(id) {
		return id[i+1:]
	}
	return id
}

// scaffoldTemplateStatic and scaffoldTemplateVite are the values accepted
// by `app new`/`bundle new --template` — shared here so both commands
// validate and describe them identically.
const (
	scaffoldTemplateStatic = "static"
	scaffoldTemplateVite   = "vite"
)

// validateScaffoldTemplate rejects any --template value other than
// scaffoldTemplateStatic or scaffoldTemplateVite.
func validateScaffoldTemplate(template string) error {
	switch template {
	case scaffoldTemplateStatic, scaffoldTemplateVite:
		return nil
	default:
		return fmt.Errorf("unknown template %q (want %q or %q)", template, scaffoldTemplateStatic, scaffoldTemplateVite)
	}
}
