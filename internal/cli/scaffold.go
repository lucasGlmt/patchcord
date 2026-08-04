package cli

import "strings"

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
